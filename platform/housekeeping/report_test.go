package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Report.String() is this job's whole user interface (it is what reaches the
// CronJob's log), and it was entirely untested: every branch — the dry-run
// verb swap, the per-run-bound note, and the removed-names listing — needs
// its own case.
func TestReportStringRemoved(t *testing.T) {
	r := Report{Kind: "workspaces", Scanned: 5, Removed: []string{"a", "b"}, Retained: 3}
	s := r.String()
	if !strings.Contains(s, "workspaces: scanned 5, removed 2, retained 3") {
		t.Fatalf("String() = %q", s)
	}
	if !strings.Contains(s, ": a, b") {
		t.Fatalf("String() = %q, want the removed names listed", s)
	}
	if strings.Contains(s, "left for the next run") {
		t.Fatalf("String() = %q, must not mention the bound when nothing was skipped", s)
	}
}

func TestReportStringDryRun(t *testing.T) {
	r := Report{Kind: "sessions", DryRun: true, Scanned: 1, Removed: []string{"x.jsonl"}}
	s := r.String()
	if !strings.Contains(s, "would remove 1") {
		t.Fatalf("String() = %q, want the dry-run verb", s)
	}
}

func TestReportStringSkippedByBound(t *testing.T) {
	r := Report{Kind: "workspaces", Scanned: 10, Removed: []string{"a"}, Skipped: 4, Retained: 5}
	s := r.String()
	if !strings.Contains(s, "4 left for the next run (per-run bound)") {
		t.Fatalf("String() = %q, want the bound note", s)
	}
}

func TestReportStringNothingRemoved(t *testing.T) {
	r := Report{Kind: "workspaces", Scanned: 0, Retained: 0}
	want := "workspaces: scanned 0, removed 0, retained 0"
	if got := r.String(); got != want {
		t.Fatalf("String() = %q, want %q (no removed-list suffix, no bound note)", got, want)
	}
}

// Options.Now is the injectable clock the grace-period cutoff is computed
// from. No existing test ever set it, so the override branch of now() (as
// opposed to the time.Now() fallback) had never actually run — meaning the
// mechanism that makes ReclaimSessions's cutoff deterministic in tests was
// itself unverified.
func TestOptionsNowUsesInjectedClock(t *testing.T) {
	root := t.TempDir()
	mkSession(t, root, "ctx-fixed", 2*time.Hour) // 2h old by wall-clock mtime

	fixed := time.Now().Add(48 * time.Hour) // "now" is far in the future
	opts := Options{
		SessionGrace: time.Hour,
		Now:          func() time.Time { return fixed },
	}
	if got := opts.now(); !got.Equal(fixed) {
		t.Fatalf("now() = %v, want the injected clock's %v", got, fixed)
	}

	// With the injected clock, the file (2h old in real time) is far older
	// than the 1h grace period measured from the FUTURE "now" — so it must
	// be reclaimed. This is only true if now() actually used the injected
	// clock rather than the real one, where 2h old would also clear a 1h
	// grace — so also prove the converse: a clock in the PAST retains it.
	lister := &stubLister{}
	rep, err := ReclaimSessions(context.Background(), root, lister, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 1 || rep.Removed[0] != "ctx-fixed.jsonl" {
		t.Fatalf("want the file reclaimed against the injected future clock: %+v", rep)
	}

	past := Options{
		SessionGrace: time.Hour,
		Now:          func() time.Time { return time.Now().Add(-48 * time.Hour) },
	}
	rep2, err := ReclaimSessions(context.Background(), root, lister, past)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.Removed) != 0 {
		t.Fatalf("a clock in the past must not have aged the file into the grace cutoff: %+v", rep2)
	}
}
