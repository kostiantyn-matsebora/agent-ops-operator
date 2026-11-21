package chat

import (
	"context"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func treeConvs(root *agentopsv1alpha1.Conversation) []*agentopsv1alpha1.Conversation {
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	member.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: root.Name}
	grand := &agentopsv1alpha1.Conversation{}
	grand.Name, grand.Namespace = "grandchild-1", testNS
	grand.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: member.Name, Entry: "helper"}
	grand.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: member.Name}
	return []*agentopsv1alpha1.Conversation{member, grand}
}

// A root whose cascade cannot be read is never left Closed over open members.
func TestCloseDoesNotCloseTheRootWhenTheCascadeFails(t *testing.T) {
	root := boundConv("root-1", "c1")
	kids := treeConvs(root)
	r, _, _ := closeFixture(t, nsChannel("c1", "slack"), root, kids[0], kids[1])
	rec, _, err := r.Conversations.Get(context.Background(), state.Key("root-1"))
	if err != nil {
		t.Fatal(err)
	}
	healthy := r.Conversations
	r.Conversations = unreadableConversations{healthy}
	if err := r.CloseConversation(context.Background(), &rec); err == nil {
		t.Fatal("a failed cascade must fail the close")
	}
	got, _, _ := healthy.Get(context.Background(), state.Key("root-1"))
	if got.Phase == state.PhaseClosed {
		t.Fatal("the root was left Closed over members that were never closed")
	}
}

type recordingDeletes struct {
	state.Collection[state.ConversationRecord]
	order *[]string
}

func (r recordingDeletes) Delete(ctx context.Context, k state.Key) error {
	*r.order = append(*r.order, string(k))
	return r.Collection.Delete(ctx, k)
}

// Deleting a root removes its tree leaves first and the root last, and an
// OPEN member does not stop it or survive it.
func TestDeleteTreeGoesLeavesToRootAndTakesOpenMembers(t *testing.T) {
	root := boundConv("root-1", "c1")
	kids := treeConvs(root)
	r, _, _ := closeFixture(t, nsChannel("c1", "slack"), root, kids[0], kids[1])
	var order []string
	healthy := r.Conversations
	r.Conversations = recordingDeletes{healthy, &order}

	if err := r.DeleteTree(context.Background(), "root-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"grandchild-1", "member-1", "root-1"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("delete order = %v, want %v", order, want)
	}
	for _, n := range want {
		if _, _, err := healthy.Get(context.Background(), state.Key(n)); err == nil {
			t.Fatalf("%s survived the delete", n)
		}
	}
}
