// Package storagebreaker decides whether context storage being unreachable is
// ONE conversation's loss or an INSTALL-WIDE outage.
//
// Unavailability is treated as an OUTAGE before it is treated as a LOSS.
//
// One conversation reporting that its context could not be reached is a loss.
// Every conversation reporting one at the same moment is a storage incident, and
// the difference matters enormously: failing fast on each report would convert a
// recoverable two-minute outage into the permanent destruction of every active
// conversation's context, irreversibly, in the time it takes each to receive one
// message. That is worse than the silent degradation this whole change replaces,
// and it is not undoable.
//
// A runtime pod cannot see this — it handles one unit and exits. The manager
// can, because it sees every run, and it already holds cross-conversation state
// for admission and cooldown, so this is the same shape rather than a new idea.
//
// TWO EDGES, ONE BREAKER. The evidence arrives two ways and they are the same
// incident:
//
//   - a run REPORTS that it could not reach its context (the reporting edge),
//   - a runtime pod cannot be PROVISIONED because its volume will not attach
//     (the provisioning edge).
//
// It lived in the HTTP API and watched only the first, which is why it never
// fired on 2026-08-20: a corrupt filesystem meant no pod ever started, so there
// was no run to file a report. The most total form of the outage — every
// conversation blocked, nothing running — was invisible to the mechanism built
// for it. A SECOND breaker for the second edge would be worse than none: two
// judgements disagreeing about whether storage is down would surface as a bug
// report far from either.
package storagebreaker

import (
	"sync"
	"time"
)

// Window is how far back reports are counted. Long enough to catch a
// filesystem incident rolling across conversations, short enough that unrelated
// losses spread over an afternoon never accumulate into a false outage.
const Window = 2 * time.Minute

// Threshold is how many reports inside the window mean "this is the
// infrastructure, not these conversations". Two could be coincidence; three
// conversations losing context within two minutes is not.
const Threshold = 3

// Breaker decides whether unavailable context is one conversation's loss or a
// symptom of an outage. The zero value is usable and starts closed.
type Breaker struct {
	mu      sync.Mutex
	reports []time.Time
	open    bool
	since   time.Time
	// lastProbe stamps the most recent canary attempt let through while open.
	lastProbe time.Time
	now       func() time.Time
}

// New builds a breaker using the wall clock.
func New() *Breaker { return &Breaker{now: time.Now} }

// NewWithClock builds a breaker with an injected clock, for tests.
func NewWithClock(now func() time.Time) *Breaker { return &Breaker{now: now} }

func (b *Breaker) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// Report records one piece of evidence that context storage is unreachable and
// returns whether the system is now treating unavailability as an outage.
//
// Both edges call this. They share one window and one threshold on purpose:
// three signals in two minutes is infrastructure whether they arrived as failed
// continuations, unprovisionable pods, or a mixture. Splitting the counters
// would let an incident that produced two of each look like no incident at all.
func (b *Breaker) Report() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	cutoff := now.Add(-Window)
	kept := b.reports[:0]
	for _, t := range b.reports {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	b.reports = append(kept, now)
	if len(b.reports) >= Threshold && !b.open {
		b.open, b.since = true, now
	}
	return b.open
}

// Continued records a successful continuation, which is the probe that closes
// the breaker: contexts are reachable again, so held work can proceed.
func (b *Breaker) Continued() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.open, b.reports, b.lastProbe = false, nil, time.Time{}
}

// Open reports whether unavailability is currently being treated as an outage,
// and since when.
func (b *Breaker) Open() (bool, time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.open, b.since
}

// ProbeInterval is how often ONE attempt is let through while the breaker is
// open, to find out whether storage has come back.
const ProbeInterval = time.Minute

// ProbeDue reports whether it is time to let a single canary attempt through,
// and records that it was taken.
//
// The reporting edge closes itself: a run that continues its context proves
// storage is back. The PROVISIONING edge cannot, and that asymmetry is the
// whole reason this exists. While no pod can start there are no runs, so
// nothing would ever report success and a breaker opened by unprovisionable
// pods would stay open forever — turning a mechanism that holds work into one
// that strands it.
//
// ONE attempt, not everyone's: letting every waiting conversation retry is a
// thundering herd against a filesystem that is already unwell, and it is also
// how the original incident spent fifteen hours failing to attach the same
// volume 466 times.
func (b *Breaker) ProbeDue(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.open {
		return true // nothing to probe: attempts are free
	}
	if !b.lastProbe.IsZero() && now.Sub(b.lastProbe) < ProbeInterval {
		return false
	}
	b.lastProbe = now
	return true
}
