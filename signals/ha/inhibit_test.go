package main

import (
	"testing"
	"time"
)

// inhibit.go carried NO test file at all before this one — Observe, Inhibited,
// prune and equalLabels were entirely unexercised (0%/12.5%/22.2% in the
// coverage profile). These tests close that gap against the real compiled
// ruleSet a config produces, not a hand-built one.

// The ordinary case: a cause observed on one signal suppresses a later signal
// that matches the rule's target half, PROVIDED the Equal-named labels agree.
func TestInhibitorSuppressesMatchingConsequence(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`component="hub"`},
		TargetMatchers: []string{`component="device"`},
		Equal:          []string{"house"},
	}}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	in := newInhibitor()
	cause := &Signal{Labels: map[string]string{"component": "hub", "house": "1"}}
	in.Observe("src", rs, cause)

	target := &Signal{Labels: map[string]string{"component": "device", "house": "1"}}
	if !in.Inhibited("src", rs, target) {
		t.Fatal("expected the device consequence to be inhibited by the hub cause in the same house")
	}

	// Equal binds the two: a device behind a DIFFERENT house's hub is not a
	// consequence of this cause at all.
	other := &Signal{Labels: map[string]string{"component": "device", "house": "2"}}
	if in.Inhibited("src", rs, other) {
		t.Fatal("a mismatched Equal label must not be inhibited")
	}
}

// A cause must not inhibit itself: a signal matching BOTH halves of a rule
// (source and target) would otherwise suppress the very thing being reported.
func TestInhibitorNeverInhibitsItself(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`severity="critical"`},
		TargetMatchers: []string{`severity="critical"`},
	}}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	in := newInhibitor()
	sig := &Signal{Labels: map[string]string{"severity": "critical"}}
	in.Observe("src", rs, sig)
	if in.Inhibited("src", rs, sig) {
		t.Fatal("a signal matching both halves of one rule must not suppress itself")
	}
}

// activeTTL bounds how long a cause keeps suppressing: without pruning, one
// hub error would silence its consequences forever.
func TestInhibitorPrunesExpiredCauses(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`component="hub"`},
		TargetMatchers: []string{`component="device"`},
	}}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	in := newInhibitor()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in.now = func() time.Time { return now }
	in.Observe("src", rs, &Signal{Labels: map[string]string{"component": "hub"}})

	// Still within the TTL: the cause is active.
	if !in.Inhibited("src", rs, &Signal{Labels: map[string]string{"component": "device"}}) {
		t.Fatal("expected the fresh cause to still be inhibiting")
	}

	now = now.Add(activeTTL + time.Second)
	if in.Inhibited("src", rs, &Signal{Labels: map[string]string{"component": "device"}}) {
		t.Fatal("a cause older than activeTTL must be pruned and no longer inhibit anything")
	}
}

// No inhibit rules configured (the common case) must be a complete no-op on
// both a nil and an empty ruleSet — neither Observe nor Inhibited may panic or
// suppress anything.
func TestInhibitorNoRulesIsNoOp(t *testing.T) {
	in := newInhibitor()
	rs, err := compileRules(nil, Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sig := &Signal{Labels: map[string]string{"x": "y"}}

	in.Observe("src", rs, sig)
	if in.Inhibited("src", rs, sig) {
		t.Fatal("a ruleSet with no inhibit rules must never inhibit anything")
	}
	in.Observe("src", nil, sig)
	if in.Inhibited("src", nil, sig) {
		t.Fatal("a nil ruleSet must never inhibit anything")
	}
}

// equalLabels is the primitive Inhibited delegates to: an empty Equal list
// binds globally (Alertmanager's own behavior), a named one requires the
// value to agree on both sides.
func TestEqualLabelsGlobalWhenEmpty(t *testing.T) {
	if !equalLabels(nil, map[string]string{"a": "1"}, map[string]string{"a": "2"}) {
		t.Fatal("an empty Equal list means the cause inhibits its targets globally")
	}
	if equalLabels([]string{"a"}, map[string]string{"a": "1"}, map[string]string{"a": "2"}) {
		t.Fatal("a named Equal label with differing values must not match")
	}
	if !equalLabels([]string{"a"}, map[string]string{"a": "1"}, map[string]string{"a": "1"}) {
		t.Fatal("a named Equal label with matching values must match")
	}
}
