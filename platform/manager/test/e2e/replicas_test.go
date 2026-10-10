//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// durable-chat-ops-broker — the one failure mode this change exists to
// close, and only a real multi-pod Service and a real kubelet-scheduled
// rollout can exercise it: the chart's own default is replicas: 2 now (see
// chart/values.yaml), so this pack's shared install already runs the
// manager at two pods for EVERY lane, not only this one.
//
// Before this change, a console start whose /channel/ops poll landed on
// the non-leader manager pod saw a permanently empty queue and never got a
// thread — roughly half of them, at random, with no error anywhere. This
// lane posts a batch of starts in a tight loop specifically to make that
// landing distribution matter, and asserts every one of them gets a
// thread.
func TestReplicasTwoDeliversEveryConsoleThread(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	// Force it, never trust the install's own rollout: measured live, this
	// pack's single-node k3d cluster leaves the chart's own replicas:2
	// install with only ONE manager pod ever Ready — the second sits
	// permanently Pending behind the hard anti-affinity, and the OLD
	// precondition here (len(pods) >= 2, no readiness check) passed on that
	// for as long as this lane has existed, proving nothing about a second
	// replica ever racing a poll.
	scaleManagerReplicas(t, ctx, e, 2)
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 2)

	const n = 8
	start := time.Now().Add(-5 * time.Second)
	for i := 0; i < n; i++ {
		task := fmt.Sprintf("echo replicas %s %d", stamp, i)
		code, out := e.ConsoleStart(t, task)
		if code/100 != 2 {
			t.Fatalf("start %d: %d %s", i, code, out)
		}
	}

	// Every one of the n conversations this batch opened must reach a real
	// thread id, not merely exist — a conversation stuck on a claim-only
	// placeholder (durable-chat-ops-broker's own ThreadBinding with no
	// ThreadID) is exactly the symptom this change closes.
	waitFor(t, fmt.Sprintf("all %d conversations to get a console thread", n), 3*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			return false, err
		}
		got := 0
		for i := range items {
			c := &items[i]
			if c.Spec.Signal == nil || c.Spec.Signal.SourceRef == nil ||
				c.Spec.Signal.SourceRef.Name != SourceConsole || !c.CreationTimestamp.Time.After(start) {
				continue
			}
			if tid := c.ThreadFor("console"); tid != nil && *tid != "" {
				got++
			}
		}
		return got >= n, nil
	})
}

// assertManagerRunsAtLeastNReadyReplicas is the precondition every lane in
// this file depends on: it proves nothing about the fix if the install
// silently ran at fewer pods, or at the right pod COUNT with one of them
// permanently unschedulable (the anti-affinity case scaleManagerReplicas
// exists for). Ready, not merely listed — a Pending pod satisfies neither
// the manager's own cap accounting nor this lane's claim about how many
// replicas were actually in the race.
func assertManagerRunsAtLeastNReadyReplicas(t *testing.T, ctx context.Context, e *Env, n int) {
	t.Helper()
	pods, err := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	if err != nil {
		t.Fatal(err)
	}
	ready := 0
	for i := range pods {
		if pods[i].Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pods[i].Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready++
				break
			}
		}
	}
	if ready < n {
		t.Fatalf("this lane needs at least %d READY manager pods, got %d of %d listed", n, ready, len(pods))
	}
}

// scaleManagerReplicas resizes the manager Deployment to n replicas for the
// rest of this test, restoring the original replica count on cleanup so
// later lanes get the chart's own default back. Affinity, once cleared, is
// left cleared — see below for why putting it back is the wrong direction.
//
// The anti-affinity is a HARD requirement (deployment.yaml: one real node per
// replica, deliberately), which this pack's single-node k3d cluster cannot
// satisfy for n > 1. Patching the Deployment alone does not unblock it:
// the Deployment controller's RollingUpdate will not place a new pod on a
// node already holding one whose EXISTING, already-created pod spec still
// carries the old anti-affinity rule — a pod's spec is immutable once
// created, so clearing the field on the Deployment only reaches pods
// created AFTER the patch. Measured live, twice: once by hand on a local
// Rancher Desktop cluster (the rollout sat at "1 out of N new replicas
// updated" until the OLD pods were deleted), and once in this exact pack
// in CI before this comment existed — the same stall, the same fix.
//
// So this force-deletes the CURRENT pods right after patching: the
// Deployment controller recreates them from the new, affinity-free
// template, and only then can more than one land on this cluster's one
// node. No lane in this file (or any other) asserts the anti-affinity is
// PRESENT, so there is nothing to restore it for — leaving it cleared for
// the rest of the run is simpler than restoring a field whose only
// property anyone here cares about is "it must be gone for this cluster
// to schedule more than one pod," which stays true either way.
func scaleManagerReplicas(t *testing.T, ctx context.Context, e *Env, n int) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	origReplicas := dep.Spec.Replicas

	forceManagerRollout(t, ctx, e, n)

	t.Cleanup(func() {
		forceManagerRollout(t, context.Background(), e, int(derefOr(origReplicas, 1)))
	})
}

