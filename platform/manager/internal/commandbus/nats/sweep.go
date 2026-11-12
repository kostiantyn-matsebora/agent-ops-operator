package nats

import (
	"context"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Run runs the background sweep (task 3.4) until ctx is done or b is
// closed: reaping a lapsed lease or an expired TTL anywhere in the
// mirror — not only inside an active Claim's own queue, which is the
// liveness a queue nobody is claiming from still needs — releasing a
// parked CommandID's "ids" mapping once it stops suppressing (design
// D-4), and enforcing the ledger's and the "dead" bucket's own,
// independent retention caps.
//
// Not started by Open: a production wiring (a later change) runs it
// beside the rest of the manager's own background work, mirroring
// internal/state/nats.Backend.Run's own split between bootstrap and run.
func (b *Bus) Run(ctx context.Context) {
	ticker := time.NewTicker(b.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.closed:
			return
		case <-ticker.C:
			b.sweepOnce(ctx)
		}
	}
}

// sweepOnce runs every concern task 3.4 names, once. Exported as a
// lower-case method a test can call directly, rather than waiting out
// DefaultSweepInterval, exactly as internal/state/nats's own
// exp.sweepExpired is called directly from its tests.
func (b *Bus) sweepOnce(ctx context.Context) {
	now := b.clock.now()
	b.reapAndReleaseOnce(ctx, now)
	b.enforceDeadLetterRetention(ctx, now)
}

// reapAndReleaseOnce walks the whole mirror once: a task needing a reap
// (lease lapse or TTL expiry, reapTransform's own two checks) gets one,
// a task that has stopped suppressing its CommandID (commandbus.Task.
// Suppresses) has its "ids" mapping released, and a settled task old
// enough past WithLedgerRetention is purged from "tasks" outright — the
// ledger is declared bounded and lossy telemetry (invariants.md), and
// this is the bound.
func (b *Bus) reapAndReleaseOnce(ctx context.Context, now time.Time) {
	for uid, e := range b.mirror.snapshot() {
		t := e.Task
		if b.needsReap(t, now) {
			next, _, ok, err := b.loadAndReap(ctx, uid, now)
			if err != nil || !ok {
				continue
			}
			t = next
		}
		if !t.State.Settled() {
			continue
		}
		decl, ok := commandbus.Resolve(b.inventory, t.Queue, t.Kind)
		if !ok || t.Suppresses(now, decl.Policy.DedupWindow) {
			continue
		}
		b.releaseID(ctx, t)
		if now.Sub(terminalAt(t)) > b.ledgerRetention {
			b.purgeLedgerEntry(ctx, uid, t)
		}
	}
}

func (b *Bus) needsReap(t commandbus.Task, now time.Time) bool {
	if t.State == commandbus.StateRunning && t.Lease != nil && now.After(t.Lease.Until) {
		return true
	}
	if t.State != commandbus.StateQueued && t.State != commandbus.StateRetrying {
		return false
	}
	decl, ok := commandbus.Resolve(b.inventory, t.Queue, t.Kind)
	return ok && decl.Policy.TTL > 0 && now.After(t.SubmittedAt.Add(decl.Policy.TTL))
}

// terminalAt reads the timestamp a settled task reached its terminal
// state at — the instant the ledger retention clock (above) counts from.
func terminalAt(t commandbus.Task) time.Time {
	switch t.State {
	case commandbus.StateSucceeded:
		return t.SucceededAt
	case commandbus.StateDead:
		return t.DeadAt
	default: // StateExpired: never ran, there is no other timestamp to prefer
		return t.UpdatedAt
	}
}

// releaseID deletes t's "ids" mapping, but only while it still points at
// t — a fresher Submit may already have remapped the CommandID to a new
// task, and that mapping must survive this sweep untouched.
func (b *Bus) releaseID(ctx context.Context, t commandbus.Task) {
	key := dedupKey(t.Queue, t.Kind, t.CommandID)
	entry, err := b.ids.Get(ctx, key)
	if err != nil || string(entry.Value()) != t.UID {
		return
	}
	_ = b.ids.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
}

// purgeLedgerEntry removes uid's record from "tasks" outright (Purge, not
// Delete: no reason to keep a settled task's superseded revisions once
// its retention has passed). A Dead task is re-archived defensively
// first — idempotent insurance against a crash between Fail's own
// archiveDead call and this sweep ever reaching it.
func (b *Bus) purgeLedgerEntry(ctx context.Context, uid string, t commandbus.Task) {
	if t.State == commandbus.StateDead {
		b.archiveDead(ctx, t)
	}
	entry, err := b.tasks.Get(ctx, uid)
	if err != nil {
		return
	}
	_ = b.tasks.Purge(ctx, uid, jetstream.LastRevision(entry.Revision()))
}

// enforceDeadLetterRetention bounds the "dead" bucket independently of
// the ledger's own retention (WithDeadLetterRetention): an age cap, a
// count cap keeping the newest entries, or both.
func (b *Bus) enforceDeadLetterRetention(ctx context.Context, now time.Time) {
	if b.deadLetterMaxAge <= 0 && b.deadLetterMaxCount <= 0 {
		return
	}
	lister, err := b.dead.ListKeys(ctx)
	if err != nil {
		return // including ErrNoKeysFound — nothing to cap
	}
	type listed struct {
		uid    string
		deadAt time.Time
		rev    uint64
	}
	var entries []listed
	for uid := range lister.Keys() {
		e, err := b.dead.Get(ctx, uid)
		if err != nil {
			continue
		}
		t, err := decodeTask(e.Value())
		if err != nil {
			continue
		}
		entries = append(entries, listed{uid, t.DeadAt, e.Revision()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].deadAt.After(entries[j].deadAt) }) // newest first

	for i, e := range entries {
		switch {
		case b.deadLetterMaxAge > 0 && now.Sub(e.deadAt) > b.deadLetterMaxAge:
			_ = b.dead.Purge(ctx, e.uid, jetstream.LastRevision(e.rev))
		case b.deadLetterMaxCount > 0 && i >= b.deadLetterMaxCount:
			_ = b.dead.Purge(ctx, e.uid, jetstream.LastRevision(e.rev))
		}
	}
}
