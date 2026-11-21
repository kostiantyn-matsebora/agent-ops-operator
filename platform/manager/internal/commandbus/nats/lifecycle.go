package nats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/nats-io/nuid"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

const maxWriteAttempts = 8

// Submit durably accepts a request: the message is stored in the stream, then
// the task's record is written, and the call returns. It reads nothing from any
// consumer, so no consumer's view can hold it up. A command id that is already
// queued, running, retrying, parked or recently succeeded returns the task it
// already has.
func (b *Bus) Submit(ctx context.Context, req commandbus.SubmitRequest) (commandbus.Task, error) {
	if req.CommandID == "" || req.Key == "" {
		return commandbus.Task{}, fmt.Errorf("%w: CommandID and Key are both required", commandbus.ErrInvalidRequest)
	}
	decl, err := b.decl(req.Queue, req.Kind)
	if err != nil {
		return commandbus.Task{}, err
	}
	rk := taskKey(req.Queue, req.Kind, req.CommandID)
	for i := 0; i < maxWriteAttempts; i++ {
		now := time.Now()
		cur, rev, err := b.getDoc(ctx, rk)
		if err != nil {
			return commandbus.Task{}, err
		}
		if cur != nil {
			if t := b.viewOf(cur.Task, now); t.Suppresses(now, decl.Policy.DedupWindow) {
				return t, nil
			}
		}
		if cur != nil {
			if old := b.viewOf(cur.Task, now); old.State == commandbus.StateDead || old.State == commandbus.StateExpired {
				arch := *cur
				arch.Task = old
				if _, err := b.putDoc(ctx, archiveKey(old.UID), arch, 0); err != nil && !errors.Is(err, errConflict) {
					return commandbus.Task{}, err
				}
			}
		}
		uid := newUID(rk)
		m := message.NewMessage(uid, req.Payload)
		m.SetContext(ctx)
		m.Metadata.Set(KeyMetadata, string(req.Key))
		m.Metadata.Set(metaUID, uid)
		seq, err := b.pub.PublishSeq(string(req.Queue), m)
		if err != nil {
			return commandbus.Task{}, err
		}
		doc := taskDoc{Seq: seq, Task: commandbus.Task{
			UID: uid, CommandID: req.CommandID, Queue: req.Queue, Kind: req.Kind, Key: req.Key,
			Payload: req.Payload, InvocationID: req.InvocationID, State: commandbus.StateQueued,
			MaxAttempts: decl.Policy.Attempts, SubmittedAt: now, UpdatedAt: now,
		}}
		if _, err := b.putDoc(ctx, rk, doc, rev); err != nil {
			if errors.Is(err, errConflict) {
				continue // another submitter won; read what it wrote
			}
			return commandbus.Task{}, err
		}
		return doc.Task, nil
	}
	return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit %q: the record kept changing", req.CommandID)
}

// Claim blocks until a task of the queue may be handed out, then returns it
// with its lease.
//
// On a regular queue or a sequential convoy the message comes from the
// broker, which keeps one message of a key in flight and redelivers one nobody
// answers. On a hand-over queue the claim is made against the task records.
func (b *Bus) Claim(ctx context.Context, queue commandbus.Queue, consumerID string) (commandbus.Task, error) {
	if b.handover(queue) {
		return b.claimHandover(ctx, queue, consumerID)
	}
	ch, err := b.feedFor(queue)
	if err != nil {
		return commandbus.Task{}, err
	}
	for {
		select {
		case <-ctx.Done():
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: claim on %q canceled: %w", queue, ctx.Err())
		case <-b.ctx.Done():
			return commandbus.Task{}, errors.New("commandbus/nats: the bus is closed")
		case m, ok := <-ch:
			if !ok {
				return commandbus.Task{}, errors.New("commandbus/nats: the queue feed ended")
			}
			if t, ok, err := b.take(ctx, m, consumerID); err != nil {
				return commandbus.Task{}, err
			} else if ok {
				return t, nil
			}
		}
	}
}

// feedFor opens, once per queue and process, the subscription Claim reads.
func (b *Bus) feedFor(queue commandbus.Queue) (<-chan *message.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.feeds[queue]; ok {
		return ch, nil
	}
	decl, err := b.decl(queue, commandbus.KindDefault)
	if err != nil {
		return nil, err
	}
	sub := newSubscriber(b.pubsubConfig(), b.cfg.concurrency)
	sub.ackWait = b.cfg.leaseDuration
	sub.maxDeliver = max(decl.Policy.Attempts, 1)
	sub.renew = false
	ch, err := sub.Subscribe(b.ctx, string(queue))
	if err != nil {
		return nil, err
	}
	b.subs = append(b.subs, sub)
	b.feeds[queue] = ch
	return ch, nil
}

