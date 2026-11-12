package nats

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// DefaultSweepInterval is how often an expiring collection sweeps the bucket
// for expired records when the caller takes the default — the same cadence
// internal/state/kube uses.
const DefaultSweepInterval = 30 * time.Second

// Expiring is this backend's state.Expiring[T]: a collection whose records
// carry an expiry (design D-6), with the sweep that removes them. Run is not
// started at construction — an unsolicited goroutine would leak in every
// test that merely builds one — the caller wiring the backend into the
// manager starts it (open.go's Backend.Run, mirroring kube's own shape).
type Expiring[T any] struct {
	*collection[T]
	sweepInterval time.Duration
}

// newExpiring wraps c, requiring its Decl to carry an expiry.
func newExpiring[T any](c *collection[T], sweepInterval time.Duration) (*Expiring[T], error) {
	if c.layout.Decl.Expiry == nil {
		return nil, errNotExpiring(c.layout.Decl.Name)
	}
	if sweepInterval <= 0 {
		sweepInterval = DefaultSweepInterval
	}
	return &Expiring[T]{collection: c, sweepInterval: sweepInterval}, nil
}

// Expiry reads rec's expiry through the declaration, the instant the store
// itself applies to Get, List and the feed.
func (e *Expiring[T]) Expiry(rec T) *time.Time {
	return e.layout.Decl.Expiry(&rec)
}

// Run starts the expiry sweep: every interval, delete every mirrored record
// whose expiry has passed, under the exact revision the mirror observed it
// at (so a write landing between the snapshot and the delete keeps the
// record, as kube's own sweep documents). The resulting delete flows through
// the same KV watch as any other, so a Watch consumer sees it as the
// contract requires.
func (e *Expiring[T]) Run(ctx context.Context) {
	e.sweepExpired(ctx) // covers a record a previous process (or another replica) already stamped
	ticker := time.NewTicker(e.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sweepExpired(ctx)
		}
	}
}

func (e *Expiring[T]) sweepExpired(ctx context.Context) {
	now := time.Now()
	for key, m := range e.mirror.snapshot() {
		rec := m.Env.Record
		at := e.expiry(&rec)
		if at == nil || !at.Before(now) {
			continue
		}
		wk := wireKey(string(key))
		if err := e.kv.Delete(ctx, wk, jetstream.LastRevision(m.Revision)); err != nil && !isWrongLastSequence(err) && !isNotFound(err) {
			continue // best-effort: a transient failure is indistinguishable from "nothing to sweep", the next tick tries again
		}
	}
}

var _ state.Expiring[struct{}] = (*Expiring[struct{}])(nil)
