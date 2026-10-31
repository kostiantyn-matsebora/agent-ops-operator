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

// TestSignalStoreWritesTheObjectPublicationAlwaysWrote pins "byte-identical":
// a Signal created through the store carries the same spec and owner reference
// as one created by the Client call publication made before the store existed.
func TestSignalStoreWritesTheObjectPublicationAlwaysWrote(t *testing.T) {
	ctx := context.Background()
	ns, _ := newTestNamespace(t)
	src := newSignalSource(t, ns, "src")
	store := NewSignalStore(wwClient, ns)

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
	ns, _ := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	store := NewSignalStore(wwClient, ns)
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
// gone reads empty and refuses a write by name.
func TestCooldownLivesOnTheSourceStatusAndAGoneSourceIsNotFound(t *testing.T) {
	ctx := context.Background()
	ns, _ := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	store := NewSignalStore(wwClient, ns)
	at := time.Now().UTC().Truncate(time.Second)
	if err := store.PutCooldown(ctx, "src", []state.CooldownEntry{{InitiationID: "i1", At: at}}); err != nil {
		t.Fatal(err)
	}
	var src agentopsv1alpha1.SignalSource
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "src"}, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Status.Cooldown) != 1 || src.Status.Cooldown[0].InitiationID != "i1" || !src.Status.Cooldown[0].At.Time.Equal(at) {
		t.Fatalf("status.cooldown = %+v, want the one entry", src.Status.Cooldown)
	}
	if got, err := store.GetCooldown(ctx, "gone"); err != nil || len(got) != 0 {
		t.Fatalf("GetCooldown of a gone source = %v, %v, want empty", got, err)
	}
	if err := store.PutCooldown(ctx, "gone", nil); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("PutCooldown of a gone source error = %v, want ErrNotFound", err)
	}
	if err := store.ReleaseSource(ctx, "gone", ""); err != nil {
		t.Fatalf("ReleaseSource of a gone source: %v, want nil", err)
	}
}

// TestReleaseSourceSparesARecreatedSource: a source deleted and recreated under
// one name must lose the previous incarnation's signals only. The new source's
// signal and its suppression window stay.
func TestReleaseSourceSparesARecreatedSource(t *testing.T) {
	ctx := context.Background()
	ns, _ := newTestNamespace(t)
	store := NewSignalStore(wwClient, ns)

	first := newSignalSource(t, ns, "src")
	if _, err := store.Create(ctx, "sig-old", state.SignalRecord{Name: "sig-old", SourceRef: "src", SourceID: string(first.UID), Adapter: "cron", InitiationID: "i-old"}); err != nil {
		t.Fatal(err)
	}
	if err := wwClient.Delete(ctx, first); err != nil { // envtest runs no garbage collector
		t.Fatal(err)
	}
	second := newSignalSource(t, ns, "src")
	if _, err := store.Create(ctx, "sig-new", state.SignalRecord{Name: "sig-new", SourceRef: "src", SourceID: string(second.UID), Adapter: "cron", InitiationID: "i-new"}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := store.PutCooldown(ctx, "src", []state.CooldownEntry{{InitiationID: "i-new", At: at}}); err != nil {
		t.Fatal(err)
	}

	if err := store.ReleaseSource(ctx, "src", string(first.UID)); err != nil {
		t.Fatalf("ReleaseSource: %v", err)
	}
	if _, _, err := store.Get(ctx, "sig-old"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("the previous incarnation's signal: error = %v, want released", err)
	}
	if _, _, err := store.Get(ctx, "sig-new"); err != nil {
		t.Errorf("the recreated source's signal: error = %v, want kept", err)
	}
	if got, err := store.GetCooldown(ctx, "src"); err != nil || len(got) != 1 {
		t.Errorf("the recreated source's window = %v, %v, want kept", got, err)
	}
}

// TestSignalSweepDeletesTheExpiredObjectAndKeepsTheLiveOne reads the API object
// itself, not the store: an expired Signal is invisible through Get the moment
// its instant passes, so only the object proves the sweep removed it.
func TestSignalSweepDeletesTheExpiredObjectAndKeepsTheLiveOne(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ns, _ := newTestNamespace(t)
	store := NewSignalStoreWithSweepInterval(wwClient, ns, 50*time.Millisecond)
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

// TestRecordReceivedWritesTheStatusTheRouterAlwaysWrote: the counters stay
// status.receivedTotal and status.lastReceived on the source, and a gone source
// is not found.
func TestRecordReceivedWritesTheStatusTheRouterAlwaysWrote(t *testing.T) {
	ctx := context.Background()
	ns, _ := newTestNamespace(t)
	newSignalSource(t, ns, "src")
	store := NewSignalStore(wwClient, ns)
	at := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		if err := store.RecordReceived(ctx, "src", at); err != nil {
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
	if err := store.RecordReceived(ctx, "gone", at); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("RecordReceived of a gone source error = %v, want ErrNotFound", err)
	}
	if got, err := store.GetReceived(ctx, "gone"); err != nil || got.Total != 0 {
		t.Fatalf("GetReceived of a gone source = %+v, %v, want zero", got, err)
	}
}
