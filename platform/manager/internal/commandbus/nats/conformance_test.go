package nats

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/conformance"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
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
	mu            sync.Mutex // guards buses: a scenario opens handles from several goroutines
	buses         []*Bus
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
	b.mu.Lock()
	b.buses = append(b.buses, bus)
	b.mu.Unlock()
	return bus
}

// Entries reads every entry of the backing's log straight from the stream, one
// message at a time in sequence order — the entries as the server stores them,
// not as any Bus value's mirror folded them.
func (b *backing) Entries() []entrylog.Entry {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := testConn.js.Stream(ctx, bucketName(b.scope, streamLog))
	if err != nil {
		b.t.Fatalf("open log stream: %v", err)
	}
	last := stream.CachedInfo().State.LastSeq
	var out []entrylog.Entry
	for seq := uint64(1); seq <= last; seq++ {
		raw, err := stream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue // removed: a consumed task's entry
		}
		if err != nil {
			b.t.Fatalf("read log entry %d: %v", seq, err)
		}
		var e entrylog.Entry
		if err := json.Unmarshal(raw.Data, &e); err != nil {
			b.t.Fatalf("decode log entry %d: %v", seq, err)
		}
		out = append(out, e)
	}
	return out
}

// Compact lets the settled grace pass and removes the entries of every consumed
// task, as the background sweep would.
func (b *backing) Compact() {
	b.clock.advance(DefaultSettledGrace + time.Second)
	ctx := context.Background()
	b.mu.Lock()
	buses := append([]*Bus(nil), b.buses...)
	b.mu.Unlock()
	for _, bus := range buses {
		if err := bus.sync(ctx); err != nil {
			b.t.Fatalf("sync before compacting: %v", err)
		}
		bus.removeConsumed(ctx, 0)
	}
}

func (b *backing) Advance(d time.Duration)      { b.clock.advance(d) }
func (b *backing) LeaseDuration() time.Duration { return b.leaseDuration }

func TestConformance(t *testing.T) {
	conformance.Run(t, func() conformance.Backing { return newBacking(t) })
}

func BenchmarkConformance(b *testing.B) {
	conformance.Bench(b, func() conformance.Backing { return newBacking(b) })
}
