package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A nil Activity log is a DEPLOYMENT without telemetry configured, not a
// bug — every emission site already calls it unguarded — but the three
// /activity* handlers must still say so rather than panicking on a nil
// receiver.
func TestActivityHandlersAreDisabledWithNoLog(t *testing.T) {
	s := &Server{}

	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"handleActivity":       s.handleActivity,
		"handleActivityStream": s.handleActivityStream,
		"handleActivityReport": s.handleActivityReport,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/activity", nil)
			rec := httptest.NewRecorder()
			call(rec, req)
			if rec.Code != 503 {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), errActivityDisabled) {
				t.Fatalf("body = %q, want it to name why", rec.Body.String())
			}
		})
	}
}
