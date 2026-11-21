package nats

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Run sweeps until ctx ends. The sweep is safe on any replica: every change it
// makes is conditional on the revision it read, so two sweeps never both act.
func (b *Bus) Run(ctx context.Context) {
	t := time.NewTicker(b.cfg.sweepInterval)
	defer t.Stop()
	for {
		b.Sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-b.ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Sweep makes the facts a record only implies, and removes what is finished:
//   - a lease that lapsed with its attempts spent, and a task waiting past its
//     TTL, are written as dead and expired, and a hand-over task's message is
//     dropped from the stream;
//   - a succeeded task is dropped once its dedup window has passed;
//   - dead letters are kept to their age and count;
//   - a convoy queue that has sat empty past the idle time is removed.
func (b *Bus) Sweep(ctx context.Context) {
	now := time.Now()
	var dead []mirrored
	for _, d := range b.mirror.all() {
		decl, _ := commandbus.Resolve(b.cfg.inventory, d.doc.Task.Queue, d.doc.Task.Kind)
		t := b.viewOf(d.doc.Task, now)
		switch {
		case t.State == commandbus.StateSucceeded:
			if now.After(t.SucceededAt.Add(decl.Policy.DedupWindow)) {
				b.drop(ctx, d)
			}
		case t.State == commandbus.StateDead:
			if d.doc.Task.State != commandbus.StateDead {
				b.settleImplied(ctx, d, t)
			}
			if now.Sub(t.DeadAt) > b.cfg.deadLetterMaxAge {
				b.drop(ctx, d)
			} else {
				dead = append(dead, d)
			}
		case t.State == commandbus.StateExpired:
			b.settleImplied(ctx, d, t)
			b.drop(ctx, d)
		}
	}
	if over := len(dead) - b.cfg.deadLetterMaxCount; over > 0 {
		sort.Slice(dead, func(i, j int) bool { return dead[i].doc.Task.DeadAt.Before(dead[j].doc.Task.DeadAt) })
		for _, d := range dead[:over] {
			b.drop(ctx, d)
		}
	}
	b.sweepConvoys(ctx, now)
}

// settleImplied writes what the record only implied, so it is a fact.
func (b *Bus) settleImplied(ctx context.Context, d mirrored, t commandbus.Task) {
	doc := d.doc
	doc.Task = t
	if _, err := b.putDoc(ctx, d.key, doc, d.rev); err == nil && b.handover(t.Queue) {
		_ = b.deleteMessage(doc.Seq)
	}
}

func (b *Bus) drop(ctx context.Context, d mirrored) {
	if err := b.tasks.Delete(ctx, d.key, jetstream.LastRevision(d.rev)); err == nil {
		b.mirror.remove(d.key, d.rev+1)
		if b.handover(d.doc.Task.Queue) {
			_ = b.deleteMessage(d.doc.Seq)
		}
	}
}

// sweepConvoys removes the convoy queues that hold nothing and have delivered
// nothing for the idle time. The registry entry goes first, by revision, so an
// entry made again meanwhile survives, then the consumer; a message that
// arrives later makes the queue again.
func (b *Bus) sweepConvoys(ctx context.Context, now time.Time) {
	convoys, err := b.convoys.List(ctx)
	if err != nil {
		return
	}
	s, err := b.js.Stream(ctx, b.stream)
	if err != nil {
		return
	}
	for _, cv := range convoys {
		cons, err := s.Consumer(ctx, cv.Consumer)
		if err != nil {
			if errors.Is(err, jetstream.ErrConsumerNotFound) && now.Sub(cv.CreatedAt) > b.cfg.convoyIdle {
				_ = b.convoys.Remove(ctx, cv)
			}
			continue
		}
		info, err := cons.Info(ctx)
		if err != nil || info.NumPending > 0 || info.NumAckPending > 0 {
			continue
		}
		last := info.Created
		if info.Delivered.Last != nil && info.Delivered.Last.After(last) {
			last = *info.Delivered.Last
		}
		if now.Sub(last) < b.cfg.convoyIdle {
			continue
		}
		if err := b.convoys.Remove(ctx, cv); err == nil {
			_ = s.DeleteConsumer(ctx, cv.Consumer)
		}
	}
}
