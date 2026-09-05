package main

import (
	"strings"
	"testing"
	"time"
)

// fakeHealth answers the verification ladder from a fixed table.
type fakeHealth map[string]verdict

func (f fakeHealth) verdict(ref objectRef) verdict {
	if v, ok := f[ref.name]; ok {
		return v
	}
	return verdictUnknown
}

type captured struct {
	source string
	sigs   []Signal
}

// testQueue builds a queue with a controllable clock and a capture sink.
func testQueue(t *testing.T, health fakeHealth, base time.Time) (*pendingQueue, *[]captured) {
	t.Helper()
	var got []captured
	q := newPendingQueue(health.verdict, func(source string, sigs []Signal) {
		got = append(got, captured{source, sigs})
	})
	q.now = func() time.Time { return base }
	return q, &got
}

// sigFor builds a normalized signal as deliver() would.
func sigFor(ns, workload, kind, name, reason string) Signal {
	e := evt("Warning", ns, kind, name, reason)
	enr := enrichment{Workload: workload}
	return normalize("src", &e, enr)
}

func ruleWithDwell(d time.Duration) compiledRule {
	return compiledRule{dwell: d, escalate: defaultEscalateAfterObjects}
}

// ---- the rollout case, both branches ----------------------------------------

// The terminating pod of a rollout: gone by the end of the window.
func TestHealthyRolloutGoneBranchEmitsNothing(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-old-xk2p9": verdictGone}, base)

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-old-xk2p9", "Unhealthy"), ruleWithDwell(3*time.Minute))
	q.Flush(base.Add(4 * time.Minute))

	if len(*got) != 0 {
		t.Fatalf("a pod that terminated during the window must emit nothing: %+v", *got)
	}
}

// The starting pod of a rollout: Ready by the end of the window.
func TestHealthyRolloutRecoveredBranchEmitsNothing(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-new-9jk2l": verdictHealthy}, base)

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-new-9jk2l", "Unhealthy"), ruleWithDwell(3*time.Minute))
	q.Flush(base.Add(4 * time.Minute))

	if len(*got) != 0 {
		t.Fatalf("a pod that became Ready during the window must emit nothing: %+v", *got)
	}
}

// A rollout that did NOT recover still fires — exactly once.
func TestBrokenRolloutEmitsExactlyOnce(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-new-9jk2l": verdictUnhealthy}, base)

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-new-9jk2l", "BackOff"), ruleWithDwell(3*time.Minute))
	q.Flush(base.Add(4 * time.Minute))
	q.Flush(base.Add(9 * time.Minute)) // a second sweep must not re-emit

	if len(*got) != 1 || len((*got)[0].sigs) != 1 {
		t.Fatalf("expected exactly one signal: %+v", *got)
	}
}

// One broken pod in a workload whose siblings recovered is still a broken
// workload — the entry survives if ANY member is unhealthy.
func TestOneBrokenMemberKeepsTheEntry(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{
		"api-a": verdictHealthy,
		"api-b": verdictGone,
		"api-c": verdictUnhealthy,
	}, base)

	for _, n := range []string{"api-a", "api-b", "api-c"} {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), compiledRule{dwell: 3 * time.Minute, escalate: 99})
	}
	q.Flush(base.Add(4 * time.Minute))

	if len(*got) != 1 {
		t.Fatalf("a workload with one still-broken pod must emit: %+v", *got)
	}
}

// ---- coalescing --------------------------------------------------------------

// The motivating case: 27 events across 3 pods become ONE signal carrying more
// than the 27 ever did.
func TestBurstCoalescesIntoOneEnrichedSignal(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{
		"api-a": verdictUnhealthy, "api-b": verdictUnhealthy, "api-c": verdictUnhealthy,
	}, base)
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99} // escalation off for this test

	for i := 0; i < 23; i++ {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", []string{"api-a", "api-b", "api-c"}[i%3], "BackOff"), rule)
	}
	for i := 0; i < 8; i++ {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", []string{"api-a", "api-b"}[i%2], "Unhealthy"), rule)
	}
	q.Flush(base.Add(4 * time.Minute))

	if len(*got) != 1 || len((*got)[0].sigs) != 1 {
		t.Fatalf("31 events must collapse to one signal: %+v", *got)
	}
	payload := (*got)[0].sigs[0].Payload
	for _, want := range []string{"BackOff", "×23", "Unhealthy", "×8", "objects (3)", "Deployment/api"} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload must carry %q:\n%s", want, payload)
		}
	}
}

