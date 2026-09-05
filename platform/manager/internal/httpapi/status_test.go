package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"
)

// handleStatus counts capacity the SAME way admission does -- from the live
// pod list, not from any status field -- so this pins that a pod without a
// matching Conversation, and a Conversation without a pod but nothing
// pending, are each counted exactly once, in the right bucket.
func TestHandleStatusCountsSlotsFromLivePodsNotStatus(t *testing.T) {
	runningPod := &corev1.Pod{}
	runningPod.Name, runningPod.Namespace = "conv-a-pod", "agent-ops"
	runningPod.Labels = map[string]string{runtimepod.LabelApp: runtimepod.LabelAppValue, runtimepod.LabelConversation: "conv-a"}
	runningPod.Status.Phase = corev1.PodRunning

	// conv-a has a pod: not waiting, whatever its own pending inputs say.
	convA := &agentopsv1alpha1.Conversation{}
	convA.Name, convA.Namespace = "conv-a", "agent-ops"
	convA.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1"}

	// conv-b has no pod and inflight work: it IS waiting.
	convB := &agentopsv1alpha1.Conversation{}
	convB.Name, convB.Namespace = "conv-b", "agent-ops"
	convB.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r2"}

	// conv-c has no pod and nothing pending: idle, not waiting.
	convC := &agentopsv1alpha1.Conversation{}
	convC.Name, convC.Namespace = "conv-c", "agent-ops"

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(runningPod, convA, convB, convC).Build()
	s := &Server{Reader: c, Client: c, Namespace: "agent-ops", MaxActiveConversations: 5, Version: "test"}

	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest("GET", "/status", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out statusResponse
	mustUnmarshal(t, rec.Body.Bytes(), &out)
	if out.RuntimeSlots.InUse != 1 || out.RuntimeSlots.Waiting != 1 || out.RuntimeSlots.Max != 5 {
		t.Fatalf("slots: %+v", out.RuntimeSlots)
	}
	if out.Version != "test" {
		t.Fatalf("version not reported: %+v", out)
	}
	if out.Leader != "" {
		t.Fatalf("no lease exists: leader must be reported empty, not guessed: %q", out.Leader)
	}
}

// An open breaker is reported as an install-wide storage outage on /status --
// the same fact MetricsSample turns into a gauge, from the one breaker both
// edges feed.
func TestHandleStatusReportsAnOpenStorageBreaker(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build()
	s := &Server{Reader: c, Client: c, Namespace: "agent-ops"}
	for i := 0; i < storagebreaker.Threshold; i++ {
		s.breaker().Report()
	}

	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest("GET", "/status", nil))
	var out statusResponse
	mustUnmarshal(t, rec.Body.Bytes(), &out)
	if out.StorageOutage == nil || out.StorageOutage.Since == "" || out.StorageOutage.ForSeconds < 0 {
		t.Fatalf("open breaker must be reported: %+v", out.StorageOutage)
	}

	sample := s.MetricsSample()
	if !sample.StorageOutage || sample.StorageOutageAge < 0 {
		t.Fatalf("MetricsSample must report the SAME outage: %+v", sample)
	}
}

// The lease holder is reported when readable, and empty -- never this
// replica's own name -- when the lease cannot be read at all.
func TestLeaderIdentityReadsTheLeaseHolder(t *testing.T) {
	holder := "agentops-manager-abc"
	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = LeaseName, "agent-ops"
	lease.Spec.HolderIdentity = &holder
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(lease).Build()
	s := &Server{Reader: c, Namespace: "agent-ops"}

	if got := s.leaderIdentity(context.Background()); got != holder {
		t.Fatalf("got %q, want %q", got, holder)
	}

	empty := &Server{Reader: fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build(), Namespace: "agent-ops"}
	if got := empty.leaderIdentity(context.Background()); got != "" {
		t.Fatalf("unreadable lease must report empty, got %q", got)
	}
}

// handlePipelineResolved is the console's ONE way to see the composed
// allowlist without reimplementing composition itself — every branch here is
// a way that composition can be partial: a ref that resolves, one that
// doesn't, and no refs at all.

