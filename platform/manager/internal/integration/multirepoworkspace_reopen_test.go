// multi-repo-workspace, task 2.2: "a reopened conversation keeps its
// snapshot" — the reopen verb must not re-resolve the workspace layout,
// mirroring conversation_verbs_test.go's own
// TestReopenRestoresIdleAndLeavesWiringExactlyAsItWas for the three new
// fields.
package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// mkMRWReopenProfile is its own fixture, never shared with mkProfile's
// "verb-profile": this test mutates it after creation, and a profile
// fixture other tests in this suite also read must stay untouched by that.
//
// It declares the DEPRECATED singular Repository rather than Repositories
// simply because this test is about REOPEN leaving the Conversation's own
// snapshot untouched, not about which spelling resolved it —
// TestProfileRepositoryAloneStillAdmits covers that spelling on its own.
func mkMRWReopenProfile(t *testing.T, name string) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Agent = "tester"
	p.Spec.MaxTurns = 5
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Repository = &agentopsv1alpha1.RepositorySpec{URL: "https://example.com/original.git", Ref: "main"}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// mkVerbConvWithRepositories is mkVerbConv plus a resolved workspace
// snapshot, standing in for what createConversation would have written at
// creation — this test exercises the reopen verb alone, not the
// resolution path signalroute/multirepoworkspace_test.go already pins.
func mkVerbConvWithRepositories(t *testing.T, name, profile string, channels []string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	for _, c := range channels {
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	conv.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git", Ref: "main"},
		{Name: "infra", URL: "https://example.com/infra.git", Ref: "main"},
	}
	conv.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Repo: "app", Path: "agents/app.md"}
	conv.Spec.WorkingDir = "infra"
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

func TestReopenLeavesTheWorkspaceSnapshotUntouched(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-mrw", "tg")
	mkMRWReopenProfile(t, "verb-profile-mrw")
	mkVerbConvWithRepositories(t, "verb-reopen-mrw", "verb-profile-mrw", []string{"verb-ch-mrw"})

	// The originating profile changes AFTER creation and BEFORE reopen — if
	// reopen re-resolved anything, this is what it would pick up instead.
	var profile agentopsv1alpha1.AgentProfile
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "verb-profile-mrw"}, &profile); err != nil {
		t.Fatal(err)
	}
	profile.Spec.Repository.URL = "https://example.com/completely-different.git"
	if err := k8sClient.Update(context.Background(), &profile); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/channel/conversations/verb-reopen-mrw/reopen",
		map[string]string{"channel": "verb-ch-mrw"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("reopen: %d %s", rec.Code, rec.Body.String())
	}
	drainConvTasks(t)

	got := getConv(t, "verb-reopen-mrw")
	if got.Status.Phase != agentopsv1alpha1.ConversationIdle {
		t.Fatalf("reopen must restore Idle, got %q", got.Status.Phase)
	}
	if len(got.Spec.Repositories) != 2 || got.Spec.Repositories[0].Name != "app" || got.Spec.Repositories[1].Name != "infra" {
		t.Errorf("reopen must not re-resolve repositories, got %+v", got.Spec.Repositories)
	}
	if got.Spec.AgentFile == nil || got.Spec.AgentFile.Repo != "app" || got.Spec.AgentFile.Path != "agents/app.md" {
		t.Errorf("reopen must not re-resolve agentFile, got %+v", got.Spec.AgentFile)
	}
	if got.Spec.WorkingDir != "infra" {
		t.Errorf("reopen must not re-resolve workingDir, got %q", got.Spec.WorkingDir)
	}
}
