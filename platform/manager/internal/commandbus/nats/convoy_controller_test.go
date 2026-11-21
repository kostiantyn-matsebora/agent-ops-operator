package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

func newTestController(t *testing.T) (*convoyController, context.Context) {
	t.Helper()
	srv, err := newTestServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Shutdown)
	nc, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "cbconvoys_test", History: 1})
	if err != nil {
		t.Fatal(err)
	}
	return newConvoyController(kv, newSubjects("test")), ctx
}

// Concurrent Ensure calls, as every replica makes them, agree on one entry.
func TestEnsureIsIdempotentAndAtomicAcrossCallers(t *testing.T) {
	c, ctx := newTestController(t)
	var wg sync.WaitGroup
	got := make([]Convoy, 20)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cv, err := c.Ensure(ctx, "conv", "chat-a.b", time.Now())
			if err != nil {
				t.Error(err)
			}
			got[i] = cv
		}()
	}
	wg.Wait()
	for _, cv := range got {
		if cv.Consumer != got[0].Consumer || cv.Subject != got[0].Subject || cv.Revision != got[0].Revision {
			t.Fatalf("callers saw different entries: %+v vs %+v", cv, got[0])
		}
	}
	all, err := c.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("want one entry, got %d (err %v)", len(all), err)
	}
}

func TestDifferentKeysAreDifferentConvoyQueues(t *testing.T) {
	c, ctx := newTestController(t)
	a, _ := c.Ensure(ctx, "conv", "a", time.Now())
	b, _ := c.Ensure(ctx, "conv", "b", time.Now())
	o, _ := c.Ensure(ctx, "out.console", "a", time.Now())
	if a.Consumer == b.Consumer || a.Consumer == o.Consumer || a.Subject == b.Subject || a.Subject == o.Subject {
		t.Fatalf("convoy queues collide: %+v %+v %+v", a, b, o)
	}
}

func TestWatchSeesExistingThenNewThenDeleted(t *testing.T) {
	c, ctx := newTestController(t)
	first, _ := c.Ensure(ctx, "conv", "first", time.Now())
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	ev, err := c.Watch(wctx)
	if err != nil {
		t.Fatal(err)
	}
	next := func() ConvoyEvent {
		select {
		case e := <-ev:
			return e
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
			return ConvoyEvent{}
		}
	}
	if e := next(); e.Deleted || e.Convoy.Key != "first" {
		t.Fatalf("want the existing entry, got %+v", e)
	}
	c.Ensure(ctx, "conv", "second", time.Now())
	if e := next(); e.Deleted || e.Convoy.Key != "second" {
		t.Fatalf("want the new entry, got %+v", e)
	}
	if err := c.Remove(ctx, first); err != nil {
		t.Fatal(err)
	}
	if e := next(); !e.Deleted {
		t.Fatalf("want a deletion, got %+v", e)
	}
}

// A delete made from a stale read must not remove an entry recreated since.
func TestRemoveWithAStaleRevisionKeepsARecreatedEntry(t *testing.T) {
	c, ctx := newTestController(t)
	old, _ := c.Ensure(ctx, "conv", "k", time.Now())
	if err := c.Remove(ctx, old); err != nil {
		t.Fatal(err)
	}
	fresh, _ := c.Ensure(ctx, "conv", "k", time.Now())
	if err := c.Remove(ctx, old); err == nil {
		t.Fatal("a stale revision removed a recreated entry")
	}
	all, _ := c.List(ctx)
	if len(all) != 1 || all[0].Revision != fresh.Revision {
		t.Fatalf("recreated entry lost: %+v", all)
	}
}

var _ = commandbus.KindDefault
