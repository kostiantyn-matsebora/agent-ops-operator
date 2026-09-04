package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// opAndFailingComplete returns a Manager whose NextOp succeeds once with op
// and whose CompleteOp always fails -- exercises pollOnce's two "complete
// op" error branches (the ordinary path and the redelivered one), neither
// reachable from opsLoop's own tests since the loop never returns.
func opAndFailingComplete(t *testing.T, op Op) *Manager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/channel/ops") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(op)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return NewManager(srv.URL, "tok")
}

func TestPollOnceLogsAndSleepsOnAFailedPoll(t *testing.T) {
	a := &adapter{
		mgr: failingManager(t), pace: newPacer(), completed: newCompletedOps(16),
		channels: map[string]servedChannel{}, reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	// The 5s sleep on a failed poll is cut short by the context deadline
	// rather than waited out -- pollOnce must not panic either way.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	a.pollOnce(ctx)
}

func TestPollOnceLogsAndSwallowsAFailedCompleteOp(t *testing.T) {
	op := Op{ID: "op-2", Channel: "not-served", Kind: "send"}
	a := &adapter{
		mgr: opAndFailingComplete(t, op), pace: newPacer(), completed: newCompletedOps(16),
		channels: map[string]servedChannel{}, reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	// op.Channel names no served channel, so execute() returns an opErr
	// without touching Telegram -- CompleteOp is then the one call that fails.
	a.pollOnce(context.Background())
}

func TestPollOnceLogsAndSwallowsAFailedCompleteOnARedeliveredOp(t *testing.T) {
	op := Op{ID: "op-1", Channel: "not-served", Kind: "send"}
	a := &adapter{
		mgr: opAndFailingComplete(t, op), pace: newPacer(), completed: newCompletedOps(16),
		channels: map[string]servedChannel{}, reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	a.completed.add("op-1", "thread-1") // already completed once
	a.pollOnce(context.Background())
}

// fakeBotAPI answers every Bot API method with success ("ok": true, an empty
// result) except the ones named in fail, which get "ok": false with no
// retry_after -- Telegram.API returns such an error immediately, no retry.
func fakeBotAPI(t *testing.T, fail ...string) string {
	t.Helper()
	failing := map[string]bool{}
	for _, m := range fail {
		failing[m] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		w.Header().Set("Content-Type", "application/json")
		if failing[method] {
			_, _ = w.Write([]byte(`{"ok":false,"description":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_thread_id":7}}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestExecuteLogsAndSwallowsAFailedPointAtTopicSend(t *testing.T) {
	sc := servedChannel{
		cfg:     channelConfig{ChatID: "-1001234567890"},
		token:   "bot-token",
		apiBase: fakeBotAPI(t, "sendMessage"),
	}
	a := &adapter{
		channels: map[string]servedChannel{"telegram-ops": sc},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	op := &Op{Channel: "telegram-ops", Kind: "ensure-topic", Topic: &TopicDescriptor{
		Conversation: "conv-1", Kind: "chat", Title: "t",
	}}
	// CreateTopic succeeds (the fake only fails sendMessage); the pointer
	// message it triggers is what fails, and must not fail the op -- it
	// already has a topic id to return.
	threadID, opErr := a.execute(context.Background(), op)
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty (a signpost failure is best-effort)", opErr)
	}
	if threadID != "7" {
		t.Fatalf("threadID = %q, want the created topic's id", threadID)
	}
}

func TestExecuteLogsAndSwallowsAFailedDeleteConversationNote(t *testing.T) {
	sc := servedChannel{
		cfg: channelConfig{
			ChatID:                          "-1001234567890",
			DeleteTopicOnConversationDelete: true,
		},
		token:   "bot-token",
		apiBase: fakeBotAPI(t, "sendMessage"),
	}
	a := &adapter{
		channels: map[string]servedChannel{"telegram-ops": sc},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	tid := "7"
	op := &Op{Channel: "telegram-ops", Kind: "delete-conversation", Conversation: "conv-1", ThreadID: &tid}
	// DeleteTopic succeeds; the tombstone note is what fails, and must not
	// fail the op -- the deletion itself already succeeded.
	threadID, opErr := a.execute(context.Background(), op)
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty (the note is best-effort once the delete succeeded)", opErr)
	}
	if threadID != "" {
		t.Fatalf("threadID = %q, want empty", threadID)
	}
}
