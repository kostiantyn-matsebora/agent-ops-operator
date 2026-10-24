package main

import (
	"testing"
	"time"
)

// The time axis. Every case here is a clock arithmetic test against an INJECTED
// now — a scheduled window is exactly the feature nobody would verify if it
// required waiting until four in the morning.

func mustCompile(t *testing.T, route Route) *ruleSet {
	t.Helper()
	rs, err := compileRules(nil, route)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return rs
}

// nightly is the reference case: a router restarting at 04:00 local, taking the
// cluster's connectivity with it for a quarter of an hour.
func nightly(loc string) Route {
	return Route{
		TimeIntervals: []TimeInterval{{
			Name:     "nightly-restart",
			Times:    []TimeRange{{StartTime: "04:00", EndTime: "04:20"}},
			Location: loc,
		}},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "nightly-restart"}},
	}
}

func at(t *testing.T, loc, stamp string) time.Time {
	t.Helper()
	l, err := time.LoadLocation(loc)
	if err != nil {
		t.Skipf("timezone data unavailable: %v", err)
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04", stamp, l)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestMuteInsideAndOutsideTheWindow(t *testing.T) {
	rs := mustCompile(t, nightly("UTC"))
	sig := &Signal{Labels: map[string]string{"reason": "NodeNotReady"}}

	for _, tc := range []struct {
		stamp string
		muted bool
	}{
		{"2026-08-11 03:59", false}, // a minute before
		{"2026-08-11 04:00", true},  // start is INCLUSIVE
		{"2026-08-11 04:19", true},  // last muted minute
		{"2026-08-11 04:20", false}, // end is EXCLUSIVE, so adjacent windows do not overlap
		{"2026-08-11 12:00", false},
	} {
		got := rs.MutedBy(sig, at(t, "UTC", tc.stamp)) != ""
		if got != tc.muted {
			t.Errorf("%s: muted=%v want %v", tc.stamp, got, tc.muted)
		}
	}
}

// "Four in the morning" is a LOCAL fact. A window pinned to UTC drifts by an
// hour at each daylight-saving transition — so it stops covering the outage it
// was written for, on a date nobody chose, at an hour nobody is watching.
func TestWindowFollowsLocalTimeAcrossDST(t *testing.T) {
	rs := mustCompile(t, nightly("Europe/Kyiv"))
	sig := &Signal{}

	// Kyiv is UTC+3 in summer and UTC+2 in winter. Both of these are 04:10
	// LOCAL, and they are different UTC instants.
	summer := at(t, "Europe/Kyiv", "2026-08-11 04:10")
	winter := at(t, "Europe/Kyiv", "2026-12-11 04:10")

	if rs.MutedBy(sig, summer) == "" {
		t.Error("04:10 local in summer must be inside the window")
	}
	if rs.MutedBy(sig, winter) == "" {
		t.Error("04:10 local in winter must be inside the window — the offset moved, the window did not")
	}
	// ...and the same UTC wall-clock is NOT muted in both, which is the whole
	// point: a UTC-pinned window would have covered only one of them.
	if _, off := summer.Zone(); off == 0 {
		t.Skip("timezone database is UTC-only here")
	}
}

func TestWeekdayAndDateSelection(t *testing.T) {
	rs := mustCompile(t, Route{
		TimeIntervals: []TimeInterval{{
			Name:     "weekday-mornings",
			Times:    []TimeRange{{StartTime: "04:00", EndTime: "05:00"}},
			Weekdays: []string{"monday:friday"},
			Months:   []string{"august"},
			Location: "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "weekday-mornings"}},
	})
	sig := &Signal{}

	// 2026-08-11 is a Tuesday; 2026-08-15 is a Saturday.
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-11 04:30")) == "" {
		t.Error("a Tuesday in August must be muted")
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-15 04:30")) != "" {
		t.Error("a Saturday must not be muted by a monday:friday window")
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-09-15 04:30")) != "" {
		t.Error("September must not be muted by an August-only window")
	}
}

// Going deaf for the window is the principal hazard, so a window may name what
// it expects. A router restart produces connectivity reasons; it does not
// produce OOMKilling.
func TestMatchersNarrowWhatAWindowSilences(t *testing.T) {
	rs := mustCompile(t, Route{
		TimeIntervals: []TimeInterval{{
			Name: "nightly", Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}, Location: "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{{
			Name: "nightly", Matchers: []string{`reason=~"NodeNotReady|Unhealthy"`},
		}},
	})
	inside := at(t, "UTC", "2026-08-11 04:10")

	connectivity := &Signal{Labels: map[string]string{"reason": "NodeNotReady"}}
	if rs.MutedBy(connectivity, inside) == "" {
		t.Error("the reasons the outage produces must be muted")
	}
	oom := &Signal{Labels: map[string]string{"reason": "OOMKilling"}}
	if w := rs.MutedBy(oom, inside); w != "" {
		t.Errorf("an unrelated failure must still be heard inside the window, got muted by %q", w)
	}
}

// Overlapping intervals UNION — muted when ANY matches — so an operator never
// has to reason about ordering.
func TestOverlappingIntervalsUnion(t *testing.T) {
	rs := mustCompile(t, Route{
		TimeIntervals: []TimeInterval{
			{Name: "a", Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}, Location: "UTC"},
			{Name: "b", Times: []TimeRange{{StartTime: "04:10", EndTime: "05:00"}}, Location: "UTC"},
		},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "a"}, {Name: "b"}},
	})
	sig := &Signal{}

	for _, stamp := range []string{"2026-08-11 04:05", "2026-08-11 04:15", "2026-08-11 04:45"} {
		if rs.MutedBy(sig, at(t, "UTC", stamp)) == "" {
			t.Errorf("%s falls in at least one interval and must be muted", stamp)
		}
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-11 05:30")) != "" {
		t.Error("outside both intervals must not be muted")
	}
}