// Later events must not push the deadline out, or a steadily-flapping problem
// would never be reported.
func TestWindowEndsAfterTheFirstEventNotTheLatest(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-a": verdictUnhealthy}, base)
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99}

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "BackOff"), rule)
	// two more minutes of events
	q.now = func() time.Time { return base.Add(2 * time.Minute) }
	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "BackOff"), rule)

	// 3m after the FIRST event, not the latest
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 1 {
		t.Fatalf("the window must close 3m after the first event: %+v", *got)
	}
}

// ---- escalation ---------------------------------------------------------------

func TestBreadthEscalatesBeforeTheDwellElapses(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{
		"api-a": verdictUnhealthy, "api-b": verdictUnhealthy, "api-c": verdictUnhealthy,
	}, base)
	rule := compiledRule{dwell: 10 * time.Minute, escalate: 3}

	for _, n := range []string{"api-a", "api-b", "api-c"} {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), rule)
	}
	// escalation SHORTENS the 10m dwell to 2m30s; it does not remove it
	q.Flush(base.Add(1 * time.Minute))
	if len(*got) != 0 {
		t.Fatalf("breadth must not fire instantly — a rollout also makes every replica unready: %+v", *got)
	}
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 1 {
		t.Fatalf("three affected objects must report well before the full dwell: %+v", *got)
	}
}

// The bug this constant exists to prevent, as it happened on a real cluster:
// three replicas with a 45-second start are all legitimately not-Ready at
// first, and an escalation that fired immediately opened a conversation about
// a deployment that was fine.
func TestSlowStartDoesNotFalselyEscalate(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	health := fakeHealth{"api-a": verdictUnhealthy, "api-b": verdictUnhealthy, "api-c": verdictUnhealthy}
	q, got := testQueue(t, health, base)
	rule := compiledRule{dwell: 10 * time.Minute, escalate: 3}

	// the first 45 seconds: every replica unready, warnings pouring in
	for i := 0; i < 20; i++ {
		at := base.Add(time.Duration(i*2) * time.Second)
		q.now = func() time.Time { return at }
		for _, n := range []string{"api-a", "api-b", "api-c"} {
			q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), rule)
		}
		q.Flush(at)
	}
	if len(*got) != 0 {
		t.Fatalf("a slow start must not open a conversation: %+v", *got)
	}

	// at 45s they all become Ready
	health["api-a"], health["api-b"], health["api-c"] = verdictHealthy, verdictHealthy, verdictHealthy
	q.Flush(base.Add(5 * time.Minute))

	if len(*got) != 0 {
		t.Fatalf("a workload that recovered must emit nothing: %+v", *got)
	}
}

// A rule whose dwell is itself shorter than minEscalationDwell must not let
// escalation extend the wait: the floor is clamped back down to the rule's
// own dwell rather than overriding it. Neither the floor branch nor the
// clamp-back-down branch of dueAt was reachable by the existing tests, which
// all use dwells long enough that escalation's own division stays above the
// floor.
func TestEscalationDwellIsFlooredThenClampedBackToTheRule(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-a": verdictUnhealthy, "api-b": verdictUnhealthy}, base)
	// dwell/escalationDivisor(4) = 7.5s, well under minEscalationDwell(1m) —
	// the floor applies, and then must be clamped back down since 1m now
	// exceeds this rule's own 30s dwell.
	rule := compiledRule{dwell: 30 * time.Second, escalate: 2}

	for _, n := range []string{"api-a", "api-b"} {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), rule)
	}
	q.Flush(base.Add(29 * time.Second))
	if len(*got) != 0 {
		t.Fatalf("must not fire before its own (clamped) dwell: %+v", *got)
	}
	q.Flush(base.Add(30 * time.Second))
	if len(*got) != 1 {
		t.Fatalf("must fire once its own dwell elapses: %+v", *got)
	}
}

