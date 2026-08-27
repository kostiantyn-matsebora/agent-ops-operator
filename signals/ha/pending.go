package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// The dwell queue: a record's `for` window, and the verification that happens
// at the end of it.
//
// A log record is a point-in-time FACT about a system whose noise is churn by
// design — a device sleeps, a hub reconnects, an integration retries its setup
// and succeeds. `for` turns the fact into a STATE claim: "this was true, and it
// is still true N minutes later". That is the only way to tell a Z-Wave stick
// that dropped one frame apart from one that is gone.
//
// Entries are keyed by INTEGRATION, not by record. One failing integration logs
// from several code paths for one underlying problem, and the useful output is
// one signal saying "zwave_js is failing in three places" rather than three
// signals nobody can correlate.

// verdict is the outcome of re-checking a condition at the end of its window.
type verdict int

const (
	verdictGone      verdict = iota // the condition's subject no longer exists
	verdictHealthy                  // it exists and recovered
	verdictUnhealthy                // it exists and is still broken
	verdictUnknown                  // we cannot say
)

// pendingEntry is one integration's accumulating evidence.
type pendingEntry struct {
	source      string
	integration string
	// members are the record identities seen, keyed by logger@location.
	members map[string]recordRef
	// levelCounts is the evidence: how many of each level.
	levelCounts map[string]int
	firstSeen   time.Time
	// deadline is set by the FIRST record and never extended. A later record
	// carrying a shorter dwell may SHORTEN it — a strict rule must not be
	// delayed because a lax one happened to arrive first.
	deadline time.Time
	rule     compiledRule
	// lastSeen is when the most recent record arrived. It is the whole basis
	// of verification rung 2, where no health predicate exists: not WHETHER
	// the record recurred, but whether it was still recurring as the window
	// closed. A network blip that logs for thirty seconds and stops has
	// recurred, and is the transient the dwell exists to drop.
	lastSeen time.Time
	// sample is the most recent signal, used as the emitted signal's base so
	// labels, fingerprint and title stay exactly as normalize built them.
	sample Signal
}

// recordRef identifies one recurring log record and remembers how many times
// Home Assistant had seen it when the window opened. The count is what makes
// the re-check possible: a record whose count has risen is still happening.
type recordRef struct {
	// source names the SignalSource this record came from, because the health
	// snapshot is per source: two sources may watch two different houses.
	source      string
	integration string
	logger      string
	location    string
	countAtOpen int
}

func (r recordRef) key() string { return r.logger + "@" + r.location }

// minRefractory floors the quiet period after an entry emits. Without a floor,
// a rule with a short dwell would re-report a persistent problem at the tick
// rate.
const minRefractory = time.Minute

// minEscalationDwell floors how quickly breadth may report. Escalation SHORTENS
// the dwell; it does not remove it. Home Assistant logs a burst from several
// code paths during an ordinary restart, and every one of them recovers.
const minEscalationDwell = time.Minute

// escalationDivisor sets how much breadth shortens the wait — a quarter, so a
// 10m rule reports a real outage in 2m30s and still outlasts a restart.
const escalationDivisor = 4

// closingWindowDivisor and minClosingWindow define the CLOSING part of a dwell
// — the tail of the window a record must still be arriving in for the re-check
// to believe it. A third of the window actually waited, floored at thirty
// seconds, derived rather than configured. signals/k8s-events/pending.go
// carries the same two numbers, and they are changed together.
const closingWindowDivisor = 3
const minClosingWindow = 30 * time.Second

// closingWindow is how long before `now` the closing part of the window began.
func closingWindow(waited time.Duration) time.Duration {
	w := waited / closingWindowDivisor
	if w < minClosingWindow {
		w = minClosingWindow
	}
	return w
}

// closingSince is the instant the closing part of this entry's window began —
// the `since` the health predicate is asked about.
func (e *pendingEntry) closingSince(now time.Time) time.Time {
	return now.Add(-closingWindow(now.Sub(e.firstSeen)))
}

// stillRecurring is rung 2: was the record still arriving as the window closed?
func (e *pendingEntry) stillRecurring(now time.Time) bool {
	if e.lastSeen.Equal(e.firstSeen) {
		return false // never recurred at all
	}
	return !e.lastSeen.Before(e.closingSince(now))
}

