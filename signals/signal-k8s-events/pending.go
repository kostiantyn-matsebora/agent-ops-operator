package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// The dwell queue: an event's `for` window, and the verification that happens
// at the end of it.
//
// A Kubernetes Event is a point-in-time FACT about an object whose lifecycle is
// churn by design. `for` turns the fact into a STATE claim — "this was true,
// and it is still true N minutes later" — which is the only way to tell a
// readiness probe failing on a pod twenty seconds from Ready apart from one
// that will never be Ready.
//
// Entries are keyed by WORKLOAD, not by object. A rolling Deployment produces
// warnings on several pods for one underlying problem, and the useful output is
// one signal saying "three pods of Deployment/api are unhealthy" rather than
// three signals nobody can correlate.

// verdict is the outcome of re-checking an object at the end of its window.
type verdict int

const (
	verdictGone      verdict = iota // the object no longer exists
	verdictHealthy                  // it exists and recovered
	verdictUnhealthy                // it exists and is still broken
	verdictUnknown                  // we cannot say
)

// badWaitingReasons are container waiting states that mean broken rather than
// starting. A container merely "ContainerCreating" is not a problem.
var badWaitingReasons = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"CreateContainerError":       true,
	"CreateContainerConfigError": true,
	"InvalidImageName":           true,
}

// pendingEntry is one workload's accumulating evidence.
type pendingEntry struct {
	source   string
	workload string
	// members are the involved objects seen, keyed "ns/Kind/name".
	members map[string]objectRef
	// reasonCounts is the evidence: how many of each reason.
	reasonCounts map[string]int
	firstSeen    time.Time
	// deadline is set by the FIRST event and never extended. A later event
	// carrying a shorter dwell may SHORTEN it — a strict rule must not be
	// delayed because a lax one happened to arrive first.
	deadline time.Time
	rule     compiledRule
	// recurred records whether any event arrived after the first. It is the
	// whole basis of verification rung 2, for kinds with no health predicate.
	recurred bool
	// sample is the most recent signal, used as the emitted signal's base so
	// labels, fingerprint and title stay exactly as normalize built them.
	sample Signal
}

type objectRef struct {
	ns, kind, name string
}

// minRefractory floors the quiet period after an entry emits. Without a floor,
// a rule with a short dwell would re-report a persistent problem at the tick
// rate.
const minRefractory = time.Minute

// minEscalationDwell floors how quickly breadth may report.
//
// Escalation SHORTENS the dwell; it does not remove it. An earlier version made
// a broad entry due immediately, and that fires on every ordinary slow rollout:
// three replicas starting together are all legitimately not-Ready for the first
// minute, so "three objects failing" is true and premature at the same time.
// Verified against a real 45-second start — it opened a conversation about a
// deployment that was fine.
const minEscalationDwell = time.Minute

// escalationDivisor sets how much breadth shortens the wait. A tenth of the
// full dwell would be indistinguishable from firing immediately; a half would
// barely help. A quarter turns the default 10m Unhealthy rule into 2m30s —
// four times faster for a real outage, and still long enough that a slow start
// resolves first.
const escalationDivisor = 4

type pendingQueue struct {
	mu      sync.Mutex
	entries map[string]*pendingEntry // key: source + "\x00" + workload
	// emitted is the refractory window: when each key last produced a signal.
	//
	// Without it a broken workload re-reports every few SECONDS. The entry is
	// removed when it emits, the next event immediately opens a fresh one, and
	// escalation makes that one due at once because the same three objects are
	// still failing — so escalation, which exists to report an outage FASTER,
	// turns a persistent outage into a stream. Found in a live smoke test, not
	// in a unit test: it needs a real workload that keeps emitting.
	emitted map[string]time.Time

	// health answers the verification ladder.
	health func(objectRef) verdict
	// emit posts a batch for one source.
	emit func(source string, sigs []Signal)
	now  func() time.Time
}

func newPendingQueue(health func(objectRef) verdict, emit func(string, []Signal)) *pendingQueue {
	return &pendingQueue{
		entries: map[string]*pendingEntry{},
		emitted: map[string]time.Time{},
		health:  health,
		emit:    emit,
		now:     time.Now,
	}
}

// Add places a matched signal in the queue, coalescing into the workload's
// existing entry when there is one.
func (q *pendingQueue) Add(source string, sig Signal, rule compiledRule) {
	q.mu.Lock()
	defer q.mu.Unlock()

	workload := sig.Labels["workload"]
	if workload == "" {
		// Unresolvable workload: fall back to the object so unrelated problems
		// do not merge into one entry.
		workload = sig.Labels["kind"] + "/" + sig.Labels["name"]
	}
	key := source + "\x00" + workload
	now := q.now()

	// Refractory: this workload was reported recently, so there is nothing to
	// add by reporting it again. The manager already collapses repeats
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
			source:       source,
			workload:     workload,
			members:      map[string]objectRef{},
			reasonCounts: map[string]int{},
			firstSeen:    now,
			deadline:     now.Add(rule.dwell),
			rule:         rule,
		}
		q.entries[key] = e
	} else {
		e.recurred = true
		if d := now.Add(rule.dwell); d.Before(e.deadline) {
			e.deadline = d
		}
		// keep the strictest escalation threshold seen
		if rule.escalate < e.rule.escalate {
			e.rule.escalate = rule.escalate
		}
	}

	ref := objectRef{ns: sig.Labels["namespace"], kind: sig.Labels["kind"], name: sig.Labels["name"]}
	e.members[ref.ns+"/"+ref.kind+"/"+ref.name] = ref
	e.reasonCounts[sig.Labels["alertname"]]++
	e.sample = sig
}

