// A runtime pod that never STARTS used to hold its capacity slot forever.
//
// On 2026-08-20 a corrupt filesystem on the shared context volume left five pods
// in ContainerCreating for fifteen hours. They held every slot, six more
// conversations starved behind them, and nothing anywhere said why: the only
// condition on the stuck conversations read DeliveryPending=False /
// AllDelivered, which looks like health.
//
// envtest runs no kubelet, so a created runtime pod stays Pending forever —
// which is precisely the stuck pod these tests need, with no fault injection
// at all. A tiny start deadline is the only thing test-specific here.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// markPodUnschedulableOnVolume gives a pod the status a kubelet writes when a
// volume will not attach: scheduled, but never ready to start containers.
func markPodUnschedulableOnVolume(t *testing.T, conv string) {
	t.Helper()
	var pod corev1.Pod
	key := types.NamespacedName{Namespace: ns, Name: "agentops-conv-" + conv}
	if err := k8sClient.Get(context.Background(), key, &pod); err != nil {
		t.Fatalf("get pod for %s: %v", conv, err)
	}
	patch := client.MergeFrom(pod.DeepCopy())
	pod.Status.Phase = corev1.PodPending
	pod.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
		{
			Type: corev1.PodReadyToStartContainers, Status: corev1.ConditionFalse,
			Reason:             "ContainersNotReady",
			Message:            "Waiting for volume share to be available",
			LastTransitionTime: metav1.Now(),
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "runtime",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
	}}
	if err := k8sClient.Status().Patch(context.Background(), &pod, patch); err != nil {
		t.Fatalf("patch pod status for %s: %v", conv, err)
	}
}

func conditionOf(t *testing.T, name, condType string) *metav1.Condition {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return apimeta.FindStatusCondition(conv.Status.Conditions, condType)
}

// TestUnstartedPodIsReapedAndSaysWhy is the whole outage in one test: a pod
// that cannot start is removed, and the conversation carries the kubelet's own
// explanation rather than a bare timeout.
func TestUnstartedPodIsReapedAndSaysWhy(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond // every pod is instantly overdue

	mkProfile(t, "p-stuck")
	mkChanConv(t, "stuck-1", "p-stuck")
	reconcileWith(t, r, "stuck-1")
	if !podExists(t, "stuck-1") {
		t.Fatal("expected a runtime pod on first reconcile")
	}
	markPodUnschedulableOnVolume(t, "stuck-1")

	reconcileWith(t, r, "stuck-1")
	if podExists(t, "stuck-1") {
		t.Fatal("a pod past its start deadline must be reaped, not left holding a slot")
	}

	cond := conditionOf(t, "stuck-1", controller.ConditionRuntimeStarted)
	if cond == nil {
		t.Fatal("no RuntimeStarted condition: the outage's real failure was silence")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("RuntimeStarted = %v, want False", cond.Status)
	}
	if cond.Reason != controller.ReasonVolumeUnavailable {
		t.Fatalf("reason = %q, want %q", cond.Reason, controller.ReasonVolumeUnavailable)
	}
	// The point of the whole mechanism: an operator reading this learns what
	// is wrong, not merely that a timer expired.
	if want := "Waiting for volume share to be available"; !strings.Contains(cond.Message, want) {
		t.Fatalf("condition message must quote the kubelet: got %q, want it to contain %q",
			cond.Message, want)
	}
}

// TestStuckPodHoldsItsSlotUntilReaped pins the half that must NOT change. A
// stuck pod still counts against the cap while it exists — un-counting it would
// provision past the cap against resources the cluster has not released.
func TestStuckPodHoldsItsSlotUntilReaped(t *testing.T) {
	r := reconcilerWithCap(nil, 1) // one slot, so the second must wait
	r.RuntimeStartDeadline = time.Hour

	mkProfile(t, "p-hold")
	mkChanConv(t, "holder-1", "p-hold")
	reconcileWith(t, r, "holder-1")
	if !podExists(t, "holder-1") {
		t.Fatal("expected a runtime pod for the first conversation")
	}
	markPodUnschedulableOnVolume(t, "holder-1")

	mkChanConv(t, "waiter-1", "p-hold")
	reconcileWith(t, r, "waiter-1")

	if podExists(t, "waiter-1") {
		t.Fatal("the cap was exceeded: a stuck pod must still count as active")
	}
	if got := phaseOf(t, "waiter-1"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("waiter phase = %q, want Pending", got)
	}
}

