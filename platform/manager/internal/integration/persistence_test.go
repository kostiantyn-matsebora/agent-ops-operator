// PERSISTENCE IS WIRING: the Pipeline declares where a route's conversations
// keep their state, the Conversation freezes what that resolved to, and nothing
// reads the Pipeline again.
//
// Four properties carry the change, and they are the storage reading of the
// four identity_test.go pins:
//
//	the CLAIM is frozen        — re-wiring must not move a conversation that has
//	                             ALREADY WRITTEN to a volume
//	the RUNTIME is not in it   — one runtime, two routes, two volumes
//	naming a VOLUME creates    — a pod mounts a claim, never a PersistentVolume
//	deleting the wiring does not delete the storage
package integration

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// bindPersistence points an existing Pipeline's context volume at a binding.
func bindPersistence(t *testing.T, name string, b *agentopsv1alpha1.PersistenceBinding) {
	t.Helper()
	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &p); err != nil {
		t.Fatal(err)
	}
	p.Spec.Persistence = &agentopsv1alpha1.PipelinePersistence{Context: b}
	if err := k8sClient.Update(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
}

// claimOf returns the claim a pod's context volume mounts, or "" for ephemeral.
func claimOf(pod *corev1.Pod, volume string) string {
	for _, v := range pod.Spec.Volumes {
		if v.Name != volume {
			continue
		}
		if v.PersistentVolumeClaim == nil {
			return ""
		}
		return v.PersistentVolumeClaim.ClaimName
	}
	return ""
}

// 1.3 — the API server refuses the broken combination, rather than a reconciler
// reporting it after the object is stored. A binding that named both would be
// two answers to "who creates the claim".
func TestABindingCannotNameBothAClaimAndAVolume(t *testing.T) {
	mkProfile(t, "pv-prof-xor")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "pv-xor-pipe", ns
	p.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "pv-prof-xor"}
	p.Spec.Persistence = &agentopsv1alpha1.PipelinePersistence{
		Context: &agentopsv1alpha1.PersistenceBinding{
			ClaimName: "a-claim", VolumeName: "a-volume",
		},
	}

	err := k8sClient.Create(context.Background(), p)

	if err == nil {
		_ = k8sClient.Delete(context.Background(), p)
		t.Fatal("the API server accepted claimName AND volumeName on one binding — they decide WHO " +
			"renders the claim, so both is not a preference, it is two answers")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("refused, but not by the CEL rule: %v", err)
	}
}

// 6.4 — THE REQUIREMENT THE WHOLE MOVE EXISTS FOR. Two routes, ONE runtime, two
// volumes. Before this the second volume needed a cloned AgentRuntime whose only
// difference was the claim it named.
func TestTwoPipelinesOneRuntimeTwoVolumes(t *testing.T) {
	mkProfile(t, "pv-prof-two")
	mkRuntime(t, "pv-rt-shared", "example/agent:vol", "")
	mkSignalSource(t, "pv-src-a", "pv-sig-a", "")
	mkSignalSource(t, "pv-src-b", "pv-sig-b", "")
	mkWiredPipeline(t, "pv-pipe-a", []string{"pv-src-a"}, "pv-prof-two", "pv-rt-shared", "")
	mkWiredPipeline(t, "pv-pipe-b", []string{"pv-src-b"}, "pv-prof-two", "pv-rt-shared", "")
	bindPersistence(t, "pv-pipe-a", &agentopsv1alpha1.PersistenceBinding{ClaimName: "team-a-context"})
	bindPersistence(t, "pv-pipe-b", &agentopsv1alpha1.PersistenceBinding{ClaimName: "team-b-context"})
	reconcilePipeline(t, "pv-pipe-a")
	reconcilePipeline(t, "pv-pipe-b")
	srv := apiServer()

	if rec := taskSignal(t, srv, "pv-src-a", "pv-a-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal a: %d %s", rec.Code, rec.Body.String())
	}
	if rec := taskSignal(t, srv, "pv-src-b", "pv-b-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal b: %d %s", rec.Code, rec.Body.String())
	}

	byClaim := map[string]bool{}
	for _, c := range convsForProfile(t, "pv-prof-two") {
		pod := podFor(t, c.Name)
		if got := pod.Spec.Containers[0].Image; got != "example/agent:vol" {
			t.Fatalf("%s: image = %q — both routes run ONE runtime, the volume is the difference", c.Name, got)
		}
		byClaim[claimOf(pod, "context")] = true
	}
	for _, want := range []string{"team-a-context", "team-b-context"} {
		if !byClaim[want] {
			t.Fatalf("no pod mounted %q — got %v", want, byClaim)
		}
	}
}

// 6.5 / 2.2 — THE FROZEN CLAIM, and the reason the snapshot exists. This is
// sharper than the identity case it copies: a privilege change is applied to
// work in progress, a storage change is applied to work that is already on disk.
func TestEditingAPipelineDoesNotMoveARunningConversationsVolume(t *testing.T) {
	mkProfile(t, "pv-prof-frozen")
	mkRuntime(t, "pv-rt-frozen", "example/agent:frozen", "")
	mkSignalSource(t, "pv-src-frozen", "pv-sig-frozen", "")
	mkWiredPipeline(t, "pv-frozen-pipe", []string{"pv-src-frozen"}, "pv-prof-frozen", "pv-rt-frozen", "")
	bindPersistence(t, "pv-frozen-pipe", &agentopsv1alpha1.PersistenceBinding{ClaimName: "claim-at-creation"})
	reconcilePipeline(t, "pv-frozen-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "pv-src-frozen", "pv-frozen-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "pv-prof-frozen")
	if conv.Spec.ContextClaimName != "claim-at-creation" {
		t.Fatalf("the conversation must snapshot the resolved claim: got %q", conv.Spec.ContextClaimName)
	}

	// Re-wire, as an operator moving the route to new storage would.
	bindPersistence(t, "pv-frozen-pipe", &agentopsv1alpha1.PersistenceBinding{ClaimName: "claim-after-edit"})

	pod := podFor(t, conv.Name)
	if got := claimOf(pod, "context"); got != "claim-at-creation" {
		t.Fatalf("the next pod mounted %q — a Pipeline edit moved a conversation that has already "+
			"written to the volume it was created against", got)
	}
}