// take turns one delivered message into a claim, or answers the broker so it
// moves on: a message of a task that is no longer waiting is acknowledged and
// dropped.
func (b *Bus) take(ctx context.Context, m *message.Message, holder string) (commandbus.Task, bool, error) {
	uid := m.Metadata.Get(metaUID)
	rk, ok := keyOfUID(uid)
	if !ok {
		m.Ack()
		return commandbus.Task{}, false, nil
	}
	doc, rev, err := b.getDoc(ctx, rk)
	if err != nil {
		b.nack(m, 500*time.Millisecond)
		return commandbus.Task{}, false, nil
	}
	if doc == nil { // the record is written just after the message
		b.nack(m, 200*time.Millisecond)
		return commandbus.Task{}, false, nil
	}
	if doc.Task.UID != uid {
		m.Ack() // superseded or orphaned
		return commandbus.Task{}, false, nil
	}
	now := time.Now()
	t := b.viewOf(doc.Task, now)
	switch {
	case t.State == commandbus.StateDead && doc.Task.State != commandbus.StateDead:
		doc.Task = t // a lease that lapsed with its attempts spent: make it a fact
		_, _ = b.putDoc(ctx, rk, *doc, rev)
		m.Ack()
		return commandbus.Task{}, false, nil
	case t.State.Settled():
		m.Ack()
		return commandbus.Task{}, false, nil
	case t.State == commandbus.StateRetrying && t.NotBefore.After(now):
		b.nack(m, t.NotBefore.Sub(now))
		return commandbus.Task{}, false, nil
	case t.State == commandbus.StateRunning:
		b.nack(m, time.Second) // held elsewhere, and its lease still holds
		return commandbus.Task{}, false, nil
	}
	token := m.Metadata.Get(metaReply)
	t.State, t.NotBefore, t.UpdatedAt = commandbus.StateRunning, time.Time{}, now
	t.Attempt = attemptOf(m, doc.Task.Attempt)
	t.Lease = &commandbus.Lease{Token: token, Holder: holder, Until: now.Add(b.cfg.leaseDuration)}
	doc.Task = t
	if _, err := b.putDoc(ctx, rk, *doc, rev); err != nil {
		if errors.Is(err, errConflict) {
			b.nack(m, 100*time.Millisecond)
			return commandbus.Task{}, false, nil
		}
		b.nack(m, time.Second)
		return commandbus.Task{}, false, err
	}
	b.mu.Lock()
	b.held[token] = m
	b.mu.Unlock()
	return t, true, nil
}

func attemptOf(m *message.Message, prior int) int {
	var n int
	if _, err := fmt.Sscanf(m.Metadata.Get(metaAttempt), "%d", &n); err == nil && n > prior {
		return n
	}
	return prior + 1
}

func (b *Bus) nack(m *message.Message, delay time.Duration) {
	m.Metadata.Set(metaNakDelay, delay.String())
	m.Nack()
}

// Settle marks a task succeeded. The token must be the task's live lease; a
// lease that lapsed, or a task already settled, is ErrLeaseLost, which a caller
// treats as success: at-least-once delivery handed over work already done.
func (b *Bus) Settle(ctx context.Context, taskUID, leaseToken string) error {
	return b.answer(ctx, taskUID, leaseToken, func(t commandbus.Task, decl commandbus.Declaration, now time.Time) (commandbus.Task, reply) {
		t.State, t.SucceededAt, t.Lease, t.UpdatedAt = commandbus.StateSucceeded, now, nil, now
		return t, reply{ack: true}
	})
}

// Fail records one failed attempt. Retryability comes from the code: a
// retryable one with attempts left waits out its backoff and is delivered
// again, anything else, or attempts spent, ends the task dead.
func (b *Bus) Fail(ctx context.Context, taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	code = commandbus.ParseErrorCode(string(code))
	return b.answer(ctx, taskUID, leaseToken, func(t commandbus.Task, decl commandbus.Declaration, now time.Time) (commandbus.Task, reply) {
		t.Code, t.Detail, t.Lease, t.UpdatedAt = code, detail, nil, now
		if code.Retryable() && t.Attempt < t.MaxAttempts && decl.Policy.DeliveryTTL <= 0 {
			delay := entrylog.Backoff(decl.Policy.Backoff, t.Attempt)
			t.State, t.NotBefore = commandbus.StateRetrying, now.Add(delay)
			return t, reply{delay: delay}
		}
		t.State, t.DeadAt, t.ParkedUntil = commandbus.StateDead, now, now.Add(decl.Policy.ParkCoolOff)
		return t, reply{term: true}
	})
}

