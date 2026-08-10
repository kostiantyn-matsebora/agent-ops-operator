// Package ingest: signal → conversation grouping and fingerprint cooldown.
// Semantics ported from claude-runner v0.6 (single source of behavioral truth).
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultSignatureLabels compose a signature when a SignalSource doesn't set its own.
var DefaultSignatureLabels = []string{"alertgroup", "alertname", "namespace"}

// Signature builds the grouping key from labels: values of keys joined by "/",
// missing values empty (matches v0.6: alertgroup/alertname/namespace).
func Signature(labels map[string]string, keys []string) string {
	if len(keys) == 0 {
		keys = DefaultSignatureLabels
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, labels[k])
	}
	return strings.Join(parts, "/")
}

// SignatureHash is a label-safe digest of a signature (labels forbid "/").
func SignatureHash(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:])[:16]
}

// Cooldown suppresses fingerprints seen within a TTL window.
//
// The map is the HOT PATH, not the record: it is loaded from the owning
// SignalSource on first use per source and written back whenever a fingerprint
// is newly admitted, so a manager restart mid-incident no longer re-opens
// conversations for signals still inside their window. Entries() is what the
// caller persists.
type Cooldown struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	now  func() time.Time
}

// Entry is one recorded suppression: a fingerprint and when it was admitted.
type Entry struct {
	Fingerprint string
	At          time.Time
}

// MaxCooldownEntries bounds what one source records. Exceeding it evicts the
// OLDEST suppression, which degrades to today's behavior for that fingerprint —
// one duplicate investigation — rather than to an unbounded object. A source
// churning through more distinct fingerprints than this is a reason to look at
// its grouping configuration, not at the bound.
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
		c.seen[e.Fingerprint] = e.At
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
	for fp, at := range c.seen {
		if now.Sub(at) >= c.ttl {
			continue
		}
		out = append(out, Entry{Fingerprint: fp, At: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > MaxCooldownEntries {
		out = out[:MaxCooldownEntries]
	}
	return out
}

// Fresh returns the subset of fingerprints not seen within the window and
// records them as seen. Old entries are pruned (bounded memory).
func (c *Cooldown) Fresh(fingerprints []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	var out []string
	for _, fp := range fingerprints {
		if last, ok := c.seen[fp]; ok && now.Sub(last) < c.ttl {
			continue
		}
		c.seen[fp] = now
		out = append(out, fp)
	}
	for fp, ts := range c.seen {
		if now.Sub(ts) > 7*24*time.Hour {
			delete(c.seen, fp)
		}
	}
	return out
}

// Stats reports how many fingerprints are currently SUPPRESSED (inside the
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
