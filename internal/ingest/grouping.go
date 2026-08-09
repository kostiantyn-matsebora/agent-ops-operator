// Package ingest: signal → conversation grouping and fingerprint cooldown.
// Semantics ported from claude-runner v0.6 (single source of behavioral truth).
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
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

// Cooldown suppresses fingerprints seen within a TTL window (in-memory; the
// manager is a singleton via leader election — restart resets the window,
// which only risks one duplicate investigation, matching v0.6's tolerance).
type Cooldown struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	now  func() time.Time
}

// NewCooldown builds a store with the given suppression window.
func NewCooldown(ttl time.Duration) *Cooldown {
	return &Cooldown{seen: map[string]time.Time{}, ttl: ttl, now: time.Now}
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
