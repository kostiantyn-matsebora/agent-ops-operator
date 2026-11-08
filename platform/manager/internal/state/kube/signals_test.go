package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func newSignalSource(t *testing.T, ns, name string) *agentopsv1alpha1.SignalSource {
	t.Helper()
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, ns
	src.Spec.Adapter = "cron"
	if err := wwClient.Create(context.Background(), src); err != nil {
		t.Fatalf("create source: %v", err)
	}
	return src
}

// TestSignalsCollectionWritesTheObjectPublicationAlwaysWrote pins "byte-identical":
// a Signal created through the store carries the same spec and owner reference
// as one created by the Client call publication made before the store existed.
func TestSignalsCollectionWritesTheObjectPublicationAlwaysWrote(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	src := newSignalSource(t, ns, "src")
	store := openSignals(t, wwClient, ns, DefaultSweepInterval)

	rec := state.SignalRecord{
		Name: "sig-new", SourceRef: src.Name, SourceID: string(src.UID), Adapter: "cron", InitiationID: "init-1",
		SignalID: "id-1", Kind: "alert", Title: "t", Severity: "warning",
		Labels: map[string]string{"a": "b"}, Reader: "r", Payload: "body",
	}
	if _, err := store.Create(ctx, "sig-new", rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	old := &agentopsv1alpha1.Signal{}
	old.Name, old.Namespace = "sig-old", ns
	old.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef: agentopsv1alpha1.ObjectRef{Name: src.Name}, Adapter: "cron", InitiationID: "init-1",
		SignalID: "id-1", Kind: "alert", Title: "t", Severity: "warning",
		Labels: map[string]string{"a": "b"}, Reader: "r", Payload: "body",
	}
	if err := controllerutil.SetControllerReference(src, old, wwClient.Scheme()); err != nil {
		t.Fatal(err)
	}
	if err := wwClient.Create(ctx, old); err != nil {
		t.Fatal(err)
	}

	var got, want agentopsv1alpha1.Signal
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sig-new"}, &got); err != nil {
		t.Fatal(err)
	}
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sig-old"}, &want); err != nil {
		t.Fatal(err)
	}
	if !equality.Semantic.DeepEqual(got.Spec, want.Spec) {
		t.Errorf("spec differs:\n got %+v\nwant %+v", got.Spec, want.Spec)
	}
	if !equality.Semantic.DeepEqual(got.OwnerReferences, want.OwnerReferences) {
		t.Errorf("owner references differ:\n got %+v\nwant %+v", got.OwnerReferences, want.OwnerReferences)
	}
	if len(got.Annotations) != 0 {
		t.Errorf("a record with no expiry carries annotations %v, want none", got.Annotations)
	}
	read, _, err := store.Get(ctx, "sig-new")
	if err != nil || read.SourceID != string(src.UID) {
		t.Errorf("SourceID read back = %q, %v, want the source's identity", read.SourceID, err)
	}
}

// TestSignalExpiryIsTheOneAnnotationAndOnlyOnTheWriteThatSetsIt: a terminal
// write sets the annotation, the status write alone changes nothing else.
func TestSignalExpiryIsTheOneAnnotationAndOnlyOnTheWriteThatSetsIt(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	newSignalSource(t, ns, "src")
	store := openSignals(t, wwClient, ns, DefaultSweepInterval)
	rec := state.SignalRecord{Name: "sig-x", SourceRef: "src", Adapter: "cron", InitiationID: "i"}
	rev, err := store.Create(ctx, "sig-x", rec)
	if err != nil {
		t.Fatal(err)
	}
	rec.Attempts = 1
	rev2, err := store.Update(ctx, "sig-x", rev, rec)
	if err != nil || rev2 == rev {
		t.Fatalf("status write: %v (rev %s -> %s)", err, rev, rev2)
	}
	var sig agentopsv1alpha1.Signal
	_ = wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sig-x"}, &sig)
	if len(sig.Annotations) != 0 {
		t.Fatalf("annotations after a non-terminal write = %v, want none", sig.Annotations)
	}
	at := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	rec.Phase, rec.ExpiresAt = state.SignalRouted, &at
	if _, err := store.Update(ctx, "sig-x", rev2, rec); err != nil {
		t.Fatal(err)
	}
	_ = wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sig-x"}, &sig)
	if len(sig.Annotations) != 1 || sig.Annotations[ExpiresAtAnnotation] == "" || sig.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("object = annotations %v phase %q, want the one expiry annotation and Routed", sig.Annotations, sig.Status.Phase)
	}
}

