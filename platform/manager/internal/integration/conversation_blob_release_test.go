package integration

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A queued input too large for the record lives in a blob that belongs to its
// conversation. Deleting the conversation, by any path, must release it: no
// behaviour may depend on garbage collection, so the manager does it itself.
func TestDeletingAConversationReleasesTheBlobsOfItsQueuedInputs(t *testing.T) {
	clearRuntimePods(t)
	ctx := context.Background()
	mkProfile(t, "prof-blob-release")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 5)

	conv := mkChanConv(t, "blob-release-1", "prof-blob-release")
	for _, key := range []state.Key{"blob-release-in", "blob-release-other"} {
		if err := stores.Blobs.Put(ctx, key, state.Blob{ConversationName: "blob-release-1", Type: "task", Payload: "the full text"}); err != nil {
			t.Fatalf("put blob %q: %v", key, err)
		}
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "blob-release-1"}, conv); err != nil {
		t.Fatal(err)
	}
	conv.Spec.Inputs = append(conv.Spec.Inputs, agentopsv1alpha1.InputItem{
		ID: "i2", Type: agentopsv1alpha1.InputTask, PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "blob-release-in"},
	})
	if err := k8sClient.Update(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, rc, "blob-release-1") // takes the finalizer

	if err := k8sClient.Delete(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, rc, "blob-release-1")

	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "blob-release-1"}, &after); !apierrors.IsNotFound(err) {
		t.Fatalf("the conversation must be gone: err=%v finalizers=%v", err, after.Finalizers)
	}
	if _, err := stores.Blobs.Get(ctx, "blob-release-in"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("the blob of a queued input outlived its conversation: %v", err)
	}
	// Only what the queue names is the conversation's to release from here; a
	// blob nothing points at is the orphan sweep's business, not this path's.
	if _, err := stores.Blobs.Get(ctx, "blob-release-other"); err != nil {
		t.Fatalf("a blob no queued input names was released by the delete path: %v", err)
	}
	_ = stores.Blobs.Delete(ctx, "blob-release-other")
}
