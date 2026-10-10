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

	// EVERY earlier lane in this package's shared cluster shares ONE
	// admission cap (maxActiveConversations, 5 by default) — it is NOT
	// scoped per source. A sibling lane on `vm-alerts`, `tg-ops` or
	// `e2e-fanout` that never calls `/close` leaves its conversation
	// Idle, still holding a slot. Measured live on CI: 7 such leftovers
	// from unrelated lanes, competing with this lane's own 8 and the
	// next lane's 10 for 5 slots — closing only THIS file's own backlog
	// (below) was not enough, because the backlog this lane inherited
	// was never this file's to begin with.
	//
	// A ONE-TIME sweep is not enough either: `e2e-cron` keeps firing on
	// its own schedule, independent of any test's lifetime, so a new
	// leftover can appear WHILE this lane or the next one is still
	// running — measured live as the same starvation recurring
	// intermittently even with a sweep at the top of this function.
	// `pauseCronLaneAndClearLeftovers` is TestAdmissionFIFOOnPodDelete's
	// own fix for exactly this: it un-claims the cron source so its
	// ticks stop admitting anything (restored on cleanup), then deletes
	// every existing conversation outright.
	clearAdmissionPoolForReplicaLane(t, ctx, e)

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

	// This lane's own 8 conversations stay Idle, each still holding an
	// ADMISSION SLOT under maxActiveConversations (5 by default) until
	// their idle TTL evicts them on its own schedule. A LATER lane in the
	// same process (TestReplicasThreeDeliversEveryConsoleThreadInParallel)
	// creates 10 more immediately — measured live: one of its 10 sat
	// behind "evicting idle worker to make room" for over five minutes,
	// not because anything was stuck, but because up to 18 conversations
	// were competing for 5 pod slots. Closing this lane's own backlog
	// before returning is what a well-behaved caller of a capped resource
	// does, and it is the fix — not a longer wait in the lane that merely
	// inherited the contention.
	closeConsoleConversationsMatching(t, ctx, e, SourceConsole, start)
}

// closeConsoleConversationsMatching sends "/close" through the console to
// every conversation on the given source created after start, and WAITS for
// each to actually reach phase Closed before returning. "/close" only
// enqueues an input — a caller that returns the moment it is sent, without
// confirming the phase transition, hands the NEXT test the same still-Idle
// backlog it meant to clear: measured live, one run of this fix passed and
// the next still hit "evicting idle worker to make room" because the
// reconciler had not yet caught up when the next lane's conversations were
// created a few hundred milliseconds later.
func closeConsoleConversationsMatching(t *testing.T, ctx context.Context, e *Env, source string, start time.Time) {
	t.Helper()
	items, err := e.K.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for i := range items {
		c := &items[i]
		if c.Spec.Signal == nil || c.Spec.Signal.SourceRef == nil ||
			c.Spec.Signal.SourceRef.Name != source || !c.CreationTimestamp.Time.After(start) {
			continue
		}
		e.ConsoleSend(t, c.Name, "/close")
		names = append(names, c.Name)
	}
	waitFor(t, fmt.Sprintf("%d conversations to close", len(names)), 2*time.Minute, func() (bool, error) {
		for _, name := range names {
			var c agentopsv1alpha1.Conversation
			if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: name}, &c); err != nil {
				return false, err
			}
			if c.Status.Phase != agentopsv1alpha1.ConversationClosed {
				return false, nil
			}
		}
		return true, nil
	})
}