func TestBelowEscalationThresholdStillWaits(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-a": verdictUnhealthy, "api-b": verdictUnhealthy}, base)
	rule := compiledRule{dwell: 10 * time.Minute, escalate: 3}

	for _, n := range []string{"api-a", "api-b"} {
		q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), rule)
	}
	q.Flush(base.Add(1 * time.Minute))
	if len(*got) != 0 {
		t.Fatalf("two objects is below the threshold — it must still wait: %+v", *got)
	}
}

// ---- verification ladder rung 2 -----------------------------------------------

// An uninspectable kind that went quiet: existence alone is not confirmation.
func TestUninspectableKindThatWentQuietIsDropped(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{}, base) // everything unknown

	q.Add("src", sigFor("prod", "HorizontalPodAutoscaler/api", "HorizontalPodAutoscaler", "api", "FailedGetResourceMetric"),
		ruleWithDwell(3*time.Minute))
	q.Flush(base.Add(4 * time.Minute))

	if len(*got) != 0 {
		t.Fatalf("a one-off warning on an uninspectable kind must be dropped: %+v", *got)
	}
}

// The same kind, still complaining right up to the deadline: it was still
// recurring as the window closed, so it is still happening.
func TestUninspectableKindStillRecurringAtTheCloseIsEmitted(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{}, base)
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99}

	sig := sigFor("prod", "Volume/n8n", "Volume", "n8n", "FailedSnapshotPurge")
	for _, at := range []time.Duration{0, 20 * time.Second, 60 * time.Second, 150 * time.Second, 170 * time.Second} {
		q.now = func() time.Time { return base.Add(at) }
		q.Add("src", sig, rule)
	}
	q.Flush(base.Add(3 * time.Minute))

	if len(*got) != 1 {
		t.Fatalf("a warning still recurring at the close must be emitted: %+v", *got)
	}
	payload := (*got)[0].sigs[0].Payload
	if !strings.Contains(payload, "last seen 10s before the window closed") {
		t.Fatalf("evidence must name the gap to the last arrival:\n%s", payload)
	}
}

// A burst that retried for forty seconds and then healed HAS recurred — and is
// exactly the transient rung 2 exists to drop. Longhorn's snapshot purge
// retries every few seconds while it is failing, so "did it recur" was true
// for every blip that healed in under a minute, and each one opened a
// conversation whose whole answer was "self-resolved".
func TestUninspectableKindThatHealedBeforeTheCloseIsDropped(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{}, base)
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99}

	sig := sigFor("prod", "Volume/n8n", "Volume", "n8n", "FailedSnapshotPurge")
	for i := 0; i < 6; i++ {
		at := time.Duration(i) * 8 * time.Second // six events over forty seconds
		q.now = func() time.Time { return base.Add(at) }
		q.Add("src", sig, rule)
	}
	q.Flush(base.Add(3 * time.Minute))

	if len(*got) != 0 {
		t.Fatalf("a burst that went quiet before the closing minute must be dropped: %+v", *got)
	}
}

// The closing window follows the window actually waited, floored at 30s: a
// dwell shortened to one minute believes an event from its last thirty
// seconds and not from its first.
func TestClosingWindowFollowsAShortenedDwell(t *testing.T) {
	if got := closingWindow(3 * time.Minute); got != time.Minute {
		t.Fatalf("closingWindow(3m) = %s, want 1m", got)
	}
	if got := closingWindow(time.Minute); got != 30*time.Second {
		t.Fatalf("closingWindow(1m) = %s, want the 30s floor", got)
	}
	if got := closingWindow(10 * time.Minute); got != 200*time.Second {
		t.Fatalf("closingWindow(10m) = %s, want 3m20s", got)
	}

	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{}, base)
	lax := compiledRule{dwell: 3 * time.Minute, escalate: 99}
	strict := compiledRule{dwell: time.Minute, escalate: 99}
	sig := sigFor("prod", "Volume/n8n", "Volume", "n8n", "FailedSnapshotPurge")

	q.Add("src", sig, lax)
	q.now = func() time.Time { return base.Add(20 * time.Second) }
	q.Add("src", sig, strict) // shortens the deadline to 1m20s
	q.Flush(base.Add(80 * time.Second))
	if len(*got) != 0 {
		t.Fatalf("last event 60s before a 1m20s close is outside the 30s floor, must drop: %+v", *got)
	}

	q2, got2 := testQueue(t, fakeHealth{}, base)
	q2.Add("src", sig, lax)
	q2.now = func() time.Time { return base.Add(20 * time.Second) }
	q2.Add("src", sig, strict)
	q2.now = func() time.Time { return base.Add(65 * time.Second) }
	q2.Add("src", sig, strict)
	q2.Flush(base.Add(80 * time.Second))
	if len(*got2) != 1 {
		t.Fatalf("last event 15s before the close is inside the 30s floor, must emit: %+v", *got2)
	}
}

