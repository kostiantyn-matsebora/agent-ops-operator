package integration

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// Explicit workload release (state-store design D-7): the manager deletes a
// conversation's runtime pod and MCP ConfigMap itself on close and on delete.
// envtest runs no garbage collector, so an ownerRef alone would leave both
// behind here — which is what makes this the proof that the release does not
// lean on it.

// mkProvisioned reconciles a conversation until it owns a pod, and gives it
// the MCP ConfigMap an MCP-bound conversation would carry.
func mkProvisioned(t *testing.T, name string) {
	t.Helper()
	clearRuntimePods(t)
	mkProfile(t, name+"-profile")
	mkChanConv(t, name, name+"-profile")
	reconcileWith(t, reconciler(), name)
	if !podExists(t, name) {
		t.Fatal("the conversation must be admitted and get a pod")
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: ns, Name: controller.MCPConfigMapName(name),
	}}
	if err := k8sClient.Create(context.Background(), cm); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
}

func configMapExists(t *testing.T, name string) bool {
	t.Helper()
	var cm corev1.ConfigMap
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: controller.MCPConfigMapName(name)}, &cm)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestCloseReleasesPodAndConfigMap(t *testing.T) {
	mkProvisioned(t, "rel-close")

	conv := getConv(t, "rel-close")
	now := metav1.Now()
	conv.Status.Phase, conv.Status.ClosedAt = agentopsv1alpha1.ConversationClosed, &now
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, reconciler(), "rel-close")

	if podExists(t, "rel-close") {
		t.Error("a closed conversation must have no runtime pod")
	}
	if configMapExists(t, "rel-close") {
		t.Error("a closed conversation must have no MCP ConfigMap")
	}
	if getConv(t, "rel-close").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("the closed object must survive its release")
	}
}

func TestDeleteReleasesPodAndConfigMap(t *testing.T) {
	mkProvisioned(t, "rel-delete")

	if err := k8sClient.Delete(context.Background(), getConv(t, "rel-delete")); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, reconciler(), "rel-delete")

	if podExists(t, "rel-delete") {
		t.Error("a deleted conversation must have no runtime pod")
	}
	if configMapExists(t, "rel-delete") {
		t.Error("a deleted conversation must have no MCP ConfigMap")
	}
}