// clearAdmissionPoolForReplicaLane pauses the cron lane and deletes every
// existing conversation, then WAITS for their runtime pods to actually be
// gone before returning. `pauseCronLaneAndClearLeftovers`'s own delete runs
// with `--wait=false` — a deleted conversation lingers under its
// close-topics finalizer for up to two minutes, still holding an admission
// slot the whole time. Measured live: proceeding right after the delete
// call, as `TestAdmissionFIFOOnPodDelete` does not, raced this lane's own
// 8 new conversations against the OLD ones still finalizing, failing to
// close within this lane's own 2-minute budget — the same starvation this
// whole fix exists to remove, just moved one step earlier.
func clearAdmissionPoolForReplicaLane(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	pauseCronLaneAndClearLeftovers(t, ctx, e)
	waitFor(t, "no runtime pods", 4*time.Minute, func() (bool, error) {
		pods, err := e.K.Pods(ctx, "agentops.dev/conversation")
		return err == nil && len(pods) == 0, err
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
	origReplicas := int(derefOr(dep.Spec.Replicas, 1))

	// Registered BEFORE the risky call below, never after: t.Fatal (which
	// forceManagerRollout uses throughout) stops this goroutine at once via
	// runtime.Goexit, so a Cleanup registered only after a successful call
	// is never reached on the failure path — leaving the Deployment stuck
	// mid-rollout for every test that runs after this one. Measured live:
	// exactly that, cascading unrelated failures through the rest of the
	// pack on a CI run where the scale-up itself timed out.
	t.Cleanup(func() {
		forceManagerRollout(t, context.Background(), e, origReplicas)
	})

	forceManagerRollout(t, ctx, e, n)
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

	current, clearedAnAffinityRS := zeroAffinityReplicaSets(t, ctx, e)

	// The new pods' FIRST scheduling attempt races the old pod's deletion
	// above — the Deployment's patch already asked for them before this
	// function ever reaches the old ReplicaSet. One that lost that race
	// fails with the same anti-affinity error and then sits in the
	// scheduler's OWN backoff queue, which does not reliably wake up on
	// the old pod's deletion despite that being exactly what would let it
	// succeed next try — measured live: a FailedScheduling event, and no
	// second attempt for 4+ minutes after the pod that blocked it was long
	// gone. Deleting a PENDING pod (never one already Running) forces the
	// ReplicaSet to create a FRESH pod object, which gets an ordinary,
	// un-backed-off scheduling attempt instead of a retry of the failed one.
	//
	// ONLY WHEN AN OLD-AFFINITY ReplicaSet WAS ACTUALLY CLEARED ABOVE. A
	// later call that only changes the replica COUNT (no affinity left to
	// clear) creates a pod with no competing anti-affinity rule at all —
	// nothing for it to lose a race against. Deleting that pod anyway was
	// measured live to kill a brand-new, correctly-scheduling replica
	// before the scheduler had even placed it once, reporting Completed
	// after the manager's own graceful-shutdown path ran — not stuck, just
	// unlucky to be Pending at the instant this function looked.
	if current != nil && clearedAnAffinityRS {
		deletePendingPods(t, ctx, e, current)
	}

	// 5m was still too tight on a FRESH cluster: measured live, both new
	// pods eventually reached Running/Ready (confirmed in the failed run's
	// own diagnostics snapshot), just a little past the 5-minute mark —
	// cold image import plus container start on a brand-new node, not the
	// scheduling deadlock above. 8m is the margin, not the expectation.
	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace,
		"rollout", "status", "deployment/agentops-manager", "--timeout=8m"); err != nil {
		t.Fatalf("manager rollout to %d replicas: %v\n%s", n, err, out)
	}
}

// zeroAffinityReplicaSets zeroes every manager ReplicaSet still carrying the
// old affinity and returns the Deployment's current one (nil if none) plus
// whether any was cleared.
func zeroAffinityReplicaSets(t *testing.T, ctx context.Context, e *Env) (*appsv1.ReplicaSet, bool) {
	t.Helper()
	// The Deployment controller has not necessarily created the NEW
	// ReplicaSet (the one with nil affinity) in the instant after the
	// patch lands — only the OLD one may exist yet. Listing once and
	// finding none with nil affinity left `current` nil, silently
	// skipping the fresh-reschedule step below for every pod the new
	// ReplicaSet eventually creates: measured live, three pods stuck on
	// their one and only FailedScheduling event for 5+ minutes, with a
	// cleanly zeroed old ReplicaSet proving the rest of this function had
	// already done its job. Retry until the new ReplicaSet actually
	// exists, rather than acting on a list taken too early.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var sets appsv1.ReplicaSetList
		if err := e.K.List(ctx, &sets, client.InNamespace(Namespace),
			client.MatchingLabels{"app.kubernetes.io/name": "agentops-manager"}); err != nil {
			t.Fatal(err)
		}
		var current *appsv1.ReplicaSet
		cleared := false
		for i := range sets.Items {
			rs := &sets.Items[i]
			if rs.Spec.Template.Spec.Affinity != nil {
				zeroStaleReplicaSet(t, ctx, e, rs)
				cleared = true
				continue
			}
			current = rs // the Deployment's current one — leave ITS REPLICAS to the controller
		}
		if current != nil || time.Now().After(deadline) {
			return current, cleared
		}
		time.Sleep(2 * time.Second)
	}
}