// forceManagerRollout is scaleManagerReplicas' shared mechanics: patch
// (replicas=n, affinity cleared), zero any ReplicaSet STILL carrying the old
// affinity so it stops recreating pods that block the new template, and wait
// for the rollout to settle.
//
// Deleting the BLOCKING PODS alone was tried and is not enough — their own
// ReplicaSet's desired count is untouched by that, so it recreates them
// immediately, identical, still carrying the rule. Measured live: two more
// pods from the same old ReplicaSet appeared within seconds of deleting the
// first two. Scaling that ReplicaSet itself to zero is the only thing that
// stops it recreating them. A ReplicaSet whose template ALREADY has no
// affinity (e.g. a later call in the same test that only changes the
// replica COUNT) is left alone — it is the Deployment's current one, and
// zeroing it would fight the Deployment controller's own reconciliation of
// it rather than help.
func forceManagerRollout(t *testing.T, ctx context.Context, e *Env, n int) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(dep.DeepCopy())
	want := int32(n)
	dep.Spec.Replicas = &want
	dep.Spec.Template.Spec.Affinity = nil
	if err := e.K.Patch(ctx, &dep, patch); err != nil {
		t.Fatal(err)
	}

	var sets appsv1.ReplicaSetList
	if err := e.K.List(ctx, &sets, client.InNamespace(Namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "agentops-manager"}); err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	for i := range sets.Items {
		rs := &sets.Items[i]
		if rs.Spec.Template.Spec.Affinity == nil {
			continue // the Deployment's current one — leave it to the controller
		}
		rsPatch := client.MergeFrom(rs.DeepCopy())
		rs.Spec.Replicas = &zero
		if err := e.K.Patch(ctx, rs, rsPatch); err != nil && !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}

	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace,
		"rollout", "status", "deployment/agentops-manager", "--timeout=3m"); err != nil {
		t.Fatalf("manager rollout to %d replicas: %v\n%s", n, err, out)
	}
}

// derefOr reads an *int32, falling back when the pointer is nil — the
// Deployment's own default when the chart renders no explicit replicas.
func derefOr(p *int32, def int32) int32 {
	if p == nil {
		return def
	}
	return *p
}

// durable-chat-ops-broker's harsher sibling: THREE replicas (one more than
// the chart's own default, so landing on a non-leader is MORE likely per
// poll, not less), TRUE concurrency (goroutines, not a tight sequential
// loop — the loop above can serialize through one lucky pinned connection
// without ever proving a second poll happening at the same instant as the
// first), and a stronger assertion: every conversation must reach a REAL
// thread AND have its reply actually DELIVERED there, not merely exist.
//
// The second half is the one the sequential lane above cannot catch: a
// run's reply enqueued against a claim-only placeholder (no ThreadID yet)
// gets delivered to a channel-level pseudo-thread and marked delivered
// PERMANENTLY, so the real thread ensureTopics creates moments later never
// receives the answer — measured live, roughly 1 in 10 under this lane's
// own concurrency. assertManagerRunsAtLeastNReadyReplicas is instance 7.3.
func TestReplicasThreeDeliversEveryConsoleThreadInParallel(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	scaleManagerReplicas(t, ctx, e, 3)
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 3)

	const n = 10
	start := time.Now().Add(-5 * time.Second)

	var wg sync.WaitGroup
	codes := make([]int, n)
	outs := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task := fmt.Sprintf("echo three-replicas-parallel %s %d", stamp, i)
			codes[i], outs[i] = e.ConsoleStart(t, task)
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if codes[i]/100 != 2 {
			t.Fatalf("start %d: %d %s", i, codes[i], outs[i])
		}
	}

	waitFor(t, fmt.Sprintf("all %d conversations to get a console thread AND a delivered reply", n),
		3*time.Minute, func() (bool, error) {
			items, err := e.K.Conversations(ctx)
			if err != nil {
				return false, err
			}
			return countDeliveredConsoleConversations(items, start) >= n, nil
		})
}

// countDeliveredConsoleConversations counts console-started conversations
// created after start that hold a real thread and a delivered reply.
func countDeliveredConsoleConversations(items []agentopsv1alpha1.Conversation, start time.Time) int {
	got := 0
	for i := range items {
		if consoleConversationDelivered(&items[i], start) {
			got++
		}
	}
	return got
}

func consoleConversationDelivered(c *agentopsv1alpha1.Conversation, start time.Time) bool {
	if c.Spec.Signal == nil || c.Spec.Signal.SourceRef == nil ||
		c.Spec.Signal.SourceRef.Name != SourceConsole || !c.CreationTimestamp.Time.After(start) {
		return false
	}
	if tid := c.ThreadFor("console"); tid == nil || *tid == "" {
		return false
	}
	for _, r := range c.Status.Runs {
		if r.DeliveredTo("console") {
			return true
		}
	}
	return false
}
