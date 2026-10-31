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
	if err := store.ReleaseSource(ctx, "gone"); err != nil {
		t.Fatalf("ReleaseSource of a gone source: %v, want nil", err)
	}
}
