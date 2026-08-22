package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	// The IANA timezone database, EMBEDDED IN THE BINARY.
	//
	// This image runs on gcr.io/distroless/static, which carries no zoneinfo, so
	// without this `time.LoadLocation("Europe/Kyiv")` fails and a perfectly valid
	// window is rejected as "not an IANA timezone" — a wrong error, about the one
	// field most likely to be set, discovered at four in the morning. Costs ~450KB
	// and removes an entire class of "works on my machine".
	_ "time/tzdata"
)

// The TIME axis of suppression.
//
// The other three cannot express "do not wake anyone between 04:00 and 04:20",
// and each looks like it might:
//
//   - `for:` dwell verifies a condition still holds. During a scheduled outage
//     it genuinely does — for a quarter of an hour — so a dwell long enough to
//     cover it would delay every real incident by the same amount, all day.
//   - inhibition suppresses the CONSEQUENCES of a cause that is already
//     reported, keyed on a cause event. A router losing power produces no
//     in-cluster object to key on; the cluster only ever sees consequences.
//   - matchers select on labels, and there is no label for the time of day.
//
// Alertmanager already models this, so the names, schema and semantics are
// borrowed exactly rather than approximated. `rules` speaks Prometheus and
// `route` speaks Alertmanager; a concept already specified in the half it
// belongs to must not be re-spelled, which is the same rule that keeps `for:`
// from being called `group_wait`.

// TimeInterval is one named interval of `route.timeIntervals`, in Alertmanager's
// shape.
type TimeInterval struct {
	Name string `json:"name"`
	// Times are clock ranges within a day, e.g. {startTime: "04:00", endTime: "04:20"}.
	Times []TimeRange `json:"times,omitempty"`
	// Weekdays like "monday" or "monday:friday"; empty = every day.
	Weekdays []string `json:"weekdays,omitempty"`
	// DaysOfMonth like "1" or "1:7"; negative counts from the end, as Alertmanager does.
	DaysOfMonth []string `json:"daysOfMonth,omitempty"`
	// Months like "1", "january" or "1:3"; empty = every month.
	Months []string `json:"months,omitempty"`
	// Years like "2026" or "2026:2030"; empty = every year.
	Years []string `json:"years,omitempty"`
	// Location is an IANA timezone. Defaulting to UTC is Alertmanager's
	// behaviour and is kept, but "four in the morning" is a LOCAL fact: a window
	// pinned to UTC drifts by an hour at each daylight-saving transition, so it
	// stops covering the outage it was written for, on a date nobody chose, at
	// an hour nobody is watching.
	Location string `json:"location,omitempty"`
}

// TimeRange is a clock range within a day. Alertmanager requires start < end, so
// a window spanning midnight is written as two entries.
type TimeRange struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// MuteTimeInterval references named intervals and optionally narrows what they
// silence.
type MuteTimeInterval struct {
	// Name of a TimeInterval declared in route.timeIntervals.
	Name string `json:"name"`
	// Matchers restrict what this window mutes. Absent = everything from the
	// source for the interval's duration.
	//
	// Going deaf for the length of the window is the principal hazard of this
	// feature: a scheduled outage produces connectivity-shaped reasons and does
	// NOT produce OOMKilling. Narrowing means the safe configuration does not
	// depend on the window being short, or on anyone reviewing it later.
	Matchers []string `json:"matchers,omitempty"`
}

// compiledInterval is one interval with its location resolved and its ranges
// parsed, so matching is arithmetic rather than parsing.
type compiledInterval struct {
	name  string
	loc   *time.Location
	times []clockRange
	// Each set is empty when the interval does not constrain that field.
	weekdays    []numRange
	daysOfMonth []numRange
	months      []numRange
	years       []numRange
}

type clockRange struct{ startMin, endMin int } // minutes since midnight
type numRange struct{ lo, hi int }

// compiledMute is one mute reference: which interval, and what it narrows to.
type compiledMute struct {
	interval *compiledInterval
	matchers []matcher
}

// Matches reports whether t falls inside the interval, in ITS location.
func (ci *compiledInterval) Matches(t time.Time) bool {
	lt := t.In(ci.loc)
	if !inNumRanges(ci.years, lt.Year()) {
		return false
	}
	if !inNumRanges(ci.months, int(lt.Month())) {
		return false
	}
	if !matchesDayOfMonth(ci.daysOfMonth, lt) {
		return false
	}
	// time.Weekday is Sunday=0; Alertmanager names sunday..saturday the same way.
	if !inNumRanges(ci.weekdays, int(lt.Weekday())) {
		return false
	}
	if len(ci.times) == 0 {
		return true
	}
	mins := lt.Hour()*60 + lt.Minute()
	for _, r := range ci.times {
		// End is EXCLUSIVE, as Alertmanager treats it: 04:00-04:20 covers 04:00
		// up to but not including 04:20, so two adjacent ranges do not overlap.
		if mins >= r.startMin && mins < r.endMin {
			return true
		}
	}
	return false
}

// inNumRanges reports whether v is in any range, or if there are none (the
// interval does not constrain this field).
func inNumRanges(rs []numRange, v int) bool {
	if len(rs) == 0 {
		return true
	}
	for _, r := range rs {
		if v >= r.lo && v <= r.hi {
			return true
		}
	}
	return false
}

// matchesDayOfMonth handles Alertmanager's negative days, which count back from
// the end of the month (-1 is the last day).
func matchesDayOfMonth(rs []numRange, t time.Time) bool {
	if len(rs) == 0 {
		return true
	}
	day := t.Day()
	last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	for _, r := range rs {
		lo, hi := r.lo, r.hi
		if lo < 0 {
			lo = last + 1 + lo
		}
		if hi < 0 {
			hi = last + 1 + hi
		}
		if day >= lo && day <= hi {
			return true
		}
	}
	return false
}

