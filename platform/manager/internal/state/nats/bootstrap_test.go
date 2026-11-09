package nats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// blockStream creates a memory-stored stream under name, which no KV bucket or
// object store can then take over: a stream's storage type cannot change.
func blockStream(t *testing.T, conn *Conn, name string) {
	t.Helper()
	_, err := conn.js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name: name, Subjects: []string{"blocked." + name}, Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("block %q: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.js.DeleteStream(context.Background(), name) })
}

func TestOpeningFailsNamingTheBucketThatCannotBeCreated(t *testing.T) {
	conn := sharedTestConn(t)
	kinds := []string{bucketConversations, bucketRuns, bucketSignals, bucketCooldowns, bucketReceipts, bucketAdapterState}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			scope := newScope(t)
			blockStream(t, conn, kvStreamName(bucketName(scope, kind)))
			_, err := OpenStores(context.Background(), conn, scope)
			if err == nil || !strings.Contains(err.Error(), bucketName(scope, kind)) {
				t.Fatalf("OpenStores = %v, want a failure naming %q", err, bucketName(scope, kind))
			}
		})
	}
	t.Run("blobs", func(t *testing.T) {
		scope := newScope(t)
		blockStream(t, conn, "OBJ_"+bucketName(scope, objectStoreBlobs))
		_, err := OpenStores(context.Background(), conn, scope)
		if err == nil || !strings.Contains(err.Error(), "object store") {
			t.Fatalf("OpenStores = %v, want an object store failure", err)
		}
	})
}

