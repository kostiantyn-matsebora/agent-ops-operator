// multi-repo-workspace, task 2.3: "An inconsistent declaration is refused
// at resolution" — the Ready condition on Pipeline and Coordinator names
// each of the four workspace-layout causes with its own stable reason.
package integration

import (
	"context"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// mrwRepos is a two-entry repositories[] list, reused by every case below
// that needs "two or more resolved" as its baseline.
func mrwRepos() []agentopsv1alpha1.RepositoryEntry {
	return []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git"},
		{Name: "infra", URL: "https://example.com/infra.git"},
	}
}

func mustReadyCondition(t *testing.T, conditions []metav1.Condition) *metav1.Condition {
	t.Helper()
	cond := apimeta.FindStatusCondition(conditions, "Ready")
	if cond == nil {
		t.Fatalf("no Ready condition at all: %+v", conditions)
	}
	return cond
}

// TestWorkingDirNeedsTwoOrMoreRepositories — a workingDir set against a
// single resolved repository.
func TestWorkingDirNeedsTwoOrMoreRepositories(t *testing.T) {
	mkProfile(t, "mrw-ready-prof-1")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-workingdir-count", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-1"}
	p.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: "app", URL: "https://example.com/app.git"}}
	p.Spec.WorkingDir = "app"
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-workingdir-count")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonWorkingDirNeedsRepositories {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonWorkingDirNeedsRepositories)
	}
}

// TestWorkingDirNamesAnUnresolvedRepository — two repositories resolve, but
// workingDir names a third.
func TestWorkingDirNamesAnUnresolvedRepository(t *testing.T) {
	mkProfile(t, "mrw-ready-prof-2")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-workingdir-unknown", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-2"}
	p.Spec.Repositories = mrwRepos()
	p.Spec.WorkingDir = "ghost"
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-workingdir-unknown")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonWorkingDirUnresolved {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonWorkingDirUnresolved)
	}
}

// TestAgentFileRepoNamesAnUnresolvedRepository — agentFile.repo names a
// repository outside the resolved list.
func TestAgentFileRepoNamesAnUnresolvedRepository(t *testing.T) {
	mkProfile(t, "mrw-ready-prof-3")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-agentfile-unknown", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-3"}
	p.Spec.Repositories = mrwRepos()
	p.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Repo: "ghost", Path: "agents/x.md"}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-agentfile-unknown")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonAgentFileRepoUnresolved {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonAgentFileRepoUnresolved)
	}
}

// TestAgentFileRepoIsRequiredWithTwoOrMoreRepositories — agentFile is
// declared with no repo while two repositories resolve.
func TestAgentFileRepoIsRequiredWithTwoOrMoreRepositories(t *testing.T) {
	mkProfile(t, "mrw-ready-prof-4")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-agentfile-required", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-4"}
	p.Spec.Repositories = mrwRepos()
	p.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Path: "agents/x.md"}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-agentfile-required")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonAgentFileRepoRequired {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonAgentFileRepoRequired)
	}
}

// TestDanglingPromptQualifierFailsReady — prompt carries a `<repo>:<path>`
// qualifier naming a repository outside the resolved list. Prompt lives
// only on AgentProfile, so this Pipeline reaches it through profileRef
// while still declaring its OWN inline repositories — the capability wins
// the list, the profile still supplies the prompt that qualifies it.
func TestDanglingPromptQualifierFailsReady(t *testing.T) {
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "mrw-ready-prof-prompt", ns
	profile.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	profile.Spec.Prompt = "ghost:prompts/review.md"
	if err := k8sClient.Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-prompt-qualifier", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-prompt"}
	p.Spec.Repositories = mrwRepos()
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-prompt-qualifier")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonAgentFileRepoUnresolved {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonAgentFileRepoUnresolved)
	}
}

// TestConsistentWorkspaceLayoutStaysReady is the control: every cause above
// made False by ONE deliberate mistake each, so a Pipeline that gets every
// one of them right must still be Ready — the new checks must not fire on
// a declaration that is actually fine.
func TestConsistentWorkspaceLayoutStaysReady(t *testing.T) {
	mkProfile(t, "mrw-ready-prof-ok")
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-ready-ok", ns
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-prof-ok"}
	p.Spec.Repositories = mrwRepos()
	p.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Repo: "app", Path: "agents/x.md"}
	p.Spec.WorkingDir = "infra"
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "mrw-ready-ok")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "True" {
		t.Fatalf("a consistent declaration must stay Ready: %+v", cond)
	}
}

// TestCoordinatorWorkingDirNeedsTwoOrMoreRepositories proves the SAME cause
// is reported on the Coordinator too — the requirement names both holders.
func TestCoordinatorWorkingDirNeedsTwoOrMoreRepositories(t *testing.T) {
	mkProfile(t, "mrw-ready-co-prof")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "mrw-ready-co-workingdir", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-ready-co-prof"}
	co.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: "app", URL: "https://example.com/app.git"}}
	co.Spec.WorkingDir = "app"
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "mrw-ready-co-workingdir")
	cond := mustReadyCondition(t, got.Status.Conditions)
	if cond.Status != "False" || cond.Reason != controller.ReasonWorkingDirNeedsRepositories {
		t.Fatalf("got status=%s reason=%s message=%q, want False/%s",
			cond.Status, cond.Reason, cond.Message, controller.ReasonWorkingDirNeedsRepositories)
	}
}
