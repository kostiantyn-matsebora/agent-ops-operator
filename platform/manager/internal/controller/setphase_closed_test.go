package controller

import (
	"context"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A reconcile pass decides its phase from the record it read at the start. A
// close that lands during the pass must survive it: Closed is sticky until an
// explicit reopen, and the pass writing Idle over it left the conversation
// open with its close time still stamped (TestConsoleLifecycle/close_then_delete).
func TestSetPhaseNeverReopensAConversationClosedDuringThePass(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-1")).Build()
	store := claimStore(c)
	r := &ConversationReconciler{Store: store}
	ctx := context.Background()

	stale := claimRec(t, store, "c-1") // what the pass read: not closed yet
	if _, err := state.Mutate(ctx, store, "c-1", func(rec *state.ConversationRecord) error {
		rec.Phase = state.PhaseClosed
		return nil
	}); err != nil {
		t.Fatalf("close lands during the pass: %v", err)
	}

	if err := r.setPhase(ctx, stale, state.PhaseIdle); err != nil {
		t.Fatalf("setPhase: %v", err)
	}
	if got := claimRec(t, store, "c-1").Phase; got != state.PhaseClosed {
		t.Fatalf("phase = %q after the pass, want %q kept", got, state.PhaseClosed)
	}
}

// The ordinary case keeps working: an open conversation moves between open phases.
func TestSetPhaseMovesAnOpenConversation(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-2")).Build()
	store := claimStore(c)
	r := &ConversationReconciler{Store: store}

	rec := claimRec(t, store, "c-2")
	if err := r.setPhase(context.Background(), rec, state.PhaseQueued); err != nil {
		t.Fatalf("setPhase: %v", err)
	}
	if got := claimRec(t, store, "c-2").Phase; got != state.PhaseQueued {
		t.Fatalf("phase = %q, want %q", got, state.PhaseQueued)
	}
}

