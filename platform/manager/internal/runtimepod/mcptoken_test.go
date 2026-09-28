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

	if got := envOf(container(pod, "worker"), "AOPS_MCP_TOKEN"); got != "" {
		t.Fatal("an ordinary Pipeline-rooted conversation must never get AOPS_MCP_TOKEN")
	}
}

func TestBuildOmitsAOPSMCPTokenWhenTheWiringNeverBoundTheAopsServer(t *testing.T) {
	conv := conversation("root-1")
	conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	pod := Build(conv, &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", Resolved{}, "master-key")

	if got := envOf(container(pod, "worker"), "AOPS_MCP_TOKEN"); got != "" {
		t.Fatal("a Coordinator root whose capability never lists the aops toolset must get no token")
	}
}
