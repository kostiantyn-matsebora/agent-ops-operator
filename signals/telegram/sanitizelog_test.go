package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
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

// handleUpdate logs a non-empty InboundResult.Reason through sanitizeLogText
// too -- it is manager-derived text that can itself carry Telegram-relayed
// content, exactly like the error path sanitizeLog already covers.
func TestHandleUpdateSanitizesTheManagersReason(t *testing.T) {
	mgr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"reason": "cooldown\nFAKE line\r\n"})
	}))
	defer mgr.Close()

	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001234567890", Channel: "telegram-ops"},
	})
	a.mgr = NewManager(mgr.URL, "tok")

	raw, err := os.ReadFile("../../test/fixtures/telegram-update-message.json")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	rec := httptest.NewRecorder()
	a.handleUpdate(rec, httptest.NewRequest("POST", "/updates", strings.NewReader(string(raw))))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	// log.Printf appends exactly one trailing newline of its own, so a
	// sanitized Reason produces ONE line; a forged one would split "cooldown"
	// and "FAKE line" onto two.
	got := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(got, "\n") {
		t.Fatalf("control characters inside Reason survived as a forged log line: %q", got)
	}
	if !strings.Contains(got, "cooldown") || !strings.Contains(got, "FAKE line") {
		t.Fatalf("sanitizeLogText dropped the reason content, not just the control characters: %q", got)
	}
}