func TestConnectPassesCredentialsAndTheTimeout(t *testing.T) {
	conn, err := Connect(Config{URL: testServer.ClientURL(), User: "u", Password: "p", ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()
	if err := conn.Reachable(context.Background()); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	var none *Conn
	none.Close() // closing nothing is not a crash
}

func TestAnExpiredRecordWhoseDeleteFailsIsLeftForTheNextSweep(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	signals := b.signals
	exp, err := newExpiring(signals, 0)
	if err != nil || exp.sweepInterval != DefaultSweepInterval {
		t.Fatalf("newExpiring with no interval = %v, %v, want the default", exp.sweepInterval, err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := b.Stores.Signals.Create(ctx, "stale", state.SignalRecord{SourceRef: "s", ExpiresAt: &past}); err != nil {
		t.Fatalf("create: %v", err)
	}
	eventually(t, "the mirror saw the signal", func() bool { _, ok := signals.mirror.snapshot()["stale"]; return ok })

	realKV := signals.kv
	signals.kv = &scriptedKV{KeyValue: realKV, del: always(errInjected)}
	exp.sweepExpired(ctx)
	if _, ok := signals.mirror.snapshot()["stale"]; !ok {
		t.Fatal("a record whose delete failed left the mirror")
	}
	signals.kv = realKV
	exp.sweepExpired(ctx)
	eventually(t, "the next sweep removed it", func() bool { _, ok := signals.mirror.snapshot()["stale"]; return !ok })
}

// blobFault wraps the object store and fails the named calls.
type blobFault struct {
	jetstream.ObjectStore
	del func() error
}

func (f *blobFault) Delete(ctx context.Context, name string) error {
	if f.del != nil {
		if err := f.del(); err != nil {
			return err
		}
	}
	return f.ObjectStore.Delete(ctx, name)
}

func TestTheSweepReportsABlobItCannotDeleteAndOneItCannotJudge(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	if err := b.Stores.Blobs.Put(ctx, "orphan", state.Blob{ConversationName: "ghost", Payload: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := *b.owned
	old.now = func() time.Time { return time.Now().Add(24 * time.Hour) }

	failing := old
	failing.blobs = &blobFault{ObjectStore: b.owned.blobs, del: func() error { return errInjected }}
	if err := failing.Sweep(ctx); !errors.Is(err, errInjected) {
		t.Fatalf("Sweep = %v, want the delete failure", err)
	}
	if _, err := b.Stores.Blobs.Get(ctx, "orphan"); err != nil {
		t.Fatalf("a blob whose delete failed is gone: %v", err)
	}

	// A conversation that cannot be looked up is not an orphan's judge.
	c := old.conversations
	realKV := c.kv
	c.kv = &scriptedKV{KeyValue: realKV, get: always(errInjected)}
	defer func() { c.kv = realKV }()
	if err := old.Sweep(ctx); !errors.Is(err, errInjected) {
		t.Fatalf("Sweep with an unreadable conversation = %v", err)
	}
	if _, err := b.Stores.Blobs.Get(ctx, "orphan"); err != nil {
		t.Fatalf("a blob was swept on a lookup that failed: %v", err)
	}
}

func TestEveryReadOfAParentOrItsItemsReportsAFailureOfEither(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	if err := a.Append(ctx, parent, state.Run{RunID: "r1", Status: "s"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The parent cannot be read.
	realParent := a.parent.kv
	a.parent.kv = &scriptedKV{KeyValue: realParent, get: always(errInjected)}
	if err := a.Append(ctx, parent, state.Run{RunID: "r2"}); !errors.Is(err, errInjected) {
		t.Errorf("Append = %v", err)
	}
	if _, err := a.List(ctx, parent); !errors.Is(err, errInjected) {
		t.Errorf("List = %v", err)
	}
	if _, _, _, err := a.GetWithItems(ctx, parent); !errors.Is(err, errInjected) {
		t.Errorf("GetWithItems = %v", err)
	}
	if err := a.Grow(ctx, parent, "r1", state.Run{}); !errors.Is(err, errInjected) {
		t.Errorf("Grow = %v", err)
	}
	a.parent.kv = realParent

	// The items cannot be listed.
	realItems := a.itemsKV
	a.itemsKV = &scriptedKV{KeyValue: realItems, keys: always(errInjected)}
	if _, err := a.List(ctx, parent); !errors.Is(err, errInjected) {
		t.Errorf("List = %v", err)
	}
	if _, _, _, err := a.GetWithItems(ctx, parent); !errors.Is(err, errInjected) {
		t.Errorf("GetWithItems = %v", err)
	}
	if err := a.Grow(ctx, parent, "r1", state.Run{}); !errors.Is(err, errInjected) {
		t.Errorf("Grow = %v", err)
	}
	// A join that cannot read a record's items drops that record, as a raced
	// delete does, and keeps the rest.
	got, err := a.ListWithItems(ctx, state.ByAll, "")
	if err != nil || len(got) != 0 {
		t.Errorf("ListWithItems = %v, %v, want the unreadable record left out", got, err)
	}
	if err := a.Append(ctx, parent, state.Run{RunID: "r3", Status: "s"}); err != nil {
		t.Errorf("Append whose trim cannot list = %v, want nil", err)
	}
	a.itemsKV = realItems
}

func TestGrowSeesAnItemThatChangesBetweenTheScanAndTheWrite(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	if err := a.Append(ctx, parent, state.Run{RunID: "r1", Status: "s"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	real := a.itemsKV
	t.Cleanup(func() { a.itemsKV = real })

	// The scan finds the item; the read right after finds it gone.
	reads := 0
	a.itemsKV = &scriptedKV{KeyValue: real, get: func(k string) error {
		reads++
		if reads == 2 {
			return jetstream.ErrKeyNotFound
		}
		return nil
	}}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an item deleted after the scan = %v, want ErrNotFound", err)
	}

	// Or the read fails for another reason.
	reads = 0
	a.itemsKV = &scriptedKV{KeyValue: real, get: func(k string) error {
		reads++
		if reads == 2 {
			return errInjected
		}
		return nil
	}}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); !errors.Is(err, errInjected) {
		t.Fatalf("Grow with an item that cannot be read = %v", err)
	}
}

func TestWaitingForAWriteTheMirrorHasNotSeenYetGivesUpWithTheCaller(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	if err := b.conversations.mirror.waitReady(shortCtx(t)); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := b.conversations.mirror.waitCaughtUpTo(ctx, 1<<40)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitCaughtUpTo = %v, want the caller's deadline", err)
	}
}
