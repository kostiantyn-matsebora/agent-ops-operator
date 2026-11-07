// Task 3.1: DeleteAt honours the UID and resourceVersion preconditions the
// source release always used, so a record rewritten since the caller read it
// is refused with the conflict error and left intact.
package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestDeleteAtRefusesAStaleRevisionAndKeepsTheRecord(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-deleteat-stale")
	stale, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatal(err)
	}
	rec.Title = "rewritten after the caller read it"
	if _, err := s.Conversations.Update(ctx, state.Key(rec.Name), stale, rec); err != nil {
		t.Fatal(err)
	}

	err = s.Conversations.DeleteAt(ctx, state.Key(rec.Name), stale)
	if !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt on a stale revision = %v, want ErrConflict", err)
	}
	got, _, err := s.Conversations.Get(ctx, state.Key(rec.Name))
	if err != nil || got.Title != rec.Title {
		t.Fatalf("the record after a refused DeleteAt = %+v, %v; want it intact", got, err)
	}
}

func TestDeleteAtTheCurrentRevisionRemovesTheRecord(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-deleteat-current")
	if _, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatal(err)
	}
	_, rev, err := s.Conversations.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Conversations.DeleteAt(ctx, state.Key(rec.Name), rev); err != nil {
		t.Fatalf("DeleteAt on the current revision: %v", err)
	}
	if _, _, err := s.Conversations.Get(ctx, state.Key(rec.Name)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after DeleteAt = %v, want ErrNotFound", err)
	}
}

func TestDeleteAtAnAbsentKeyIsNotFound(t *testing.T) {
	ns, s := newTestNamespace(t)
	ctx := context.Background()
	if err := s.Conversations.DeleteAt(ctx, "never-created", "1"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("DeleteAt of an absent conversation = %v, want ErrNotFound", err)
	}
	if err := s.Signals.DeleteAt(ctx, "never-created", "1"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("DeleteAt of an absent signal = %v, want ErrNotFound", err)
	}
	newSignalSource(t, ns, "src")
	if err := s.Cooldowns.DeleteAt(ctx, "src", "1"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("DeleteAt of a window nothing recorded = %v, want ErrNotFound", err)
	}
}

// A record holding a finalizer is marked by DeleteAt, not removed, exactly as
// Delete marks it.
func TestDeleteAtKeepsDeleteFinalizerSemantics(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-deleteat-finalizer")
	rec.Finalizers = []string{"example.com/hold"}
	if _, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatal(err)
	}
	_, rev, _ := s.Conversations.Get(ctx, state.Key(rec.Name))
	if err := s.Conversations.DeleteAt(ctx, state.Key(rec.Name), rev); err != nil {
		t.Fatalf("DeleteAt: %v", err)
	}
	marked, _, err := s.Conversations.Get(ctx, state.Key(rec.Name))
	if err != nil || marked.DeletionRequestedAt == nil {
		t.Fatalf("a held record after DeleteAt = %+v, %v; want it readable and marked", marked, err)
	}
}

// The signal's release decides on a signal and deletes it under the revision it
// read: one rewritten in between is not deleted on the strength of the old
// decision.
func TestDeleteAtASignalRewrittenSinceIsRefused(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	store := openSignals(t, wwClient, ns, DefaultSweepInterval)
	rec := state.SignalRecord{Name: "sig-d", SourceRef: "src", Adapter: "cron", InitiationID: "i"}
	if _, err := store.Create(ctx, "sig-d", rec); err != nil {
		t.Fatal(err)
	}
	_, read, err := store.Get(ctx, "sig-d")
	if err != nil {
		t.Fatal(err)
	}
	rec.Attempts = 3
	if _, err := store.Update(ctx, "sig-d", read, rec); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAt(ctx, "sig-d", read); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt on the revision read before the rewrite = %v, want ErrConflict", err)
	}
	var sig agentopsv1alpha1.Signal
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sig-d"}, &sig); err != nil || sig.Status.Attempts != 3 {
		t.Fatalf("the signal after the refused delete: attempts %d, %v; want it intact", sig.Status.Attempts, err)
	}
	_, current, _ := store.Get(ctx, "sig-d")
	if err := store.DeleteAt(ctx, "sig-d", current); err != nil {
		t.Fatalf("DeleteAt on the current revision: %v", err)
	}
	if _, _, err := store.Get(ctx, "sig-d"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after DeleteAt = %v, want ErrNotFound", err)
	}
}

// The window is a part of the source, so a conditional delete of it clears the
// window under the source's revision and leaves the source.
func TestDeleteAtAWindowIsConditionalOnTheSourceRevision(t *testing.T) {
	ctx := context.Background()
	ns, s := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	at := time.Now().UTC().Truncate(time.Second)
	stale, err := s.Cooldowns.Create(ctx, "src", state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "i", At: at}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Receipts.Create(ctx, "src", state.Received{Total: 1, Last: &at}); err != nil { // moves the source's revision
		t.Fatal(err)
	}
	if err := s.Cooldowns.DeleteAt(ctx, "src", stale); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt on a stale source revision = %v, want ErrConflict", err)
	}
	if w, _, err := s.Cooldowns.Get(ctx, "src"); err != nil || len(w.Entries) != 1 {
		t.Fatalf("the window after a refused delete = %+v, %v; want it intact", w, err)
	}
	_, cur, _ := s.Cooldowns.Get(ctx, "src")
	if err := s.Cooldowns.DeleteAt(ctx, "src", cur); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Cooldowns.Get(ctx, "src"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after DeleteAt = %v, want ErrNotFound", err)
	}
	if r, _, err := s.Receipts.Get(ctx, "src"); err != nil || r.Total != 1 {
		t.Fatalf("the counters after clearing the window = %+v, %v; want them kept", r, err)
	}
}
