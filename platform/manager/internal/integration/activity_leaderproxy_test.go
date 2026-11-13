// async-command-bus task 7/section 10's own audit: deleting
// TestChannelOpsProxiesToLeaderWhenReachable (claim_test.go, design D-11 —
// /channel/ops stopped proxying) took its sibling
// TestActivityProxiesToLeaderWhenReachable down with it, in the same old
// file and the same diff, even though /activity* is untouched by this
// change and keeps proxying exactly as before (hardcode-manager-replicas).
// Restored here, unchanged in substance, so a future widening of
// leaderProxyMiddleware's exemption list past /channel/ops/+/state/
// still fails loudly against /activity*.
package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// /activity* gets the same dumb-proxy treatment /channel/ops used to, for a
// different reason — the activity log is a per-process ring buffer with no
// cross-replica fan-out, so a hop the RECONCILER emitted (reconciliation is
// leader-elected) only ever lands in the leader's own buffer. A reader
// pinned to any replica must still see it.
func TestActivityProxiesToLeaderWhenReachable(t *testing.T) {
	ctx := context.Background()

	leaderSrv, leaderLog := apiServerWithActivity()
	leaderSrv.ReplicaIdentity = "activity-proxy-leader"
	leaderHTTP := httptest.NewServer(leaderSrv.Handler())
	t.Cleanup(leaderHTTP.Close)

	port := leaderHTTP.URL[strings.LastIndex(leaderHTTP.URL, ":"):]
	leaderSrv.Addr = port

	followerSrv, followerLog := apiServerWithActivity()
	followerSrv.ReplicaIdentity = "activity-proxy-follower"
	followerSrv.Addr = port

	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = "activity-proxy-leader", ns
	pod.Labels = map[string]string{"app.kubernetes.io/name": "agentops-manager"}
	pod.Spec.Containers = []corev1.Container{{Name: "manager", Image: "busybox:stub"}}
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), pod) })
	pod.Status.PodIP = strings.TrimPrefix(leaderHTTP.URL[:strings.LastIndex(leaderHTTP.URL, ":")], "http://")
	if err := k8sClient.Status().Update(ctx, pod); err != nil {
		t.Fatal(err)
	}

	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = httpapi.LeaseName, ns
	holder := "activity-proxy-leader"
	lease.Spec.HolderIdentity = &holder
	if err := k8sClient.Create(ctx, lease); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), lease) })

	// Only the LEADER's log ever sees this — modelling a reconciler-emitted
	// hop, which only ever runs on the leader.
	leaderLog.Emit(activity.Event{Kind: activity.KindSignalClaimed, Conversation: "activity-proxy-conv"})

	recResp := adapterReq(followerSrv, "GET", "/activity?limit=10", nil, testMasterToken)
	if recResp.Code != 200 {
		t.Fatalf("a non-leader with a reachable leader must proxy /activity, got %d: %s", recResp.Code, recResp.Body.String())
	}
	if !strings.Contains(recResp.Body.String(), "activity-proxy-conv") {
		t.Fatalf("the proxied response must be the leader's own log, got: %s", recResp.Body.String())
	}

	// The follower's OWN log never saw it — proving the response really came
	// from the leader's process, not a coincidence of a shared fixture.
	if events, _ := followerLog.Since("", 0); len(events) != 0 {
		t.Fatalf("the follower's own activity log must stay empty: %+v", events)
	}
}
