package nats

import (
	"context"
	"fmt"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// GetTask implements commandbus.Bus. A lapsed lease or an expired TTL is
// applied (loadAndReap) before the read, exactly as the interface promises
// — a ledger entry this bucket no longer holds (swept past
// WithLedgerRetention) falls back to "dead" before reporting ErrNotFound,
// since a Dead task's own archive outlives its "tasks" record on purpose
// (task.go's archiveDead).
func (b *Bus) GetTask(ctx context.Context, taskUID string) (commandbus.Task, error) {
	now := b.clock.now()
	t, _, ok, err := b.loadAndReap(ctx, taskUID, now)
	if err != nil {
		return commandbus.Task{}, err
	}
	if ok {
		return t, nil
	}
	if dead, derr := b.getDeadArchived(ctx, taskUID); derr == nil {
		return dead, nil
	}
	return commandbus.Task{}, commandbus.ErrNotFound
}

func (b *Bus) getDeadArchived(ctx context.Context, taskUID string) (commandbus.Task, error) {
	entry, err := b.dead.Get(ctx, taskUID)
	if err != nil {
		return commandbus.Task{}, err
	}
	return decodeTask(entry.Value())
}

// Settle implements commandbus.Bus. leaseToken is checked against whatever
// loadAndReap's own reap-before-read leaves current — a lease the backend
// already reclaimed as lapsed reads as State no longer Running (or a nil
// Lease), so it fails exactly like a lease some other consumer now holds
// (design D-2's Settle doc: "including a lease the backend already
// reclaimed as lapsed").
func (b *Bus) Settle(ctx context.Context, taskUID, leaseToken string) error {
	now := b.clock.now()
	t, rev, ok, err := b.loadAndReap(ctx, taskUID, now)
	if err != nil {
		return err
	}
	if !ok {
		return commandbus.ErrNotFound
	}
	if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != leaseToken {
		return commandbus.ErrLeaseLost
	}
	next := t
	next.State = commandbus.StateSucceeded
	next.SucceededAt, next.UpdatedAt = now, now
	next.Lease = nil
	if _, err := b.casWriteTask(ctx, taskUID, rev, next); err != nil {
		if isWrongLastSeq(err) {
			// Something else moved this task between our read and our
			// write — whatever we were about to settle no longer holds,
			// same as any other lease this backend already reassigned.
			return commandbus.ErrLeaseLost
		}
		return fmt.Errorf("commandbus/nats: settle %s: %w", taskUID, err)
	}
	return nil
}

// Fail implements commandbus.Bus. Retryability is derived from code
// (design D-6) by applyFail, the same pure transition reapTransform's
// lease-lapse path uses — a caller's Fail and a lapsed lease the backend
// discovers on its own are the same event from the ledger's point of view.
func (b *Bus) Fail(ctx context.Context, taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	now := b.clock.now()
	t, rev, ok, err := b.loadAndReap(ctx, taskUID, now)
	if err != nil {
		return err
	}
	if !ok {
		return commandbus.ErrNotFound
	}
	if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != leaseToken {
		return commandbus.ErrLeaseLost
	}
	next := applyFail(t, b.inventory, now, code, detail)
	if _, err := b.casWriteTask(ctx, taskUID, rev, next); err != nil {
		if isWrongLastSeq(err) {
			return commandbus.ErrLeaseLost
		}
		return fmt.Errorf("commandbus/nats: fail %s: %w", taskUID, err)
	}
	if next.State == commandbus.StateDead {
		b.archiveDead(ctx, next)
	}
	return nil
}
