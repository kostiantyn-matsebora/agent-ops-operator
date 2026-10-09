//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"
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

	assertManagerRunsAtLeastTwoReplicas(t, ctx, e)

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

// assertManagerRunsAtLeastTwoReplicas is the precondition this lane depends
// on: it proves nothing about the fix if the install silently ran at one
// pod.
func assertManagerRunsAtLeastTwoReplicas(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	pods, err := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) < 2 {
		t.Fatalf("this lane needs at least 2 manager pods, got %d — chart/values.yaml's replicas default must be >= 2", len(pods))
	}
}
