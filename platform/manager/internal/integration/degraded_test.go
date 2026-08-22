// A KNOWN-BAD volume must cost CONTINUITY, not availability.
//
// The whole thesis of this change in one behaviour: after a conversation has
// exhausted its retries against an unattachable volume, it starts WITHOUT the
// volume and says it lost its memory — rather than failing forever, which is
// what fifteen hours of outage actually looked like.
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

// failStartsFor drives a conversation to the given number of consecutive
// storage-attributable start failures.
func failStartsFor(t *testing.T, r *controller.ConversationReconciler, name string, n int32) {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	key := types.NamespacedName{Namespace: ns, Name: name}
	if err := k8sClient.Get(context.Background(), key, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.RuntimeStartFailures = n
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour)) // backoff already elapsed
	conv.Status.LastRuntimeStartFailure = &past
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type: controller.ConditionRuntimeStarted, Status: metav1.ConditionFalse,
		Reason: controller.ReasonVolumeUnavailable, Message: "Waiting for volume share to be available",
	})
	if err := k8sClient.Status().Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}
}

func podOf(t *testing.T, conv string) *corev1.Pod {
	t.Helper()
	var pod corev1.Pod
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "agentops-conv-" + conv}, &pod); err != nil {
		t.Fatalf("get pod for %s: %v", conv, err)
	}
	return &pod
}

func TestExhaustedRetriesStartWithoutTheVolume(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Hour
	r.Runtime.HomePVC = "agentops-home"

	mkProfile(t, "p-degraded")
	mkChanConv(t, "degraded-1", "p-degraded")
	setContextHandle(t, "degraded-1", "session-unreachable")
	failStartsFor(t, r, "degraded-1", 5)

	reconcileWith(t, r, "degraded-1")

	pod := podOf(t, "degraded-1")
	for _, v := range pod.Spec.Volumes {
		if v.Name == "home" && v.PersistentVolumeClaim != nil {
			t.Fatal("after exhausting retries the pod must start WITHOUT the unattachable claim")
		}
	}

	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "degraded-1"}, &conv); err != nil {
		t.Fatal(err)
	}
	// The handle must go: it names a context this pod cannot reach, and keeping
	// it would fail a continuation that can never succeed.
	if conv.ContextID() != "" {
		t.Fatalf("stale handle survived: %q", conv.ContextID())
	}
	// And the loss must be STATED, never silent.
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionContextContinuity)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("the loss must be recorded; got %+v", cond)
	}
	if !strings.Contains(cond.Message, "without its previous memory") {
		t.Fatalf("message = %q, want it to state the loss plainly", cond.Message)
	}
}

// Below the threshold the volume is still given every chance — a transient
// attach problem must not cost a conversation its memory.
func TestFewFailuresKeepTheVolume(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Hour
	r.Runtime.HomePVC = "agentops-home"

	mkProfile(t, "p-patient")
	mkChanConv(t, "patient-1", "p-patient")
	setContextHandle(t, "patient-1", "session-keep")
	failStartsFor(t, r, "patient-1", 2)

	reconcileWith(t, r, "patient-1")

	pod := podOf(t, "patient-1")
	found := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == "home" && v.PersistentVolumeClaim != nil {
			found = true
		}
	}
	if !found {
		t.Fatal("two failures is not evidence the volume is gone; the claim must still be mounted")
	}
	var conv agentopsv1alpha1.Conversation
	_ = k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "patient-1"}, &conv)
	if conv.ContextID() != "session-keep" {
		t.Fatal("the handle must survive while the volume is still being retried")
	}
}

// A conversation that could not be SCHEDULED must never be stripped of its
// context — that would lose memory for a capacity problem.
func TestNonStorageFailuresNeverStripTheVolume(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Hour
	r.Runtime.HomePVC = "agentops-home"

	mkProfile(t, "p-sched")
	mkChanConv(t, "sched-1", "p-sched")
	setContextHandle(t, "sched-1", "session-sched")

	var conv agentopsv1alpha1.Conversation
	key := types.NamespacedName{Namespace: ns, Name: "sched-1"}
	if err := k8sClient.Get(context.Background(), key, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.RuntimeStartFailures = 9 // well past the threshold
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	conv.Status.LastRuntimeStartFailure = &past
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type: controller.ConditionRuntimeStarted, Status: metav1.ConditionFalse,
		Reason: controller.ReasonUnschedulable, Message: "0/5 nodes are available",
	})
	if err := k8sClient.Status().Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}

	reconcileWith(t, r, "sched-1")

	pod := podOf(t, "sched-1")
	found := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == "home" && v.PersistentVolumeClaim != nil {
			found = true
		}
	}
	if !found {
		t.Fatal("an unschedulable pod must NOT cost the conversation its context")
	}
}
