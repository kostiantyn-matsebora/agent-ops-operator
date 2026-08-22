package main

import "testing"

// The naming convention is MCP's, not ours: mcp__<server>__<tool>. A trailing
// star grants every tool a server registers, which is how a toolset says "all
// of this server" without listing twenty names.
func TestPatternMatching(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"mcp__kubernetes__pods_list", "mcp__kubernetes__pods_list", true},
		{"mcp__kubernetes__pods_list", "mcp__kubernetes__pods_delete", false},
		{"mcp__kubernetes__*", "mcp__kubernetes__pods_exec", true},
		{"mcp__kubernetes__*", "mcp__homeassistant__get_state", false},
		{"mcp__kubernetes__pods_*", "mcp__kubernetes__pods_top", true},
		{"mcp__kubernetes__pods_*", "mcp__kubernetes__nodes_top", false},
		{"*", "anything", true},
		{"", "mcp__kubernetes__pods_list", false},
		{"Bash", "Bash", true},
		{"Bash", "BashOutput", false},
	}
	for _, c := range cases {
		if got := matchTool(c.pattern, c.name); got != c.want {
			t.Errorf("matchTool(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestQualifyComposesTheWiringName(t *testing.T) {
	if got := qualify("kubernetes", "pods_exec"); got != "mcp__kubernetes__pods_exec" {
		t.Fatalf("qualify = %q", got)
	}
	// A server key we never learned must not silently become a bare tool name
	// that a broad pattern might match.
	if got := qualify("", "pods_exec"); got != "pods_exec" {
		t.Fatalf("qualify with no key = %q", got)
	}
}

// Task 2.8 — an agent that has been dispatched no work has been granted
// nothing. "We do not know yet" and "it is allowed" must not be the same answer.
func TestNothingIsGrantedBeforeTheFirstWorkUnit(t *testing.T) {
	p := newPolicy()
	if p.ready() {
		t.Fatal("a fresh policy must not claim to know anything")
	}
	if p.permits("mcp__kubernetes__pods_list") {
		t.Fatal("nothing may be permitted before a work unit is seen")
	}
}

// Task 2.7 — a toolset edited mid-conversation takes effect on the next unit.
func TestPolicyFollowsTheLatestWorkUnit(t *testing.T) {
	p := newPolicy()
	p.set([]string{"mcp__kubernetes__pods_list"})
	if !p.permits("mcp__kubernetes__pods_list") {
		t.Fatal("the granted tool must be permitted")
	}
	if p.permits("mcp__kubernetes__pods_delete") {
		t.Fatal("an ungranted tool must not be permitted")
	}

	p.set([]string{"mcp__kubernetes__*"})
	if !p.permits("mcp__kubernetes__pods_delete") {
		t.Fatal("a widened binding must apply from the next unit, with no restart")
	}

	// Narrowing must apply just as promptly — a revoked grant that lingers is
	// the failure that matters.
	p.set([]string{"mcp__kubernetes__pods_list"})
	if p.permits("mcp__kubernetes__pods_delete") {
		t.Fatal("a narrowed binding must revoke immediately")
	}
}

// An empty allowlist is a real answer: the wiring granted nothing. It must not
// be read as "unset, so allow".
func TestAnEmptyGrantIsStillAGrant(t *testing.T) {
	p := newPolicy()
	p.set(nil)
	if !p.ready() {
		t.Fatal("an empty allowlist is a decision that was made")
	}
	if p.permits("mcp__kubernetes__pods_list") {
		t.Fatal("an empty allowlist permits nothing")
	}
}
