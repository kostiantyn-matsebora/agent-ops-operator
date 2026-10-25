package v1alpha1

import "testing"

// RollupPhase is the one function both ingest's inline first routing attempt
// and the reconciler's retries call, so the two can never disagree about what
// a claimant set means (design D-4). These pin the shape that matters most:
// no claimant yet (Pending), a mix still in flight (Pending/PartiallyRouted
// depending on whether one has already routed), and every claimant terminal
// (Routed/Failed). The full claimant-state-combination table test belongs with
// the reconciler that drives it (tasks.md 3.2) — this is the type-level
// sanity check for the function this change introduces.

func TestRollupPhaseNoClaimantsIsPending(t *testing.T) {
	if got := RollupPhase(nil); got != SignalPending {
		t.Fatalf("RollupPhase(nil) = %q, want Pending", got)
	}
	if got := RollupPhase([]SignalClaimant{}); got != SignalPending {
		t.Fatalf("RollupPhase(empty) = %q, want Pending", got)
	}
}

func TestRollupPhaseAllRoutingIsPending(t *testing.T) {
	claimants := []SignalClaimant{
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "a"}, State: ClaimantRouting},
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "b"}, State: ClaimantRouting},
	}
	if got := RollupPhase(claimants); got != SignalPending {
		t.Fatalf("RollupPhase(all Routing) = %q, want Pending", got)
	}
}

func TestRollupPhaseSomeRoutedSomeStillRoutingIsPartiallyRouted(t *testing.T) {
	claimants := []SignalClaimant{
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "a"}, State: ClaimantRouted,
			ConversationRef: &ObjectRef{Name: "conv-a"}},
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "b"}, State: ClaimantRouting},
	}
	if got := RollupPhase(claimants); got != SignalPartiallyRouted {
		t.Fatalf("RollupPhase(mixed Routed/Routing) = %q, want PartiallyRouted", got)
	}
}

func TestRollupPhaseAllTerminalWithOneRoutedIsRouted(t *testing.T) {
	// Design D-5a: retry exhaustion with at least one routed claimant rolls
	// up to Routed, even though the other claimant is visibly Failed.
	claimants := []SignalClaimant{
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "a"}, State: ClaimantRouted,
			ConversationRef: &ObjectRef{Name: "conv-a"}},
		{Ref: ClaimantRef{Kind: ClaimantKindCoordinator, Name: "b"}, State: ClaimantFailed,
			Reason: "retry deadline exceeded"},
	}
	if got := RollupPhase(claimants); got != SignalRouted {
		t.Fatalf("RollupPhase(terminal, one Routed) = %q, want Routed", got)
	}
}

func TestRollupPhaseAllTerminalNoneRoutedIsFailed(t *testing.T) {
	claimants := []SignalClaimant{
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "a"}, State: ClaimantFailed,
			Reason: "retry deadline exceeded"},
		{Ref: ClaimantRef{Kind: ClaimantKindPipeline, Name: "b"}, State: ClaimantSkipped,
			Reason: "capacity"},
	}
	if got := RollupPhase(claimants); got != SignalFailed {
		t.Fatalf("RollupPhase(terminal, none Routed) = %q, want Failed", got)
	}
}
