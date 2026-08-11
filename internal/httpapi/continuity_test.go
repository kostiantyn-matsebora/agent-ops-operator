package httpapi

import (
	"testing"
	"time"
)

// The distinction this encodes: ONE conversation reporting that its context
// could not be reached is a loss; EVERY conversation reporting one at the same
// moment is an outage. Collapsing them would turn a recoverable storage incident
// into the permanent, irreversible destruction of every active conversation's
// context — worse than the silent degradation it replaces.

func breakerAt(t *testing.T, base time.Time) (*continuityBreaker, func(time.Duration)) {
	t.Helper()
	now := base
	b := &continuityBreaker{now: func() time.Time { return now }}
	return b, func(d time.Duration) { now = now.Add(d) }
}

func TestAnIsolatedReportIsALossNotAnOutage(t *testing.T) {
	b, _ := breakerAt(t, time.Now())

	if b.Report() {
		t.Fatal("one conversation losing its context is not evidence about the infrastructure")
	}
	if open, _ := b.Open(); open {
		t.Fatal("breaker must stay closed")
	}
}

func TestEnoughReportsInTheWindowMeanOutage(t *testing.T) {
	b, advance := breakerAt(t, time.Now())

	for i := 1; i < continuityThreshold; i++ {
		if b.Report() {
			t.Fatalf("opened early at report %d", i)
		}
		advance(time.Second)
	}
	if !b.Report() {
		t.Fatal("reports past the threshold within the window must open the breaker")
	}
	open, since := b.Open()
	if !open || since.IsZero() {
		t.Fatalf("open=%v since=%v", open, since)
	}
}

// Losses spread over an afternoon are not an outage — otherwise unrelated
// conversations would eventually hold each other's work hostage.
func TestReportsOutsideTheWindowDoNotAccumulate(t *testing.T) {
	b, advance := breakerAt(t, time.Now())

	for i := 0; i < continuityThreshold*2; i++ {
		if b.Report() {
			t.Fatalf("stale reports accumulated into a false outage at %d", i)
		}
		advance(continuityWindow + time.Second)
	}
}

// The breaker closes on a PROBE that succeeded, not on a timer: what matters is
// that contexts are reachable again, and a successful continuation proves it.
func TestASuccessfulContinuationClosesTheBreaker(t *testing.T) {
	b, advance := breakerAt(t, time.Now())
	for i := 0; i < continuityThreshold; i++ {
		b.Report()
		advance(time.Second)
	}
	if open, _ := b.Open(); !open {
		t.Fatal("precondition: breaker should be open")
	}

	b.Continued()

	if open, _ := b.Open(); open {
		t.Fatal("a successful continuation means contexts are reachable — release the held work")
	}
	// ...and the history is cleared, so the next isolated loss is judged on its
	// own rather than on reports from before the outage was resolved.
	if b.Report() {
		t.Fatal("one report after recovery must not re-open immediately")
	}
}
