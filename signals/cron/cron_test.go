package main

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseCronRejects(t *testing.T) {
	for _, expr := range []string{
		"", "* * * *", "* * * * * *", "60 * * * *", "* 24 * * *",
		"*/0 * * * *", "a * * * *", "5-1 * * * *", "* * 32 * *", "* * * 13 *", "* * * * 7",
	} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("expected error for %q", expr)
		}
	}
}

func TestCronNext(t *testing.T) {
	cases := []struct {
		expr, after, want string
	}{
		// daily at 06:00
		{"0 6 * * *", "2026-08-06T05:59:00Z", "2026-08-06T06:00:00Z"},
		{"0 6 * * *", "2026-08-06T06:00:00Z", "2026-08-07T06:00:00Z"}, // strictly after
		// every 15 minutes
		{"*/15 * * * *", "2026-08-06T10:07:00Z", "2026-08-06T10:15:00Z"},
		{"*/15 * * * *", "2026-08-06T10:45:00Z", "2026-08-06T11:00:00Z"},
		// weekdays at 09:30 (2026-08-07 is a Friday, 08-08 Saturday)
		{"30 9 * * 1-5", "2026-08-07T10:00:00Z", "2026-08-10T09:30:00Z"},
		// first of month at midnight
		{"0 0 1 * *", "2026-08-06T00:00:00Z", "2026-09-01T00:00:00Z"},
		// comma list of hours
		{"0 6,18 * * *", "2026-08-06T07:00:00Z", "2026-08-06T18:00:00Z"},
		// standard OR rule: dom 15 OR sunday (2026-08-09 is a Sunday before the 15th)
		{"0 12 15 * 0", "2026-08-06T00:00:00Z", "2026-08-09T12:00:00Z"},
	}
	for _, c := range cases {
		s, err := ParseCron(c.expr)
		if err != nil {
			t.Fatalf("%q: %v", c.expr, err)
		}
		if got := s.Next(at(c.after)); !got.Equal(at(c.want)) {
			t.Errorf("%q after %s: got %s, want %s", c.expr, c.after, got, c.want)
		}
	}
}

// DELIBERATE FAILURE for sdlc-setup 2.8. A failing TEST, not a broken package:
// the module job must fail while the image build, which only compiles, stays
// green. Reverted on the next commit of this branch.
func TestDeliberateBreakForCI(t *testing.T) {
	t.Fatal("deliberate failure: proving CI attributes a module failure to signals/cron")
}
