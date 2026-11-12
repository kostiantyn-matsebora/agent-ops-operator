// async-command-bus design D-7, task 5.3: closeCascadeRequests' own
// end-to-end effect, beyond the pure ordering convtask.CloseMemberRequests
// already pins (internal/convtask/cascade_test.go).
package httpapi

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func closeCascadeMember(name, parent string) *agentopsv1alpha1.Conversation {
	m := &agentopsv1alpha1.Conversation{}
	m.Name, m.Namespace = name, "agent-ops"
	m.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: "worker"}
	return m
}

// closeCascadeRequests lists the TARGET's own subtree and builds one
// CascadeMember task per LIVE descendant, deepest-first, ahead of the
// caller's own rootReq — all sharing rootName as Key.
func TestCloseCascadeRequestsOrdersMembersBeforeTheRootAndSharesItsKey(t *testing.T) {
	root := convTestBoundConv("root-1", "c1", "t1")
	mid := closeCascadeMember("mid-1", "root-1")
	leaf := closeCascadeMember("leaf-1", "mid-1")
	s, _, _ := busServer(t, convTestChannel("c1", "slack"), root, mid, leaf)

	rootReq := commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "root-task", Key: commandbus.Key("root-1"),
		Payload: convtask.Payload{Verb: convtask.VerbClose, Conversation: "root-1"}.Encode(),
	}
	reqs, err := s.closeCascadeRequests(context.Background(), "root-1", "because", rootReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("want one task per conversation in the subtree (3), got %d", len(reqs))
	}
	for _, r := range reqs {
		if r.Key != commandbus.Key("root-1") {
			t.Fatalf("every cascade task must share the root's own name as Key, got %q", r.Key)
		}
	}
	names := make([]string, len(reqs))
	for i, r := range reqs {
		p, err := convtask.Decode(r.Payload)
		if err != nil {
			t.Fatal(err)
		}
		names[i] = p.Conversation
	}
	if names[len(names)-1] != "root-1" {
		t.Fatalf("the root's own request must be last, got %v", names)
	}
	pos := map[string]int{}
	for i, n := range names {
		pos[n] = i
	}
	if pos["leaf-1"] >= pos["mid-1"] {
		t.Fatalf("leaf-1 must precede its own parent mid-1, got %v", names)
	}
}

// design D-E: CloseBudgetExceeded's own invariant — every live member
// closed before the root escalates — now holds under the decomposed
// cascade: the member's CascadeMember task settles, on the root's own Key,
// strictly before the root's own close/budget-exceeded task becomes
// claimable, and THAT task (Router.CloseBudgetExceeded, unchanged) closes
// no member of its own that is not already closed before it escalates
// root-1 itself.
func TestCloseBudgetExceededClosesTheMemberBeforeEscalatingTheRoot(t *testing.T) {
	root := convTestBoundConv("root-1", "c1", "t1")
	member := closeCascadeMember("member-1", "root-1")
	s, c, bus := busServer(t, convTestChannel("c1", "slack"), root, member)

	rootRec, _, err := s.Conversations.Get(context.Background(), state.Key("root-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.closeBudgetExceeded(context.Background(), &rootRec, "budget-exceeded: maxAgents reached"); err != nil {
		t.Fatal(err)
	}

	// member-1's own CascadeMember task, first and keyed on root-1.
	first, err := bus.Claim(context.Background(), ConvQueue, "test")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := convtask.Decode(first.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !fp.CascadeMember || fp.Conversation != "member-1" {
		t.Fatalf("want member-1's own CascadeMember task first, got %+v", fp)
	}
	if err := s.HandleConvTask(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := bus.Settle(context.Background(), first.UID, first.Lease.Token); err != nil {
		t.Fatal(err)
	}
	var gotMember agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "member-1"}, &gotMember); err != nil {
		t.Fatal(err)
	}
	if gotMember.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("member-1 must be closed BEFORE root-1's own task is even claimable")
	}

	// root-1's own close/budget-exceeded task, last.
	second, err := bus.Claim(context.Background(), ConvQueue, "test")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := convtask.Decode(second.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if sp.CascadeMember || sp.Conversation != "root-1" || sp.CloseKind != convtask.CloseBudgetExceeded {
		t.Fatalf("want root-1's own close/budget-exceeded task last, got %+v", sp)
	}
	if err := s.HandleConvTask(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := bus.Settle(context.Background(), second.UID, second.Lease.Token); err != nil {
		t.Fatal(err)
	}
	var gotRoot agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &gotRoot); err != nil {
		t.Fatal(err)
	}
	// CloseBudgetExceeded's one closing path for an uncaused root is
	// Escalate, never a close of root-1 itself (design D-D/D-E).
	if gotRoot.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("root-1 itself must never be closed by budget-exceeded — only escalated")
	}
	if gotRoot.Status.EscalatedAt == nil {
		t.Fatal("root-1 must be escalated once its own task runs")
	}
}
