package nats

import (
	"context"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/conformance"
)

// backing adapts one scope's worth of buckets to conformance.Backing.
// Bus() opens a FRESH *Bus every call, all sharing this backing's own
// clock and lease duration — the real cross-replica shape design D-2
// describes, not two Go values wrapping one already-shared struct (the
// way commandbus/memory's handle does it): this backend's buckets are
// already the state two replicas actually share, so two independently
// constructed Bus values over the same scope is what "a claim made
// through one Bus value completes through another" means for real.
type backing struct {
	t             testing.TB
	scope         string
	clock         *clock
	leaseDuration time.Duration
}

func newBacking(t testing.TB) *backing {
	t.Helper()
	return &backing{t: t, scope: newScope(t), clock: newClock(time.Now), leaseDuration: defaultLeaseDuration}
}

func (b *backing) Bus() commandbus.Bus {
	bus, err := Open(context.Background(), testConn, b.scope, withClock(b.clock), WithLeaseDuration(b.leaseDuration))
	if err != nil {
		b.t.Fatalf("Open: %v", err)
	}
	b.t.Cleanup(bus.Close)
	return bus
}

func (b *backing) Advance(d time.Duration)      { b.clock.advance(d) }
func (b *backing) LeaseDuration() time.Duration { return b.leaseDuration }

func TestConformance(t *testing.T) {
	conformance.Run(t, func() conformance.Backing { return newBacking(t) })
}

func BenchmarkConformance(b *testing.B) {
	conformance.Bench(b, func() conformance.Backing { return newBacking(b) })
}
