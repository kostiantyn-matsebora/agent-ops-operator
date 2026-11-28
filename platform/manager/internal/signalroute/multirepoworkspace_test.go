// multi-repo-workspace, task 2.2: the resolved repository list, agent file
// and working directory are snapshotted onto a NEW conversation exactly
// once, at createConversation — the signal lane's half of the snapshot
// (chat/router.go and chat/coordinate.go are the other two call sites, and
// carry the same three lines).
package signalroute

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

const mrwNS = "agent-ops"

func mrwScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

// TestCreateConversationSnapshotsRepositoriesFromTheProfile proves the
// signal lane's half of "the resolved list is snapshotted onto the
// Conversation" (multi-repo-workspace): a Pipeline naming only a profile,
// whose profile declares a repository list, produces a conversation whose
// OWN Repositories/AgentFile/WorkingDir carry that list — not a live
// pointer back to the profile.
func TestCreateConversationSnapshotsRepositoriesFromTheProfile(t *testing.T) {
	ctx := context.Background()

	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Namespace, profile.Name = mrwNS, "mrw-profile"
	profile.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git", Ref: "main"},
		{Name: "infra", URL: "https://example.com/infra.git", Ref: "main"},
	}
	profile.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Repo: "app", Path: "agents/app.md"}
	profile.Spec.WorkingDir = "infra"

	pipe := &agentopsv1alpha1.Pipeline{}
	pipe.Namespace, pipe.Name = mrwNS, "mrw-pipe"
	pipe.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-profile"}

	fakeClient := fake.NewClientBuilder().WithScheme(mrwScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(profile, pipe).Build()

	store, err := kube.Open(fakeClient, mrwNS, kube.ConversationLayout())
	if err != nil {
		t.Fatal(err)
	}

	claimant, err := chat.ResolveClaimant(ctx, fakeClient, mrwNS, "mrw-pipe")
	if err != nil || claimant == nil {
		t.Fatalf("resolve claimant: %v, %v", claimant, err)
	}

	deps := Deps{Reader: fakeClient, Conversations: store, Namespace: mrwNS}
	sig := &state.SignalRecord{Kind: kindChat, Payload: "do the thing", InitiationID: "init-1"}

	conv, err := createConversation(ctx, deps, claimant, sig)
	if err != nil {
		t.Fatalf("createConversation: %v", err)
	}

	if len(conv.Repositories) != 2 || conv.Repositories[0].Name != "app" || conv.Repositories[1].Name != "infra" {
		t.Fatalf("Repositories = %+v, want the profile's two entries", conv.Repositories)
	}
	if conv.AgentFile == nil || conv.AgentFile.Repo != "app" || conv.AgentFile.Path != "agents/app.md" {
		t.Fatalf("AgentFile = %+v, want the profile's", conv.AgentFile)
	}
	if conv.WorkingDir != "infra" {
		t.Fatalf("WorkingDir = %q, want %q", conv.WorkingDir, "infra")
	}

	// A LATER profile edit must not reach the already-created conversation —
	// this is the resolution function's own guarantee, proved here against
	// the actual stored record rather than the in-memory one createConversation
	// returned.
	profile.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: "only-this-now", URL: "https://example.com/x.git"}}
	if err := fakeClient.Update(ctx, profile); err != nil {
		t.Fatal(err)
	}
	stored, _, err := store.Get(ctx, state.Key(conv.Name))
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Repositories) != 2 || stored.Repositories[0].Name != "app" {
		t.Fatalf("stored Repositories = %+v, a profile edit after creation must not reach it", stored.Repositories)
	}
}

// TestCreateConversationWithNoRepositoriesAnywhereSnapshotsNothing is the
// zero-checkout case at the same call site: neither the capability nor the
// profile declares a list, so the conversation's own snapshot stays empty.
func TestCreateConversationWithNoRepositoriesAnywhereSnapshotsNothing(t *testing.T) {
	ctx := context.Background()

	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Namespace, profile.Name = mrwNS, "mrw-profile-bare"

	pipe := &agentopsv1alpha1.Pipeline{}
	pipe.Namespace, pipe.Name = mrwNS, "mrw-pipe-bare"
	pipe.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-profile-bare"}

	fakeClient := fake.NewClientBuilder().WithScheme(mrwScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(profile, pipe).Build()
	store, err := kube.Open(fakeClient, mrwNS, kube.ConversationLayout())
	if err != nil {
		t.Fatal(err)
	}
	claimant, err := chat.ResolveClaimant(ctx, fakeClient, mrwNS, "mrw-pipe-bare")
	if err != nil || claimant == nil {
		t.Fatalf("resolve claimant: %v, %v", claimant, err)
	}
	deps := Deps{Reader: fakeClient, Conversations: store, Namespace: mrwNS}
	sig := &state.SignalRecord{Kind: kindChat, Payload: "hi", InitiationID: "init-2"}

	conv, err := createConversation(ctx, deps, claimant, sig)
	if err != nil {
		t.Fatalf("createConversation: %v", err)
	}
	if len(conv.Repositories) != 0 || conv.AgentFile != nil || conv.WorkingDir != "" {
		t.Fatalf("want no checkout, got Repositories=%+v AgentFile=%+v WorkingDir=%q",
			conv.Repositories, conv.AgentFile, conv.WorkingDir)
	}
}
