package state

import "testing"

// SignalRollupPhase walks every pair of claimant states, as the API package's
// own RollupPhase test does for the type it mirrors.
func TestSignalRollupPhaseWalksEveryClaimantStateCombination(t *testing.T) {
	states := []string{ClaimantRouting, ClaimantRouted, ClaimantFailed, ClaimantSkipped}
	for _, a := range states {
		for _, b := range states {
			var routing, routed bool
			for _, s := range []string{a, b} {
				routing = routing || s == ClaimantRouting
				routed = routed || s == ClaimantRouted
			}
			want := SignalFailed
			switch {
			case routing && routed:
				want = SignalPartiallyRouted
			case routing:
				want = SignalPending
			case routed:
				want = SignalRouted
			}
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