// dueAt is when this entry may be decided.
//
// Escalation exists because a long dwell is right for one flapping pod and
// wrong for an outage: the premise it rests on — one object misbehaving is
// churn — stops holding when several do at once. But breadth alone does not
// distinguish an outage from a rollout, because a rollout ALSO makes every
// replica unready at once. So breadth shortens the wait rather than removing
// it, and the shortened wait still has to be long enough for a normal start to
// finish.
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
	sort.Slice(out, func(i, j int) bool { return out[i].workload < out[j].workload })
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
		q.emitted[e.source+"\x00"+e.workload] = now
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
//	rung 1  a member has a health predicate  -> is it still unhealthy?
//	rung 2  no predicate, but the object exists -> did the event RECUR?
//	rung 3  we cannot even say it exists -> EMIT (fail open)
//
// An entry survives if ANY member is still unhealthy: a Deployment with one
// broken pod out of three is a broken Deployment.
func (q *pendingQueue) decide(e *pendingEntry, now time.Time) (Signal, bool) {
	anyUnhealthy, anyUnknown := false, false
	for _, ref := range e.members {
		switch q.health(ref) {
		case verdictUnhealthy:
			anyUnhealthy = true
		case verdictUnknown:
			anyUnknown = true
		case verdictGone, verdictHealthy:
			// this member resolved itself — the rollout case
		}
	}

	switch {
	case anyUnhealthy:
		// rung 1: confirmed still broken
	case anyUnknown && e.recurred:
		// rung 2: no predicate, but it kept happening — still live
	case anyUnknown && !e.recurred:
		// rung 2: no predicate and it went quiet — it stopped
		return Signal{}, false
	default:
		// every member is gone or healthy: churn
		return Signal{}, false
	}

	sig := e.sample
	sig.Payload = e.evidence(now)
	if len(e.members) > 1 {
		sig.Title = fmt.Sprintf("%s (%d objects)", sig.Title, len(e.members))
	}
	return sig, true
}

// evidence renders the accumulated burst. This is the reason dwell is a
// feature rather than only a filter: what emits carries strictly more than the
// N separate signals it replaced.
func (e *pendingEntry) evidence(now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — unhealthy for %s\n\n", e.workload, now.Sub(e.firstSeen).Round(time.Second))

	reasons := make([]string, 0, len(e.reasonCounts))
	for r := range e.reasonCounts {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if e.reasonCounts[reasons[i]] != e.reasonCounts[reasons[j]] {
			return e.reasonCounts[reasons[i]] > e.reasonCounts[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	for _, r := range reasons {
		fmt.Fprintf(&b, "  %-24s ×%d\n", r, e.reasonCounts[r])
	}

	names := make([]string, 0, len(e.members))
	for _, ref := range e.members {
		names = append(names, ref.kind+"/"+ref.name)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "\n  objects (%d): %s\n", len(names), strings.Join(names, ", "))
	fmt.Fprintf(&b, "  first seen %s, still true at %s\n",
		e.firstSeen.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	if len(e.sample.Payload) > 0 {
		fmt.Fprintf(&b, "\nmost recent event:\n%s\n", e.sample.Payload)
	}
	return b.String()
}

// health is the adapter's verification ladder over the object cache.
//
// Only Pod carries a health predicate. Node, Job, PVC and every other kind fall
// to rung 2 (recurrence), deliberately: giving them predicates would mean
// watching nodes, jobs and PVCs, and the reasons that concern them
// (NodeNotReady, disk pressure, BackoffLimitExceeded) ship with `for: 0` and so
// are never verified at all. Paying three more RBAC grants for a path the
// defaults never take is not a trade worth making.
func (a *adapter) health(ref objectRef) verdict {
	if a.cache == nil {
		return verdictUnknown
	}
	oi, known := a.cache.Get(ref.ns, ref.kind, ref.name)
	if !known {
		return verdictUnknown
	}
	if oi == nil {
		return verdictGone
	}
	if ref.kind != "Pod" {
		return verdictUnknown
	}
	switch oi.Phase {
	case "Failed":
		return verdictUnhealthy
	case "Succeeded":
		return verdictHealthy
	}
	for _, w := range oi.WaitingReasons {
		if badWaitingReasons[w] {
			return verdictUnhealthy
		}
	}
	if !oi.Ready {
		return verdictUnhealthy
	}
	return verdictHealthy
}
