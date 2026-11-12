package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Submit implements commandbus.Bus (design D-4/D-9). The dedup check reads
// "ids" directly from the server — Submit is the one call D-9 requires
// cheap, not zero-network, and a stale mirror could suppress a resubmit
// that should have gone through, or let one through that should have been
// suppressed, either of which is the exact bug D-4 exists to prevent.
func (b *Bus) Submit(ctx context.Context, req commandbus.SubmitRequest) (commandbus.Task, error) {
	if req.CommandID == "" || req.Key == "" {
		return commandbus.Task{}, fmt.Errorf("%w: CommandID and Key are both required", commandbus.ErrInvalidRequest)
	}
	decl, ok := commandbus.Resolve(b.inventory, req.Queue, req.Kind)
	if !ok {
		return commandbus.Task{}, fmt.Errorf("%w: queue=%q kind=%q", commandbus.ErrUnknownQueue, req.Queue, req.Kind)
	}
	now := b.clock.now()
	key := dedupKey(req.Queue, req.Kind, req.CommandID)

	if entry, err := b.ids.Get(ctx, key); err == nil {
		uid := string(entry.Value())
		if t, _, found, rerr := b.loadAndReap(ctx, uid, now); rerr == nil && found && t.Suppresses(now, decl.Policy.DedupWindow) {
			return t, nil
		}
	} else if !isNotFoundErr(err) {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: look up %q: %w", key, err)
	}

	return b.createTask(ctx, req, decl, key, now)
}

// createTask mints a fresh task and its dedup mapping. Order is assigned
// from the Create's own revision — this bucket's one strictly increasing,
// shared-across-every-queue counter (design D-14/2.6) — which the task
// learns only after Create returns, so a second, CAS write folds it into
// the stored record. Between the two writes the task is briefly visible
// to a concurrent Claim with Order still zero: a self-correcting window,
// bounded to this one round trip, that can at most mis-order one claim
// pass — Claim's own CAS still makes exclusivity exact regardless (design
// D-2's "the claim itself is a compare-and-swap").
func (b *Bus) createTask(ctx context.Context, req commandbus.SubmitRequest, decl commandbus.Declaration, key string, now time.Time) (commandbus.Task, error) {
	task := commandbus.Task{
		UID: newUID(), CommandID: req.CommandID, Queue: req.Queue, Kind: req.Kind, Key: req.Key,
		Payload: req.Payload, InvocationID: req.InvocationID,
		State: commandbus.StateQueued, MaxAttempts: decl.Policy.Attempts,
		SubmittedAt: now, UpdatedAt: now,
	}
	data, err := encodeTask(task)
	if err != nil {
		return commandbus.Task{}, err
	}
	rev, err := b.tasks.Create(ctx, task.UID, data)
	if err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: create task: %w", err)
	}

	task.Order = rev
	data, err = encodeTask(task)
	if err != nil {
		return commandbus.Task{}, err
	}
	orderRev, err := b.tasks.Update(ctx, task.UID, data, rev)
	if err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: set order on task %s: %w", task.UID, err)
	}
	b.bumpSelfSeq(orderRev)

	if _, err := b.ids.Put(ctx, key, []byte(task.UID)); err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: map dedup id %q: %w", key, err)
	}
	return task, nil
}
