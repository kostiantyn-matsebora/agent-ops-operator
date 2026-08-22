package controller

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
)

func mediated() *agentopsv1alpha1.EgressMediation {
	return &agentopsv1alpha1.EgressMediation{}
}

// A conversation that never asked for mediation makes no claim about it. An
// install that has not enabled the feature must not accumulate a condition.
func TestUnmediatedConversationCarriesNoClaim(t *testing.T) {
	if c := mediationCondition(mcpcompile.Result{Endpoints: map[string]string{
		"kubernetes": "https://elsewhere:8443/mcp",
	}}, nil); c != nil {
		t.Fatalf("no condition may be set without mediation; got %+v", c)
	}
}

func TestAllPlainEndpointsAreReportedMediated(t *testing.T) {
	c := mediationCondition(mcpcompile.Result{Endpoints: map[string]string{
		"kubernetes":    "http://agentops-mcp-k8s.agent-ops.svc:8080/mcp",
		"homeassistant": "http://agentops-mcp-ha.agent-ops.svc:8086/mcp",
	}}, mediated())
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatalf("plain endpoints must report as mediated; got %+v", c)
	}
}

// Design D7 — the failure this avoids is an install believing it is mediated
// while one server is not.
func TestAnUnenforceableEndpointIsNamed(t *testing.T) {
	c := mediationCondition(mcpcompile.Result{Endpoints: map[string]string{
		"kubernetes": "http://agentops-mcp-k8s.agent-ops.svc:8080/mcp",
		"vendor":     "https://vendor.example.com/mcp",
	}}, mediated())

	if c == nil || c.Status != metav1.ConditionFalse {
		t.Fatalf("an unenforceable endpoint must report False; got %+v", c)
	}
	if c.Reason != ReasonEndpointNotEnforceable {
		t.Fatalf("reason = %q", c.Reason)
	}
	if !strings.Contains(c.Message, "vendor") {
		t.Fatalf("the message must name the endpoint that is not enforced: %q", c.Message)
	}
	if strings.Contains(c.Message, "kubernetes") {
		t.Fatalf("the enforced endpoint must not be reported as a problem: %q", c.Message)
	}
}

// A hand-written mcp.json is opaque by construction, so nothing can be claimed
// about what it points at. Reporting it as mediated would be a guess.
func TestARawConfigIsReportedAsUnconfirmable(t *testing.T) {
	c := mediationCondition(mcpcompile.Result{RawConfigMap: "hand-written"}, mediated())
	if c == nil || c.Status != metav1.ConditionFalse || c.Reason != ReasonOpaqueConfig {
		t.Fatalf("a raw config must be reported as unconfirmable; got %+v", c)
	}
}

// Enabling mediation on a conversation that binds no MCP server at all is fine
// and is reported as such — there is nothing unenforced.
func TestNoBoundServersIsStillMediated(t *testing.T) {
	c := mediationCondition(mcpcompile.Result{}, mediated())
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatalf("a conversation binding nothing has no unenforced path; got %+v", c)
	}
}

// The condition must disappear when mediation is turned off, or a stale claim
// outlives the thing it described.
func TestTurningMediationOffClearsTheClaim(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{}
	applyMediationCondition(conv, mediationCondition(mcpcompile.Result{}, mediated()))
	if len(conv.Status.Conditions) != 1 {
		t.Fatal("the condition should have been set")
	}
	applyMediationCondition(conv, mediationCondition(mcpcompile.Result{}, nil))
	for _, c := range conv.Status.Conditions {
		if c.Type == ConditionMediated {
			t.Fatal("the claim must be removed when mediation is off")
		}
	}
}

// Task 4.2 — a proxy that will not start fails the pod, and the message says so
// rather than reading as a runtime image problem.
func TestAStuckProxyNamesItself(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		InitContainerStatuses: []corev1.ContainerStatus{
			{Name: "egress-init", State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
			{Name: "egress-proxy", State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
		},
	}}
	got := mediationSuffix(pod)
	if !strings.Contains(got, "egress-proxy") {
		t.Fatalf("the failing container must be named: %q", got)
	}
	if strings.Contains(got, "egress-init") {
		t.Fatalf("a container that completed cleanly must not be blamed: %q", got)
	}
}

// A healthy mediated pod adds nothing to the message. A permanent note about a
// component that is working is noise in every unrelated failure.
func TestAHealthyProxyAddsNothing(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		InitContainerStatuses: []corev1.ContainerStatus{
			{Name: "egress-proxy", State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{}}},
		},
	}}
	if got := mediationSuffix(pod); got != "" {
		t.Fatalf("nothing to say about a working proxy, got %q", got)
	}
}
