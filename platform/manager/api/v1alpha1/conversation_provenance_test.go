package v1alpha1

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A conversation invoked as a coordinated member carries CausedBy naming its
// immediate parent and the entry it was invoked through, independent of
// CoordinatorRef (which marks this same conversation as itself a
// Coordinator's own root, when it is one).

func TestConversationCausedByRoundTripsThroughJSON(t *testing.T) {
	c := &Conversation{
		Spec: ConversationSpec{
			CausedBy:       &Provenance{Parent: "root-conv", Entry: "worker-a"},
			CoordinatorRef: &ObjectRef{Name: "triage"},
		},
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Conversation
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Spec.CausedBy == nil || got.Spec.CausedBy.Parent != "root-conv" || got.Spec.CausedBy.Entry != "worker-a" {
		t.Fatalf("CausedBy = %+v, want {root-conv worker-a}", got.Spec.CausedBy)
	}
	if got.Spec.CoordinatorRef == nil || got.Spec.CoordinatorRef.Name != "triage" {
		t.Fatalf("CoordinatorRef = %v, want triage", got.Spec.CoordinatorRef)
	}
}

func TestConversationCausedByAbsentOnAnOrdinaryConversation(t *testing.T) {
	c := &Conversation{}
	if c.Spec.CausedBy != nil {
		t.Fatalf("CausedBy = %v, want nil on a conversation nothing invoked", c.Spec.CausedBy)
	}
	if c.Spec.CoordinatorRef != nil {
		t.Fatalf("CoordinatorRef = %v, want nil on a conversation no Coordinator roots", c.Spec.CoordinatorRef)
	}
}

// A Coordinator-rooted conversation's budget is a snapshot plus running
// counts, absent on every other conversation.

func TestConversationBudgetRoundTripsThroughJSON(t *testing.T) {
	deadline := metav1.NewTime(time.Now().Truncate(time.Second))
	c := &Conversation{
		Status: ConversationStatus{
			Budget: &ConversationBudget{
				MaxAgents:     10,
				MaxTurns:      20,
				Deadline:      &deadline,
				AgentsInvoked: 3,
				Turns:         5,
			},
		},
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Conversation
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	b := got.Status.Budget
	if b == nil {
		t.Fatalf("Budget = nil, want a snapshot")
	}
	if b.MaxAgents != 10 || b.MaxTurns != 20 || b.AgentsInvoked != 3 || b.Turns != 5 {
		t.Fatalf("Budget = %+v, want {MaxAgents:10 MaxTurns:20 AgentsInvoked:3 Turns:5}", b)
	}
	if b.Deadline == nil || !b.Deadline.Time.Equal(deadline.Time) {
		t.Fatalf("Deadline = %v, want %v", b.Deadline, deadline)
	}
}

func TestConversationEscalatedAtCloseReasonAndBriefRoundTripThroughJSON(t *testing.T) {
	at := metav1.NewTime(time.Now().Truncate(time.Second))
	c := &Conversation{
		Status: ConversationStatus{
			EscalatedAt: &at,
			CloseReason: "budget-exceeded",
			Brief:       "Investigating a disk-pressure alert on node-3.",
		},
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Conversation
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Status.EscalatedAt == nil || !got.Status.EscalatedAt.Time.Equal(at.Time) {
		t.Fatalf("EscalatedAt = %v, want %v", got.Status.EscalatedAt, at)
	}
	if got.Status.CloseReason != "budget-exceeded" {
		t.Fatalf("CloseReason = %q, want budget-exceeded", got.Status.CloseReason)
	}
	if got.Status.Brief != "Investigating a disk-pressure alert on node-3." {
		t.Fatalf("Brief = %q, want the stored sentence", got.Status.Brief)
	}
}

func TestConversationStatusFieldsAbsentOnAnOrdinaryConversation(t *testing.T) {
	c := &Conversation{}
	if c.Status.Budget != nil {
		t.Fatalf("Budget = %v, want nil on a conversation no Coordinator roots", c.Status.Budget)
	}
	if c.Status.EscalatedAt != nil {
		t.Fatalf("EscalatedAt = %v, want nil, nothing escalated it", c.Status.EscalatedAt)
	}
	if c.Status.CloseReason != "" {
		t.Fatalf("CloseReason = %q, want empty, nothing closed it", c.Status.CloseReason)
	}
	if c.Status.Brief != "" {
		t.Fatalf("Brief = %q, want empty, no runtime reported one", c.Status.Brief)
	}
}
