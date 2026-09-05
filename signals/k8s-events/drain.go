package main

import (
	"sort"
	"sync"
	"time"
)

// Drain awareness: the fourth suppression axis. While a node is deliberately
// being taken out of service, the events its workloads emit describe the
// drain and not a fault — a rolling reboot cordons a node, every pod on it
// goes NodeNotReady, and grouping by workload turns one planned reboot into
// one conversation per DaemonSet. See proposal.md and design.md.
//
// This is a STATE axis, not a cause-and-effect one: inhibition needs a cause
// EVENT and expires on a TTL, but a cordon is observable ~30s before its first
// consequence and is retracted explicitly when the node is uncordoned — so
// reading the state directly needs no race and no guessed timeout. Evaluated
// BEFORE inhibition and the dwell queue, so the two axes never interact.

// defaultDrainBound is how long a node's drain suppresses before it is
// reported and released. A Pi kernel reboot takes ~15 minutes, a slow drain
// with PodDisruptionBudgets maybe 30 — an hour is generous room for either
// without leaving a forgotten cordon silent all day.
const defaultDrainBound = time.Hour

// nodeTaint is the subset of a core/v1 Taint this adapter reads.
type nodeTaint struct {
	Key    string `json:"key"`
	Effect string `json:"effect"`
}

// conditionTaints mirrors platform/manager/internal/controller/drain.go's set
// EXACTLY, copied rather than imported — modules here are standard-library
// only, so nothing imports across components (structure.md). Both sides pin
// this literal list by test; a key added on one side without the other fails
// that test rather than drifting silently.
//
// These are applied by Kubernetes FROM NODE CONDITIONS and mean "this node is
// unwell", never "this node is being taken down deliberately". Reading them as
// a drain would suppress a node's events during exactly the incident this
// adapter exists to report — a 30-second network blip, or a partition across
// many nodes at once.
var conditionTaints = map[string]bool{
	"node.kubernetes.io/not-ready":           true,
	"node.kubernetes.io/unreachable":         true,
	"node.kubernetes.io/memory-pressure":     true,
	"node.kubernetes.io/disk-pressure":       true,
	"node.kubernetes.io/pid-pressure":        true,
	"node.kubernetes.io/network-unavailable": true,
	"node.kubernetes.io/out-of-service":      true,
}

// nodeDraining reports whether a node is being taken out of service
// DELIBERATELY — cordoned, or tainted for maintenance. Two spellings count,
// because a drain can arrive as either: `kubectl cordon` sets `spec.unschedulable`
// and adds a taint, but a maintenance controller (kured is the canonical case)
// may mark a node for reboot with a taint alone.
func nodeDraining(unschedulable bool, taints []nodeTaint) bool {
	if unschedulable {
		return true
	}
	for _, t := range taints {
		if t.Effect != "NoSchedule" && t.Effect != "NoExecute" {
			continue
		}
		if conditionTaints[t.Key] {
			continue
		}
		return true
	}
	return false
}

// drainEpisode is one node's current drain, from this source's point of view.
type drainEpisode struct {
	start      time.Time
	suppressed int
	// exceeded is set the first tick the drain outlives its bound. From then
	// on this node's events are reported normally again until it stops
	// draining, so the escalation signal fires exactly once per drain.
	exceeded bool
}

// drainSweepResult is one thing the periodic sweep decided about one node,
// for a source.
type drainSweepResult struct {
	node       string
	start      time.Time
	suppressed int
	// exceeded is true on the tick a drain first outlives its bound — the
	// caller emits the escalation signal exactly when this is true.
	exceeded bool
	// released is true on the tick a node stops draining — the caller reports
	// the count and then this node is forgotten, so a later drain starts fresh.
	released bool
}

// drainTracker tracks, per source, which nodes are currently suppressing that
// source's events and since when. It is deliberately per-SOURCE: two sources
// may configure the axis differently (route.drainingNodes, the matchers, the
// bound), so what counts as "draining" for one must not leak into another's
// count.
type drainTracker struct {
	mu    sync.Mutex
	state map[string]map[string]*drainEpisode // source -> node -> episode
}

func newDrainTracker() *drainTracker {
	return &drainTracker{state: map[string]map[string]*drainEpisode{}}
}

// Suppress decides one event against the drain axis. It is the ONLY place an
// episode is created, and only when the node the event is on is ACTUALLY
// draining right now — an ordinary event on an ordinary node must never leave
// a per-node entry behind for the sweep to later puzzle over.
func (dt *drainTracker) Suppress(
	source, node string, rs *ruleSet, cache *objectCache, sig *Signal, now time.Time,
) bool {
	if rs == nil || !rs.drainSuppress || node == "" {
		return false
	}
	draining, known := cache.NodeDraining(node)
	if !known || !draining {
		return false
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()
	byNode := dt.state[source]
	if byNode == nil {
		byNode = map[string]*drainEpisode{}
		dt.state[source] = byNode
	}
	ep := byNode[node]
	if ep == nil {
		ep = &drainEpisode{start: now}
		byNode[node] = ep
	}
	if ep.exceeded {
		return false
	}
	if len(rs.drainMatchers) > 0 && !allMatch(rs.drainMatchers, matchLabels(sig)) {
		return false // narrowed out: on a draining node, but not what this source suppresses
	}
	ep.suppressed++
	return true
}

// Sweep advances one source's tracked episodes against current node state,
// independent of whether any new event arrived. This is what lets a forgotten
// cordon surface even when nothing is left generating events for it to react
// to: without a timer, a bound with nothing left to check it against would
// never fire.
func (dt *drainTracker) Sweep(source string, cache *objectCache, bound time.Duration, now time.Time) []drainSweepResult {
	if bound <= 0 {
		bound = defaultDrainBound
	}
	dt.mu.Lock()
	defer dt.mu.Unlock()
	byNode := dt.state[source]
	var out []drainSweepResult
	for node, ep := range byNode {
		draining, known := cache.NodeDraining(node)
		if !known || !draining {
			out = append(out, drainSweepResult{node: node, start: ep.start, suppressed: ep.suppressed, released: true})
			delete(byNode, node)
			continue
		}
		if !ep.exceeded && now.Sub(ep.start) >= bound {
			ep.exceeded = true
			out = append(out, drainSweepResult{node: node, start: ep.start, suppressed: ep.suppressed, exceeded: true})
		}
	}
	if len(byNode) == 0 {
		delete(dt.state, source)
	}
	return out
}

// Active reports the nodes currently suppressing a source's events (excluding
// ones that already exceeded their bound and are reporting normally again) and
// the running suppressed total, for the Ready-condition message.
func (dt *drainTracker) Active(source string) (nodes []string, total int) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	for node, ep := range dt.state[source] {
		if ep.exceeded {
			continue
		}
		nodes = append(nodes, node)
		total += ep.suppressed
	}
	sort.Strings(nodes)
	return nodes, total
}
