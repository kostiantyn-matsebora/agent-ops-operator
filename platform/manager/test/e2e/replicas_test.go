//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
// rest of this test, restoring the original replica count and pod
// anti-affinity on cleanup so later lanes see the chart's own default again.
//
// The anti-affinity is a HARD requirement (deployment.yaml: one real node per
// replica, deliberately — see chart/templates/deployment.yaml), which this
// pack's single-node k3d cluster cannot satisfy for n > 1. Scaling alone
// would leave n-1 pods permanently Pending, proving nothing about n replicas
// racing a real poll. Cleared with a JSON MERGE patch (null removes the
// field) rather than a JSON patch "remove", which errors on a field that is
// not there to begin with — relevant on cleanup if the field was never set.
func scaleManagerReplicas(t *testing.T, ctx context.Context, e *Env, n int) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	origReplicas := dep.Spec.Replicas
	origAffinity := dep.Spec.Template.Spec.Affinity

	patch := client.MergeFrom(dep.DeepCopy())
	want := int32(n)
	dep.Spec.Replicas = &want
	dep.Spec.Template.Spec.Affinity = nil
	if err := e.K.Patch(ctx, &dep, patch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var cur appsv1.Deployment
		if err := e.K.Get(context.Background(), types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &cur); err != nil {
			t.Logf("restoring manager replicas: %v", err)
			return
		}
		restore := client.MergeFrom(cur.DeepCopy())
		cur.Spec.Replicas = origReplicas
		cur.Spec.Template.Spec.Affinity = origAffinity
		if err := e.K.Patch(context.Background(), &cur, restore); err != nil {
			t.Logf("restoring manager replicas: %v", err)
			return
		}
		if out, err := e.Cluster.Kubectl(context.Background(), "-n", Namespace,
			"rollout", "status", "deployment/agentops-manager", "--timeout=3m"); err != nil {
			t.Logf("restoring manager replicas: %v\n%s", err, out)
		}
	})
	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace,
		"rollout", "status", "deployment/agentops-manager", "--timeout=3m"); err != nil {
		t.Fatalf("manager scale to %d: %v\n%s", n, err, out)
	}
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
			got := 0
			for i := range items {
				c := &items[i]
				if c.Spec.Signal == nil || c.Spec.Signal.SourceRef == nil ||
					c.Spec.Signal.SourceRef.Name != SourceConsole || !c.CreationTimestamp.Time.After(start) {
					continue
				}
				if tid := c.ThreadFor("console"); tid == nil || *tid == "" {
					continue
				}
				delivered := false
				for _, r := range c.Status.Runs {
					if r.DeliveredTo("console") {
						delivered = true
						break
					}
				}
				if delivered {
					got++
				}
			}
			return got >= n, nil
		})
}
