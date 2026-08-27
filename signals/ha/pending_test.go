package main

import (
	"strings"
	"testing"
	"time"
)

type emitted struct {
	source string
	sigs   []Signal
}

// queueFixture wires a pending queue to a scripted verdict and a recorder.
func queueFixture(v func(recordRef, time.Time) verdict) (*pendingQueue, *[]emitted, *time.Time) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	var got []emitted
	q := newPendingQueue(v, func(source string, sigs []Signal) {
		got = append(got, emitted{source, sigs})
	})
	q.now = func() time.Time { return now }
	return q, &got, &now
}

func haSignal(integration, location, level string) Signal {
	return Signal{
		Fingerprint: "ha-logs@" + integration + "@" + location,
		Title:       integration + ": something failed",
		Labels: map[string]string{
			"integration": integration,
			"logger":      "homeassistant.components." + integration,
			"location":    location,
			"level":       level,
		},
	}
}

// The dwell exists to turn a point-in-time fact into a state claim. Still
// broken at the end of the window -> emit, with the burst attached.
func TestDwellEmitsWhenStillUnhealthy(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnhealthy })
	q.Add("ha-logs", haSignal("zwave_js", "climate.py:412", "ERROR"), compiledRule{dwell: 5 * time.Minute, escalate: 3}, 1)

	q.Flush(now.Add(time.Minute))
	if len(*got) != 0 {
		t.Fatal("nothing may emit before the window closes")
	}
	q.Flush(now.Add(6 * time.Minute))
	if len(*got) != 1 || len((*got)[0].sigs) != 1 {
		t.Fatalf("expected one signal after the window, got %+v", *got)
	}
	sig := (*got)[0].sigs[0]
	if !strings.Contains(sig.Payload, "failing for") || !strings.Contains(sig.Payload, "ERROR") {
		t.Fatalf("evidence missing from payload:\n%s", sig.Payload)
	}
}

// Recovered during the window -> the incident erased itself, which is the whole
// reason a dwell re-checks instead of merely waiting.
func TestDwellDropsWhenRecovered(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictHealthy })
	q.Add("ha-logs", haSignal("hue", "light.py:88", "ERROR"), compiledRule{dwell: time.Minute, escalate: 3}, 1)
	q.Flush(now.Add(2 * time.Minute))
	if len(*got) != 0 {
		t.Fatalf("a recovered condition must not emit, got %+v", *got)
	}
}

// No health predicate is the common case for core loggers. Then RECURRENCE is
// what the log itself can prove — and only recurrence that was still going as
// the window closed. A one-off never emits.
func TestUnknownWithNoRecurrenceIsDropped(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnknown })
	rule := compiledRule{dwell: 2 * time.Minute, escalate: 3}
	q.Add("ha-logs", haSignal("homeassistant.core", "core.py:1", "ERROR"), rule, 1)
	q.Flush(now.Add(3 * time.Minute))
	if len(*got) != 0 {
		t.Fatal("a one-off record with no predicate and no recurrence must not emit")
	}
}

// A network blip that logged the same error for thirty seconds and then
// stopped HAS recurred, and is exactly the transient the dwell exists to drop.
func TestUnknownThatWentQuietBeforeTheCloseIsDropped(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnknown })
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99}
	base := *now
	for i := 0; i < 6; i++ {
		at := base.Add(time.Duration(i) * 6 * time.Second) // six records over thirty seconds
		q.now = func() time.Time { return at }
		q.Add("ha-logs", haSignal("async_upnp_client.ssdp", "ssdp.py:1", "ERROR"), rule, i+1)
	}
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 0 {
		t.Fatalf("a burst that went quiet before the closing minute must be dropped, got %+v", *got)
	}
}

// An integration still logging as the window closes is reported once, and the
// evidence names when its last record arrived.
func TestUnknownStillRecurringAtTheCloseIsEmitted(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnknown })
	rule := compiledRule{dwell: 3 * time.Minute, escalate: 99}
	base := *now
	for i, at := range []time.Duration{0, 30 * time.Second, 90 * time.Second, 150 * time.Second, 170 * time.Second} {
		at := at
		q.now = func() time.Time { return base.Add(at) }
		q.Add("ha-logs", haSignal("async_upnp_client.ssdp", "ssdp.py:1", "ERROR"), rule, i+1)
	}
	q.Flush(base.Add(3 * time.Minute))
	if len(*got) != 1 {
		t.Fatalf("a record still recurring at the close must emit, got %+v", *got)
	}
	if p := (*got)[0].sigs[0].Payload; !strings.Contains(p, "last seen 10s before the window closed") {
		t.Fatalf("evidence must name the gap to the last arrival:\n%s", p)
	}
}