// 2.2 — the release default is what a route binding nothing resolves to, and it
// is snapshotted RESOLVED. The runtime contributes nothing either way, which is
// what the deleted field means.
func TestARouteBindingNothingTakesTheReleaseDefault(t *testing.T) {
	mkProfile(t, "pv-prof-default")
	mkSignalSource(t, "pv-src-default", "pv-sig-default", "")
	mkPipeline(t, "pv-default-pipe", []string{"pv-src-default"}, nil, "pv-prof-default")
	reconcilePipeline(t, "pv-default-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "pv-src-default", "pv-default-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "pv-prof-default")
	// suite_test.go's fixture release default.
	if conv.Spec.ContextClaimName != "test-context" {
		t.Fatalf("ContextClaimName = %q, want the release default", conv.Spec.ContextClaimName)
	}
	if got := claimOf(podFor(t, conv.Name), "context"); got != "test-context" {
		t.Fatalf("the pod mounted %q, want the release default", got)
	}
}

// 2.3 / 6.3 — NAMING A VOLUME CREATES THE CLAIM. This is the one place in this
// system where naming a resource creates it, because a pod cannot mount a
// PersistentVolume.
//
// And the claim OUTLIVES the wiring: deleting the Pipeline must never delete
// the accumulated context of the conversations that route started.
func TestNamingAVolumeRendersAClaimThatOutlivesThePipeline(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "pv-prof-bound")
	mkSignalSource(t, "pv-src-bound", "pv-sig-bound", "")
	mkPipeline(t, "pv-bound-pipe", []string{"pv-src-bound"}, nil, "pv-prof-bound")
	bindPersistence(t, "pv-bound-pipe", &agentopsv1alpha1.PersistenceBinding{
		VolumeName: "pv-precreated-context", Size: "3Gi",
	})
	reconcilePipeline(t, "pv-bound-pipe")

	name := runtimepod.PipelineClaimName("pv-bound-pipe", runtimepod.VolumeContext)
	var claim corev1.PersistentVolumeClaim
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &claim); err != nil {
		t.Fatalf("the manager rendered no claim for the named volume: %v", err)
	}
	if claim.Spec.VolumeName != "pv-precreated-context" {
		t.Fatalf("claim.spec.volumeName = %q, want the volume that was named", claim.Spec.VolumeName)
	}
	// THE EXPLICIT EMPTY STORAGE CLASS IS THE POINT. An absent field is filled
	// in by admission with the cluster's default StorageClass, which provisions
	// a SECOND volume beside the one that was named.
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName != "" {
		t.Fatalf("storageClassName = %v — it must be an EXPLICIT empty string, or the claim is "+
			"dynamically provisioned against a volume the operator already made", claim.Spec.StorageClassName)
	}
	// NO OWNERREF, which is what makes the survival below structural rather
	// than a matter of what the API server happens to garbage-collect.
	if len(claim.OwnerReferences) != 0 {
		t.Fatalf("the claim carries ownerReferences %v — deleting a Pipeline would delete the "+
			"accumulated context of every conversation it started", claim.OwnerReferences)
	}

	// Re-reconciling is idempotent: no second claim, no error.
	reconcilePipeline(t, "pv-bound-pipe")

	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "pv-bound-pipe"}, &p); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Delete(ctx, &p); err != nil {
		t.Fatal(err)
	}
	var after corev1.PersistentVolumeClaim
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &after)
	if apierrors.IsNotFound(err) {
		t.Fatal("deleting the Pipeline deleted the claim. Storage is the one thing here whose loss " +
			"cannot be repaired by reconciling again")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), &after) })
}

// A route naming a volume gets the claim the MANAGER created, under the derived
// name — that is what the conversation freezes, since the volume itself is not
// mountable.
func TestAVolumeBindingResolvesToTheCreatedClaim(t *testing.T) {
	mkProfile(t, "pv-prof-derived")
	mkSignalSource(t, "pv-src-derived", "pv-sig-derived", "")
	mkPipeline(t, "pv-derived-pipe", []string{"pv-src-derived"}, nil, "pv-prof-derived")
	bindPersistence(t, "pv-derived-pipe", &agentopsv1alpha1.PersistenceBinding{VolumeName: "pv-some-volume"})
	reconcilePipeline(t, "pv-derived-pipe")
	t.Cleanup(func() {
		claim := &corev1.PersistentVolumeClaim{}
		claim.Name, claim.Namespace = runtimepod.PipelineClaimName("pv-derived-pipe", runtimepod.VolumeContext), ns
		_ = k8sClient.Delete(context.Background(), claim)
	})
	srv := apiServer()

	if rec := taskSignal(t, srv, "pv-src-derived", "pv-derived-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "pv-prof-derived")
	want := runtimepod.PipelineClaimName("pv-derived-pipe", runtimepod.VolumeContext)
	if conv.Spec.ContextClaimName != want {
		t.Fatalf("ContextClaimName = %q, want the claim the manager rendered (%q)",
			conv.Spec.ContextClaimName, want)
	}
	if got := claimOf(podFor(t, conv.Name), "context"); got != want {
		t.Fatalf("the pod mounted %q, want %q", got, want)
	}
}
