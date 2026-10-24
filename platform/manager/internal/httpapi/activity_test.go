package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
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

// handleActivity replays from a real log, and its cursor tracks the LAST
// event returned rather than the log's own latest -- so a bounded ?limit
// still tells the truth about where the client's next `since` should start.
func TestHandleActivityReplaysAndTracksTheReturnedCursor(t *testing.T) {
	s := &Server{Activity: activity.New(10)}
	s.Activity.Emit(activity.Event{Kind: activity.KindSignalReceived})
	s.Activity.Emit(activity.Event{Kind: activity.KindSignalClaimed})

	rec := httptest.NewRecorder()
	s.handleActivity(rec, httptest.NewRequest("GET", "/activity?limit=1", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out activityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	// A first connect (no cursor) hands back the NEWEST `limit` events, so a
	// fresh console starts current rather than replaying from the beginning.
	if len(out.Events) != 1 || out.Events[0].Kind != activity.KindSignalClaimed {
		t.Fatalf("limit=1 with no cursor should return only the newest event: %+v", out.Events)
	}
	if out.Cursor != out.Events[0].Cursor {
		t.Fatalf("cursor must track the last event RETURNED, not the log's latest: got %q want %q", out.Cursor, out.Events[0].Cursor)
	}

	// An empty replay still carries the log's own latest cursor, so a client
	// polling ahead of everything can still resume from the right place.
	rec = httptest.NewRecorder()
	s.handleActivity(rec, httptest.NewRequest("GET", "/activity?since="+s.Activity.Latest(), nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 0 || out.Cursor != s.Activity.Latest() {
		t.Fatalf("empty replay: %+v, want cursor=%q", out, s.Activity.Latest())
	}
}

func reportReq(t *testing.T, scope, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", "/activity", strings.NewReader(body))
	if scope != "" {
		r = r.WithContext(context.WithValue(r.Context(), adapterScopeKey{}, scope))
	}
	return r
}

// An adapter reports only what it can see: its own hops, attributed to
// itself. The master token (no scope) has no identity of its own to
// attribute a hop to, so IT must name one explicitly.
func TestHandleActivityReportAttribution(t *testing.T) {
	s := &Server{Activity: activity.New(10)}

	rec := httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "", `not json`))
	if rec.Code != 400 {
		t.Fatalf("malformed body: got %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "", `{"kind":""}`))
	if rec.Code != 400 {
		t.Fatalf("empty kind: got %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "", `{"kind":"channel.op.completed"}`))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "master token") {
		t.Fatalf("master token naming no adapter: got %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "tg", `{"kind":"channel.op.completed","adapter":"other"}`))
	if rec.Code != 403 {
		t.Fatalf("mismatched adapter identity: got %d, want 403", rec.Code)
	}

	// A scoped adapter naming ITSELF, or naming nothing (defaulting to its
	// scope), both succeed and default from/to its own hop into the manager.
	rec = httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "tg", `{"kind":"channel.op.completed","adapter":"tg"}`))
	if rec.Code != 202 {
		t.Fatalf("self-attributed report: got %d %q", rec.Code, rec.Body.String())
	}
	events, _ := s.Activity.Since("", 0)
	if len(events) != 1 || events[0].From.Kind != activity.NodeChannelAdapter || events[0].From.Name != "tg" ||
		events[0].To.Kind != activity.NodeManager {
		t.Fatalf("defaulted from/to: %+v", events)
	}

	rec = httptest.NewRecorder()
	s.handleActivityReport(rec, reportReq(t, "", `{"kind":"channel.op.completed","adapter":"master-named"}`))
	if rec.Code != 202 {
		t.Fatalf("master token naming an adapter: got %d %q", rec.Code, rec.Body.String())
	}
}
