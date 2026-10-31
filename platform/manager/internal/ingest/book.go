package ingest

import (
	"sort"
	"sync"
	"time"
)

// CooldownBook holds one Cooldown per source, shared by every consumer of the
// signal pool and by the status surfaces that report on it. Concurrent
// consumers share it (signal-persistence D-3), so it is the one place the
// per-source map is guarded.
//
// Process memory only, on purpose: the record is the SignalSource's
// status.cooldown, recovered into a Cooldown on first use after a restart or
// a leader change.
type CooldownBook struct {
	mu      sync.Mutex
	byName  map[string]*Cooldown
	lockFor map[string]*sync.Mutex
}

// NewCooldownBook builds an empty book.
func NewCooldownBook() *CooldownBook {
	return &CooldownBook{byName: map[string]*Cooldown{}, lockFor: map[string]*sync.Mutex{}}
}

// For returns the source's Cooldown, building it with build on first use. A
// build that returns nil (its record could not be read) leaves the source
// unbuilt, so the next call builds again instead of keeping an empty window.
//
// build runs OUTSIDE the book's mutex: it reads the state store, and holding
// the mutex across that read would stall Lock and Stats for every source behind
// one slow backend. The caller serializes one source with Lock, so two builds
// of one source do not normally overlap; the second look under the mutex keeps
// the first stored window if they ever do.
func (b *CooldownBook) For(source string, build func() *Cooldown) *Cooldown {
	b.mu.Lock()
	cd := b.byName[source]
	b.mu.Unlock()
	if cd != nil {
		return cd
	}
	built := build()
	if built == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing := b.byName[source]; existing != nil {
		return existing
	}
	b.byName[source] = built
	return built
}

// Lock serializes everything that checks AND records one source's cooldown, so
// two consumers admitting different ids of one source never overwrite each
// other's recorded window. The returned func releases it.
func (b *CooldownBook) Lock(source string) (unlock func()) {
	b.mu.Lock()
	m := b.lockFor[source]
	if m == nil {
		m = &sync.Mutex{}
		b.lockFor[source] = m
	}
	b.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// Stat is one source's cooldown snapshot.
type Stat struct {
	Source     string
	Suppressed int
	Window     time.Duration
}

// Stats reports every source's cooldown, sorted by source name. A nil book
// reports nothing.
func (b *CooldownBook) Stats() []Stat {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Stat, 0, len(b.byName))
	for name, cd := range b.byName {
		n, window := cd.Stats()
		out = append(out, Stat{Source: name, Suppressed: n, Window: window})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// Release forgets an id admitted by Fresh whose admission could not be
// recorded, so a repeated pass of the same Signal is not suppressed by its own
// earlier attempt.
func (c *Cooldown) Release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.seen, id)
}