type pendingQueue struct {
	mu      sync.Mutex
	entries map[string]*pendingEntry // key: source + "\x00" + integration
	// emitted is the refractory window: when each key last produced a signal.
	// Without it a broken integration re-reports every few seconds, because the
	// entry is removed when it emits and the next record immediately opens a
	// fresh one that escalation makes due at once.
	emitted map[string]time.Time

	// health answers the verification ladder: is this record's subject still
	// broken, counting only what happened SINCE the given instant? The
	// instant is the start of the window's closing part, so a log count that
	// rose early in the window and then stopped rising is not "still broken".
	health func(ref recordRef, since time.Time) verdict
	// emit posts a batch for one source.
	emit func(source string, sigs []Signal)
	now  func() time.Time
}

func newPendingQueue(health func(recordRef, time.Time) verdict, emit func(string, []Signal)) *pendingQueue {
	return &pendingQueue{
		entries: map[string]*pendingEntry{},
		emitted: map[string]time.Time{},
		health:  health,
		emit:    emit,
		now:     time.Now,
	}
}

// Add places a matched signal in the queue, coalescing into the integration's
// existing entry when there is one.
func (q *pendingQueue) Add(source string, sig Signal, rule compiledRule, count int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	integration := sig.Labels["integration"]
	if integration == "" {
		integration = sig.Labels["logger"]
	}
	key := source + "\x00" + integration
	now := q.now()

	// Refractory: this integration was reported recently, so there is nothing
	// to add by reporting it again. The manager already collapses repeats
	// (fingerprint cooldown, signature window reuse) — re-posting only spends
	// round trips.
	if last, ok := q.emitted[key]; ok {
		quiet := rule.dwell
		if quiet < minRefractory {
			quiet = minRefractory
		}
		if now.Sub(last) < quiet {
			return
		}
		delete(q.emitted, key)
	}

	e, ok := q.entries[key]
	if !ok {
		e = &pendingEntry{
			source:      source,
			integration: integration,
			members:     map[string]recordRef{},
			levelCounts: map[string]int{},
			firstSeen:   now,
			lastSeen:    now,
			deadline:    now.Add(rule.dwell),
			rule:        rule,
		}
		q.entries[key] = e
	} else {
		e.lastSeen = now
		if d := now.Add(rule.dwell); d.Before(e.deadline) {
			e.deadline = d
		}
		if rule.escalate < e.rule.escalate {
			e.rule.escalate = rule.escalate
		}
	}

	ref := recordRef{
		source:      source,
		integration: integration,
		logger:      sig.Labels["logger"],
		location:    sig.Labels["location"],
		countAtOpen: count,
	}
	// Keep the count from the FIRST sighting: the re-check asks whether it has
	// risen since the window opened, and overwriting it would compare a value
	// with itself.
	if prev, seen := e.members[ref.key()]; seen {
		ref.countAtOpen = prev.countAtOpen
	}
	e.members[ref.key()] = ref
	e.levelCounts[sig.Labels["level"]]++
	e.sample = sig
}

// dueAt is when this entry may be decided. Breadth shortens the wait rather
// than removing it: several code paths failing at once is also what a restart
// looks like, and only duration separates the two.
func (e *pendingEntry) dueAt() time.Time {
	if len(e.members) < e.rule.escalate {
		return e.deadline
	}
	esc := e.rule.dwell / escalationDivisor
	if esc < minEscalationDwell {
		esc = minEscalationDwell
	}
	if esc > e.rule.dwell {
		esc = e.rule.dwell // never delay a rule that is already stricter
	}
	if d := e.firstSeen.Add(esc); d.Before(e.deadline) {
		return d
	}
	return e.deadline
}

// due returns the entries ready to be decided.
func (q *pendingQueue) due(now time.Time) []*pendingEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*pendingEntry
	for key, e := range q.entries {
		if !now.Before(e.dueAt()) {
			out = append(out, e)
			delete(q.entries, key)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].integration < out[j].integration })
	return out
}