// ---- compilation ------------------------------------------------------------

var weekdayNames = map[string]int{
	"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
	"thursday": 4, "friday": 5, "saturday": 6,
}

var monthNames = map[string]int{
	"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
	"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
}

// compileIntervals resolves the declared intervals. An unparseable zone or
// range is an ERROR rather than an ignored entry: a typo would otherwise leave
// a window that never fires, which looks exactly like a window that is working.
func compileIntervals(tis []TimeInterval) (map[string]*compiledInterval, error) {
	out := map[string]*compiledInterval{}
	for i, ti := range tis {
		name := strings.TrimSpace(ti.Name)
		if name == "" {
			return nil, fmt.Errorf("route.timeIntervals[%d]: name is required", i)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("route.timeIntervals[%d]: duplicate name %q", i, name)
		}
		loc := time.UTC
		if z := strings.TrimSpace(ti.Location); z != "" {
			l, err := time.LoadLocation(z)
			if err != nil {
				return nil, fmt.Errorf("route.timeIntervals[%d].location: %q is not an IANA timezone: %w", i, z, err)
			}
			loc = l
		}
		ci := &compiledInterval{name: name, loc: loc}
		for j, tr := range ti.Times {
			start, err := parseClock(tr.StartTime)
			if err != nil {
				return nil, fmt.Errorf("route.timeIntervals[%d].times[%d].startTime: %w", i, j, err)
			}
			end, err := parseClock(tr.EndTime)
			if err != nil {
				return nil, fmt.Errorf("route.timeIntervals[%d].times[%d].endTime: %w", i, j, err)
			}
			if start >= end {
				return nil, fmt.Errorf("route.timeIntervals[%d].times[%d]: startTime must be before endTime "+
					"(a window spanning midnight is two entries, as in Alertmanager)", i, j)
			}
			ci.times = append(ci.times, clockRange{start, end})
		}
		var err error
		if ci.weekdays, err = parseNumRanges(ti.Weekdays, weekdayNames, 0, 6); err != nil {
			return nil, fmt.Errorf("route.timeIntervals[%d].weekdays: %w", i, err)
		}
		if ci.months, err = parseNumRanges(ti.Months, monthNames, 1, 12); err != nil {
			return nil, fmt.Errorf("route.timeIntervals[%d].months: %w", i, err)
		}
		if ci.daysOfMonth, err = parseNumRanges(ti.DaysOfMonth, nil, -31, 31); err != nil {
			return nil, fmt.Errorf("route.timeIntervals[%d].daysOfMonth: %w", i, err)
		}
		if ci.years, err = parseNumRanges(ti.Years, nil, 1970, 9999); err != nil {
			return nil, fmt.Errorf("route.timeIntervals[%d].years: %w", i, err)
		}
		out[name] = ci
	}
	return out, nil
}

// compileMutes resolves mute references against the declared intervals.
func compileMutes(mutes []MuteTimeInterval, intervals map[string]*compiledInterval) ([]compiledMute, error) {
	var out []compiledMute
	for i, m := range mutes {
		ci, ok := intervals[strings.TrimSpace(m.Name)]
		if !ok {
			return nil, fmt.Errorf("route.muteTimeIntervals[%d]: no time interval named %q", i, m.Name)
		}
		ms, err := parseMatchers(m.Matchers)
		if err != nil {
			return nil, fmt.Errorf("route.muteTimeIntervals[%d].matchers: %w", i, err)
		}
		out = append(out, compiledMute{interval: ci, matchers: ms})
	}
	return out, nil
}

// parseClock reads "HH:MM" as minutes since midnight.
func parseClock(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("%q is not HH:MM", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 {
		return 0, fmt.Errorf("%q is not a valid time of day", s)
	}
	return h*60 + m, nil
}

// parseNumRanges reads Alertmanager's "n", "n:m" and name forms.
func parseNumRanges(in []string, names map[string]int, lo, hi int) ([]numRange, error) {
	var out []numRange
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		a, b := s, s
		if i := strings.Index(s, ":"); i >= 0 {
			a, b = strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
		}
		av, err := parseNumOrName(a, names)
		if err != nil {
			return nil, err
		}
		bv, err := parseNumOrName(b, names)
		if err != nil {
			return nil, err
		}
		if av < lo || av > hi || bv < lo || bv > hi {
			return nil, fmt.Errorf("%q is outside the allowed range %d:%d", raw, lo, hi)
		}
		if av > bv {
			return nil, fmt.Errorf("%q is reversed", raw)
		}
		out = append(out, numRange{av, bv})
	}
	return out, nil
}

func parseNumOrName(s string, names map[string]int) (int, error) {
	if names != nil {
		if v, ok := names[s]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is neither a number nor a known name", s)
	}
	return v, nil
}

// MutedBy reports the interval muting this signal at time t, or "" if none.
//
// Overlapping intervals UNION: muted when any referenced interval matches, which
// is Alertmanager's behaviour and the only one that does not require an operator
// to reason about ordering.
func (rs *ruleSet) MutedBy(sig *Signal, t time.Time) string {
	for _, m := range rs.mutes {
		if !m.interval.Matches(t) {
			continue
		}
		// No matchers = the whole source, for the interval's duration.
		if len(m.matchers) == 0 || allMatch(m.matchers, matchLabels(sig)) {
			return m.interval.name
		}
	}
	return ""
}
