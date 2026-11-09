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

// scriptedKV wraps a bucket and fails the calls a test names, per method. A
// nil hook passes the call through.
type scriptedKV struct {
	jetstream.KeyValue
	get, create, update, del, purge func(key string) error
	keys                            func(filter string) error
}

func (s *scriptedKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	if s.keys != nil {
		if err := s.keys(strings.Join(filters, ",")); err != nil {
			return nil, err
		}
	}
	return s.KeyValue.ListKeysFiltered(ctx, filters...)
}

func (s *scriptedKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if s.get != nil {
		if err := s.get(key); err != nil {
			return nil, err
		}
	}
	return s.KeyValue.Get(ctx, key)
}

func (s *scriptedKV) Create(ctx context.Context, key string, v []byte) (uint64, error) {
	if s.create != nil {
		if err := s.create(key); err != nil {
			return 0, err
		}
	}
	return s.KeyValue.Create(ctx, key, v)
}

func (s *scriptedKV) Update(ctx context.Context, key string, v []byte, last uint64) (uint64, error) {
	if s.update != nil {
		if err := s.update(key); err != nil {
			return 0, err
		}
	}
	return s.KeyValue.Update(ctx, key, v, last)
}

func (s *scriptedKV) Delete(ctx context.Context, key string, o ...jetstream.KVDeleteOpt) error {
	if s.del != nil {
		if err := s.del(key); err != nil {
			return err
		}
	}
	return s.KeyValue.Delete(ctx, key, o...)
}

func (s *scriptedKV) Purge(ctx context.Context, key string, o ...jetstream.KVDeleteOpt) error {
	if s.purge != nil {
		if err := s.purge(key); err != nil {
			return err
		}
	}
	return s.KeyValue.Purge(ctx, key, o...)
}

func always(err error) func(string) error { return func(string) error { return err } }

