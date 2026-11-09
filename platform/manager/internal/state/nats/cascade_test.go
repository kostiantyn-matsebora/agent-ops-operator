package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const finalizerHold = "example.com/hold"

// cascadeEnv is one isolated backend, the handles a cascade test reads behind
// the contract's back, and a clock the orphan sweep can be aged with.
type cascadeEnv struct {
	b   *Backend
	ctx context.Context
	now time.Time
}

func newCascadeEnv(t *testing.T) *cascadeEnv {
	t.Helper()
	b := openBackend(t, sharedTestConn(t), newScope(t))
	return &cascadeEnv{b: b, ctx: context.Background(), now: time.Now()}
}

// children is the backend's own cascade, with the test's clock.
func (e *cascadeEnv) children() *children {
	c := *e.b.owned
	c.now = func() time.Time { return e.now }
	return &c
}

func (e *cascadeEnv) conversation(t *testing.T, name string, finalizers ...string) {
	t.Helper()
	if _, err := e.b.Stores.Conversations.Create(e.ctx, state.Key(name), state.ConversationRecord{ProfileRef: "default", Finalizers: finalizers}); err != nil {
		t.Fatalf("create conversation %q: %v", name, err)
	}
}

func (e *cascadeEnv) runs(t *testing.T, conversation string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := e.b.Stores.Runs.Append(e.ctx, state.Key(conversation), state.Run{RunID: runName(i), Status: "succeeded"}); err != nil {
			t.Fatalf("append run %d to %q: %v", i, conversation, err)
		}
	}
}

func (e *cascadeEnv) blob(t *testing.T, key, conversation string) {
	t.Helper()
	if err := e.b.Stores.Blobs.Put(e.ctx, state.Key(key), state.Blob{ConversationName: conversation, Type: "task", Payload: "body of " + key}); err != nil {
		t.Fatalf("put blob %q: %v", key, err)
	}
}

func (e *cascadeEnv) runKeysOf(t *testing.T, conversation string) int {
	t.Helper()
	keys, err := e.b.owned.runKeys(e.ctx, runsPrefixFilter(conversation))
	if err != nil {
		t.Fatalf("list run keys of %q: %v", conversation, err)
	}
	return len(keys)
}

func (e *cascadeEnv) hasBlob(t *testing.T, key string) bool {
	t.Helper()
	_, err := e.b.Stores.Blobs.Get(e.ctx, state.Key(key))
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("get blob %q: %v", key, err)
	}
	return err == nil
}

func runName(i int) string { return "run-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) }

func TestRunHistoryIsBoundedToTheContractsLimit(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "long-lived")
	total := state.RunHistoryLimit + 4
	e.runs(t, "long-lived", total)

	got, err := e.b.Stores.Runs.List(e.ctx, "long-lived")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != state.RunHistoryLimit {
		t.Fatalf("kept %d runs, want the last %d", len(got), state.RunHistoryLimit)
	}
	if got[0].RunID != runName(total-state.RunHistoryLimit) || got[len(got)-1].RunID != runName(total-1) {
		t.Fatalf("kept %q..%q, want the newest %d in append order", got[0].RunID, got[len(got)-1].RunID, state.RunHistoryLimit)
	}
	err = e.b.Stores.Runs.Grow(e.ctx, "long-lived", runName(0), state.Run{DeliveryTracked: true})
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow on a run that aged out = %v, want ErrNotFound", err)
	}
}

func TestRunHistoryUnderTheLimitIsUntouched(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "short")
	e.runs(t, "short", state.RunHistoryLimit)
	got, _ := e.b.Stores.Runs.List(e.ctx, "short")
	if len(got) != state.RunHistoryLimit {
		t.Fatalf("kept %d runs, want all %d", len(got), state.RunHistoryLimit)
	}
}