// Flush decides every due entry and emits the survivors.
func (q *pendingQueue) Flush(now time.Time) {
	bySource := map[string][]Signal{}
	for _, e := range q.due(now) {
		sig, ok := q.decide(e, now)
		if !ok {
			continue
		}
		// Start this key's refractory window. An entry that DROPPED starts
		// none: it reported nothing, so there is nothing to be quiet about.
		q.mu.Lock()
		q.emitted[e.source+"\x00"+e.integration] = now
		for k, t := range q.emitted {
			if now.Sub(t) > time.Hour {
				delete(q.emitted, k)
			}
		}
		q.mu.Unlock()
		bySource[e.source] = append(bySource[e.source], sig)
	}
	for source, sigs := range bySource {
		q.emit(source, sigs)
	}
}

// decide runs the verification ladder over an entry's members.
//
//	rung 1  the integration has a health predicate  -> is it still broken?
//	rung 2  no predicate, but we can read the log   -> was it STILL RECURRING at the close?
//	rung 3  we cannot say at all                    -> EMIT (fail open)
//
// An entry survives if ANY member is still unhealthy: an integration with one
// broken code path is a broken integration.
func (q *pendingQueue) decide(e *pendingEntry, now time.Time) (Signal, bool) {
	anyUnhealthy, anyUnknown := false, false
	since := e.closingSince(now)
	for _, ref := range e.members {
		switch q.health(ref, since) {
		case verdictUnhealthy:
			anyUnhealthy = true
		case verdictUnknown:
			anyUnknown = true
		case verdictGone, verdictHealthy:
			// this member resolved itself — the retry-and-succeed case
		}
	}

	switch {
	case anyUnhealthy:
		// rung 1: confirmed still broken
	case anyUnknown && e.stillRecurring(now):
		// rung 2: no predicate, but it was still happening at the close — live
	case anyUnknown:
		// rung 2: no predicate and it went quiet before the close — it stopped.
		// Logged because this drop looks exactly like the live case until the
		// timeline is on the record.
		log.Printf("dwell: %s dropped as quiet — last record %s before the window closed (waited %s)",
			e.integration, now.Sub(e.lastSeen).Round(time.Second), now.Sub(e.firstSeen).Round(time.Second))
		return Signal{}, false
	default:
		// every member recovered: churn
		return Signal{}, false
	}

	sig := e.sample
	sig.Payload = e.evidence(now)
	if len(e.members) > 1 {
		sig.Title = fmt.Sprintf("%s (%d places)", sig.Title, len(e.members))
	}
	return sig, true
}

// evidence renders the accumulated burst. This is why dwell is a feature rather
// than only a filter: what emits carries strictly more than the N separate
// signals it replaced.
func (e *pendingEntry) evidence(now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — failing for %s\n\n", e.integration, now.Sub(e.firstSeen).Round(time.Second))

	levels := make([]string, 0, len(e.levelCounts))
	for l := range e.levelCounts {
		levels = append(levels, l)
	}
	sort.Slice(levels, func(i, j int) bool {
		if e.levelCounts[levels[i]] != e.levelCounts[levels[j]] {
			return e.levelCounts[levels[i]] > e.levelCounts[levels[j]]
		}
		return levels[i] < levels[j]
	})
	for _, l := range levels {
		fmt.Fprintf(&b, "  %-10s ×%d\n", l, e.levelCounts[l])
	}

	places := make([]string, 0, len(e.members))
	for _, ref := range e.members {
		places = append(places, ref.key())
	}
	sort.Strings(places)
	fmt.Fprintf(&b, "\n  places (%d): %s\n", len(places), strings.Join(places, ", "))
	fmt.Fprintf(&b, "  first seen %s, still true at %s\n",
		e.firstSeen.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "  last seen %s before the window closed\n", now.Sub(e.lastSeen).Round(time.Second))
	if len(e.sample.Payload) > 0 {
		fmt.Fprintf(&b, "\nmost recent record:\n%s\n", e.sample.Payload)
	}
	return b.String()
}

// HasEntries reports whether anything is waiting out a dwell. The verification
// snapshot is only refreshed when something is: a quiet install must not issue
// a command every few seconds to verify nothing.
func (q *pendingQueue) HasEntries() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries) > 0
}
