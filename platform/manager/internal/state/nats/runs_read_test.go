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
