package main

import (
	"sync"
	"time"
)

// Inhibition: suppress the CONSEQUENCES of a cause that is already reported.
//
// A hub or gateway losing its connection makes every device behind it log
// errors. Without inhibition the hub signal and a signal per integration all
// fire, and the per-integration ones are pure noise — they describe the same
// incident from the wrong end.
//
// Evaluated BEFORE the dwell queue, so an inhibited event never occupies it.

// activeTTL bounds how long a cause keeps suppressing its consequences.
// Without it, one hub error would silence everything behind it forever: causes
// are observed as LOG RECORDS, and a log record never announces that it
// stopped.
const activeTTL = 10 * time.Minute

// inhibitor tracks recently-seen inhibiting causes per source.
type inhibitor struct {
	mu sync.Mutex
	// active maps source -> the label sets of causes seen within the TTL.
	active map[string][]activeCause
	now    func() time.Time
}

type activeCause struct {
	labels map[string]string
	seen   time.Time
}

func newInhibitor() *inhibitor {
	return &inhibitor{active: map[string][]activeCause{}, now: time.Now}
}

// Observe records a signal as a potential cause when it matches any rule's
// sourceMatchers. Every signal is offered: a cause is only a cause relative to
// a rule.
func (in *inhibitor) Observe(source string, rs *ruleSet, sig *Signal) {
	if rs == nil || len(rs.inhibits) == 0 {
		return
	}
	labels := matchLabels(sig)
	for _, ir := range rs.inhibits {
		if allMatch(ir.source, labels) {
			in.mu.Lock()
			in.active[source] = append(in.prune(source, in.now()), activeCause{labels: labels, seen: in.now()})
			in.mu.Unlock()
			return
		}
	}
}

// Inhibited reports whether this signal is a consequence of an active cause.
func (in *inhibitor) Inhibited(source string, rs *ruleSet, sig *Signal) bool {
	if rs == nil || len(rs.inhibits) == 0 {
		return false
	}
	labels := matchLabels(sig)
	in.mu.Lock()
	defer in.mu.Unlock()
	causes := in.prune(source, in.now())
	in.active[source] = causes

	for _, ir := range rs.inhibits {
		if !allMatch(ir.target, labels) {
			continue
		}
		for _, c := range causes {
			// A cause must not inhibit itself: a signal matching both halves of
			// a rule would otherwise suppress the very thing being reported.
			if allMatch(ir.source, labels) {
				continue
			}
			if equalLabels(ir.equal, c.labels, labels) {
				return true
			}
		}
	}
	return false
}

// prune drops causes older than the TTL. Caller holds the lock.
func (in *inhibitor) prune(source string, now time.Time) []activeCause {
	causes := in.active[source]
	kept := causes[:0]
	for _, c := range causes {
		if now.Sub(c.seen) < activeTTL {
			kept = append(kept, c)
		}
	}
	return kept
}

// equalLabels reports whether every label named in `equal` has the same value
// on both. An empty `equal` means the cause inhibits its targets globally,
// which is Alertmanager's behavior too.
func equalLabels(equal []string, cause, target map[string]string) bool {
	for _, k := range equal {
		if cause[k] != target[k] {
			return false
		}
	}
	return true
}
