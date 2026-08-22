package main

import (
	"sync"
	"time"
)

// Per-source emit cap: a local backstop, deliberately small in ambition.
//
// It turns a fast runaway from "etcd fills up" into "a condition says something
// is wrong". It is NOT the general answer — a manager-side limit on
// CONVERSATION creation across every source type is (see the signal-triage
// change). This exists so that in the interval before that lands, a runaway
// this adapter causes is visible and bounded rather than silent and unbounded.
//
// Clipping is never silent. A dropped signal nobody is told about is how an
// incident goes missing.

const (
	emitCapWindow     = time.Minute
	defaultEmitPerMin = 60
)

// emitCap counts signals per source in a rolling window.
type emitCap struct {
	mu      sync.Mutex
	limit   int
	windows map[string]*capWindow
	now     func() time.Time
}

type capWindow struct {
	start   time.Time
	emitted int
	clipped int
}

func newEmitCap(limit int) *emitCap {
	if limit <= 0 {
		limit = defaultEmitPerMin
	}
	return &emitCap{limit: limit, windows: map[string]*capWindow{}, now: time.Now}
}

// Allow returns the prefix of sigs that may be emitted and how many were
// clipped in this window so far. A source under its cap is untouched.
func (c *emitCap) Allow(source string, sigs []Signal) ([]Signal, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	w := c.windows[source]
	if w == nil || now.Sub(w.start) >= emitCapWindow {
		w = &capWindow{start: now}
		c.windows[source] = w
	}
	room := c.limit - w.emitted
	if room <= 0 {
		w.clipped += len(sigs)
		return nil, w.clipped
	}
	if len(sigs) <= room {
		w.emitted += len(sigs)
		return sigs, w.clipped
	}
	w.clipped += len(sigs) - room
	w.emitted += room
	return sigs[:room], w.clipped
}