// TestReapingAStuckPodFreesTheSlot: deletion is what frees capacity, through
// the promotion path that already exists. This is the difference between the
// fix and the mistake it avoids — the slot comes back because the pod is gone,
// not because it stopped being counted.
func TestReapingAStuckPodFreesTheSlot(t *testing.T) {
	r := reconcilerWithCap(nil, 1)
	r.RuntimeStartDeadline = time.Hour

	mkProfile(t, "p-free")
	mkChanConv(t, "holder-2", "p-free")
	reconcileWith(t, r, "holder-2")
	markPodUnschedulableOnVolume(t, "holder-2")

	mkChanConv(t, "waiter-2", "p-free")
	reconcileWith(t, r, "waiter-2")
	if podExists(t, "waiter-2") {
		t.Fatal("waiter should be blocked while the stuck pod holds the only slot")
	}

	// The deadline passes for the holder.
	r.RuntimeStartDeadline = time.Nanosecond
	reconcileWith(t, r, "holder-2")
	if podExists(t, "holder-2") {
		t.Fatal("holder's pod should have been reaped")
	}

	// A fresh reconciler with no backoff stands in for the requeue that would
	// follow the cooldown; the point under test is that the slot is free.
	r2 := reconcilerWithCap(nil, 1)
	r2.RuntimeStartDeadline = time.Hour
	reconcileWith(t, r2, "waiter-2")
	if !podExists(t, "waiter-2") {
		t.Fatal("the freed slot must be usable by the waiting conversation")
	}
}

// TestRepeatedStartFailuresBackOff: a permanently broken volume must not be
// retried at reconcile speed. The count is on status so it survives a restart.
func TestRepeatedStartFailuresBackOff(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond

	mkProfile(t, "p-backoff")
	mkChanConv(t, "backoff-1", "p-backoff")
	reconcileWith(t, r, "backoff-1")
	markPodUnschedulableOnVolume(t, "backoff-1")
	reconcileWith(t, r, "backoff-1") // reap #1

	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "backoff-1"}, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.Status.RuntimeStartFailures != 1 {
		t.Fatalf("failures = %d, want 1", conv.Status.RuntimeStartFailures)
	}
	if conv.Status.LastRuntimeStartFailure == nil {
		t.Fatal("the failure must be stamped, or the backoff has no origin")
	}

	// Immediately after a failure the conversation is inside its cooldown, so
	// no replacement pod is created and the reconcile asks to be requeued.
	res := reconcileWith(t, r, "backoff-1")
	if podExists(t, "backoff-1") {
		t.Fatal("a replacement pod must not be created inside the backoff window")
	}
	if res.RequeueAfter <= 0 {
		t.Fatal("expected a requeue for the remaining backoff, so recovery is prompt")
	}
}

// markPodImagePullFailure gives a pod the status a kubelet writes when the
// image cannot be pulled: scheduled, volumes fine, container stuck on the
// registry. Deliberately distinct from the volume case, because the whole
// point of the classification is that these two must not be confused.
func markPodImagePullFailure(t *testing.T, conv string) {
	t.Helper()
	var pod corev1.Pod
	key := types.NamespacedName{Namespace: ns, Name: "agentops-conv-" + conv}
	if err := k8sClient.Get(context.Background(), key, &pod); err != nil {
		t.Fatalf("get pod for %s: %v", conv, err)
	}
	patch := client.MergeFrom(pod.DeepCopy())
	pod.Status.Phase = corev1.PodPending
	pod.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
		{Type: corev1.PodReadyToStartContainers, Status: corev1.ConditionFalse, LastTransitionTime: metav1.Now()},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "runtime",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "ImagePullBackOff", Message: "back-off pulling image",
		}},
	}}
	if err := k8sClient.Status().Patch(context.Background(), &pod, patch); err != nil {
		t.Fatalf("patch pod status for %s: %v", conv, err)
	}
}