func TestDeletingAConversationRemovesItsRunsAndBlobsOnly(t *testing.T) {
	cases := map[string]func(t *testing.T, e *cascadeEnv){
		"Delete": func(t *testing.T, e *cascadeEnv) {
			if err := e.b.Stores.Conversations.Delete(e.ctx, "doomed"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		},
		"DeleteAt": func(t *testing.T, e *cascadeEnv) {
			_, rev, err := e.b.Stores.Conversations.Get(e.ctx, "doomed")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if err := e.b.Stores.Conversations.DeleteAt(e.ctx, "doomed", rev); err != nil {
				t.Fatalf("DeleteAt: %v", err)
			}
		},
	}
	for name, remove := range cases {
		t.Run(name, func(t *testing.T) {
			e := newCascadeEnv(t)
			e.conversation(t, "doomed")
			e.conversation(t, "bystander")
			e.runs(t, "doomed", 3)
			e.runs(t, "bystander", 2)
			e.blob(t, "doomed-input", "doomed")
			e.blob(t, "bystander-input", "bystander")

			remove(t, e)

			if n := e.runKeysOf(t, "doomed"); n != 0 {
				t.Errorf("%d runs of the deleted conversation remain", n)
			}
			if e.hasBlob(t, "doomed-input") {
				t.Error("a blob of the deleted conversation remains")
			}
			if n := e.runKeysOf(t, "bystander"); n != 2 {
				t.Errorf("the other conversation has %d runs, want 2", n)
			}
			if !e.hasBlob(t, "bystander-input") {
				t.Error("the other conversation's blob was removed")
			}
		})
	}
}

func TestAConversationHeldByAFinalizerKeepsItsChildrenUntilReleased(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "held", finalizerHold)
	e.runs(t, "held", 2)
	e.blob(t, "held-input", "held")

	if err := e.b.Stores.Conversations.Delete(e.ctx, "held"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if e.runKeysOf(t, "held") != 2 || !e.hasBlob(t, "held-input") {
		t.Fatal("children removed while a finalizer still holds the conversation")
	}

	rec, rev, err := e.b.Stores.Conversations.Get(e.ctx, "held")
	if err != nil {
		t.Fatalf("Get a deleting conversation: %v", err)
	}
	rec.Finalizers = nil
	if _, err := e.b.Stores.Conversations.Update(e.ctx, "held", rev, rec); err != nil {
		t.Fatalf("release the finalizer: %v", err)
	}
	if e.runKeysOf(t, "held") != 0 || e.hasBlob(t, "held-input") {
		t.Fatal("children survive the release that removed the conversation")
	}
}

func TestOrphanSweepRemovesWhatNoConversationOwns(t *testing.T) {
	e := newCascadeEnv(t)
	e.conversation(t, "alive")
	e.runs(t, "alive", 2)
	e.blob(t, "alive-input", "alive")

	// A crash between deleting the record and deleting its children: write the
	// children of a conversation that never existed.
	if _, err := e.b.runsKV.Create(e.ctx, runsItemKey("ghost", "u1"), mustEnvelope(t)); err != nil {
		t.Fatalf("seed orphan run: %v", err)
	}
	e.blob(t, "ghost-input", "ghost")
	e.blob(t, "nameless-input", "")

	sweep := e.children()
	if err := sweep.Sweep(e.ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if e.runKeysOf(t, "ghost") != 0 {
		t.Error("an orphan run survives the sweep")
	}
	if !e.hasBlob(t, "ghost-input") {
		t.Error("a young blob was swept: it may be a write in flight")
	}

	e.now = e.now.Add(orphanGrace + time.Minute)
	if err := sweep.Sweep(e.ctx); err != nil {
		t.Fatalf("Sweep after the grace: %v", err)
	}
	if e.hasBlob(t, "ghost-input") {
		t.Error("an old orphan blob survives the sweep")
	}
	if !e.hasBlob(t, "alive-input") || e.runKeysOf(t, "alive") != 2 {
		t.Error("the sweep removed a live conversation's children")
	}
	if !e.hasBlob(t, "nameless-input") {
		t.Error("a blob naming no conversation was swept; there is nobody to judge it orphaned")
	}
}

func TestOrphanSweepRunsOnItsInterval(t *testing.T) {
	conn := sharedTestConn(t)
	b := openBackend(t, conn, newScope(t), WithOrphanSweepInterval(20*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if _, err := b.runsKV.Create(ctx, runsItemKey("ghost", "u1"), mustEnvelope(t)); err != nil {
		t.Fatalf("seed orphan run: %v", err)
	}
	go b.Run(ctx)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		keys, _ := b.owned.runKeys(ctx, runsPrefixFilter("ghost"))
		if len(keys) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the running backend never swept an orphan run")
}

func mustEnvelope(t *testing.T) []byte {
	t.Helper()
	e := envelope[state.Run]{AppendSeq: 1, Record: state.Run{RunID: "orphan", Status: "succeeded"}}
	data, err := e.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return data
}