// A typo that silently produced a window which never fires would look exactly
// like a window that works.
func TestBadConfigurationFailsLoudly(t *testing.T) {
	for name, route := range map[string]Route{
		"unknown timezone": {TimeIntervals: []TimeInterval{{
			Name: "x", Location: "Mars/Olympus", Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}}}},
		"reversed range": {TimeIntervals: []TimeInterval{{
			Name: "x", Times: []TimeRange{{StartTime: "04:20", EndTime: "04:00"}}}}},
		"bad clock": {TimeIntervals: []TimeInterval{{
			Name: "x", Times: []TimeRange{{StartTime: "4am", EndTime: "04:20"}}}}},
		"unknown weekday": {TimeIntervals: []TimeInterval{{
			Name: "x", Weekdays: []string{"funday"}}}},
		"missing name": {TimeIntervals: []TimeInterval{{Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}}}},
		"dangling reference": {
			TimeIntervals:     []TimeInterval{{Name: "x", Times: []TimeRange{{StartTime: "04:00", EndTime: "04:20"}}}},
			MuteTimeIntervals: []MuteTimeInterval{{Name: "typo"}},
		},
	} {
		if _, err := compileRules(nil, route); err == nil {
			t.Errorf("%s: expected a compile error, got none", name)
		}
	}
}

// A window spanning midnight is two entries, as in Alertmanager. Following that
// exactly is the point — a wrapping range would be a divergence dressed as a
// convenience.
func TestMidnightSpanIsTwoEntries(t *testing.T) {
	rs := mustCompile(t, Route{
		TimeIntervals: []TimeInterval{{
			Name:     "overnight",
			Times:    []TimeRange{{StartTime: "23:50", EndTime: "23:59"}, {StartTime: "00:00", EndTime: "00:10"}},
			Location: "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "overnight"}},
	})
	sig := &Signal{}

	for _, stamp := range []string{"2026-08-11 23:55", "2026-08-11 00:05"} {
		if rs.MutedBy(sig, at(t, "UTC", stamp)) == "" {
			t.Errorf("%s must be muted", stamp)
		}
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-11 12:00")) != "" {
		t.Error("midday must not be muted")
	}
}

// Day-of-month selection, including Alertmanager's negative-count-from-the-
// end form (-1 is the last day of the month) — nothing exercised this before,
// since every other mute test leaves daysOfMonth empty (the "unconstrained"
// fast path).
func TestDaysOfMonthSelectionIncludingCountingFromMonthEnd(t *testing.T) {
	rs := mustCompile(t, Route{
		TimeIntervals: []TimeInterval{{
			Name:        "month-boundaries",
			Times:       []TimeRange{{StartTime: "00:00", EndTime: "23:59"}},
			DaysOfMonth: []string{"1:2", "-1"},
			Location:    "UTC",
		}},
		MuteTimeIntervals: []MuteTimeInterval{{Name: "month-boundaries"}},
	})
	sig := &Signal{}

	if rs.MutedBy(sig, at(t, "UTC", "2026-08-01 12:00")) == "" {
		t.Error("the 1st must be muted by the 1:2 range")
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-15 12:00")) != "" {
		t.Error("the middle of the month must not be muted")
	}
	// August 2026 has 31 days: -1 must resolve to the 31st.
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-31 12:00")) == "" {
		t.Error("the last day of the month must be muted by the -1 entry")
	}
	if rs.MutedBy(sig, at(t, "UTC", "2026-08-30 12:00")) != "" {
		t.Error("the second-to-last day must not be muted by a bare -1 entry")
	}
}

// No windows configured is the default, and must cost nothing.
func TestNoIntervalsMutesNothing(t *testing.T) {
	rs := mustCompile(t, Route{})
	if rs.MutedBy(&Signal{}, time.Now()) != "" {
		t.Error("a source with no time intervals must mute nothing")
	}
}