// deadBackend is a working backend whose connection is then closed, so every
// call that reaches the server fails.
func deadBackend(t *testing.T) *Backend {
	t.Helper()
	conn, err := Connect(Config{URL: testServer.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	b := openBackend(t, conn, newScope(t))
	// Let every mirror finish opening its watch first: closing the connection
	// under a watch that is still being created races inside the client.
	ctx := shortCtx(t)
	for _, ready := range []func(context.Context) error{
		b.conversations.mirror.waitReady, b.signals.mirror.waitReady, b.cooldowns.mirror.waitReady,
		b.receipts.mirror.waitReady, b.adapterState.mirror.waitReady,
	} {
		if err := ready(ctx); err != nil {
			t.Fatalf("mirror never became ready: %v", err)
		}
	}
	conn.nc.Close()
	return b
}

func shortCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestEveryOperationFailsLoudlyWhenTheServerIsGone(t *testing.T) {
	b := deadBackend(t)
	ctx := shortCtx(t)
	conv := state.ConversationRecord{ProfileRef: "p"}
	ops := map[string]func() error{
		"conversations Get":    func() error { _, _, e := b.Stores.Conversations.Get(ctx, "k"); return e },
		"conversations Create": func() error { _, e := b.Stores.Conversations.Create(ctx, "k", conv); return e },
		"conversations Update": func() error { _, e := b.Stores.Conversations.Update(ctx, "k", "1", conv); return e },
		"conversations Delete": func() error { return b.Stores.Conversations.Delete(ctx, "k") },
		"conversations DeleteAt": func() error {
			return b.Stores.Conversations.DeleteAt(ctx, "k", "1")
		},
		"conversations List": func() error {
			_, e := b.Stores.Conversations.List(ctx, state.ByAll, "")
			return e
		},
		"conversations Watch": func() error { _, e := b.Stores.Conversations.Watch(ctx, "5"); return e },
		"runs Append":         func() error { return b.Stores.Runs.Append(ctx, "k", state.Run{RunID: "r"}) },
		"runs List":           func() error { _, e := b.Stores.Runs.List(ctx, "k"); return e },
		"runs Grow":           func() error { return b.Stores.Runs.Grow(ctx, "k", "r", state.Run{}) },
		"runs GetWithItems":   func() error { _, _, _, e := b.Stores.Runs.GetWithItems(ctx, "k"); return e },
		"runs ListWithItems": func() error {
			_, e := b.Stores.Runs.ListWithItems(ctx, state.ByAll, "")
			return e
		},
		"runs WatchWithItems":  func() error { _, e := b.Stores.Runs.WatchWithItems(ctx, ""); return e },
		"runs WatchWithItems2": func() error { _, e := b.Stores.Runs.WatchWithItems(ctx, "3.4"); return e },
		"blobs Put":            func() error { return b.Stores.Blobs.Put(ctx, "k", state.Blob{Payload: "x"}) },
		"blobs Get":            func() error { _, e := b.Stores.Blobs.Get(ctx, "k"); return e },
		"blobs Delete":         func() error { return b.Stores.Blobs.Delete(ctx, "k") },
		"imported":             func() error { _, e := b.Imported(ctx); return e },
		"mark imported":        func() error { return b.markImported(ctx) },
		"reachable":            func() error { return b.conn.Reachable(ctx) },
		"children removed":     func() error { return b.owned.removed(ctx, "k") },
		"children sweep":       func() error { return b.owned.Sweep(ctx) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			err := op()
			if err == nil {
				t.Fatal("succeeded against a closed connection")
			}
			if errors.Is(err, state.ErrNotFound) || errors.Is(err, state.ErrConflict) {
				t.Fatalf("a dead server was reported as %v, a fact about the record", err)
			}
		})
	}
}

func TestOpeningTheBackendOnADeadConnectionFails(t *testing.T) {
	conn, err := Connect(Config{URL: testServer.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn.nc.Close()
	if _, err := OpenStores(shortCtx(t), conn, newScope(t)); err == nil {
		t.Fatal("OpenStores succeeded on a closed connection")
	}
}

func TestConnectRefusesAnUnreachableServerAndHonoursTheTimeout(t *testing.T) {
	_, err := Connect(Config{URL: "nats://127.0.0.1:1", ConnectTimeout: 200 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatalf("Connect = %v, want a connect failure", err)
	}
}

func TestAConversationListWaitsForTheMirrorOrTheCaller(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Stores.Conversations.List(ctx, state.ByAll, ""); err == nil {
		t.Fatal("List on a cancelled context succeeded")
	}
	if err := b.conversations.mirror.waitCaughtUpTo(ctx, 1<<40); err == nil {
		t.Fatal("waitCaughtUpTo on a cancelled context succeeded")
	}
}

func TestCorruptStoredValuesAreRefusedNotGuessed(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	garbage := []byte("{not json")
	if _, err := b.conversations.kv.Put(ctx, wireKey("bad"), garbage); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, _, err := b.Stores.Conversations.Get(ctx, "bad"); err == nil || errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a corrupt record = %v, want a decode failure", err)
	}
	if _, err := b.Stores.Conversations.Update(ctx, "bad", "1", state.ConversationRecord{}); err == nil {
		t.Fatal("Update of a corrupt record succeeded")
	}
	if err := b.Stores.Conversations.Delete(ctx, "bad"); err == nil {
		t.Fatal("Delete of a corrupt record succeeded")
	}
	if err := b.Stores.Runs.Append(ctx, "bad", state.Run{RunID: "r"}); err == nil {
		t.Fatal("Append under a corrupt parent succeeded")
	}

	if err := b.blobsStore.Delete(ctx, "gone"); err == nil {
		_ = err // an absent object is the store's own business
	}
	if _, err := b.blobsStore.PutBytes(ctx, wireKey("bad-blob"), garbage); err != nil {
		t.Fatalf("seed blob: %v", err)
	}
	if _, err := b.Stores.Blobs.Get(ctx, "bad-blob"); err == nil || errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a corrupt blob = %v, want a decode failure", err)
	}
	// The sweep leaves what it cannot read alone: unreadable is not orphaned.
	c := *b.owned
	c.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := b.blobsStore.GetBytes(ctx, wireKey("bad-blob")); err != nil {
		t.Fatalf("the sweep removed a blob it could not decode: %v", err)
	}

	// A corrupt run item is skipped when listing, not fatal to its siblings.
	if err := b.Stores.Runs.Append(ctx, mustConversation(t, b, "good"), state.Run{RunID: "ok", Status: "s"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := b.runsKV.Put(ctx, runsItemKey("good", "junk"), garbage); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	runs, err := b.Stores.Runs.List(ctx, "good")
	if err != nil || len(runs) != 1 || runs[0].RunID != "ok" {
		t.Fatalf("runs = %+v, %v, want only the readable one", runs, err)
	}
}

func mustConversation(t *testing.T, b *Backend, name string) state.Key {
	t.Helper()
	if _, err := b.Stores.Conversations.Create(shortCtx(t), state.Key(name), state.ConversationRecord{ProfileRef: "p"}); err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	return state.Key(name)
}

func TestEnvelopeCodecReportsWhatItCannotEncodeOrDecode(t *testing.T) {
	bad := envelope[chan int]{Record: make(chan int)}
	if _, err := bad.encode(); err == nil {
		t.Fatal("encoded a channel")
	}
	if _, err := decodeEnvelope[state.Run]([]byte("{")); err == nil {
		t.Fatal("decoded truncated json")
	}
}

func TestMalformedWireKeysAreRefused(t *testing.T) {
	for _, wire := range []string{"abc_", "abc_4", "abc_zz"} {
		if _, err := unwireKey(wire); err == nil {
			t.Errorf("unwireKey(%q) succeeded", wire)
		}
	}
	if _, err := parentOfItemKey("nodot"); err == nil {
		t.Error("parentOfItemKey of a key with no parent succeeded")
	}
	if _, err := parentOfItemKey("_zz.item"); err == nil {
		t.Error("parentOfItemKey of a malformed parent succeeded")
	}
	if got := bucketName("", "runs"); got != "runs" {
		t.Errorf("bucketName with no scope = %q", got)
	}
}

func TestLayoutsExposeTheIdentityAndHoldTheyDeclare(t *testing.T) {
	now := time.Now()
	rec := state.ConversationRecord{ID: "id", CreatedAt: now, DeletionRequestedAt: &now}
	l := ConversationsLayout()
	if id, at := l.Identity.Get(&rec); id != "id" || !at.Equal(now) {
		t.Fatalf("identity = %q %v", id, at)
	}
	if got := l.Hold.DeletionRequestedAt(&rec); got == nil || !got.Equal(now) {
		t.Fatalf("DeletionRequestedAt = %v", got)
	}
	sig := state.SignalRecord{ID: "sid", CreatedAt: now}
	if id, at := SignalsLayout().Identity.Get(&sig); id != "sid" || !at.Equal(now) {
		t.Fatalf("signal identity = %q %v", id, at)
	}
}

func TestOnlyAnExpiringKindCanBeWrappedAsOne(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	if _, err := newExpiring(b.cooldowns, 0); err == nil {
		t.Fatal("wrapped a kind with no expiry as an expiring one")
	}
	if _, err := newAppended[state.ConversationRecord, state.Run](b.conversations, nil, b.conn.js, b.runsKV, "x", state.Decl[state.Run]{Name: "runs"}); err == nil {
		t.Fatal("built an appended kind without an ID and a Merge")
	}
}

// The scripted failures: the server answers, but not the way the happy path
// needs.

func TestAppendReportsAFailedWriteAndKeepsHistoryWhenNoLimitIsSet(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])

	inner := a.itemsKV
	a.itemsKV = &scriptedKV{KeyValue: inner, create: always(errInjected)}
	if err := a.Append(ctx, parent, state.Run{RunID: "x"}); !errors.Is(err, errInjected) {
		t.Fatalf("Append = %v, want the write failure", err)
	}
	a.itemsKV = inner

	a.limit = 0
	for i := 0; i < state.RunHistoryLimit+3; i++ {
		if err := a.Append(ctx, parent, state.Run{RunID: runName(i)}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got, _ := a.List(ctx, parent)
	if len(got) != state.RunHistoryLimit+3 {
		t.Fatalf("kept %d, want all %d when no limit is set", len(got), state.RunHistoryLimit+3)
	}
}

func TestTrimFailureLeavesTheRunAppendedAndHealsOnTheNextAppend(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	inner := a.itemsKV
	a.itemsKV = &scriptedKV{KeyValue: inner, purge: always(errInjected)}
	for i := 0; i < state.RunHistoryLimit+2; i++ {
		if err := a.Append(ctx, parent, state.Run{RunID: runName(i)}); err != nil {
			t.Fatalf("Append %d: a failed trim must not fail the append: %v", i, err)
		}
	}
	a.itemsKV = inner
	if err := a.Append(ctx, parent, state.Run{RunID: runName(99)}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, _ := a.List(ctx, parent)
	if len(got) != state.RunHistoryLimit {
		t.Fatalf("kept %d after healing, want %d", len(got), state.RunHistoryLimit)
	}
}

func TestGrowReportsWhatGoesWrong(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	parent := mustConversation(t, b, "p")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	if err := a.Append(ctx, parent, state.Run{RunID: "r1"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	inner := a.itemsKV

	a.itemsKV = &scriptedKV{KeyValue: inner, update: always(jetstream.ErrKeyExists)}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("Grow under endless contention = %v, want ErrConflict", err)
	}
	a.itemsKV = &scriptedKV{KeyValue: inner, update: always(errInjected)}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); !errors.Is(err, errInjected) {
		t.Fatalf("Grow with a failing write = %v", err)
	}
	a.itemsKV = &scriptedKV{KeyValue: inner, get: func(k string) error {
		if strings.Contains(k, ".") {
			return jetstream.ErrKeyNotFound
		}
		return nil
	}}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an item deleted meanwhile = %v, want ErrNotFound", err)
	}
	a.itemsKV = &scriptedKV{KeyValue: inner, get: func(k string) error {
		if strings.Contains(k, ".") {
			return errInjected
		}
		return nil
	}}
	if err := a.Grow(ctx, parent, "r1", state.Run{DeliveryTracked: true}); err == nil {
		t.Fatal("Grow with an unreadable item succeeded")
	}
}

func TestDeleteAndReleaseReportWhatGoesWrong(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	c := b.conversations
	realKV := c.kv
	t.Cleanup(func() { c.kv = realKV })

	plain := mustConversation(t, b, "plain")
	c.kv = &scriptedKV{KeyValue: realKV, del: always(errInjected)}
	if err := c.Delete(ctx, plain); !errors.Is(err, errInjected) {
		t.Fatalf("Delete with a failing server = %v", err)
	}
	_, rev, _ := c.Get(ctx, plain)
	if err := c.DeleteAt(ctx, plain, rev); err == nil {
		t.Fatal("DeleteAt with a failing server succeeded")
	}
	c.kv = &scriptedKV{KeyValue: realKV, del: always(jetstream.ErrKeyExists)}
	if err := c.DeleteAt(ctx, plain, rev); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt losing the race = %v, want ErrConflict", err)
	}

	held := state.Key("held")
	if _, err := c.Create(ctx, held, state.ConversationRecord{ProfileRef: "p", Finalizers: []string{finalizerHold}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	c.kv = &scriptedKV{KeyValue: realKV, update: always(jetstream.ErrKeyExists)}
	if err := c.Delete(ctx, held); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("Delete under endless contention = %v, want it to give up by name", err)
	}
	_, hrev, _ := c.Get(ctx, held)
	if err := c.DeleteAt(ctx, held, hrev); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt whose mark loses the race = %v, want ErrConflict", err)
	}
	c.kv = &scriptedKV{KeyValue: realKV, update: always(errInjected)}
	if err := c.Delete(ctx, held); !errors.Is(err, errInjected) {
		t.Fatalf("Delete whose mark fails = %v", err)
	}

	c.kv = realKV
	if err := c.Delete(ctx, held); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, hrev, _ = c.Get(ctx, held)
	if err := c.DeleteAt(ctx, held, hrev); err != nil {
		t.Fatalf("a second DeleteAt of a record already marked = %v, want nil", err)
	}
	rec, hrev, _ := c.Get(ctx, held)
	rec.Finalizers = nil
	c.kv = &scriptedKV{KeyValue: realKV, del: always(jetstream.ErrKeyExists)}
	if _, err := c.Update(ctx, held, hrev, rec); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("releasing the last finalizer while losing the race = %v, want ErrConflict", err)
	}
	c.kv = realKV
	if _, err := c.Update(ctx, held, "not-a-revision", rec); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("Update with a revision this backend never minted = %v, want ErrConflict", err)
	}
}

func TestReadsAndCreatesReportAServerThatAnswersBadly(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	c := b.conversations
	realKV := c.kv
	t.Cleanup(func() { c.kv = realKV })
	key := mustConversation(t, b, "k")

	c.kv = &scriptedKV{KeyValue: realKV, get: always(errInjected)}
	if _, _, err := c.Get(ctx, key); !errors.Is(err, errInjected) {
		t.Fatalf("Get = %v", err)
	}
	if _, err := c.Update(ctx, key, "1", state.ConversationRecord{}); !errors.Is(err, errInjected) {
		t.Fatalf("Update = %v", err)
	}
	if err := c.Delete(ctx, key); !errors.Is(err, errInjected) {
		t.Fatalf("Delete = %v", err)
	}
	c.kv = &scriptedKV{KeyValue: realKV, create: always(errInjected)}
	if _, err := c.Create(ctx, "fresh", state.ConversationRecord{}); !errors.Is(err, errInjected) {
		t.Fatalf("Create = %v", err)
	}
}

func TestAnExpiredRecordIsGoneToWriters(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	past := time.Now().Add(-time.Hour)
	if _, err := b.Stores.Signals.Create(ctx, "old", state.SignalRecord{SourceRef: "s", ExpiresAt: &past}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := b.Stores.Signals.Update(ctx, "old", "1", state.SignalRecord{}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Update of an expired record = %v, want ErrNotFound", err)
	}
	if err := b.Stores.Signals.Delete(ctx, "old"); err != nil {
		t.Fatalf("Delete of an expired record = %v, want nil", err)
	}
}

func TestResumeTokensThisBackendDidNotMintAreRefused(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	for _, resume := range []state.Revision{"junk", "x.1", "1.x", "1.2.3"} {
		if _, err := b.Stores.Runs.WatchWithItems(ctx, resume); err == nil {
			t.Errorf("WatchWithItems resumed from %q", resume)
		}
	}
	if _, err := b.Stores.Conversations.Watch(ctx, "junk"); err == nil {
		t.Error("Watch resumed from a token that is not a sequence")
	}
	// A bare sequence is what a listing returns as a record's revision, and a
	// reader resumes from exactly that: it must be accepted.
	if _, err := b.Stores.Runs.WatchWithItems(ctx, "5"); err != nil {
		t.Errorf("WatchWithItems refused a bare sequence, which is what a listing returns: %v", err)
	}
}

func TestAnIdleFeedKeepsItsResumePointMoving(t *testing.T) {
	prev := bookmarkInterval
	bookmarkInterval = 20 * time.Millisecond
	t.Cleanup(func() { bookmarkInterval = prev })

	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := b.Stores.Conversations.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	mustConversation(t, b, "wake")
	deadline := time.After(5 * time.Second)
	sawPut := false
	for {
		select {
		case ev := <-ch:
			switch ev.Type {
			case state.EventPut:
				sawPut = true
			case state.EventBookmark:
				if !sawPut || ev.Resume == "" {
					t.Fatalf("bookmark %+v before a delivered event or without a resume point", ev)
				}
				return
			}
		case <-deadline:
			t.Fatal("an idle feed never emitted a bookmark")
		}
	}
}

func TestAFeedStopsAndClosesWhenItsCallerLeavesEvenWithABookmarkOwed(t *testing.T) {
	prev := bookmarkInterval
	bookmarkInterval = 5 * time.Millisecond
	t.Cleanup(func() { bookmarkInterval = prev })

	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := b.Stores.Conversations.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	mustConversation(t, b, "w1")
	time.Sleep(50 * time.Millisecond) // a bookmark is now owed and nobody is reading
	cancel()
	closed := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-closed:
			t.Fatal("the feed never closed after its caller left")
		}
	}
}

