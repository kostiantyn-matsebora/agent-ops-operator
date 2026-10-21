package main

import (
	"sync"
	"time"
)

// The owner hold: a short, bounded wait for the object cache to catch up.
//
// A pod's event can arrive before the cache holds the pod or its owning
// ReplicaSet — a fresh adapter whose first list has not landed, or a rollout
// whose new ReplicaSet the watch has not delivered yet. Normalizing it then
// would key the initiationId on the object (or on a half-walked owner chain),
// opening a per-pod conversation beside the workload's own. So the event waits
// instead, and is released the moment the chain settles or the wait expires —
// a momentary cache lag delays the signal, and never re-mints per-object
// correlation for a workload that is merely late.
//
// The bound is ownerResolveWait: one tick of the main loop, which is the cadence
// cache watchers are reconciled on. By then the watcher covering the event's
// scope has been started and has listed, so the owner either resolves or the
// object is gone — and a gone object is emitted under its own name, exactly as
// a confirmed-unowned one is.

// ownerResolveWait bounds the hold. Reused rather than invented: it is the
// cache's own refresh cadence, not a tuning knob.
const ownerResolveWait = refreshEvery

// heldEvent is one event waiting for its owner chain to settle.
type heldEvent struct {
	source string
	ev     Event
	until  time.Time
}

// ownerHold holds events whose owning workload the cache cannot answer yet.
type ownerHold struct {
	mu    sync.Mutex
	items []heldEvent
	wait  time.Duration
}

func newOwnerHold(wait time.Duration) *ownerHold {
	return &ownerHold{wait: wait}
}

// Hold places an event in the hold for at most the configured wait.
func (h *ownerHold) Hold(source string, ev Event, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = append(h.items, heldEvent{source: source, ev: ev, until: now.Add(h.wait)})
}

// Release removes and returns every held event that is ready: its owner chain
// has settled, or its wait has expired. Order of arrival is preserved.
func (h *ownerHold) Release(now time.Time, settled func(*Event) bool) []heldEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []heldEvent
	kept := h.items[:0]
	for _, it := range h.items {
		if !now.Before(it.until) || settled(&it.ev) {
			out = append(out, it)
			continue
		}
		kept = append(kept, it)
	}
	h.items = kept
	return out
}

// Len reports how many events are waiting.
func (h *ownerHold) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.items)
}

// awaitsOwner reports whether an event must wait for the cache before it is
// normalized: its kind is one the cache tracks AND carries an owner chain to
// walk, and that chain has not settled yet.
//
// A Node is tracked (drain.go) but is never held: it carries no owner
// reference, so it is its own workload whatever the cache says, and NodeNotReady
// ships with `for: 0` precisely so that it is never delayed.
//
// A nil hold or a nil cache holds nothing — tests that drive deliver against a
// cold cache construct the adapter without one.
func (a *adapter) awaitsOwner(ev *Event) bool {
	if a.held == nil || a.cache == nil {
		return false
	}
	kind := ev.InvolvedObject.Kind
	if kind == "Node" || !a.cache.tracks(kind) || ev.InvolvedObject.Name == "" {
		return false
	}
	return !a.cache.Settled(ev.Namespace(), kind, ev.InvolvedObject.Name)
}
