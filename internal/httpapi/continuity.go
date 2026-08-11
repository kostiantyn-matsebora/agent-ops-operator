package httpapi

import (
	"sync"
	"time"
)

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

// continuityWindow is how far back reports are counted. Long enough to catch a
// filesystem incident rolling across conversations, short enough that unrelated
// losses spread over an afternoon never accumulate into a false outage.
const continuityWindow = 2 * time.Minute

// continuityThreshold is how many reports inside the window mean "this is the
// infrastructure, not these conversations". Two could be coincidence; three
// conversations losing context within two minutes is not.
const continuityThreshold = 3

// continuityBreaker decides whether an unavailable-context report is one
// conversation's loss or a symptom of an outage.
type continuityBreaker struct {
	mu      sync.Mutex
	reports []time.Time
	open    bool
	since   time.Time
	now     func() time.Time
}

func newContinuityBreaker() *continuityBreaker {
	return &continuityBreaker{now: time.Now}
}

func (b *continuityBreaker) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// Report records one unavailable-context report and returns whether the system
// is now treating unavailability as an outage.
func (b *continuityBreaker) Report() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	cutoff := now.Add(-continuityWindow)
	kept := b.reports[:0]
	for _, t := range b.reports {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	b.reports = append(kept, now)
	if len(b.reports) >= continuityThreshold && !b.open {
		b.open, b.since = true, now
	}
	return b.open
}

// Continued records a successful continuation, which is the probe that closes
// the breaker: contexts are reachable again, so held work can proceed.
func (b *continuityBreaker) Continued() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.open, b.reports = false, nil
}

// Open reports whether unavailability is currently being treated as an outage.
func (b *continuityBreaker) Open() (bool, time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.open, b.since
}

// breaker lazily builds the breaker, so a zero-valued Server (tests) works.
func (s *Server) breaker() *continuityBreaker {
	s.breakerOnce.Do(func() { s.continuity = newContinuityBreaker() })
	return s.continuity
}
