// Tasks 2.7-2.9: identity, the deletion lifecycle, run mutation and the
// lookup labels, as the kube backend maps them onto a Conversation. The
// contract's own behaviour is the conformance suite's; these tests pin what
// only this backend can show: the stored object.
package kube

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func getObject(t *testing.T, ns, name string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := wwClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatalf("get object %s: %v", name, err)
	}
	return &conv
}

func TestIdentityIsTheObjectUIDAndCreationTime(t *testing.T) {
	ns, s := newTestNamespace(t)
	rec := fixtureRecord("conv-identity-uid")
	if _, err := s.Create(context.Background(), state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, err := s.Get(context.Background(), state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	obj := getObject(t, ns, rec.Name)
	if got.ID != string(obj.UID) || !got.CreatedAt.Equal(obj.CreationTimestamp.Time) {
		t.Fatalf("ID %q CreatedAt %v, want the object's UID %q and creationTimestamp %v", got.ID, got.CreatedAt, obj.UID, obj.CreationTimestamp)
	}
}

func TestFinalizersRoundTrip(t *testing.T) {
	ns, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-finalizers")
	rec.Finalizers = []string{"example.dev/hold"}
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertRecordsEqual(t, got, rec)
	if f := getObject(t, ns, rec.Name).Finalizers; len(f) != 1 || f[0] != "example.dev/hold" {
		t.Fatalf("object finalizers = %v, want the record's", f)
	}

	// Marked, then released by Update: the object is gone, status included.
	if err := s.Delete(ctx, state.Key(rec.Name)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	marked, rev, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil || marked.DeletionRequestedAt == nil {
		t.Fatalf("Get marked = %+v, %v", marked, err)
	}
	if !getObject(t, ns, rec.Name).DeletionTimestamp.Time.Equal(*marked.DeletionRequestedAt) {
		t.Fatalf("DeletionRequestedAt is not the object's deletionTimestamp")
	}
	marked.Finalizers = nil
	marked.Phase = "Closed"
	if _, err := s.Update(ctx, state.Key(rec.Name), rev, marked); err != nil {
		t.Fatalf("Update releasing: %v", err)
	}
	if _, _, err := s.Get(ctx, state.Key(rec.Name)); err == nil {
		t.Fatalf("record still readable after its last finalizer was released")
	}
}

func TestUpdateKeepsMetadataTheRecordDoesNotModel(t *testing.T) {
	ns, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-foreign-meta")
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Something else (kubectl, another controller) labels and annotates it.
	obj := getObject(t, ns, rec.Name)
	obj.Labels["example.dev/foreign"] = "kept"
	obj.Annotations = map[string]string{"example.dev/note": "kept"}
	if err := wwClient.Update(ctx, obj); err != nil {
		t.Fatalf("foreign write: %v", err)
	}
	cur, rev, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = rev
	cur.Phase = "Closed"
	if _, err := s.Update(ctx, state.Key(rec.Name), rev, cur); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after := getObject(t, ns, rec.Name)
	if after.Labels["example.dev/foreign"] != "kept" || after.Annotations["example.dev/note"] != "kept" {
		t.Fatalf("Update dropped metadata it does not model: labels %v annotations %v", after.Labels, after.Annotations)
	}
}

func TestLookupLabelsMirrorTheRecord(t *testing.T) {
	ns, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-labels")
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	obj := getObject(t, ns, rec.Name)
	if obj.Labels[agentopsv1alpha1.LabelCausedBy] != "parent-conv" || obj.Labels[agentopsv1alpha1.LabelNameBase] != "task-fixture-words" {
		t.Fatalf("labels = %v, want %s=parent-conv and %s=task-fixture-words", obj.Labels, agentopsv1alpha1.LabelCausedBy, agentopsv1alpha1.LabelNameBase)
	}

	rec.CausedBy = nil
	if _, err := s.Update(ctx, state.Key(rec.Name), rev, rec); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, has := getObject(t, ns, rec.Name).Labels[agentopsv1alpha1.LabelCausedBy]; has {
		t.Fatalf("caused-by label outlived the record's CausedBy")
	}
}

func TestUpdateRunWritesNothingWhenAlreadySatisfied(t *testing.T) {
	ns, s := newTestNamespace(t)
	ctx := context.Background()
	key := state.Key("conv-run-noop")
	if _, err := s.Create(ctx, key, fixtureRecord("conv-run-noop")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendRun(ctx, key, state.Run{RunID: "r1", DeliveryTracked: true, RoutedToParent: true, Delivered: []string{"chan-a"}}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	before := getObject(t, ns, string(key)).ResourceVersion
	if err := s.UpdateRun(ctx, key, "r1", state.RunUpdate{MarkDeliveryTracked: true, MarkRoutedToParent: true, MarkDelivered: []string{"chan-a"}}); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	if after := getObject(t, ns, string(key)).ResourceVersion; after != before {
		t.Fatalf("an already-satisfied UpdateRun wrote: resourceVersion %s -> %s", before, after)
	}
}

// The cause of the failures under a non-UTC TZ: the apiserver client decodes
// every metav1.Time into the process's local zone, so a mapping that copied
// the value through handed the caller local-zone instants.
func TestInstantsReadBackInUTCUnderANonUTCLocalZone(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("UTC+3", 3*60*60)
	t.Cleanup(func() { time.Local = saved })

	_, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-utc-kube")
	rec.Finalizers = nil
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertRecordsEqual(t, got, rec)
	if got.CreatedAt.Location() != time.UTC || got.Inflight.DispatchedAt.Location() != time.UTC || got.Inputs[0].ReceivedAt.Location() != time.UTC {
		t.Fatalf("instants not in UTC: %v %v %v", got.CreatedAt.Location(), got.Inflight.DispatchedAt.Location(), got.Inputs[0].ReceivedAt.Location())
	}
}
