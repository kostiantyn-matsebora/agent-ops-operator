package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
)

// Drain awareness: get off a node BEFORE it goes down.
//
// The corruption this exists to avoid needs three things at once — the volume
// attached, its filesystem mounted read-write with dirty metadata, and the node
// going away. agent-ops controls the first two more than it looks: when the last
// consumer of a shared context volume leaves, the volume detaches and its
// filesystem unmounts cleanly.
//
// What this does NOT claim:
//
//   - It is not a substitute for a working drain. Kubernetes already refuses to
//     schedule new pods onto a cordoned node, so nothing here needs to exclude
//     one from admission — that is the scheduler's job and it already does it.
//     An ordinary eviction also unmounts cleanly on its own.
//   - It does not close the window. A storage provider picks where a shared
//     volume is SERVED independently of where runtime pods run, so the node
//     holding the mount may not be the node being drained. Claiming otherwise
//     in the docs would be worse than the gap.
//
// What it buys is PROMPTNESS: idle pods go at cordon time rather than whenever
// the drain reaches them. That matters precisely when it went wrong — a reboot
// manager configured to force-reboot on a drain TIMEOUT does not wait for the
// drain to finish, so every second of unmount earned before the deadline counts.
//
// Off by default, because seeing a cordon at all means reading nodes, and nodes
// are cluster-scoped. Every other permission this manager holds is namespaced.

// conditionTaints are applied by Kubernetes from a node's CONDITIONS, and mean
// "this node is unwell", never "this node is being taken down deliberately".
//
// They must not be read as a drain, and the distinction is not academic. A node
// that goes NotReady for thirty seconds of network trouble grows
// node.kubernetes.io/not-ready:NoSchedule automatically. Treating that as a
// drain would release runtime pods during exactly the incident where the
// manager can least afford to act on a stale view — and during a partition,
// across many nodes at once.
//
// node.kubernetes.io/unschedulable is deliberately NOT here: that one IS the
// cordon, and it is the taint `kubectl cordon` adds alongside the flag.
var conditionTaints = map[string]bool{
	"node.kubernetes.io/not-ready":           true,
	"node.kubernetes.io/unreachable":         true,
	"node.kubernetes.io/memory-pressure":     true,
	"node.kubernetes.io/disk-pressure":       true,
	"node.kubernetes.io/pid-pressure":        true,
	"node.kubernetes.io/network-unavailable": true,
	"node.kubernetes.io/out-of-service":      true,
}

// nodeUnschedulable reports whether a node is being taken out of service
// DELIBERATELY — cordoned, or tainted for maintenance.
//
// Two spellings count, because a drain can arrive as either. `kubectl cordon`
// sets `spec.unschedulable` and adds a taint, but a maintenance controller may
// mark a machine for reboot with a taint alone, and reading only the flag would
// miss precisely the automated reboot this feature exists for.
//
// Condition taints are excluded: an unwell node is not a draining node.
func nodeUnschedulable(node *corev1.Node) bool {
	if node.Spec.Unschedulable {
		return true
	}
	for _, t := range node.Spec.Taints {
		if t.Effect != corev1.TaintEffectNoSchedule && t.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		if conditionTaints[t.Key] {
			continue
		}
		return true
	}
	return false
}

// releaseIfNodeDraining deletes a conversation's runtime pod when the node it
// sits on is going away and the conversation needs no worker.
//
// It reports whether the pod was released.
//
// The predicate is dispatch.NeedsWorker, SHARED with `/exit` and with idle
// eviction rather than restated. An inflight run keeps its pod for the reason
// `/exit` already refuses one: the replacement would get nothing from /work,
// idle out, be reaped as Succeeded, and re-run work that may already have acted.
// A node reboot is not a good enough reason to do that to a running agent —
// the eviction will take it anyway, and the sidecar's final checkpoint is what
// protects its context.
func (r *ConversationReconciler) releaseIfNodeDraining(
	ctx context.Context, conv *agentopsv1alpha1.Conversation, pod *corev1.Pod,
) (bool, error) {
	if !r.DrainAware || pod.Spec.NodeName == "" {
		return false, nil
	}
	if dispatch.NeedsWorker(conv) {
		return false, nil // busy: the drain may evict it, but we will not
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, &node); err != nil {
		// A node we cannot read is not evidence of a drain. Staying quiet is
		// right: the alternative is releasing pods across the install because
		// one RBAC rule is missing.
		return false, client.IgnoreNotFound(err)
	}
	if !nodeUnschedulable(&node) {
		return false, nil
	}

	log.FromContext(ctx).Info("releasing an idle runtime pod ahead of a node going down",
		"conversation", conv.Name, "pod", pod.Name, "node", node.Name)
	if err := r.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if r.Recorder != nil {
		r.Recorder.Event(conv, corev1.EventTypeNormal, "RuntimeReleasedForDrain",
			"released idle runtime pod "+pod.Name+" because node "+node.Name+
				" is draining; the conversation and its context are untouched")
	}
	if conv.Status.RuntimePod != "" {
		patch := client.MergeFrom(conv.DeepCopy())
		conv.Status.RuntimePod = ""
		if err := r.Status().Patch(ctx, conv, patch); err != nil {
			return true, err
		}
	}
	return true, nil
}
