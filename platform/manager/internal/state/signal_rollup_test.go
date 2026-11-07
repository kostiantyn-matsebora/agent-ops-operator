package state

import "testing"

// expectedRollup is the phase a pair of claimant states must roll up to.
func expectedRollup(a, b string) string {
	var routing, routed bool
	for _, s := range []string{a, b} {
		routing = routing || s == ClaimantRouting
		routed = routed || s == ClaimantRouted
	}
	switch {
	case routing && routed:
		return SignalPartiallyRouted
	case routing:
		return SignalPending
	case routed:
		return SignalRouted
	}
	return SignalFailed
}

// SignalRollupPhase walks every pair of claimant states, as the API package's
// own RollupPhase test does for the type it mirrors.
func TestSignalRollupPhaseWalksEveryClaimantStateCombination(t *testing.T) {
	states := []string{ClaimantRouting, ClaimantRouted, ClaimantFailed, ClaimantSkipped}
	for _, a := range states {
		for _, b := range states {
			want := expectedRollup(a, b)
			got := SignalRollupPhase([]SignalClaimant{{Name: "a", State: a}, {Name: "b", State: b}})
			if got != want {
				t.Errorf("(%s,%s): phase %s, want %s", a, b, got, want)
			}
		}
	}
	if got := SignalRollupPhase(nil); got != SignalPending {
		t.Errorf("no claimants: phase %s, want Pending", got)
	}
}
