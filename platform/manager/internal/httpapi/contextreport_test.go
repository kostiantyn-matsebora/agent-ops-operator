package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

func postContext(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/work/context", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleContextReport(rec, req)
	return rec
}

// An unknown kind is REFUSED rather than passed through: a future sidecar must
// not be able to invent event kinds the metrics layer has never seen.
func TestUnknownContextKindIsRefused(t *testing.T) {
	s := &Server{Activity: activity.New(10)}
	rec := postContext(t, s, `{"kind":"context.whatever","conversation":"c1"}`)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestContextReportRequiresAConversation(t *testing.T) {
	s := &Server{Activity: activity.New(10)}
	rec := postContext(t, s, `{"kind":"context.skip"}`)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// A skip is EMITTED, not swallowed. "Nothing changed" and "nothing ran" are
// different facts, and a stale context needs them told apart.
func TestSkipIsEmittedAsTelemetry(t *testing.T) {
	log := activity.New(10)
	s := &Server{Activity: log}
	rec := postContext(t, s, `{"kind":"context.skip","conversation":"c1","reason":"interval","durationMs":3}`)
	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	evs, _ := log.Since("", 10)
	if len(evs) != 1 {
		t.Fatalf("emitted %d events, want 1", len(evs))
	}
	if evs[0].Kind != activity.KindContextSkipped {
		t.Fatalf("kind = %q", evs[0].Kind)
	}
	if evs[0].Code != activity.CodeContextInterval {
		t.Fatalf("code = %q, want the bounded interval code", evs[0].Code)
	}
	if evs[0].Conversation != "c1" {
		t.Fatalf("conversation = %q", evs[0].Conversation)
	}
}

// Code is a METRIC LABEL, so an unrecognised reason must become empty rather
// than being forwarded — an unbounded label grows series without limit.
func TestUnboundedReasonIsNotUsedAsALabel(t *testing.T) {
	log := activity.New(10)
	s := &Server{Activity: log}
	rec := postContext(t, s,
		`{"kind":"context.skip","conversation":"c1","reason":"because-of-run-1a2b3c4d"}`)
	if rec.Code != 204 {
		t.Fatalf("status = %d", rec.Code)
	}
	evs, _ := log.Since("", 10)
	if evs[0].Code != "" {
		t.Fatalf("code = %q, want empty: an arbitrary reason must never become a metric label", evs[0].Code)
	}
}

func TestFailedContextReportIsAnErrorEvent(t *testing.T) {
	log := activity.New(10)
	s := &Server{Activity: log}
	postContext(t, s, `{"kind":"context.failed","conversation":"c1","error":"volume gone"}`)
	evs, _ := log.Since("", 10)
	if evs[0].Status != activity.StatusError {
		t.Fatalf("status = %q, want error", evs[0].Status)
	}
	if !strings.Contains(evs[0].Detail, "volume gone") {
		t.Fatalf("detail = %q, want the reported error", evs[0].Detail)
	}
}