func TestAFeedSkipsAnEntryItCannotDecodeAndKeepsGoing(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := b.Stores.Conversations.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if _, err := b.conversations.kv.Put(ctx, wireKey("garbled"), []byte("{")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mustConversation(t, b, "fine")
	for {
		select {
		case ev := <-ch:
			if ev.Key == "garbled" {
				t.Fatalf("delivered an event for an undecodable entry: %+v", ev)
			}
			if ev.Key == "fine" {
				return
			}
		case <-ctx.Done():
			t.Fatal("the feed stalled behind an undecodable entry")
		}
	}
}

func TestTheFeedWithItemsDeliversADeleteForARemovedParent(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	parent := mustConversation(t, b, "going")
	ch, err := b.Stores.Runs.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf("WatchWithItems: %v", err)
	}
	if err := b.Stores.Conversations.Delete(ctx, parent); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for {
		select {
		case ev := <-ch:
			if ev.Type == state.EventDelete && ev.Key == parent {
				if ev.Resume == "" || !strings.Contains(string(ev.Resume), ".") {
					t.Fatalf("delete carried resume %q, want a <parent>.<items> pair", ev.Resume)
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("no delete arrived for a removed parent")
		}
	}
}

func TestTheFeedWithItemsEndsRatherThanLoseAnEventItCannotBuild(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	parent := mustConversation(t, b, "unreadable")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	inner := a.parent.kv
	t.Cleanup(func() { a.parent.kv = inner })

	ch, err := a.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf("WatchWithItems: %v", err)
	}
	// The parent cannot be read for a moment: delivering a delete would evict a
	// record that still exists, and skipping would lose the change.
	a.parent.kv = &scriptedKV{KeyValue: inner, get: always(errInjected)}
	if _, err := a.itemsKV.Create(ctx, runsItemKey(string(parent), "u1"), mustEnvelope(t)); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return // the feed ended; the consumer resumes from its last revision
			}
			if ev.Key == parent {
				t.Fatalf("an unreadable parent produced %+v, want the feed to end", ev)
			}
		case <-ctx.Done():
			t.Fatal("the feed neither delivered nor ended")
		}
	}
}