// reply is what the broker is told about the message: acknowledged, delivered
// again after a delay, or never again.
type reply struct {
	ack, term bool
	delay     time.Duration
}

func (b *Bus) answer(ctx context.Context, uid, token string, next func(commandbus.Task, commandbus.Declaration, time.Time) (commandbus.Task, reply)) error {
	rk, ok := keyOfUID(uid)
	if !ok {
		return commandbus.ErrNotFound
	}
	for i := 0; i < maxWriteAttempts; i++ {
		doc, rev, err := b.getDoc(ctx, rk)
		if err != nil {
			return err
		}
		if doc == nil || doc.Task.UID != uid {
			return commandbus.ErrNotFound
		}
		now := time.Now()
		t := b.viewOf(doc.Task, now)
		if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != token {
			return commandbus.ErrLeaseLost
		}
		decl, _ := commandbus.Resolve(b.cfg.inventory, t.Queue, t.Kind)
		out, rep := next(t, decl, now)
		doc.Task = out
		if _, err := b.putDoc(ctx, rk, *doc, rev); err != nil {
			if errors.Is(err, errConflict) {
				continue
			}
			return err
		}
		b.release(token, rep, *doc)
		return nil
	}
	return commandbus.ErrLeaseLost
}

// release tells the broker how the held message went, and drops the message of
// a hand-over task, which no consumer would ever acknowledge.
func (b *Bus) release(token string, rep reply, doc taskDoc) {
	if b.handover(doc.Task.Queue) {
		_ = b.deleteMessage(doc.Seq)
		return
	}
	b.mu.Lock()
	m := b.held[token]
	delete(b.held, token)
	b.mu.Unlock()
	if m != nil {
		switch {
		case rep.ack:
			m.Ack()
		case rep.term:
			m.Metadata.Set(metaTerm, "1")
			m.Nack()
		default:
			b.nack(m, rep.delay)
		}
		return
	}
	// Held by another handle or process: answer on the message's own reply
	// subject, which the broker accepts from any connection.
	if !strings.HasPrefix(token, "$JS.ACK.") {
		return
	}
	switch {
	case rep.ack:
		_ = b.nc.Publish(token, []byte("+ACK"))
	case rep.term:
		_ = b.nc.Publish(token, []byte("+TERM"))
	default:
		_ = b.nc.Publish(token, []byte(fmt.Sprintf(`-NAK {"delay":%d}`, rep.delay.Nanoseconds())))
	}
}

func (b *Bus) deleteMessage(seq uint64) error {
	s, err := b.js.Stream(b.ctx, b.stream)
	if err != nil {
		return err
	}
	return s.DeleteMsg(b.ctx, seq)
}

