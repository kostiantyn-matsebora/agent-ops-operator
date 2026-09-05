package main

import (
	"testing"
	"time"
)

// The condition-taint set must match platform/manager/internal/controller's
// EXACTLY — see drain.go. Pinned literally on both sides so a key added to
// one without the other fails the test that would have caught the drift,
// rather than the incident it would eventually cause.
func TestConditionTaintSet(t *testing.T) {
	want := map[string]bool{
		"node.kubernetes.io/not-ready":           true,
		"node.kubernetes.io/unreachable":         true,
		"node.kubernetes.io/memory-pressure":     true,
		"node.kubernetes.io/disk-pressure":       true,
		"node.kubernetes.io/pid-pressure":        true,
		"node.kubernetes.io/network-unavailable": true,
		"node.kubernetes.io/out-of-service":      true,
	}
	if len(conditionTaints) != len(want) {
		t.Fatalf("condition taint set changed size: got %d want %d (%v)", len(conditionTaints), len(want), conditionTaints)
	}
	for k := range want {
		if !conditionTaints[k] {
			t.Errorf("condition taint set is missing %q", k)
		}
	}
}

func TestNodeDraining(t *testing.T) {
	cases := []struct {
		name          string
		unschedulable bool
		taints        []nodeTaint
		want          bool
	}{
		{"a healthy node is not draining", false, nil, false},
		{"cordon sets the flag", true, nil, true},
		{"cordon's own taint counts even without the flag", false,
			[]nodeTaint{{Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule"}}, true},
		{"a maintenance taint counts", false,
			[]nodeTaint{{Key: "weave.works/kured", Effect: "NoSchedule"}}, true},
		{"NotReady is a condition, not a drain", false,
			[]nodeTaint{{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}}, false},
		{"unreachable is a condition, not a drain", false,
			[]nodeTaint{{Key: "node.kubernetes.io/unreachable", Effect: "NoExecute"}}, false},
		{"disk pressure is a condition, not a drain", false,
			[]nodeTaint{{Key: "node.kubernetes.io/disk-pressure", Effect: "NoSchedule"}}, false},
		{"a PreferNoSchedule taint is a hint, not a drain", false,
			[]nodeTaint{{Key: "example.com/soft", Effect: "PreferNoSchedule"}}, false},
		{"an unwell node that is ALSO cordoned is draining", true,
			[]nodeTaint{{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeDraining(tc.unschedulable, tc.taints); got != tc.want {
				t.Fatalf("nodeDraining = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- drainTracker -----------------------------------------------------------

func mustRules(t *testing.T, route Route) *ruleSet {
	t.Helper()
	rs, err := compileRules(nil, route)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func draining(t *testing.T, node string) *objectCache {
	t.Helper()
	c := syncedCache()
	c.put(&objectInfo{Kind: "Node", Name: node, Unschedulable: true})
	return c
}

func TestDrainTrackerSuppressesEventsOnADrainingNode(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	if !dt.Suppress("src", "node-a", rs, c, sig, time.Now()) {
		t.Fatal("an event on a draining node must be suppressed by default")
	}
}

func TestDrainTrackerDoesNotSuppressOnANonDrainingNode(t *testing.T) {
	rs := mustRules(t, Route{})
	c := syncedCache()
	c.put(&objectInfo{Kind: "Node", Name: "node-a"})
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	if dt.Suppress("src", "node-a", rs, c, sig, time.Now()) {
		t.Fatal("an event on an ordinary node must not be suppressed")
	}
}

// An event whose node the cache cannot confirm — unsynced, or never granted —
// must fail toward REPORTING, never toward suppression.
func TestDrainTrackerDoesNotSuppressAnUnknownNode(t *testing.T) {
	rs := mustRules(t, Route{})
	c := newObjectCache() // unsynced
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	if dt.Suppress("src", "node-a", rs, c, sig, time.Now()) {
		t.Fatal("an unconfirmable node must not be suppressed")
	}
}

func TestDrainTrackerEmptyNodeIsNeverSuppressed(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	if dt.Suppress("src", "", rs, c, sig, time.Now()) {
		t.Fatal("an event resolving to no node at all must not be suppressed")
	}
}

// Opt-out per source: route.drainingNodes: report disables the axis entirely.
func TestDrainTrackerReportModeDoesNotSuppress(t *testing.T) {
	rs := mustRules(t, Route{DrainingNodes: "report"})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	if dt.Suppress("src", "node-a", rs, c, sig, time.Now()) {
		t.Fatal("report mode must evaluate events on a draining node exactly as before")
	}
}

func TestDrainTrackerInvalidModeIsRejected(t *testing.T) {
	if _, err := compileRules(nil, Route{DrainingNodes: "silence"}); err == nil {
		t.Fatal("an unknown route.drainingNodes value must be rejected")
	}
}

// A matcher narrows what a draining node suppresses; the rest of that node's
// events still reach the ordinary rules.
func TestDrainTrackerMatchersNarrowSuppression(t *testing.T) {
	rs := mustRules(t, Route{DrainingNodeMatchers: []string{`reason=~"NodeNotReady"`}})
	c := draining(t, "node-a")
	dt := newDrainTracker()

	matching := &Signal{Labels: map[string]string{"alertname": "NodeNotReady"}}
	if !dt.Suppress("src", "node-a", rs, c, matching, time.Now()) {
		t.Fatal("a matching reason on a draining node must be suppressed")
	}
	other := &Signal{Labels: map[string]string{"alertname": "BackOff"}}
	if dt.Suppress("src", "node-a", rs, c, other, time.Now()) {
		t.Fatal("a non-matching reason must not be suppressed by the matcher")
	}
}

func TestDrainTrackerBadMatcherIsRejected(t *testing.T) {
	if _, err := compileRules(nil, Route{DrainingNodeMatchers: []string{"not a matcher"}}); err == nil {
		t.Fatal("an invalid drainingNodeMatchers entry must be rejected")
	}
}

func TestDrainTrackerBoundParsesAndRejectsZero(t *testing.T) {
	rs := mustRules(t, Route{DrainingNodeBound: "30m"})
	if rs.drainBound != 30*time.Minute {
		t.Fatalf("drainBound = %v, want 30m", rs.drainBound)
	}
	if _, err := compileRules(nil, Route{DrainingNodeBound: "0s"}); err == nil {
		t.Fatal("a zero bound must be rejected — it would never let anything suppress")
	}
	if _, err := compileRules(nil, Route{DrainingNodeBound: "soon"}); err == nil {
		t.Fatal("an unparsable bound must be rejected")
	}
}

func TestDrainTrackerBoundDefaults(t *testing.T) {
	rs := mustRules(t, Route{})
	if rs.drainBound != defaultDrainBound {
		t.Fatalf("drainBound = %v, want the default %v", rs.drainBound, defaultDrainBound)
	}
}

// ---- sweep --------------------------------------------------------------

func TestDrainTrackerSweepEscalatesExactlyOnceAcrossRepeatedTicks(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	start := time.Now()
	dt.Suppress("src", "node-a", rs, c, sig, start)

	// Before the bound: no escalation.
	if r := dt.Sweep("src", c, time.Hour, start.Add(30*time.Minute)); len(r) != 0 {
		t.Fatalf("must not escalate before the bound: %+v", r)
	}
	// Past the bound: escalate exactly once.
	r := dt.Sweep("src", c, time.Hour, start.Add(90*time.Minute))
	if len(r) != 1 || !r[0].exceeded || r[0].node != "node-a" {
		t.Fatalf("expected exactly one exceeded result for node-a, got %+v", r)
	}
	// A later tick, still draining, must not re-escalate.
	if r := dt.Sweep("src", c, time.Hour, start.Add(120*time.Minute)); len(r) != 0 {
		t.Fatalf("must not escalate twice for the same drain: %+v", r)
	}
	// And once exceeded, its events are no longer suppressed.
	if dt.Suppress("src", "node-a", rs, c, sig, start.Add(120*time.Minute)) {
		t.Fatal("an exceeded drain must report its node's events normally again")
	}
}

func TestDrainTrackerSweepReleasesAndReArms(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	start := time.Now()
	dt.Suppress("src", "node-a", rs, c, sig, start)
	dt.Suppress("src", "node-a", rs, c, sig, start)

	// The node stops draining.
	c.put(&objectInfo{Kind: "Node", Name: "node-a"})
	r := dt.Sweep("src", c, time.Hour, start.Add(time.Minute))
	if len(r) != 1 || !r[0].released || r[0].suppressed != 2 {
		t.Fatalf("expected one released result carrying the suppressed count, got %+v", r)
	}
	if nodes, _ := dt.Active("src"); len(nodes) != 0 {
		t.Fatalf("a released node must drop out of Active: %v", nodes)
	}

	// It drains again: a fresh episode, not a continuation of the old one.
	c.put(&objectInfo{Kind: "Node", Name: "node-a", Unschedulable: true})
	if !dt.Suppress("src", "node-a", rs, c, sig, start.Add(2*time.Minute)) {
		t.Fatal("a re-drained node must suppress again")
	}
	nodes, total := dt.Active("src")
	if len(nodes) != 1 || total != 1 {
		t.Fatalf("expected a fresh single-event episode, got nodes=%v total=%d", nodes, total)
	}
}

func TestDrainTrackerActiveExcludesExceededNodes(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	start := time.Now()
	dt.Suppress("src", "node-a", rs, c, sig, start)
	dt.Sweep("src", c, time.Hour, start.Add(90*time.Minute))

	if nodes, total := dt.Active("src"); len(nodes) != 0 || total != 0 {
		t.Fatalf("an exceeded node must not appear as actively suppressing: nodes=%v total=%d", nodes, total)
	}
}

// Two sources tracking the same node must not share counts: each configures
// the axis independently.
func TestDrainTrackerIsPerSource(t *testing.T) {
	rs := mustRules(t, Route{})
	c := draining(t, "node-a")
	dt := newDrainTracker()
	sig := &Signal{Labels: map[string]string{}}
	dt.Suppress("a", "node-a", rs, c, sig, time.Now())
	dt.Suppress("a", "node-a", rs, c, sig, time.Now())
	dt.Suppress("b", "node-a", rs, c, sig, time.Now())

	if _, total := dt.Active("a"); total != 2 {
		t.Fatalf("source a: total = %d, want 2", total)
	}
	if _, total := dt.Active("b"); total != 1 {
		t.Fatalf("source b: total = %d, want 1", total)
	}
}