// ---- the health predicate ------------------------------------------------------

func TestPodHealthPredicate(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("prod", "ready", "n", nil, nil, true, "Running"))
	putPod(t, c, podJSON("prod", "notready", "n", nil, nil, false, "Running"))
	putPod(t, c, podJSON("prod", "crashloop", "n", nil, nil, true, "Running", "CrashLoopBackOff"))
	putPod(t, c, podJSON("prod", "starting", "n", nil, nil, true, "Running", "ContainerCreating"))
	putPod(t, c, podJSON("prod", "failed", "n", nil, nil, false, "Failed"))
	putPod(t, c, podJSON("prod", "done", "n", nil, nil, false, "Succeeded"))
	a := adapterWithCache(c)

	cases := map[string]verdict{
		"ready":     verdictHealthy,
		"notready":  verdictUnhealthy,
		"crashloop": verdictUnhealthy,
		// a container merely being created is not a problem
		"starting": verdictHealthy,
		"failed":   verdictUnhealthy,
		"done":     verdictHealthy,
		// absent from a synced cache = gone
		"vanished": verdictGone,
	}
	for name, want := range cases {
		if got := a.health(objectRef{ns: "prod", kind: "Pod", name: name}); got != want {
			t.Errorf("%s: got verdict %d want %d", name, got, want)
		}
	}
}

// A kind with no predicate must report unknown, so rung 2 decides rather than
// rung 1 guessing. (Node used to be this example; it is now TRACKED, for
// drain awareness — health() still refuses it a predicate explicitly, see
// TestTrackedNonPodKindStillHasNoHealthPredicate.)
func TestNonPodKindHasNoHealthPredicate(t *testing.T) {
	a := adapterWithCache(syncedCache())
	if got := a.health(objectRef{ns: "prod", kind: "Job", name: "backup-27"}); got != verdictUnknown {
		t.Fatalf("a Job must be unknown to the predicate, got %d", got)
	}
}

// A Node is tracked now (drain.go), but tracked is not the same as having a
// health predicate: only Pod does. Present in the cache, a Node must still
// come back unknown rather than being read through the Pod-shaped switch
// below it — a Node's zero-value Ready/Phase fields would otherwise read as
// "unhealthy" by accident.
func TestTrackedNonPodKindStillHasNoHealthPredicate(t *testing.T) {
	c := syncedCache()
	c.put(&objectInfo{Kind: "Node", Name: "node-1"})
	a := adapterWithCache(c)
	if got := a.health(objectRef{ns: "", kind: "Node", name: "node-1"}); got != verdictUnknown {
		t.Fatalf("a Node must be unknown to the predicate, got %d", got)
	}
}

// ---- emit cap ------------------------------------------------------------------

// A non-positive configured limit (unset EMIT_PER_MINUTE, or a bad value from
// envInt's own fallback) must not disable the cap — it must fall back to the
// default rather than becoming a limit of zero or negative that clips
// everything.
func TestNewEmitCapFallsBackOnANonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -5} {
		c := newEmitCap(limit)
		allowed, clipped := c.Allow("src", make([]Signal, 3))
		if len(allowed) != 3 || clipped != 0 {
			t.Fatalf("limit=%d: expected the default cap to apply, got allowed=%d clipped=%d", limit, len(allowed), clipped)
		}
	}
}

func TestEmitCapPassesNormalVolume(t *testing.T) {
	c := newEmitCap(10)
	sigs := make([]Signal, 5)
	allowed, clipped := c.Allow("src", sigs)
	if len(allowed) != 5 || clipped != 0 {
		t.Fatalf("under the cap nothing may be clipped: allowed=%d clipped=%d", len(allowed), clipped)
	}
}

