package main

import "testing"

// HOME_PVC IS GONE, NOT DUAL-READ, and this pins the difference.
//
// The sessionId precedent does not apply and reading it as though it did is the
// trap: that was a field RENAMED IN PLACE, so an alias pointed at something
// real. The concept behind HOME_PVC moved to a DIFFERENT CR — persistence is
// the Pipeline's now — so an alias would resolve to a field that is not there,
// and the only honest behaviour is to ignore it loudly. The chart and the
// manager ship together in this release, so there is no window to cover.
func TestTheRetiredBootstrapSpellingIsNotRead(t *testing.T) {
	t.Setenv("CONTEXT_PVC", "")
	t.Setenv("HOME_PVC", "agentops-home")

	if got := bootstrapContextClaim(); got != "" {
		t.Fatalf("HOME_PVC was honoured (%q). It is retired, not aliased: an install still setting it "+
			"must get an ephemeral volume and read the CHANGELOG, not a quiet resolution to a "+
			"concept that lives on another object now", got)
	}

	t.Setenv("CONTEXT_PVC", "agentops-context")
	if got := bootstrapContextClaim(); got != "agentops-context" {
		t.Fatalf("CONTEXT_PVC = %q, want the claim it names", got)
	}
}