// deletePendingPods force-deletes the Pending pods of rs, so each is
// recreated with a fresh, un-backed-off scheduling attempt.
func deletePendingPods(t *testing.T, ctx context.Context, e *Env, rs *appsv1.ReplicaSet) {
	t.Helper()
	var pending corev1.PodList
	if err := e.K.List(ctx, &pending, client.InNamespace(Namespace),
		client.MatchingLabels{"pod-template-hash": rs.Labels["pod-template-hash"]}); err != nil {
		t.Fatal(err)
	}
	for j := range pending.Items {
		if pending.Items[j].Status.Phase != corev1.PodPending {
			continue
		}
		if err := e.K.Delete(ctx, &pending.Items[j], client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}
}

// zeroStaleReplicaSet scales rs to zero and force-deletes its pods when its
// template still carries the old anti-affinity. See forceManagerRollout.
func zeroStaleReplicaSet(t *testing.T, ctx context.Context, e *Env, rs *appsv1.ReplicaSet) {
	t.Helper()
	if rs.Spec.Template.Spec.Affinity == nil {
		return // the Deployment's current one — leave it to the controller
	}
	zero := int32(0)
	rsPatch := client.MergeFrom(rs.DeepCopy())
	rs.Spec.Replicas = &zero
	if err := e.K.Patch(ctx, rs, rsPatch); err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	// Scaling the ReplicaSet to zero only asks its pod(s) to terminate —
	// a graceful SIGTERM, up to the pod's own terminationGracePeriod
	// (30s default). Each one still carries the OLD anti-affinity rule
	// until it is actually GONE, so the new pods stay unschedulable for
	// that whole window. A test fixture has no reason to wait out a
	// graceful shutdown, so delete the pods outright instead.
	var pods corev1.PodList
	if err := e.K.List(ctx, &pods, client.InNamespace(Namespace),
		client.MatchingLabels{"pod-template-hash": rs.Labels["pod-template-hash"]}); err != nil {
		t.Fatal(err)
	}
	for j := range pods.Items {
		if err := e.K.Delete(ctx, &pods.Items[j], client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
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

	// This lane's OWN admission cap exposure, independent of the "Two"
	// lane before it: TestReplicasTwoDeliversEveryConsoleThread's cleanup
	// already restored the cron Pipeline by the time this lane starts, so
	// its ticks are live again and this lane needs the same pause —
	// skipping it here reopens exactly the race the comment above
	// describes, just one lane later.
	clearAdmissionPoolForReplicaLane(t, ctx, e)

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

	// The straggler here was never a claim or delivery bug: with
	// TestReplicasTwoDeliversEveryConsoleThread now closing its own 8
	// conversations before returning, this lane's 10 no longer compete for
	// maxActiveConversations (5 by default) against a preceding lane's
	// still-idle backlog — measured live, "evicting idle worker to make
	// room" for a conversation that had done nothing wrong but arrive
	// behind 8 occupied slots. 3m is back to a real margin, not a guess.
	waitFor(t, fmt.Sprintf("all %d conversations to get a console thread AND a delivered reply", n),
		3*time.Minute, func() (bool, error) {
			items, err := e.K.Conversations(ctx)
			if err != nil {
				return false, err
			}
			return countDeliveredConsoleConversations(items, start) >= n, nil
		})

	// Same hygiene as the "Two" lane — leave no idle backlog for whatever
	// runs after this one in the same process.
	closeConsoleConversationsMatching(t, ctx, e, SourceConsole, start)
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