func TestEmitCapClipsAndReportsCount(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	c := newEmitCap(3)
	c.now = func() time.Time { return base }

	allowed, clipped := c.Allow("src", make([]Signal, 5))
	if len(allowed) != 3 || clipped != 2 {
		t.Fatalf("expected 3 allowed / 2 clipped, got %d/%d", len(allowed), clipped)
	}
	allowed, clipped = c.Allow("src", make([]Signal, 2))
	if len(allowed) != 0 || clipped != 4 {
		t.Fatalf("a full window must clip everything and accumulate: %d/%d", len(allowed), clipped)
	}

	// next window starts clean
	c.now = func() time.Time { return base.Add(2 * time.Minute) }
	allowed, clipped = c.Allow("src", make([]Signal, 2))
	if len(allowed) != 2 || clipped != 0 {
		t.Fatalf("a new window must start clean: %d/%d", len(allowed), clipped)
	}
}

func TestEmitCapIsPerSource(t *testing.T) {
	c := newEmitCap(2)
	if allowed, _ := c.Allow("a", make([]Signal, 2)); len(allowed) != 2 {
		t.Fatal("source a should fill its own window")
	}
	if allowed, _ := c.Allow("b", make([]Signal, 2)); len(allowed) != 2 {
		t.Fatal("source b must have its own budget")
	}
}

// ---- inhibition ------------------------------------------------------------------

func inhibitRuleSet(t *testing.T) *ruleSet {
	t.Helper()
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`reason="NodeNotReady"`},
		TargetMatchers: []string{`reason=~"Unhealthy|FailedScheduling"`},
		Equal:          []string{"node"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestNodeDownInhibitsItsOwnPods(t *testing.T) {
	rs := inhibitRuleSet(t)
	in := newInhibitor()

	cause := sigFor("", "Node/node-a", "Node", "node-a", "NodeNotReady")
	cause.Labels["node"] = "node-a"
	in.Observe("src", rs, &cause)

	victim := sigFor("prod", "Deployment/api", "Pod", "api-1", "Unhealthy")
	victim.Labels["node"] = "node-a"
	if !in.Inhibited("src", rs, &victim) {
		t.Fatal("a pod on the down node must be inhibited")
	}
}

func TestPodOnHealthyNodeIsNotInhibited(t *testing.T) {
	rs := inhibitRuleSet(t)
	in := newInhibitor()

	cause := sigFor("", "Node/node-a", "Node", "node-a", "NodeNotReady")
	cause.Labels["node"] = "node-a"
	in.Observe("src", rs, &cause)

	victim := sigFor("prod", "Deployment/api", "Pod", "api-2", "Unhealthy")
	victim.Labels["node"] = "node-b"
	if in.Inhibited("src", rs, &victim) {
		t.Fatal("a pod on a different, healthy node must not be inhibited")
	}
}

// A signal whose reason is not named by ANY inhibit rule's targetMatchers must
// never be inhibited, however active a cause is — Inhibited's loop must
// actually fall through the "target does not match" branch rather than
// stopping at the first rule.
func TestReasonNotNamedByAnyTargetIsNeverInhibited(t *testing.T) {
	rs := inhibitRuleSet(t)
	in := newInhibitor()

	cause := sigFor("", "Node/node-a", "Node", "node-a", "NodeNotReady")
	cause.Labels["node"] = "node-a"
	in.Observe("src", rs, &cause)

	unrelated := sigFor("prod", "Deployment/api", "Pod", "api-1", "OOMKilling")
	unrelated.Labels["node"] = "node-a"
	if in.Inhibited("src", rs, &unrelated) {
		t.Fatal("a reason no inhibit rule targets must never be inhibited")
	}
}

// The cause itself must still be reported, or inhibition would silence the very
// thing it exists to surface.
func TestCauseIsNotInhibitedByItself(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`reason=~"NodeNotReady"`},
		TargetMatchers: []string{`reason=~"NodeNotReady|Unhealthy"`},
		Equal:          []string{"node"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	in := newInhibitor()
	cause := sigFor("", "Node/node-a", "Node", "node-a", "NodeNotReady")
	cause.Labels["node"] = "node-a"
	in.Observe("src", rs, &cause)

	if in.Inhibited("src", rs, &cause) {
		t.Fatal("a cause must never inhibit itself")
	}
}

// A cause is observed as an EVENT and never announces that it stopped, so the
// suppression has to expire on its own.
func TestInhibitionExpires(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	rs := inhibitRuleSet(t)
	in := newInhibitor()
	in.now = func() time.Time { return base }

	cause := sigFor("", "Node/node-a", "Node", "node-a", "NodeNotReady")
	cause.Labels["node"] = "node-a"
	in.Observe("src", rs, &cause)

	victim := sigFor("prod", "Deployment/api", "Pod", "api-1", "Unhealthy")
	victim.Labels["node"] = "node-a"
	if !in.Inhibited("src", rs, &victim) {
		t.Fatal("inhibited while the cause is fresh")
	}

	in.now = func() time.Time { return base.Add(activeTTL + time.Minute) }
	if in.Inhibited("src", rs, &victim) {
		t.Fatal("inhibition must expire, or one node event silences its pods forever")
	}
}

// A persistent problem must be reported at a sane rate, not at the tick rate.
//
// Found live, not here: the entry is removed when it emits, the next event
// opens a fresh one, and escalation makes THAT one due immediately because the
// same three objects are still failing. Escalation, whose whole purpose is
// reporting an outage faster, turned a persistent outage into a stream of
// signals every few seconds.
func TestPersistentProblemDoesNotReEmitEveryTick(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{
		"api-a": verdictUnhealthy, "api-b": verdictUnhealthy, "api-c": verdictUnhealthy,
	}, base)
	rule := compiledRule{dwell: 10 * time.Minute, escalate: 3}

	add := func(at time.Time) {
		q.now = func() time.Time { return at }
		for _, n := range []string{"api-a", "api-b", "api-c"} {
			q.Add("src", sigFor("prod", "Deployment/api", "Pod", n, "Unhealthy"), rule)
		}
	}

	// the workload keeps failing, and events keep arriving every few seconds
	for i := 0; i < 40; i++ {
		at := base.Add(time.Duration(i*5) * time.Second)
		add(at)
		q.Flush(at)
	}

	// ~3 minutes of continuous failure: escalation should report it ONCE,
	// not once per tick
	if len(*got) != 1 {
		t.Fatalf("a persistent problem must be reported once per window, got %d emissions", len(*got))
	}
}