func TestTheFeedWithItemsSkipsEntriesItCannotDecodeAndStopsWithItsCaller(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx, cancel := context.WithCancel(context.Background())
	parent := mustConversation(t, b, "mixed")
	a := b.Stores.Runs.(*appended[state.ConversationRecord, state.Run])
	ch, err := a.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf("WatchWithItems: %v", err)
	}
	if _, err := a.parent.kv.Put(ctx, wireKey("garbled-parent"), []byte("{")); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if _, err := a.itemsKV.Put(ctx, "garbled-item.x", []byte("{")); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := a.Append(ctx, parent, state.Run{RunID: "r"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	deadline := time.After(10 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-ch:
			if ev.Key == "garbled-parent" {
				t.Fatalf("delivered an event for an undecodable parent: %+v", ev)
			}
			got = ev.Type == state.EventPut && ev.Key == parent && len(ev.Record.Items) == 1
		case <-deadline:
			t.Fatal("the feed stalled behind undecodable entries")
		}
	}
	cancel()
	for done := false; !done; {
		select {
		case _, ok := <-ch:
			done = !ok
		case <-time.After(5 * time.Second):
			t.Fatal("the feed never closed after its caller left")
		}
	}
}

func TestTheOrphanSweepReportsWhatItCannotRemove(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	ctx := shortCtx(t)
	if _, err := b.runsKV.Create(ctx, runsItemKey("ghost", "u"), mustEnvelope(t)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	inner := b.owned.runsKV
	b.owned.runsKV = &scriptedKV{KeyValue: inner, purge: always(errInjected)}
	t.Cleanup(func() { b.owned.runsKV = inner })
	if err := b.owned.Sweep(ctx); !errors.Is(err, errInjected) {
		t.Fatalf("Sweep = %v, want the purge failure", err)
	}
	if err := b.owned.purgeRuns(ctx, "ghost"); !errors.Is(err, errInjected) {
		t.Fatalf("purgeRuns = %v, want the purge failure", err)
	}
	// A conversation the sweep cannot look up is an error, not an orphan.
	c := b.owned.conversations
	realKV := c.kv
	c.kv = &scriptedKV{KeyValue: realKV, get: always(errInjected)}
	t.Cleanup(func() { c.kv = realKV })
	b.owned.runsKV = inner
	if err := b.owned.Sweep(ctx); !errors.Is(err, errInjected) {
		t.Fatalf("Sweep with an unreadable conversation = %v", err)
	}
	if n := len(mustKeys(t, b, "ghost")); n != 1 {
		t.Fatalf("the sweep removed a run it could not prove orphaned (%d left)", n)
	}
}

func mustKeys(t *testing.T, b *Backend, parent string) []string {
	t.Helper()
	keys, err := b.owned.runKeys(shortCtx(t), runsPrefixFilter(parent))
	if err != nil {
		t.Fatalf("runKeys: %v", err)
	}
	return keys
}

func signalFixtureForRetry() state.SignalRecord { return state.SignalRecord{SourceRef: "s"} }
