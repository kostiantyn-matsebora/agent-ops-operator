package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Malformed JSON bodies are refused with the same message across every
// endpoint that reads one -- errInvalidJSON's whole reason for existing.
// Two of its four call sites need no more than a Server zero value: neither
// handler reads anything before the body parse.
func TestMalformedBodyIsRefusedWithoutReadingAnything(t *testing.T) {
	s := &Server{}

	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"handleWorkDone":      s.handleWorkDone,
		"handleChannelOpDone": s.handleChannelOpDone,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/work/done", strings.NewReader("{not json"))
			rec := httptest.NewRecorder()
			call(rec, req)
			if rec.Code != 400 {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), errInvalidJSON) {
				t.Fatalf("body = %q, want it to name the parse failure", rec.Body.String())
			}
		})
	}
}