// The refractory window must expire, or a problem that recurs later goes
// unreported forever.
func TestRefractoryWindowExpires(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	q, got := testQueue(t, fakeHealth{"api-a": verdictUnhealthy}, base)
	rule := compiledRule{dwell: 2 * time.Minute, escalate: 99}

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "BackOff"), rule)
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 1 {
		t.Fatalf("first report expected: %d", len(*got))
	}

	// well past the refractory window
	later := base.Add(30 * time.Minute)
	q.now = func() time.Time { return later }
	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "BackOff"), rule)
	q.Flush(later.Add(3 * time.Minute))

	if len(*got) != 2 {
		t.Fatalf("after the refractory window a recurrence must report again, got %d", len(*got))
	}
}

// An entry that DROPPED must not start a refractory window: it reported
// nothing, so a real problem arriving right after must not be swallowed.
func TestDroppedEntryStartsNoRefractoryWindow(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	health := fakeHealth{"api-a": verdictHealthy}
	q, got := testQueue(t, health, base)
	rule := compiledRule{dwell: 2 * time.Minute, escalate: 99}

	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "Unhealthy"), rule)
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 0 {
		t.Fatalf("a recovered pod must emit nothing: %+v", *got)
	}

	// now it genuinely breaks, one second later
	health["api-a"] = verdictUnhealthy
	at := base.Add(3*time.Minute + time.Second)
	q.now = func() time.Time { return at }
	q.Add("src", sigFor("prod", "Deployment/api", "Pod", "api-a", "BackOff"), rule)
	q.Flush(at.Add(3 * time.Minute))

	if len(*got) != 1 {
		t.Fatalf("a drop must not suppress the next real problem, got %d", len(*got))
	}
}
