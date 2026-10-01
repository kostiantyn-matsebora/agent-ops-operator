package controller

import (
	"context"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// TestDeriveCoordinatorTokenMatchesChatPackage pins runtimepod's DUPLICATED
// derivation against chat's own — chat imports runtimepod, so runtimepod
// cannot import chat back, and this package is the one place that can hold
// both and prove they never drift apart (the same shape
// labelSignatureHash is pinned by, one package over).
func TestDeriveCoordinatorTokenMatchesChatPackage(t *testing.T) {
	got := runtimepod.DeriveCoordinatorToken("master-key", "co-a", "root-1")
	want := chat.DeriveCoordinatorToken("master-key", "co-a", "root-1")
	if got != want {
		t.Fatalf("runtimepod.DeriveCoordinatorToken diverged from chat.DeriveCoordinatorToken: got %q, want %q", got, want)
	}
}

// TestResolveActingCoordinatorMatchesChatPackage pins runtimepod's DUPLICATED
// resolution walk (design D-A) against chat.Router.ResolveActingCoordinator's
// own Name half, on an identical member/root/pipeline-addressed fixture set
// — the same reverse-import constraint as DeriveCoordinatorToken above.
func TestResolveActingCoordinatorMatchesChatPackage(t *testing.T) {
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "root-1", "agent-ops"
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}

	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", "agent-ops"
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}

	plain := &agentopsv1alpha1.Conversation{}
	plain.Name, plain.Namespace = "plain-1", "agent-ops"

	c := fakeCoordinatorClient(t, root, member, plain).Build()
	router := &chat.Router{Client: c, Reader: c, Namespace: "agent-ops"}

	for _, conv := range []*agentopsv1alpha1.Conversation{root, member, plain} {
		gotName, err := runtimepod.ResolveActingCoordinator(context.Background(), c, "agent-ops", conv)
		if err != nil {
			t.Fatalf("runtimepod.ResolveActingCoordinator(%s): %v", conv.Name, err)
		}
		want, err := router.ResolveActingCoordinator(context.Background(), conv)
		if err != nil {
			t.Fatalf("chat.Router.ResolveActingCoordinator(%s): %v", conv.Name, err)
		}
		if gotName != want.Name {
			t.Fatalf("%s: runtimepod resolved %q, chat resolved %q", conv.Name, gotName, want.Name)
		}
	}
}