// claimHandover hands the oldest waiting task of a hand-over queue to a holder
// in this process, blocking until there is one. The task is the holder's for
// the queue's delivery TTL and is never offered again.
func (b *Bus) claimHandover(ctx context.Context, queue commandbus.Queue, holder string) (commandbus.Task, error) {
	for {
		wake := b.mirror.waiter()
		for _, c := range b.eligible(queue, time.Now(), 16) {
			if t, err := b.holdHandover(ctx, c, holder); err == nil {
				return t, nil
			} else if !errors.Is(err, errConflict) {
				return commandbus.Task{}, err
			}
		}
		select {
		case <-ctx.Done():
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: claim on %q canceled: %w", queue, ctx.Err())
		case <-b.ctx.Done():
			return commandbus.Task{}, errors.New("commandbus/nats: the bus is closed")
		case <-wake:
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (b *Bus) holdHandover(ctx context.Context, c mirrored, holder string) (commandbus.Task, error) {
	decl, _ := commandbus.Resolve(b.cfg.inventory, c.doc.Task.Queue, c.doc.Task.Kind)
	now := time.Now()
	t := c.doc.Task
	t.State, t.Attempt, t.NotBefore, t.UpdatedAt = commandbus.StateRunning, t.Attempt+1, time.Time{}, now
	t.Lease = &commandbus.Lease{Token: "ho." + nuid.Next(), Holder: holder, Until: now.Add(decl.Policy.DeliveryTTL)}
	doc := c.doc
	doc.Task = t
	if _, err := b.putDoc(ctx, c.key, doc, c.rev); err != nil {
		return commandbus.Task{}, err
	}
	return t, nil
}

// Peek lists the tasks of a hand-over queue an adapter may be handed next,
// oldest first, changing nothing.
func (b *Bus) Peek(ctx context.Context, queue commandbus.Queue, limit int) ([]commandbus.Task, error) {
	if !b.handover(queue) {
		return nil, fmt.Errorf("%w: queue %q is not a hand-over queue", commandbus.ErrInvalidRequest, queue)
	}
	if limit <= 0 {
		limit = 1 << 30
	}
	b.fresh(ctx)
	var out []commandbus.Task
	for _, c := range b.eligible(queue, time.Now(), limit) {
		out = append(out, c.doc.Task)
	}
	return out, nil
}

// Acknowledge hands the message of a command id to a holder. It is idempotent
// while the hold lasts, so of several racing acknowledgements one takes the
// message and the others read back its lease. ErrNotFound when the queue has no
// such message, ErrNotOffered when it is not the next of its key.
func (b *Bus) Acknowledge(ctx context.Context, queue commandbus.Queue, id commandbus.CommandID, holder string) (commandbus.Task, error) {
	if !b.handover(queue) {
		return commandbus.Task{}, fmt.Errorf("%w: queue %q is not a hand-over queue", commandbus.ErrInvalidRequest, queue)
	}
	for i := 0; i < maxWriteAttempts; i++ {
		var found *mirrored
		for _, d := range b.cfg.inventory {
			if d.Queue != familyOf(queue) || d.Policy.DeliveryTTL <= 0 {
				continue
			}
			rk := taskKey(queue, d.Kind, id)
			doc, rev, err := b.getDoc(ctx, rk)
			if err != nil {
				return commandbus.Task{}, err
			}
			if doc != nil {
				found = &mirrored{doc: *doc, rev: rev, key: rk}
				break
			}
		}
		if found == nil {
			return commandbus.Task{}, commandbus.ErrNotFound
		}
		now := time.Now()
		t := b.viewOf(found.doc.Task, now)
		if t.State == commandbus.StateRunning && t.Lease != nil {
			return t, nil
		}
		b.fresh(ctx)
		offered := false
		for _, c := range b.eligible(queue, now, 1<<30) {
			if c.key == found.key {
				found.doc.Task = c.doc.Task
				offered = true
				break
			}
		}
		if !offered {
			// Another acknowledgement may have taken it since: read the record
			// itself, and answer with the lease that won.
			if doc, _, err := b.getDoc(ctx, found.key); err == nil && doc != nil {
				if now := b.viewOf(doc.Task, time.Now()); now.State == commandbus.StateRunning && now.Lease != nil {
					return now, nil
				}
			}
			return commandbus.Task{}, fmt.Errorf("%w: %q on %q is %s", commandbus.ErrNotOffered, id, queue, t.State)
		}
		held, err := b.holdHandover(ctx, *found, holder)
		if errors.Is(err, errConflict) {
			continue // someone else changed it: read what they did
		}
		return held, err
	}
	return commandbus.Task{}, fmt.Errorf("%w: %q on %q kept changing", commandbus.ErrNotOffered, id, queue)
}

// Renew extends a held task's lease by one lease duration, for a handler that
// is still working. It tells the broker the message is in progress, so it is
// not delivered to another consumer, and moves the lease in the record. A lease
// that already lapsed, or was taken, is ErrLeaseLost.
func (b *Bus) Renew(ctx context.Context, taskUID, leaseToken string) error {
	rk, ok := keyOfUID(taskUID)
	if !ok {
		return commandbus.ErrNotFound
	}
	for i := 0; i < maxWriteAttempts; i++ {
		doc, rev, err := b.getDoc(ctx, rk)
		if err != nil {
			return err
		}
		if doc == nil || doc.Task.UID != taskUID {
			return commandbus.ErrNotFound
		}
		now := time.Now()
		t := b.viewOf(doc.Task, now)
		if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != leaseToken {
			return commandbus.ErrLeaseLost
		}
		decl, _ := commandbus.Resolve(b.cfg.inventory, t.Queue, t.Kind)
		lease := b.cfg.leaseDuration
		if decl.Policy.DeliveryTTL > 0 {
			lease = decl.Policy.DeliveryTTL
		}
		doc.Task.Lease = &commandbus.Lease{Token: t.Lease.Token, Holder: t.Lease.Holder, Until: now.Add(lease)}
		if _, err := b.putDoc(ctx, rk, *doc, rev); err != nil {
			if errors.Is(err, errConflict) {
				continue
			}
			return err
		}
		if strings.HasPrefix(leaseToken, "$JS.ACK.") {
			_ = b.nc.Publish(leaseToken, []byte("+WPI"))
		}
		return nil
	}
	return commandbus.ErrLeaseLost
}
