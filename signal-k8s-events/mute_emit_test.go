package main

import (
	"context"
	"strings"
	"testing"
)

// Where the mute sits in the pipeline is the whole design: AFTER the dwell queue
// and BEFORE the emit cap. These tests pin both ends of that.

// mutedFilter builds a filter whose source mutes 04:00–04:20 UTC.
func mutedFilter(t *testing.T, matchers ...string) *filter {
	t.Helper()
	var mute MuteTimeInterval
	mute = MuteTimeInterval{Name: "nightly", Matchers: matchers}
	rs, err := compileRules(nil, Route{
		TimeIntervals: []TimeInterval{{
			Name: "nightly", Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}, Location: "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{mute},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &filter{rules: rs}
}

// allDayMutedFilter mutes around the clock, for the tests that go through
// post() and therefore evaluate against the real wall clock. 00:00–24:00 is an
// ordinary range — the window's WIDTH is not what those tests are about.
func allDayMutedFilter(t *testing.T, matchers ...string) *filter {
	t.Helper()
	rs, err := compileRules(nil, Route{
		TimeIntervals: []TimeInterval{{
			Name: "nightly", Times: []TimeRange{{StartTime: "00:00", EndTime: "24:00"}}, Location: "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "nightly", Matchers: matchers}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &filter{rules: rs}
}

func sigWith(reason string) Signal {
	return Signal{Fingerprint: "fp-" + reason, Labels: map[string]string{"reason": reason}}
}

func TestTransientNoiseInsideTheWindowIsDropped(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	a := newTestAdapter(f.start(t), nil, "events", mutedFilter(t))
	inside := at(t, "UTC", "2026-08-11 04:10")

	kept, muted, window := a.applyMute("events", []Signal{sigWith("NodeNotReady")}, inside)

	if len(kept) != 0 || muted != 1 || window != "nightly" {
		t.Fatalf("kept=%d muted=%d window=%q — the window should have swallowed it", len(kept), muted, window)
	}
}

// The property that makes muting safe: a problem that OUTLIVES the window still
// gets reported, because the cluster keeps producing events for anything
// genuinely broken and the next one emits normally once the window closes.
func TestAProblemOutlivingTheWindowStillSurfaces(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	a := newTestAdapter(f.start(t), nil, "events", mutedFilter(t))

	if kept, _, _ := a.applyMute("events", []Signal{sigWith("NodeNotReady")}, at(t, "UTC", "2026-08-11 04:10")); len(kept) != 0 {
		t.Fatal("inside the window it must be muted")
	}
	// Same failure, still producing events after the window closed.
	kept, muted, _ := a.applyMute("events", []Signal{sigWith("NodeNotReady")}, at(t, "UTC", "2026-08-11 04:25"))
	if len(kept) != 1 || muted != 0 {
		t.Fatalf("after the window the same problem must emit: kept=%d muted=%d", len(kept), muted)
	}
}

func TestANarrowedWindowLetsUnrelatedFailuresThrough(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	a := newTestAdapter(f.start(t), nil, "events", mutedFilter(t, `reason="NodeNotReady"`))
	inside := at(t, "UTC", "2026-08-11 04:10")

	kept, muted, _ := a.applyMute("events",
		[]Signal{sigWith("NodeNotReady"), sigWith("OOMKilling")}, inside)

	if muted != 1 || len(kept) != 1 {
		t.Fatalf("muted=%d kept=%d — only the outage's own reason should be silenced", muted, len(kept))
	}
	if kept[0].Labels["reason"] != "OOMKilling" {
		t.Fatalf("the wrong signal survived: %v", kept[0].Labels)
	}
}

// Muted signals were never emitted, so charging them to the emit cap would make
// a maintenance window read as a runaway.
func TestMutingDoesNotConsumeTheEmitCap(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	// Narrowed so the connectivity noise is muted and an unrelated failure is not
	// — the same shape a real maintenance window has.
	a := newTestAdapter(f.start(t), nil, "events", allDayMutedFilter(t, `reason="NodeNotReady"`))

	var many []Signal
	for i := 0; i < defaultEmitPerMin*2; i++ {
		many = append(many, sigWith("NodeNotReady"))
	}
	a.post(context.Background(), "events", many)

	// Nothing posted, nothing clipped: the cap never saw them.
	if got := len(f.signalsFor("events")); got != 0 {
		t.Fatalf("muted signals reached the manager: %d", got)
	}
	for _, st := range f.allStatuses() {
		if st.Reason == "EmitCapReached" {
			t.Fatal("muting was charged to the emit cap — a window would read as a runaway")
		}
	}
	// ...and the budget is intact: twice the per-minute cap has just been muted,
	// so if muting were charged, this unrelated failure would be clipped.
	a.post(context.Background(), "events", []Signal{sigWith("OOMKilling")})
	if got := len(f.signalsFor("events")); got != 1 {
		t.Fatalf("the emit budget was spent on muted signals: posted=%d", got)
	}
}

// Muting is never silent: a muted lane and an idle lane look identical from
// outside and only one of them means the cluster is healthy.
func TestAnActiveMuteIsReportedAndItsCostSummarised(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	a := newTestAdapter(f.start(t), nil, "events", mutedFilter(t))

	a.reportMute(context.Background(), "events", 3, "nightly")
	// Ready stays TRUE: the source is obeying its configuration, and marking it
	// unhealthy for that would train an operator to ignore the condition.
	waitFor(t, func() bool {
		for _, st := range f.allStatuses() {
			if st.Reason == "Muted" {
				return st.Ready && strings.Contains(st.Message, "nightly")
			}
		}
		return false
	})

	// More muting in the same window does not re-report; the count accumulates.
	a.reportMute(context.Background(), "events", 5, "nightly")

	// The window closes: report what it cost.
	a.reportMute(context.Background(), "events", 0, "")
	// Closing the window reports what it cost — 3 + 5, accumulated.
	waitFor(t, func() bool {
		for _, st := range f.allStatuses() {
			if st.Reason == "MuteEnded" {
				return strings.Contains(st.Message, "8 event(s)")
			}
		}
		return false
	})
}


