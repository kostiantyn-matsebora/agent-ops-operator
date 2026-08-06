package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Minimal five-field cron parser (minute hour day-of-month month day-of-week),
// deliberately dependency-free (module rule). Supported per field: "*", "*/n",
// "N", "a-b", and comma lists of those. Standard cron semantics: when BOTH
// day-of-month and day-of-week are restricted, a time matches if EITHER does.
// Anything fancier belongs in an adopter's own adapter with a real cron
// library.

// Schedule is a parsed cron expression.
type Schedule struct {
	minute, hour, dom, month, dow map[int]bool
	domAny, dowAny                bool
}

// bounds per field position.
var cronBounds = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}

// ParseCron parses a five-field cron expression.
func ParseCron(expr string) (*Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("want 5 fields (minute hour dom month dow), got %d", len(fields))
	}
	sets := make([]map[int]bool, 5)
	anys := make([]bool, 5)
	for i, f := range fields {
		set, isAny, err := parseCronField(f, cronBounds[i][0], cronBounds[i][1])
		if err != nil {
			return nil, fmt.Errorf("field %d (%q): %w", i+1, f, err)
		}
		sets[i], anys[i] = set, isAny
	}
	return &Schedule{
		minute: sets[0], hour: sets[1], dom: sets[2], month: sets[3], dow: sets[4],
		domAny: anys[2], dowAny: anys[4],
	}, nil
}

func parseCronField(f string, lo, hi int) (map[int]bool, bool, error) {
	set := map[int]bool{}
	if f == "*" {
		for v := lo; v <= hi; v++ {
			set[v] = true
		}
		return set, true, nil
	}
	for _, part := range strings.Split(f, ",") {
		switch {
		case strings.HasPrefix(part, "*/"):
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, false, fmt.Errorf("bad step %q", part)
			}
			for v := lo; v <= hi; v += step {
				set[v] = true
			}
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			a, errA := strconv.Atoi(ab[0])
			b, errB := strconv.Atoi(ab[1])
			if errA != nil || errB != nil || a < lo || b > hi || a > b {
				return nil, false, fmt.Errorf("bad range %q", part)
			}
			for v := a; v <= b; v++ {
				set[v] = true
			}
		default:
			v, err := strconv.Atoi(part)
			if err != nil || v < lo || v > hi {
				return nil, false, fmt.Errorf("bad value %q", part)
			}
			set[v] = true
		}
	}
	return set, false, nil
}

// matches reports whether t (minute precision) satisfies the schedule.
func (s *Schedule) matches(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}
	domOK := s.dom[t.Day()]
	dowOK := s.dow[int(t.Weekday())]
	if !s.domAny && !s.dowAny {
		return domOK || dowOK // standard cron OR rule
	}
	return domOK && dowOK
}

// Next returns the first scheduled time strictly after t (zero time when none
// within ~13 months — impossible for well-formed expressions).
func (s *Schedule) Next(t time.Time) time.Time {
	tick := t.Truncate(time.Minute).Add(time.Minute)
	limit := tick.Add(400 * 24 * time.Hour)
	for ; tick.Before(limit); tick = tick.Add(time.Minute) {
		if s.matches(tick) {
			return tick
		}
	}
	return time.Time{}
}
