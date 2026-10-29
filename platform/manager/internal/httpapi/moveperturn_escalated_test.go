package httpapi

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestJudgeRootTurnLeavesAnEscalatedRootAlone(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{}
	conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	ev := turnEvidence{Dispatched: true}

	if got := judgeRootTurn(conv, "succeeded", ev, nil); got != turnFirstMiss {
		t.Fatalf("an open root's turn without a move is handed back once, got %v", got)
	}
	now := metav1.Now()
	conv.Status.EscalatedAt = &now
	if got := judgeRootTurn(conv, "succeeded", ev, nil); got != turnEnded {
		t.Fatalf("an escalated root waits on a person and is never handed back, got %v", got)
	}
}