// The health predicate is asked about the CLOSING part of the window, and that
// part follows the window actually waited: a third, floored at thirty seconds.
func TestHealthIsAskedSinceTheClosingWindow(t *testing.T) {
	var asked time.Time
	q, _, now := queueFixture(func(_ recordRef, since time.Time) verdict { asked = since; return verdictUnhealthy })
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), compiledRule{dwell: 3 * time.Minute, escalate: 99}, 1)
	q.Flush(now.Add(3 * time.Minute))
	if want := now.Add(2 * time.Minute); !asked.Equal(want) {
		t.Fatalf("since = %s, want %s (the last third of a 3m window)", asked, want)
	}
	if got := closingWindow(time.Minute); got != 30*time.Second {
		t.Fatalf("closingWindow(1m) = %s, want the 30s floor", got)
	}
}

// Breadth SHORTENS the dwell; it never removes it. A Home Assistant restart
// also makes several code paths fail at once, and only duration separates the
// two.
func TestEscalationShortensButKeepsAFloor(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnhealthy })
	rule := compiledRule{dwell: 20 * time.Minute, escalate: 3}
	for _, loc := range []string{"a.py:1", "b.py:2", "c.py:3"} {
		q.Add("ha-logs", haSignal("zwave_js", loc, "ERROR"), rule, 1)
	}
	q.Flush(now.Add(30 * time.Second))
	if len(*got) != 0 {
		t.Fatal("escalation must not make an entry due immediately")
	}
	q.Flush(now.Add(6 * time.Minute)) // 20m/4 = 5m
	if len(*got) != 1 {
		t.Fatalf("expected the shortened window to fire, got %+v", *got)
	}
	if !strings.Contains((*got)[0].sigs[0].Title, "(3 places)") {
		t.Fatalf("a multi-place entry should say so: %q", (*got)[0].sigs[0].Title)
	}
}

func TestEscalationFloorIsOneMinute(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnhealthy })
	rule := compiledRule{dwell: 2 * time.Minute, escalate: 2}
	q.Add("ha-logs", haSignal("mqtt", "a.py:1", "ERROR"), rule, 1)
	q.Add("ha-logs", haSignal("mqtt", "b.py:2", "ERROR"), rule, 1)
	q.Flush(now.Add(30 * time.Second))
	if len(*got) != 0 {
		t.Fatal("the escalation floor is a minute, not zero")
	}
	q.Flush(now.Add(70 * time.Second))
	if len(*got) != 1 {
		t.Fatalf("expected the floored window to fire, got %+v", *got)
	}
}

// A persistent problem must not re-report at the tick rate.
func TestRefractoryQuietsARepeatingProblem(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnhealthy })
	rule := compiledRule{dwell: 5 * time.Second, escalate: 3}
	q.Add("ha-logs", haSignal("zwave_js", "a.py:1", "ERROR"), rule, 1)
	q.Flush(now.Add(10 * time.Second))
	if len(*got) != 1 {
		t.Fatalf("expected the first emit, got %+v", *got)
	}
	q.now = func() time.Time { return now.Add(11 * time.Second) }
	q.Add("ha-logs", haSignal("zwave_js", "a.py:1", "ERROR"), rule, 2)
	q.Flush(now.Add(20 * time.Second))
	if len(*got) != 1 {
		t.Fatalf("the refractory window must suppress an immediate repeat, got %d emits", len(*got))
	}
}

// A stricter rule arriving second must not be delayed by a lax one that
// happened to arrive first.
func TestDeadlineShortensNeverExtends(t *testing.T) {
	q, got, now := queueFixture(func(recordRef, time.Time) verdict { return verdictUnhealthy })
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), compiledRule{dwell: 30 * time.Minute, escalate: 3}, 1)
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), compiledRule{dwell: time.Minute, escalate: 3}, 2)
	q.Flush(now.Add(2 * time.Minute))
	if len(*got) != 1 {
		t.Fatalf("the shorter dwell must win, got %+v", *got)
	}
}

// The re-check asks whether the count has risen SINCE THE WINDOW OPENED, so the
// opening count must not be overwritten by later sightings.
func TestCountAtOpenIsKept(t *testing.T) {
	var seen recordRef
	q, _, now := queueFixture(func(r recordRef, _ time.Time) verdict { seen = r; return verdictUnhealthy })
	rule := compiledRule{dwell: time.Minute, escalate: 3}
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), rule, 4)
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), rule, 9)
	q.Flush(now.Add(2 * time.Minute))
	if seen.countAtOpen != 4 {
		t.Fatalf("countAtOpen = %d, want the first sighting (4)", seen.countAtOpen)
	}
	if seen.source != "ha-logs" {
		t.Fatalf("the ref must carry its source, got %q", seen.source)
	}
}

func TestHasEntries(t *testing.T) {
	q, _, now := queueFixture(func(recordRef, time.Time) verdict { return verdictHealthy })
	if q.HasEntries() {
		t.Fatal("a fresh queue holds nothing")
	}
	q.Add("ha-logs", haSignal("hue", "a.py:1", "ERROR"), compiledRule{dwell: time.Minute, escalate: 3}, 1)
	if !q.HasEntries() {
		t.Fatal("expected a pending entry")
	}
	q.Flush(now.Add(2 * time.Minute))
	if q.HasEntries() {
		t.Fatal("a decided entry must leave the queue")
	}
}
