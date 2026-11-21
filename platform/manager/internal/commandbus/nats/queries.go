package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// GetTask reads one task by uid, from its record. A task that succeeded has
// been consumed: ErrNotFound, though its command id stays suppressed for the
// queue's dedup window.
func (b *Bus) GetTask(ctx context.Context, taskUID string) (commandbus.Task, error) {
	rk, ok := keyOfUID(taskUID)
	if !ok {
		return commandbus.Task{}, commandbus.ErrNotFound
	}
	doc, _, err := b.getDoc(ctx, rk)
	if err != nil {
		return commandbus.Task{}, err
	}
	if doc == nil || doc.Task.UID != taskUID {
		if doc, _, err = b.getDoc(ctx, archiveKey(taskUID)); err != nil {
			return commandbus.Task{}, err
		}
		if doc == nil {
			return commandbus.Task{}, commandbus.ErrNotFound
		}
	}
	t := b.viewOf(doc.Task, time.Now())
	if t.State == commandbus.StateSucceeded {
		return commandbus.Task{}, commandbus.ErrNotFound
	}
	return t, nil
}

// DeadLetters lists the dead tasks of a queue, or of every queue when none is
// named, oldest first.
func (b *Bus) DeadLetters(ctx context.Context, queue commandbus.Queue) ([]commandbus.Task, error) {
	b.fresh(ctx)
	now := time.Now()
	var out []commandbus.Task
	for _, d := range b.mirror.all() {
		if queue != "" && d.doc.Task.Queue != queue {
			continue
		}
		if t := b.viewOf(d.doc.Task, now); t.State == commandbus.StateDead {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DeadAt.Before(out[j].DeadAt) })
	return out, nil
}

// Running lists the tasks held under a live lease on the queues, in line order,
// those after the command id when one is given.
func (b *Bus) Running(ctx context.Context, queues []commandbus.Queue, after commandbus.CommandID) ([]commandbus.Task, error) {
	b.fresh(ctx)
	in := map[commandbus.Queue]bool{}
	for _, q := range queues {
		in[q] = true
	}
	var floor uint64
	docs := b.mirror.all()
	now := time.Now()
	if after != "" {
		for _, d := range docs {
			// A consumed task is gone: its id is unknown, and the listing starts over.
			if in[d.doc.Task.Queue] && d.doc.Task.CommandID == after && b.viewOf(d.doc.Task, now).State != commandbus.StateSucceeded {
				floor = max(floor, d.doc.Seq)
			}
		}
	}
	var out []commandbus.Task
	for _, d := range docs {
		if !in[d.doc.Task.Queue] || d.doc.Seq <= floor {
			continue
		}
		if t := b.viewOf(d.doc.Task, now); t.State == commandbus.StateRunning && t.Lease != nil {
			out = append(out, t)
		}
	}
	return out, nil
}

// Stats counts one queue and kind.
func (b *Bus) Stats(ctx context.Context, queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	b.fresh(ctx)
	if queue == "" {
		return commandbus.QueueStats{}, fmt.Errorf("%w: Stats needs a queue", commandbus.ErrInvalidRequest)
	}
	now := time.Now()
	out := commandbus.QueueStats{Queue: queue, Kind: kind}
	var oldestPending, oldestRunning time.Time
	pending := func(t commandbus.Task) {
		out.Pending++
		if oldestPending.IsZero() || t.SubmittedAt.Before(oldestPending) {
			oldestPending = t.SubmittedAt
			out.OldestPendingID, out.OldestPendingKey = t.CommandID, t.Key
		}
	}
	for _, d := range b.mirror.queueDocs(queue) {
		if d.doc.Task.Kind != kind {
			continue
		}
		switch t := b.viewOf(d.doc.Task, now); t.State {
		case commandbus.StateQueued:
			pending(t)
		case commandbus.StateRetrying:
			if t.NotBefore.After(now) {
				out.Scheduled++
			} else {
				pending(t)
			}
		case commandbus.StateRunning:
			out.Running++
			if oldestRunning.IsZero() || t.UpdatedAt.Before(oldestRunning) {
				oldestRunning = t.UpdatedAt
				out.OldestRunningID, out.OldestRunningKey = t.CommandID, t.Key
			}
		case commandbus.StateDead:
			out.Dead++
			if t.Parked(now) {
				out.Parked++
			}
		}
	}
	if !oldestPending.IsZero() {
		out.OldestPendingAge = now.Sub(oldestPending)
	}
	if !oldestRunning.IsZero() {
		out.OldestRunningAge = now.Sub(oldestRunning)
	}
	return out, nil
}

// Feed streams every change of a task, in order and without a gap. Zero starts
// at the next change; a sequence resumes after it. Every write of a record is
// one entry of the bucket's own stream, which is what this reads.
func (b *Bus) Feed(ctx context.Context, resume commandbus.Sequence) (<-chan commandbus.TaskEvent, error) {
	s, err := b.js.Stream(ctx, b.tasksStream)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: %w", err)
	}
	start := uint64(resume) + 1
	if resume == 0 {
		info, err := s.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("commandbus/nats: feed: %w", err)
		}
		start = info.State.LastSeq + 1
	}
	cons, err := s.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy, OptStartSeq: start,
	})
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: open ordered consumer: %w", err)
	}
	msgs, err := cons.Messages()
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: %w", err)
	}
	out := make(chan commandbus.TaskEvent)
	go func() {
		defer close(out)
		go func() { <-ctx.Done(); msgs.Stop() }()
		for {
			m, err := msgs.Next()
			if err != nil {
				return
			}
			md, err := m.Metadata()
			if err != nil {
				continue
			}
			var d taskDoc
			if json.Unmarshal(m.Data(), &d) != nil || d.Task.UID == "" {
				continue // a delete marker, or a value this process never wrote
			}
			t := d.Task
			ev := commandbus.TaskEvent{
				Sequence: commandbus.Sequence(md.Sequence.Stream), UID: t.UID, Queue: t.Queue, Kind: t.Kind, Key: t.Key,
				CommandID: t.CommandID, InvocationID: t.InvocationID, State: t.State, Attempt: t.Attempt, Code: t.Code,
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
