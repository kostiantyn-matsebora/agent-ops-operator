package httpapi

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// memberWorkReportServer is a Coordinator-rooted MEMBER conversation
// (spec.causedBy set) with one inflight run, wired with a Router so
// handleWorkDone can route its result to the parent.
func memberWorkReportServer(t *testing.T, parent *agentopsv1alpha1.Conversation, budget *agentopsv1alpha1.ConversationBudget) (*Server, *agentopsv1alpha1.Conversation) {
	t.Helper()
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", "agent-ops"
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent.Name, Entry: "worker"}
	member.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", DispatchedAt: metav1.Now()}
	member.Status.Budget = budget

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(member, parent).
		WithStatusSubresource(member, parent).Build()
	q := &chat.OpQueue{Client: c, Namespace: "agent-ops", Registry: chat.NewRegistry()}
	router := &chat.Router{Client: c, Reader: c, Namespace: "agent-ops", Ops: q}
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", Activity: activity.New(64), Router: router}, member
}

func TestHandleWorkDoneRoutesAMemberResultToItsParent(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	s, member := memberWorkReportServer(t, parent, nil)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "found the leak",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var gotParent agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &gotParent); err != nil {
		t.Fatal(err)
	}
	if len(gotParent.Spec.Inputs) != 1 || gotParent.Spec.Inputs[0].Payload != "found the leak" {
		t.Fatalf("the member's result must land as an input on the parent, got %+v", gotParent.Spec.Inputs)
	}
	if gotParent.Spec.Inputs[0].Origin == nil || gotParent.Spec.Inputs[0].Origin.Kind != agentopsv1alpha1.OriginMember ||
		gotParent.Spec.Inputs[0].Origin.Name != member.Name || gotParent.Spec.Inputs[0].Origin.Entry != "worker" {
		t.Fatalf("the input's origin must cross the causedBy edge, got %+v", gotParent.Spec.Inputs[0].Origin)
	}

	var gotMember agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &gotMember); err != nil {
		t.Fatal(err)
	}
	if len(gotMember.Status.Runs) != 1 || !gotMember.Status.Runs[0].RoutedToParent {
		t.Fatalf("the run must be marked routed once the append succeeds, got %+v", gotMember.Status.Runs)
	}
}

func TestHandleWorkDoneNeverAppendsToAClosedParent(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	parent.Status.Phase = agentopsv1alpha1.ConversationClosed
	s, member := memberWorkReportServer(t, parent, nil)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "too late",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var gotParent agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &gotParent)
	if len(gotParent.Spec.Inputs) != 0 {
		t.Fatalf("a closed parent gets nothing appended, got %+v", gotParent.Spec.Inputs)
	}
}

func TestHandleWorkDoneIncrementsTurnsOnItsOwnBudgetOnly(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	budget := &agentopsv1alpha1.ConversationBudget{MaxTurns: 5, Turns: 1}
	s, member := memberWorkReportServer(t, parent, budget)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "ok",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &got)
	if got.Status.Budget == nil || got.Status.Budget.Turns != 2 {
		t.Fatalf("turns must increment on the conversation's OWN budget, got %+v", got.Status.Budget)
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("turns below maxTurns must not close anything")
	}
}

func TestHandleWorkDoneClosesOnMaxTurnsReached(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	budget := &agentopsv1alpha1.ConversationBudget{MaxTurns: 2, Turns: 1}
	s, member := memberWorkReportServer(t, parent, budget)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "ok",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("reaching maxTurns must close this conversation via budget-exceeded, phase=%s", got.Status.Phase)
	}
}
