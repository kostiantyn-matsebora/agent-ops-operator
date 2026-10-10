package integration

import (
	"strings"
	"testing"
)

// durable-chat-ops-broker: replicas is a value again, not hardcoded, and its
// new default is above 1 — the whole point of the resilience this release
// adds (a durable claim on the Conversation CR, a non-leader rejecting a
// poll with 503) is that a second pod is there to take over. Checked against
// a value no other template in the chart renders by default, so a false
// positive from an unrelated Deployment is not possible.
func TestManagerReplicasDefaultsAboveOneAndIsOverridable(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "replicas: 2") {
		t.Fatalf("default replicas must be 2:\n%s", out)
	}

	out = helmTemplate(t, "--set", "replicas=7")
	if !strings.Contains(out, "replicas: 7") {
		t.Fatalf("replicas must be overridable: %s", out)
	}
}

// The claim staleness bound is a chart value (durable-chat-ops-broker),
// never a compiled-in constant, so an install with a slower transport can
// raise it.
func TestClaimStalenessSecondsRendersAndIsOverridable(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "name: CLAIM_STALENESS_SECONDS") || !strings.Contains(out, `value: "90"`) {
		t.Fatalf("default claimStalenessSeconds (90) must render:\n%s", out)
	}

	out = helmTemplate(t, "--set", "claimStalenessSeconds=180")
	if !strings.Contains(out, `value: "180"`) {
		t.Fatalf("claimStalenessSeconds must be overridable: %s", out)
	}
}
