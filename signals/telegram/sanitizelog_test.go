package main

import (
	"errors"
	"strings"
	"testing"
)

// sanitizeLog exists to stop an error carrying Telegram-relayed content
// from forging a second log line -- gosecurity:S5145.
func TestSanitizeLogStripsControlCharacters(t *testing.T) {
	err := errors.New("boom\n[signal-telegram] FAKE line\r\n")
	got := sanitizeLog(err)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("control characters survived: %q", got)
	}
	if !strings.Contains(got, "boom") || !strings.Contains(got, "FAKE line") {
		t.Fatalf("sanitizeLog dropped the message content, not just the control characters: %q", got)
	}
}

func TestSanitizeLogLeavesOrdinaryTextUnchanged(t *testing.T) {
	err := errors.New("inbound: source not claimed")
	if got := sanitizeLog(err); got != err.Error() {
		t.Fatalf("got %q, want unchanged %q", got, err.Error())
	}
}
