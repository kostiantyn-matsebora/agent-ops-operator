// The corrective input a Coordinator root receives after a turn without a move
// (coordination-loop, design D-C) is an ORDINARY input of origin `manager`, so
// it reaches the root's bound threads by the existing per-destination rule
// (conversation-opens-with-its-input) with no clause of its own — which only a
// real reconcile over a real API server proves, since the fake client in
// httpapi's own unit tests never runs DeliverInputs and never validates the
// widened `origin.kind` enum.
package integration

import (
	"context"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

func TestManagerOriginInputReachesEveryBoundChannelOnce(t *testing.T) {
	mkProfile(t, "prof-mpt")
	mkChannel(t, "mpt-desk", "mpt-ta")
	mkCoordinator(t, "co-mpt", nil, []string{"mpt-desk"}, "prof-mpt", nil)
	reconcileCoordinator(t, "co-mpt")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-mpt-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-mpt"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-mpt"}
	root.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "mpt-desk"}}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, root.Name) })
	srv := apiServer()

	// The thread exists before the hand-back: bound at creation, as every
	// root's channels are.
	reconcileWithOps(t, srv, root.Name)
	bindThread(t, srv, drainOps(t, srv, "mpt-ta"), "mpt-thread-1")

	// The append `/work/done` performs on a first miss, through the same
	// Router helper, admitted by the real schema's widened origin enum.
	inputID := chat.MoveInputID(root.Name, "r1")
	if err := srv.Router.AppendManagerInput(context.Background(), root.Name, inputID,
		"Your last turn ended without a move."); err != nil {
		t.Fatalf("append manager input: %v", err)
	}
	got := getConv(t, root.Name)
	if len(got.Spec.Inputs) != 1 || got.Spec.Inputs[0].Origin == nil ||
		got.Spec.Inputs[0].Origin.Kind != agentopsv1alpha1.OriginManager ||
		got.Spec.Inputs[0].Origin.Name != chat.MoveRuleName {
		t.Fatalf("the manager-origin input must round-trip through the real schema, got %+v", got.Spec.Inputs)
	}

	reconcileWithOps(t, srv, root.Name)
	cards := cardsFor(drainOps(t, srv, "mpt-ta"))
	if len(cards) != 1 {
		t.Fatalf("want exactly one input-delivery op for the one bound channel, got %d", len(cards))
	}
	card := cards[0]
	if card.ID != chat.InputOpID(root.Name, inputID, "mpt-desk") {
		t.Fatalf("the delivery must take the existing stable input op id, got %q", card.ID)
	}
	if card.Message.OriginKind != agentopsv1alpha1.OriginManager || card.Message.Source != chat.MoveRuleName {
		t.Fatalf("the card must be attributed to the manager's rule, got kind=%q source=%q",
			card.Message.OriginKind, card.Message.Source)
	}
	if card.ThreadID == nil || *card.ThreadID != "mpt-thread-1" {
		t.Fatalf("the card must land in the root's own thread, got %v", card.ThreadID)
	}

	// Re-deriving on later passes posts nothing twice: the same rule every
	// other input obeys, no clause added.
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, root.Name)
	}
	if n := len(cardsFor(drainOps(t, srv, "mpt-ta"))); n != 0 {
		t.Fatalf("a delivered corrective input must not be reposted, got %d", n)
	}
}
