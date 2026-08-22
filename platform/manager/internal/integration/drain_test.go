// Drain awareness: get off a node before it goes down.
//
// A node reboot took the block device away while the shared context volume's
// filesystem was still mounted and dirty, and that is what corrupted it. The
// lever agent-ops actually holds is releasing the last consumer promptly, so
// the volume detaches and unmounts while there is still time.
//
// Off unless enabled, because reading nodes is this manager's only
// cluster-scoped permission.
package integration

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// mkNode creates a node and registers cleanup. Nodes are cluster-scoped, so
// the name must not collide with another test's.
func mkNode(t *testing.T, name string, cordoned bool) {
	t.Helper()
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	n.Spec.Unschedulable = cordoned
	if err := k8sClient.Create(context.Background(), n); err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), n) })
}

// placePodOnNode binds a conversation's runtime pod to a node. envtest runs no
// scheduler, so the assignment has to be made by hand.
func placePodOnNode(t *testing.T, conv, node string) {
	t.Helper()
	var pod corev1.Pod
	key := types.NamespacedName{Namespace: ns, Name: "agentops-conv-" + conv}
	if err := k8sClient.Get(context.Background(), key, &pod); err != nil {
		t.Fatalf("get pod for %s: %v", conv, err)
	}
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: pod.Name},
		Target:     corev1.ObjectReference{Kind: "Node", Name: node},
	}
	if err := k8sClient.Create(context.Background(), binding); err != nil {
		t.Fatalf("bind pod to %s: %v", node, err)
	}
}

// TestIdlePodIsReleasedWhenItsNodeIsCordoned: the whole point — the last
// consumer leaves so the volume can detach before the machine reboots.
func TestIdlePodIsReleasedWhenItsNodeIsCordoned(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.DrainAware = true
	r.RuntimeStartDeadline = time.Hour

	mkNode(t, "drain-node-a", false)
	mkProfile(t, "p-drain")
	mkChanConv(t, "drain-1", "p-drain")
	reconcileWith(t, r, "drain-1")
	if !podExists(t, "drain-1") {
		t.Fatal("expected a runtime pod")
	}
	placePodOnNode(t, "drain-1", "drain-node-a")

	// Nothing pending and nothing inflight: the conversation needs no worker.
	drainClearInputs(t, "drain-1")

	// Still scheduleable — nothing should happen yet.
	reconcileWith(t, r, "drain-1")
	if !podExists(t, "drain-1") {
		t.Fatal("an idle pod on a healthy node must be left alone")
	}

	cordon(t, "drain-node-a")
	reconcileWith(t, r, "drain-1")
	if podExists(t, "drain-1") {
		t.Fatal("an idle pod on a cordoned node must be released so the volume can detach")
	}

	// The conversation itself is untouched — this is /exit's contract, not
	// /close's.
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "drain-1"}, &conv); err != nil {
		t.Fatalf("the conversation must survive its pod being released: %v", err)
	}
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("releasing a runtime must never close the conversation")
	}
}

// TestBusyPodSurvivesACordon: an inflight run keeps its pod, for the reason
// /exit already refuses one — the replacement would re-run work that may
// already have acted. The eviction will take it; we will not.
func TestBusyPodSurvivesACordon(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.DrainAware = true
	r.RuntimeStartDeadline = time.Hour

	mkNode(t, "drain-node-b", false)
	mkProfile(t, "p-busy")
	mkChanConv(t, "busy-1", "p-busy") // keeps its pending input: needs a worker
	reconcileWith(t, r, "busy-1")
	placePodOnNode(t, "busy-1", "drain-node-b")

	cordon(t, "drain-node-b")
	reconcileWith(t, r, "busy-1")

	if !podExists(t, "busy-1") {
		t.Fatal("a conversation with work to do must keep its pod through a cordon")
	}
}

// TestDrainAwarenessIsOffByDefault: reading nodes is the manager's only
// cluster-scoped permission, so the behaviour must not happen unless asked for.
func TestDrainAwarenessIsOffByDefault(t *testing.T) {
	r := reconcilerWithCap(nil, 100) // DrainAware left false
	r.RuntimeStartDeadline = time.Hour

	mkNode(t, "drain-node-c", true) // cordoned from the start
	mkProfile(t, "p-off")
	mkChanConv(t, "off-1", "p-off")
	reconcileWith(t, r, "off-1")
	placePodOnNode(t, "off-1", "drain-node-c")
	drainClearInputs(t, "off-1")

	reconcileWith(t, r, "off-1")
	if !podExists(t, "off-1") {
		t.Fatal("without drain awareness the manager must not touch pods on cordoned nodes")
	}
}

// TestNoScheduleTaintCountsAsDraining: a maintenance controller can mark a node
// for reboot with a taint and never set spec.unschedulable, and reading only
// the flag would miss exactly that case.
func TestNoScheduleTaintCountsAsDraining(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.DrainAware = true
	r.RuntimeStartDeadline = time.Hour

	mkNode(t, "drain-node-d", false)
	mkProfile(t, "p-taint")
	mkChanConv(t, "taint-1", "p-taint")
	reconcileWith(t, r, "taint-1")
	placePodOnNode(t, "taint-1", "drain-node-d")
	drainClearInputs(t, "taint-1")

	taintNoSchedule(t, "drain-node-d")
	reconcileWith(t, r, "taint-1")
	if podExists(t, "taint-1") {
		t.Fatal("a NoSchedule taint without spec.unschedulable must still count as draining")
	}
}

func cordon(t *testing.T, name string) {
	t.Helper()
	var node corev1.Node
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name}, &node); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Unschedulable = true
	if err := k8sClient.Patch(context.Background(), &node, patch); err != nil {
		t.Fatal(err)
	}
}

func taintNoSchedule(t *testing.T, name string) {
	t.Helper()
	var node corev1.Node
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name}, &node); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
		Key: "node.example/reboot", Effect: corev1.TaintEffectNoSchedule,
	})
	if err := k8sClient.Patch(context.Background(), &node, patch); err != nil {
		t.Fatal(err)
	}
}

// drainClearInputs empties a conversation's pending inputs so it needs no
// worker — the same state an answered conversation reaches.
func drainClearInputs(t *testing.T, name string) {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Spec.Inputs = nil
	if err := k8sClient.Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}
}