// TestCooldownLivesOnTheSourceStatusAndAGoneSourceIsNotFound: the window is
// SignalSource.status.cooldown, read and written as before; a source that is
// gone reads not found and refuses a write by name, and a source with nothing
// recorded reads not found too (design D-3).
func TestCooldownLivesOnTheSourceStatusAndAGoneSourceIsNotFound(t *testing.T) {
	ctx := context.Background()
	ns, store := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	at := time.Now().UTC().Truncate(time.Second)

	if _, _, err := store.Cooldowns.Get(ctx, "src"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a source with no window = %v, want ErrNotFound", err)
	}
	rev, err := store.Cooldowns.Create(ctx, "src", state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "i1", At: at}}})
	if err != nil {
		t.Fatal(err)
	}
	var src agentopsv1alpha1.SignalSource
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "src"}, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Status.Cooldown) != 1 || src.Status.Cooldown[0].InitiationID != "i1" || !src.Status.Cooldown[0].At.Time.Equal(at) {
		t.Fatalf("status.cooldown = %+v, want the one entry", src.Status.Cooldown)
	}
	if _, err := store.Cooldowns.Create(ctx, "src", state.CooldownWindow{}); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create = %v, want ErrAlreadyExists", err)
	}
	got, gotRev, err := store.Cooldowns.Get(ctx, "src")
	if err != nil || len(got.Entries) != 1 || gotRev != rev {
		t.Fatalf("Get = %+v rev %q, %v; want the entry at the create revision %q", got, gotRev, err, rev)
	}

	if _, _, err := store.Cooldowns.Get(ctx, "gone"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a gone source = %v, want ErrNotFound", err)
	}
	if _, err := store.Cooldowns.Create(ctx, "gone", state.CooldownWindow{Entries: got.Entries}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Create on a gone source error = %v, want ErrNotFound", err)
	}
	if err := store.Cooldowns.Delete(ctx, "gone"); err != nil {
		t.Fatalf("Delete on a gone source = %v, want nil", err)
	}

	// Clearing removes the window and nothing else of the status.
	if err := store.Cooldowns.Delete(ctx, "src"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Cooldowns.Get(ctx, "src"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

// TestSignalSweepDeletesTheExpiredObjectAndKeepsTheLiveOne reads the API object
// itself, not the store: an expired Signal is invisible through Get the moment
// its instant passes, so only the object proves the sweep removed it.
func TestSignalSweepDeletesTheExpiredObjectAndKeepsTheLiveOne(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ns := newNamespace(t)
	store := openSignals(t, wwClient, ns, 50*time.Millisecond)
	go store.Run(ctx)

	now := time.Now()
	for name, at := range map[string]time.Time{
		"sig-past": now.Add(-time.Minute), "sig-soon": now.Add(2 * time.Second), "sig-later": now.Add(time.Hour),
	} {
		at := at
		if _, err := store.Create(ctx, state.Key(name), state.SignalRecord{Name: name, SourceRef: "src", Adapter: "cron", InitiationID: name, ExpiresAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool {
		var sig agentopsv1alpha1.Signal
		return wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &sig) == nil
	}
	waitGone := func(name string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for exists(name) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: the object still exists after its expiry", name)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitGone("sig-past")
	if !exists("sig-later") {
		t.Fatal("a sweep took an object that had not expired")
	}
	waitGone("sig-soon") // a later expiry is found by a later sweep
	if !exists("sig-later") {
		t.Fatal("the one expiring in an hour was swept")
	}
}

// TestReceiptsWriteTheStatusTheRouterAlwaysWrote: the counters stay
// status.receivedTotal and status.lastReceived on the source, written by a
// status merge patch under the object's revision, and a gone source is not
// found.
func TestReceiptsWriteTheStatusTheRouterAlwaysWrote(t *testing.T) {
	ctx := context.Background()
	ns, store := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	at := time.Now().UTC().Truncate(time.Second)
	if _, err := store.Receipts.Create(ctx, "src", state.Received{Total: 1, Last: &at}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := state.Mutate(ctx, store.Receipts, "src", func(r *state.Received) error {
			r.Total++
			r.Last = &at
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var src agentopsv1alpha1.SignalSource
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "src"}, &src); err != nil {
		t.Fatal(err)
	}
	if src.Status.ReceivedTotal != 3 || src.Status.LastReceived == nil || !src.Status.LastReceived.Time.Equal(at) {
		t.Fatalf("source status = total %d last %v, want 3 at %v", src.Status.ReceivedTotal, src.Status.LastReceived, at)
	}
	if _, err := store.Receipts.Create(ctx, "gone", state.Received{Total: 1, Last: &at}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Create on a gone source error = %v, want ErrNotFound", err)
	}
	if _, _, err := store.Receipts.Get(ctx, "gone"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a gone source = %v, want ErrNotFound", err)
	}
}