func TestHandlePipelineResolvedUnknownPipeline(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build()
	s := &Server{Reader: c, Client: c, Namespace: "agent-ops"}

	req := httptest.NewRequest("GET", "/pipelines/nope/resolved", nil)
	req.SetPathValue("name", "nope")
	rec := httptest.NewRecorder()
	s.handlePipelineResolved(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlePipelineResolvedMissingProfileIsUnresolvedNotAnError(t *testing.T) {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "p1", "agent-ops"
	p.Spec.ProfileRef.Name = "ghost"
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(p).Build()
	s := &Server{Reader: c, Client: c, Namespace: "agent-ops"}

	rec := decodeResolved(t, s, "p1")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out resolvedResponse
	mustUnmarshal(t, rec.Body.Bytes(), &out)
	if len(out.Unresolved) != 1 || out.Unresolved[0] != "AgentProfile/ghost" {
		t.Fatalf("unresolved profile: %+v", out.Unresolved)
	}
	if out.Runtime != "" {
		t.Fatalf("no profile resolved: runtime must stay empty, got %q", out.Runtime)
	}
	if out.AllowedTools == nil || len(out.AllowedTools) != 0 {
		t.Fatalf("no toolsets bound: AllowedTools must be an empty list, not nil: %#v", out.AllowedTools)
	}
}

func TestHandlePipelineResolvedComposesToolsetsAndMCPConfigs(t *testing.T) {
	prof := &agentopsv1alpha1.AgentProfile{}
	prof.Name, prof.Namespace = "prof", "agent-ops"

	ts := &agentopsv1alpha1.MCPToolset{}
	ts.Name, ts.Namespace = "ts1", "agent-ops"
	ts.Spec.Tools = []string{"Read", "Bash(git:*)"}

	cfg := &agentopsv1alpha1.MCPConfig{}
	cfg.Name, cfg.Namespace = "mc1", "agent-ops"
	cfg.Spec.Servers = map[string]agentopsv1alpha1.MCPServer{"z-server": {}, "a-server": {}}

	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "p1", "agent-ops"
	p.Spec.ProfileRef.Name = "prof"
	p.Spec.Toolsets = &agentopsv1alpha1.ToolsetBinding{
		Mode: "merge",
		Refs: []agentopsv1alpha1.ObjectRef{{Name: "ts1"}, {Name: "missing-ts"}},
	}
	p.Spec.MCPConfigs = &agentopsv1alpha1.ToolingBinding{
		Refs: []agentopsv1alpha1.ObjectRef{{Name: "mc1"}, {Name: "missing-mc"}},
	}

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(p, prof, ts, cfg).Build()
	s := &Server{Reader: c, Client: c, Namespace: "agent-ops"}

	rec := decodeResolved(t, s, "p1")
	var out resolvedResponse
	mustUnmarshal(t, rec.Body.Bytes(), &out)

	if out.Runtime != "default" {
		t.Fatalf("no runtime named anywhere: want the bootstrap fallback \"default\", got %q", out.Runtime)
	}
	if len(out.AllowedTools) != 2 || out.AllowedTools[0] != "Read" || out.AllowedTools[1] != "Bash(git:*)" {
		t.Fatalf("composed allowlist: %+v", out.AllowedTools)
	}
	if len(out.MCPServers) != 2 || out.MCPServers[0] != "a-server" || out.MCPServers[1] != "z-server" {
		t.Fatalf("mcp servers must be sorted: %+v", out.MCPServers)
	}
	wantUnresolved := map[string]bool{"MCPToolset/missing-ts": true, "MCPConfig/missing-mc": true}
	if len(out.Unresolved) != 2 || !wantUnresolved[out.Unresolved[0]] || !wantUnresolved[out.Unresolved[1]] {
		t.Fatalf("unresolved refs: %+v", out.Unresolved)
	}
}

func decodeResolved(t *testing.T, s *Server, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/pipelines/"+name+"/resolved", nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	s.handlePipelineResolved(rec, req)
	return rec
}

func mustUnmarshal(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
}
