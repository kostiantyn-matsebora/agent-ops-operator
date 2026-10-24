package runtimepod

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
)

func TestBuildInjectsAOPSMCPTokenWhenCoordinatorRootBindsTheAopsServer(t *testing.T) {
	conv := conversation("root-1")
	conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: map[string]string{"aops": "http://agentops-mcp-aops:8080/mcp"}},
		"mcp-cm", Resolved{}, "master-key")

	got := envOf(container(pod, "worker"), "AOPS_MCP_TOKEN")
	if got == "" {
		t.Fatal("want AOPS_MCP_TOKEN set")
	}
	want := DeriveCoordinatorToken("master-key", "co-a", "root-1")
	if got != want {
		t.Fatalf("got %q, want the derived token %q", got, want)
	}
}

func TestBuildOmitsAOPSMCPTokenWhenNotCoordinatorRooted(t *testing.T) {
	conv := conversation("plain-1")
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: map[string]string{"aops": "http://agentops-mcp-aops:8080/mcp"}},
		"mcp-cm", Resolved{}, "master-key")

	if envOf(container(pod, "worker"), "AOPS_MCP_TOKEN") != "" {
		t.Fatal("an ordinary Pipeline-rooted conversation must never get AOPS_MCP_TOKEN")
	}
}

func TestBuildOmitsAOPSMCPTokenWhenTheWiringNeverBoundTheAopsServer(t *testing.T) {
	conv := conversation("root-1")
	conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", Resolved{}, "master-key")

	if envOf(container(pod, "worker"), "AOPS_MCP_TOKEN") != "" {
		t.Fatal("a Coordinator root whose capability never lists the aops toolset must get no token")
	}
}

// A plain MEMBER (no coordinatorRef of its own — the self-heal reaper's own
// shape) still gets AOPS_MCP_TOKEN when ResolveFor has resolved which
// Coordinator it acts for (design D-A, coordinator-owner-reach), derived
// against that RESOLVED name rather than a field read directly off the
// conversation.
func TestBuildInjectsAOPSMCPTokenForAMemberViaTheResolvedActingCoordinator(t *testing.T) {
	conv := conversation("member-1")
	conv.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: map[string]string{"aops": "http://agentops-mcp-aops:8080/mcp"}},
		"mcp-cm", Resolved{ActingCoordinator: "co-a"}, "master-key")

	got := envOf(container(pod, "worker"), "AOPS_MCP_TOKEN")
	want := DeriveCoordinatorToken("master-key", "co-a", "member-1")
	if got != want {
		t.Fatalf("got %q, want the derived token %q", got, want)
	}
}

// A member resolving to NO Coordinator (Resolved.ActingCoordinator empty) —
// the ordinary case for a conversation not wired for coordination at all —
// gets no token, exactly as a plain Pipeline-rooted conversation does.
func TestBuildOmitsAOPSMCPTokenForAMemberWithNoResolvedCoordinator(t *testing.T) {
	conv := conversation("member-1")
	conv.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: map[string]string{"aops": "http://agentops-mcp-aops:8080/mcp"}},
		"mcp-cm", Resolved{}, "master-key")

	if envOf(container(pod, "worker"), "AOPS_MCP_TOKEN") != "" {
		t.Fatal("a member with no resolved acting Coordinator must get no token")
	}
}

// A Coordinator's own ROOT ignores Resolved.ActingCoordinator entirely — its
// own coordinatorRef always wins, exactly as before this change.
func TestBuildPrefersTheConversationsOwnCoordinatorRefOverTheResolvedOne(t *testing.T) {
	conv := conversation("root-1")
	conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: map[string]string{"aops": "http://agentops-mcp-aops:8080/mcp"}},
		"mcp-cm", Resolved{ActingCoordinator: "co-wrong"}, "master-key")

	got := envOf(container(pod, "worker"), "AOPS_MCP_TOKEN")
	want := DeriveCoordinatorToken("master-key", "co-a", "root-1")
	if got != want {
		t.Fatalf("got %q, want the token derived from the conversation's own coordinatorRef %q", got, want)
	}
}
