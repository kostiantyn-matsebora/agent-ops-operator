package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Both handlers below log a wrapped error (via sanitizeLog) when the
// manager call fails -- an already-existing gap this session's mechanical
// %v -> %s, sanitizeLog(err) edit made "new" without a test ever having
// reached it. Neither call must panic or hide the failure.

func failingManager(t *testing.T) *Manager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return NewManager(srv.URL, "tok")
}

func TestDispatchLogsAndSwallowsAFailedInbound(t *testing.T) {
	a := &adapter{
		mgr:      failingManager(t),
		channels: map[string]servedChannel{"telegram-ops": {cfg: channelConfig{ChatID: "-1001"}, token: "bot-token"}},
		reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	// Must not panic even though the manager call fails -- dispatch has no
	// response to check, only that a bad Inbound never brings the handler down.
	a.dispatch(context.Background(), mustUpdate(t,
		`{"update_id":1,"message":{"text":"go","chat":{"id":-1001},"from":{"id":42},"is_topic_message":true,"message_thread_id":7}}`))
}

func TestOffsetPutReturnsBadGatewayOnAFailedWrite(t *testing.T) {
	a := &adapter{
		mgr:      failingManager(t),
		channels: map[string]servedChannel{"telegram-ops": {cfg: channelConfig{ChatID: "-1001"}, token: "bot-token"}},
		reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/offset", strings.NewReader(`{"value":"123"}`))
	a.handleOffsetPut(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestRefreshChannelsLogsAndSwallowsAFailedList(t *testing.T) {
	a := &adapter{
		mgr:      failingManager(t),
		channels: map[string]servedChannel{},
		reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	// Must not panic even though a.mgr.Channels fails, and must leave the
	// served set untouched rather than clearing it on a transient error.
	a.refreshChannels(context.Background())
	if len(a.channels) != 0 {
		t.Fatalf("channels = %v, want unchanged (empty)", a.channels)
	}
}

func TestOffsetGetReturnsBadGatewayOnAFailedRead(t *testing.T) {
	a := &adapter{
		mgr:      failingManager(t),
		channels: map[string]servedChannel{"telegram-ops": {cfg: channelConfig{ChatID: "-1001"}, token: "bot-token"}},
		reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/offset", nil)
	a.handleOffsetGet(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
