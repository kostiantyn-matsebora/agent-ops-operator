package nats

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// consumerCountingKV counts the calls that make the server create a consumer:
// every key listing and every watch. A read of a conversation's runs that
// makes one is a cluster-wide operation per reconcile.
type consumerCountingKV struct {
	jetstream.KeyValue
	n atomic.Int64
}

func (c *consumerCountingKV) ListKeysFiltered(ctx context.Context, f ...string) (jetstream.KeyLister, error) {
	c.n.Add(1)
	return c.KeyValue.ListKeysFiltered(ctx, f...)
}
func (c *consumerCountingKV) ListKeys(ctx context.Context, o ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	c.n.Add(1)
	return c.KeyValue.ListKeys(ctx, o...)
}
func (c *consumerCountingKV) Keys(ctx context.Context, o ...jetstream.WatchOpt) ([]string, error) {
	c.n.Add(1)
	return c.KeyValue.Keys(ctx, o...)
}
func (c *consumerCountingKV) WatchAll(ctx context.Context, o ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	c.n.Add(1)
	return c.KeyValue.WatchAll(ctx, o...)
}

// Reading a conversation's runs, however often, creates no consumer on the
// server and still sees what was just appended.
func TestReadingRunsCreatesNoConsumerPerCall(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	if err := a.Append(ctx, parent, state.Run{RunID: "r1", Status: "s"}); err != nil {
		t.Fatal(err)
	}

	spy := &consumerCountingKV{KeyValue: a.itemsKV}
	a.itemsKV = spy
	defer func() { a.itemsKV = spy.KeyValue }()

	for i := 0; i < 20; i++ {
		_, _, runs, err := a.GetWithItems(ctx, parent)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 1 {
			t.Fatalf("read %d runs, want 1", len(runs))
		}
	}
	if got := spy.n.Load(); got != 0 {
		t.Fatalf("20 reads made %d consumer-creating calls on the server, want 0", got)
	}

	if err := a.Append(ctx, parent, state.Run{RunID: "r2", Status: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Grow(ctx, parent, "r2", state.Run{DeliveryTracked: true}); err != nil {
		t.Fatalf("Grow of a run appended a moment ago: %v", err)
	}
	if runs, _ := a.List(ctx, parent); len(runs) != 2 {
		t.Fatalf("a read after an append saw %d runs, want 2", len(runs))
	}
	if got := spy.n.Load(); got != 0 {
		t.Fatalf("append, grow and list made %d consumer-creating calls, want 0", got)
	}
}

// A conversation holding more runs than the limit (trims that failed in the
// past left them behind) is brought back to the limit by the sweep, newest
// kept, with no append needed.
func TestSweepTrimsAConversationHoldingMoreRunsThanTheLimit(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "heavy")
	e.conversation(t, "light")
	over := state.RunHistoryLimit + 7
	for i := 0; i < over; i++ {
		env := envelope[state.Run]{AppendSeq: int64(1000 + i), Record: state.Run{RunID: runName(i), Status: "succeeded"}}
		data, err := env.encode()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.b.runsKV.Create(e.ctx, runsItemKey("heavy", runName(i)), data); err != nil {
			t.Fatal(err)
		}
	}
	e.runs(t, "light", 3)

	if err := e.children().Sweep(e.ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if got := e.runKeysOf(t, "heavy"); got != state.RunHistoryLimit {
		t.Fatalf("heavy keeps %d runs after the sweep, want %d", got, state.RunHistoryLimit)
	}
	if got := e.runKeysOf(t, "light"); got != 3 {
		t.Fatalf("a conversation under the limit lost runs: %d, want 3", got)
	}
	kept, _ := e.b.Stores.Runs.List(e.ctx, "heavy")
	if len(kept) == 0 || kept[len(kept)-1].RunID != runName(over-1) {
		t.Fatalf("the newest run was not kept: %+v", kept)
	}
}

// The cascade and the sweep read runs from the mirror too: a delete or a sweep
// pass must not make the server create a consumer.
func TestCascadeAndSweepCreateNoConsumers(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "doomed")
	e.runs(t, "doomed", 3)
	spy := &consumerCountingKV{KeyValue: e.b.runsKV}
	c := e.children()
	c.runsKV = spy
	if err := c.removed(e.ctx, "doomed"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sweep(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got := spy.n.Load(); got != 0 {
		t.Fatalf("a delete and a sweep made %d consumer-creating calls, want 0", got)
	}
}
