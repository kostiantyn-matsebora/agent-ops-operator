package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// A non-leader proxies to the leader. When that attempt fails (the leader is
// mid-rollout and refuses the connection) the request falls through to the
// local handler. The proxy has already read the body by then, so a POST must
// still carry it: otherwise the handler sees nothing and answers a 400 that
// reads as the caller's mistake, and a close made during a rollout is lost.
func TestAPostSurvivesAFailedProxyToTheLeader(t *testing.T) {
	ctx := context.Background()

	// A leader that dies mid-rollout: it accepts the connection and takes the
	// whole request, then drops it without an answer. The proxy has sent (and
	// so consumed) the body by the time it learns the attempt failed.
	var hits atomic.Int32
	dying := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(dying.Close)
	port := dying.URL[strings.LastIndex(dying.URL, ":"):]

	follower, _ := apiServerWithActivity()
	follower.ReplicaIdentity = "body-follower"
	follower.Addr = port

	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = "body-leader", ns
	pod.Labels = map[string]string{"app.kubernetes.io/name": "agentops-manager"}
	pod.Spec.Containers = []corev1.Container{{Name: "manager", Image: "busybox:stub"}}
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), pod) })
	pod.Status.PodIP = "127.0.0.1"
	if err := k8sClient.Status().Update(ctx, pod); err != nil {
		t.Fatal(err)
	}

	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = httpapi.LeaseName, ns
	holder := "body-leader"
	lease.Spec.HolderIdentity = &holder
	if err := k8sClient.Create(ctx, lease); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), lease) })

	rec := adapterReq(follower, "POST", "/channel/conversations/nobody/reopen", map[string]any{"channel": "some-channel"}, testMasterToken)
	if hits.Load() == 0 {
		t.Fatalf("the follower never tried the leader, so this proves nothing: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Code == 400 && strings.Contains(rec.Body.String(), `need {\"channel\"}`) {
		t.Fatalf("the body was lost to the failed proxy attempt: %d %s", rec.Code, rec.Body.String())
	}
}
