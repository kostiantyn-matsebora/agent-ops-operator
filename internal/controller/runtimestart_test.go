package controller

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// podAt builds a pod created `age` ago with the given phase and conditions.
func podAt(age time.Duration, phase corev1.PodPhase, conds ...corev1.PodCondition) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "agentops-conv-x",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Status: corev1.PodStatus{Phase: phase, Conditions: conds},
	}
}

func cond(t corev1.PodConditionType, s corev1.ConditionStatus, reason, msg string) corev1.PodCondition {
	return corev1.PodCondition{Type: t, Status: s, Reason: reason, Message: msg}
}

func TestRuntimeStartOverdue(t *testing.T) {
	deadline := 10 * time.Minute
	now := time.Now()

	cases := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{"fresh pending pod is not overdue", podAt(time.Minute, corev1.PodPending), false},
		{"stuck pending pod is overdue", podAt(30*time.Minute, corev1.PodPending), true},
		// The whole point of the cap invariant: a running pod is doing its job,
		// and an exited one is already handled by the exit reaping.
		{"running pod is never overdue", podAt(30*time.Minute, corev1.PodRunning), false},
		{"succeeded pod is never overdue", podAt(30*time.Minute, corev1.PodSucceeded), false},
		{"failed pod is never overdue", podAt(30*time.Minute, corev1.PodFailed), false},
		// Exactly at the deadline is not yet past it.
		{"at the deadline is not overdue", podAt(deadline, corev1.PodPending), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeStartOverdue(tc.pod, deadline, now); got != tc.want {
				t.Fatalf("runtimeStartOverdue = %v, want %v", got, tc.want)
			}
		})
	}
}

// The classification is what decides whether a failure may be offered to the
// storage breaker. Misfiling an image pull as a volume problem would hold every
// conversation in the install for a reason that has nothing to do with storage.
func TestClassifyStuckPod(t *testing.T) {
	t.Run("unattachable volume is storage-attributable", func(t *testing.T) {
		pod := podAt(30*time.Minute, corev1.PodPending,
			cond(corev1.PodScheduled, corev1.ConditionTrue, "", ""),
			cond(corev1.PodReadyToStartContainers, corev1.ConditionFalse, "", ""),
		)
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
		}}
		got := classifyStuckPod(pod)
		if got.Reason != ReasonVolumeUnavailable {
			t.Fatalf("reason = %q, want %q", got.Reason, ReasonVolumeUnavailable)
		}
		if !got.Storage {
			t.Fatal("an unattachable volume must be storage-attributable")
		}
		if !strings.Contains(got.Message, "ContainerCreating") {
			t.Fatalf("message should carry the kubelet's own waiting reason, got %q", got.Message)
		}
	})

	t.Run("unschedulable is not storage", func(t *testing.T) {
		got := classifyStuckPod(podAt(30*time.Minute, corev1.PodPending,
			cond(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable", "0/5 nodes are available"),
		))
		if got.Reason != ReasonUnschedulable {
			t.Fatalf("reason = %q, want %q", got.Reason, ReasonUnschedulable)
		}
		if got.Storage {
			t.Fatal("an unschedulable pod must NOT open a storage breaker")
		}
		if !strings.Contains(got.Message, "0/5 nodes are available") {
			t.Fatalf("message should quote the scheduler verbatim, got %q", got.Message)
		}
	})

	t.Run("image pull failure is not storage", func(t *testing.T) {
		pod := podAt(30*time.Minute, corev1.PodPending,
			cond(corev1.PodScheduled, corev1.ConditionTrue, "", ""),
			// Deliberately ALSO false: a pod stuck pulling can report this, and
			// the image reason must win so a slow registry is never reported as
			// a bad volume.
			cond(corev1.PodReadyToStartContainers, corev1.ConditionFalse, "", ""),
		)
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "ImagePullBackOff", Message: "back-off pulling image",
			}},
		}}
		got := classifyStuckPod(pod)
		if got.Reason != ReasonImageUnavailable {
			t.Fatalf("reason = %q, want %q", got.Reason, ReasonImageUnavailable)
		}
		if got.Storage {
			t.Fatal("an image pull failure must NOT open a storage breaker")
		}
	})

	t.Run("a missing configmap is not filed as an image problem", func(t *testing.T) {
		pod := podAt(30*time.Minute, corev1.PodPending,
			cond(corev1.PodScheduled, corev1.ConditionTrue, "", ""),
			cond(corev1.PodReadyToStartContainers, corev1.ConditionTrue, "", ""),
		)
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "CreateContainerConfigError", Message: "configmap not found",
			}},
		}}
		got := classifyStuckPod(pod)
		if got.Reason != ReasonNotStarted {
			t.Fatalf("reason = %q, want %q — a wiring problem must not be reported as a registry one",
				got.Reason, ReasonNotStarted)
		}
		if got.Storage {
			t.Fatal("a config error must NOT open a storage breaker")
		}
	})

	t.Run("an unclassifiable pod says so honestly", func(t *testing.T) {
		got := classifyStuckPod(podAt(30*time.Minute, corev1.PodPending))
		if got.Reason != ReasonNotStarted {
			t.Fatalf("reason = %q, want %q", got.Reason, ReasonNotStarted)
		}
		if got.Storage {
			t.Fatal("an unclassified failure must NOT be attributed to storage")
		}
	})
}

// A message reading only "deadline exceeded" would reproduce the outage this
// whole mechanism exists to end, so the evidence is pinned.
func TestClassifyStuckPodCarriesEvidence(t *testing.T) {
	got := classifyStuckPod(podAt(30*time.Minute, corev1.PodPending,
		cond(corev1.PodReadyToStartContainers, corev1.ConditionFalse,
			"ContainersNotReady", "Waiting for volume share to be available"),
	))
	if !strings.Contains(got.Message, "Waiting for volume share to be available") {
		t.Fatalf("the kubelet's message must survive verbatim, got %q", got.Message)
	}
	if !strings.Contains(got.Message, "ContainersNotReady") {
		t.Fatalf("the condition reason must survive, got %q", got.Message)
	}
}

func TestRuntimeStartBackoff(t *testing.T) {
	if got := runtimeStartBackoff(0); got != 0 {
		t.Fatalf("no failures must not back off, got %v", got)
	}
	if got := runtimeStartBackoff(1); got != runtimeStartBackoffBase {
		t.Fatalf("first failure = %v, want %v", got, runtimeStartBackoffBase)
	}
	if got := runtimeStartBackoff(2); got != 2*runtimeStartBackoffBase {
		t.Fatalf("second failure = %v, want %v", got, 2*runtimeStartBackoffBase)
	}
	// Must saturate rather than growing without bound: a recovery that goes
	// unnoticed for an hour is its own outage.
	if got := runtimeStartBackoff(50); got != runtimeStartBackoffMax {
		t.Fatalf("many failures = %v, want the cap %v", got, runtimeStartBackoffMax)
	}
}
