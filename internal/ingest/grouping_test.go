package ingest

import (
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
