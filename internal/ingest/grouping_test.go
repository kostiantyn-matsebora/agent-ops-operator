package ingest

import (
	"strconv"
	"testing"
	"time"
)

func TestSignature(t *testing.T) {
	labels := map[string]string{"alertgroup": "home-assistant", "alertname": "HAErrorBurst", "namespace": "ha"}
	if got := Signature(labels, nil); got != "home-assistant/HAErrorBurst/ha" {
		t.Fatalf("default keys: got %q", got)
	}
	if got := Signature(labels, []string{"alertname"}); got != "HAErrorBurst" {
		t.Fatalf("custom keys: got %q", got)
	}
	if got := Signature(map[string]string{"alertname": "X"}, nil); got != "/X/" {
		t.Fatalf("missing labels keep positions: got %q", got)
	}
}

func TestSignatureHashLabelSafe(t *testing.T) {
	h := SignatureHash("a/b/c")
	if len(h) != 16 {
		t.Fatalf("hash length: %q", h)
	}
	for _, r := range h {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			t.Fatalf("non-hex rune %q", r)
		}
	}
}

func TestCooldown(t *testing.T) {
	c := NewCooldown(6 * time.Hour)
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return base }

	fresh := c.Fresh([]string{"fp1", "fp2"})
	if len(fresh) != 2 {
		t.Fatalf("first sight: %v", fresh)
	}
	fresh = c.Fresh([]string{"fp1", "fp3"})
	if len(fresh) != 1 || fresh[0] != "fp3" {
		t.Fatalf("within window fp1 suppressed: %v", fresh)
	}
	c.now = func() time.Time { return base.Add(7 * time.Hour) }
	fresh = c.Fresh([]string{"fp1"})
	if len(fresh) != 1 {
		t.Fatalf("after window fp1 fresh again: %v", fresh)
	}
}

// The point of recording the window: a restart mid-incident must not re-open
// conversations for signals still being suppressed.
func TestCooldownSurvivesARestart(t *testing.T) {
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	before := NewCooldown(6 * time.Hour)
	before.now = func() time.Time { return base }
	before.Fresh([]string{"fp1", "fp2"})

	// restart: a new process rebuilds from what was recorded
	after := NewCooldownFrom(6*time.Hour, before.Entries())
	after.now = func() time.Time { return base.Add(time.Hour) }

	if fresh := after.Fresh([]string{"fp1", "fp2"}); len(fresh) != 0 {
		t.Fatalf("suppression must hold across a restart, got %v", fresh)
	}
	if fresh := after.Fresh([]string{"fp3"}); len(fresh) != 1 {
		t.Fatalf("an unseen fingerprint is still fresh, got %v", fresh)
	}
	// ...and the window still expires on its own schedule.
	after.now = func() time.Time { return base.Add(7 * time.Hour) }
	if fresh := after.Fresh([]string{"fp1"}); len(fresh) != 1 {
		t.Fatalf("recovered entries must age out, got %v", fresh)
	}
}

func TestCooldownEntriesArePrunedAndBounded(t *testing.T) {
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	c := NewCooldown(6 * time.Hour)
	c.now = func() time.Time { return base }
	c.Fresh([]string{"old-1", "old-2"})

	c.now = func() time.Time { return base.Add(7 * time.Hour) }
	c.Fresh([]string{"recent"})
	entries := c.Entries()
	if len(entries) != 1 || entries[0].Fingerprint != "recent" {
		t.Fatalf("entries past the window must be pruned, got %+v", entries)
	}

	// Exceeding the bound loses the OLDEST suppression — one duplicate
	// investigation — rather than growing the object without limit.
	big := NewCooldown(6 * time.Hour)
	tick := base
	big.now = func() time.Time { return tick }
	for i := 0; i < MaxCooldownEntries+50; i++ {
		big.Fresh([]string{"fp-" + strconv.Itoa(i)})
		tick = tick.Add(time.Second)
	}
	entries = big.Entries()
	if len(entries) != MaxCooldownEntries {
		t.Fatalf("bound not enforced: %d entries", len(entries))
	}
	newest := "fp-" + strconv.Itoa(MaxCooldownEntries+49)
	if entries[0].Fingerprint != newest {
		t.Fatalf("the newest suppression must survive eviction, got %q", entries[0].Fingerprint)
	}
}

// A disabled window (the chat default) records nothing: there is no suppression
// to recover, and writing one would be noise on the object.
func TestDisabledCooldownRecordsNothing(t *testing.T) {
	c := NewCooldown(0)
	c.Fresh([]string{"fp1", "fp2"})
	if entries := c.Entries(); entries != nil {
		t.Fatalf("a disabled window must record nothing, got %+v", entries)
	}
}
