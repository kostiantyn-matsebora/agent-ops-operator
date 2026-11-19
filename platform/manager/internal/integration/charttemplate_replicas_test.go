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

// Demo mode is one node. A second manager would sit Pending behind its own
// required anti-affinity, and three NATS servers are one failure domain, so
// demo mode defaults both to one. An explicit value still wins.
func TestDemoModeDefaultsToOneManagerAndOneNats(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true")
	for _, want := range []string{
		"name: MANAGER_REPLICAS\n              value: \"1\"",
		"name: NATS_REPLICAS\n              value: \"1\"",
		"name: COMMANDBUS_NATS_REPLICAS\n              value: \"1\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("demo mode must render %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "requiredDuringSchedulingIgnoredDuringExecution") {
		t.Fatalf("one manager replica must carry no required anti-affinity")
	}

	out = helmTemplate(t, "--set", "global.demo.enabled=true", "--set", "replicas=2", "--set", "nats.replicas=3", "--set", "nats.storageReplicas=3")
	for _, want := range []string{
		"name: MANAGER_REPLICAS\n              value: \"2\"",
		"name: NATS_REPLICAS\n              value: \"3\"",
		"name: COMMANDBUS_NATS_REPLICAS\n              value: \"3\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("an explicit value must win under demo mode, want %q", want)
		}
	}
}
