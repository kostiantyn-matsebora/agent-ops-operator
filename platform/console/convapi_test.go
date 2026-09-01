package main

import (
	"errors"
	"strings"
	"testing"
)

// sanitizeLog exists to stop an error carrying user-controlled content (a
// task's text, a manager-relayed reason) from forging a second log line —
// gosecurity:S5145.
func TestSanitizeLogStripsControlCharacters(t *testing.T) {
	err := errors.New("boom\n[console] FAKE line\r\n")
	got := sanitizeLog(err)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("control characters survived: %q", got)
	}
	if !strings.Contains(got, "boom") || !strings.Contains(got, "FAKE line") {
		t.Fatalf("sanitizeLog dropped the message content, not just the control characters: %q", got)
	}
}

func TestSanitizeLogLeavesOrdinaryTextUnchanged(t *testing.T) {
	err := errors.New("origination failed: pipeline not ready")
	if got := sanitizeLog(err); got != err.Error() {
		t.Fatalf("got %q, want unchanged %q", got, err.Error())
	}
}
