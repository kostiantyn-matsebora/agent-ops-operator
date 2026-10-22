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
	return Signal{InitiationID: "id-" + reason, Labels: map[string]string{"reason": reason}}
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
	filt := mutedFilter(t)
	a := newTestAdapter(f.start(t), nil, "events", filt)
	ctx := context.Background()

	a.reportMute(ctx, "events", 3, "nightly")
	// Ready stays TRUE: the source is obeying its configuration, and marking it
	// unhealthy for that would train an operator to ignore the condition.
	if st, ok := lastStatus(f, "Muted"); !ok || !st.Ready ||
		!strings.Contains(st.Message, "nightly") || !strings.Contains(st.Message, "3 event(s)") {
		t.Fatalf("the open window must be reported with its interval and count: %+v", st)
	}

	// More muting in the same window does not re-report per batch...
	a.reportMute(ctx, "events", 5, "nightly")
	if n := countReason(f, "Muted"); n != 1 {
		t.Fatalf("a batch must not re-report an open window: %d Muted reports", n)
	}
	// ...the sweep carries the running count, on the window's own clock.
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:12"))
	if st, _ := lastStatus(f, "Muted"); !strings.Contains(st.Message, "8 event(s) muted so far") {
		t.Fatalf("the active report must carry the running count: %+v", st)
	}

	// A batch with nothing muted closes nothing — the window is still open.
	a.reportMute(ctx, "events", 0, "")
	if _, ok := lastStatus(f, "MuteEnded"); ok {
		t.Fatal("a batch with nothing muted must not close an open window")
	}

	// The window's interval ends: the sweep reports what it cost.
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:25"))
	if st, ok := lastStatus(f, "MuteEnded"); !ok || !strings.Contains(st.Message, "8 event(s)") {
		t.Fatalf("the close must report the accumulated count: %+v", st)
	}
}

// A lane that goes quiet after being muted still has its window closed: the
// sweep timer fires MuteEnded with the final count, no next batch required.
func TestAMutedLaneThatGoesQuietStillReportsItsClose(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	filt := mutedFilter(t)
	a := newTestAdapter(f.start(t), nil, "events", filt)
	ctx := context.Background()

	a.reportMute(ctx, "events", 4, "nightly") // the last batch this lane ever sends
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:10"))
	if _, ok := lastStatus(f, "MuteEnded"); ok {
		t.Fatal("an open window must not be closed early")
	}
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:21"))
	st, ok := lastStatus(f, "MuteEnded")
	if !ok || !st.Ready || !strings.Contains(st.Message, `"nightly"`) || !strings.Contains(st.Message, "4 event(s)") {
		t.Fatalf("the sweep must close a quiet lane's window with its final count: %+v", f.allStatuses())
	}
	// Closed once: a later sweep reports nothing more.
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:30"))
	if n := countReason(f, "MuteEnded"); n != 1 {
		t.Fatalf("a closed window must be reported closed exactly once: %d", n)
	}
}

// A window over a lane that produces no events at all still reports itself
// while open, and its close with a count of zero.
func TestAQuietLaneReportsItsWindowAndAZeroCountClose(t *testing.T) {
	f := newFakeManager(SourceInfo{Name: "events"})
	filt := mutedFilter(t)
	a := newTestAdapter(f.start(t), nil, "events", filt)
	ctx := context.Background()

	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 03:59"))
	if len(f.allStatuses()) != 0 {
		t.Fatalf("no window is open yet: %+v", f.allStatuses())
	}
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:01"))
	if st, ok := lastStatus(f, "Muted"); !ok || !strings.Contains(st.Message, "0 event(s) muted so far") {
		t.Fatalf("an open window over a quiet lane must report itself: %+v", f.allStatuses())
	}
	a.sweepMute(ctx, "events", filt.rules, at(t, "UTC", "2026-08-11 04:20"))
	if st, ok := lastStatus(f, "MuteEnded"); !ok || !strings.Contains(st.Message, "0 event(s)") {
		t.Fatalf("the close must report a zero count: %+v", f.allStatuses())
	}
}

// lastStatus returns the most recent report carrying a reason.
func lastStatus(f *fakeManager, reason string) (reportedStatus, bool) {
	all := f.allStatuses()
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Reason == reason {
			return all[i], true
		}
	}
	return reportedStatus{}, false
}

// countReason counts the reports carrying a reason.
func countReason(f *fakeManager, reason string) int {
	n := 0
	for _, st := range f.allStatuses() {
		if st.Reason == reason {
			n++
		}
	}
	return n
}
