// Package ingest: initiationId cooldown — the one piece of per-source policy
// the manager still owns. Grouping/signature derivation is gone: equality
// routing matches a signal's initiationId against a conversation's own
// spec.conversationId directly, with no manager-side computation at all
// (signal-correlation-id).
package ingest

import (
	"sort"
	"sync"
	"time"
)

// Cooldown suppresses initiationIds seen within a TTL window.
//
// The map is the HOT PATH, not the record: it is loaded from the owning
// SignalSource on first use per source and written back whenever an id is
// newly admitted, so a manager restart mid-incident no longer re-opens
// conversations for signals still inside their window. Entries() is what the
// caller persists.
type Cooldown struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	now  func() time.Time
}

// Entry is one recorded suppression: an initiationId and when it was
// admitted.
type Entry struct {
	InitiationID string
	At           time.Time
}

// MaxCooldownEntries bounds what one source records. Exceeding it evicts the
// OLDEST suppression, which degrades to today's behavior for that id — one
// duplicate conversation — rather than to an unbounded object. A source
// churning through more distinct ids than this is a reason to look at its
// cooldownHours, not at the bound.
const MaxCooldownEntries = 200

// NewCooldown builds a store with the given suppression window.
func NewCooldown(ttl time.Duration) *Cooldown {
	return &Cooldown{seen: map[string]time.Time{}, ttl: ttl, now: time.Now}
}

// NewCooldownFrom rebuilds a store from recorded entries — the recovery path
// after a restart.
//
// Recovered entries are NOT filtered here: an entry past its window suppresses
// nothing in Fresh and is dropped by Entries before it could be written back,
// so filtering at construction would only add a second reading of the clock —
// and a wrong one, since the caller may not have wound it yet.
func NewCooldownFrom(ttl time.Duration, entries []Entry) *Cooldown {
	c := NewCooldown(ttl)
	for _, e := range entries {
		c.seen[e.InitiationID] = e.At
	}
	return c
}

// Entries reports what should be recorded on the source: everything still
// inside the window, newest first, capped at MaxCooldownEntries.
//
// Nil for a disabled window (cooldownHours: 0, the chat default) — there is no
// suppression to recover, so recording one would be noise on the object.
func (c *Cooldown) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ttl <= 0 {
		return nil
	}
	now := c.now()
	out := make([]Entry, 0, len(c.seen))
	for id, at := range c.seen {
		if now.Sub(at) >= c.ttl {
			continue
		}
		out = append(out, Entry{InitiationID: id, At: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > MaxCooldownEntries {
		out = out[:MaxCooldownEntries]
	}
	return out
}

// Fresh returns the subset of initiationIds not seen within the window and
// records them as seen. Old entries are pruned (bounded memory).
func (c *Cooldown) Fresh(ids []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	var out []string
	for _, id := range ids {
		if last, ok := c.seen[id]; ok && now.Sub(last) < c.ttl {
			continue
		}
		c.seen[id] = now
		out = append(out, id)
	}
	for id, ts := range c.seen {
		if now.Sub(ts) > 7*24*time.Hour {
			delete(c.seen, id)
		}
	}
	return out
}

// Stats reports how many initiationIds are currently SUPPRESSED (inside the
// window) and the window itself. A cooldown lane looks exactly like an idle one
// from outside, so this is what lets an operator tell "nothing is happening"
// from "everything is being swallowed".
func (c *Cooldown) Stats() (suppressed int, window time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for _, ts := range c.seen {
		if now.Sub(ts) < c.ttl {
			suppressed++
		}
	}
	return suppressed, c.ttl
}
