package httpapi

import (
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestJudgeRootTurnLeavesAnEscalatedRootAlone(t *testing.T) {
	conv := &state.ConversationRecord{CoordinatorRef: "co-a"}
	ev := turnEvidence{Dispatched: true}

	if got := judgeRootTurn(conv, "succeeded", ev, nil); got != turnFirstMiss {
		t.Fatalf("an open root's turn without a move is handed back once, got %v", got)
	}
	now := time.Now()
	conv.EscalatedAt = &now
	if got := judgeRootTurn(conv, "succeeded", ev, nil); got != turnEnded {
		t.Fatalf("an escalated root waits on a person and is never handed back, got %v", got)
	}
}
