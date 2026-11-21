package integration

import (
	"strings"
	"testing"
)

// Chart-render assertions for separate-member-slot-pool: the conversations
// and Coordinator-members pools are two independently-capped values, each
// its own env var, and the ceiling alert reads the metric's `pool` label
// rather than comparing one series to itself.
//
// Helpers (helmTemplate, envValue) live in charttemplate_test.go and
// charttemplate_signals_test.go, same package.

func TestCapacityPoolsRenderWithDefaults(t *testing.T) {
	out := helmTemplate(t)
	for name, want := range map[string]string{
		"MAX_ACTIVE_CONVERSATIONS": "5",
		"MAX_ACTIVE_MEMBERS":       "5",
	} {
		if got := envValue(t, out, name); got != want {
			t.Fatalf("%s defaults to %q, want %q", name, got, want)
		}
	}
}

func TestCapacityPoolsCanBeOverriddenIndependently(t *testing.T) {
	out := helmTemplate(t,
		"--set", "maxActiveConversations=3",
		"--set", "maxActiveMembers=7")
	if got := envValue(t, out, "MAX_ACTIVE_CONVERSATIONS"); got != "3" {
		t.Fatalf("MAX_ACTIVE_CONVERSATIONS = %q, want 3", got)
	}
	if got := envValue(t, out, "MAX_ACTIVE_MEMBERS"); got != "7" {
		t.Fatalf("MAX_ACTIVE_MEMBERS = %q, want 7", got)
	}
}

// The ceiling alert used to compare one unlabelled series to itself. Now the
// gauges carry a `pool` label, so the rule must read it on both sides of
// each comparison or a members-pool ceiling would silently vanish into the
// conversations-only series the old expression named.
func TestRuntimeCeilingAlertReadsThePoolLabel(t *testing.T) {
	out := helmTemplate(t, "--set", "metrics.rules.enabled=true")
	for _, needle := range []string{
		`agentops_runtime_slots_in_use{pool="conversations"} >= agentops_runtime_slots_max{pool="conversations"}`,
		`agentops_runtime_slots_in_use{pool="members"} >= agentops_runtime_slots_max{pool="members"}`,
		"alert: AgentOpsConversationsAtRuntimeCeiling",
		"alert: AgentOpsMembersAtRuntimeCeiling",
		"maxActiveConversations or shed load",
		"maxActiveMembers or shed load",
	} {
		if !strings.Contains(out, needle) {
			t.Errorf("the rule must contain %q:\n%s", needle, out)
		}
	}
	// The old unlabelled-comparison alert is retired, not kept beside the new
	// two — a sum across pools would mislead (design D-6).
	if strings.Contains(out, "AgentOpsAtRuntimeCeiling\n") {
		t.Errorf("the retired single-pool alert must not render:\n%s", out)
	}
}
