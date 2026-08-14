package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// inboundCapture stands in for the manager, recording what reached
// /channel/inbound.
type inboundCapture struct {
	Channel  string  `json:"channel"`
	ThreadID *string `json:"threadId"`
	Text     string  `json:"text"`
}

func testAdapter(t *testing.T, channels map[string]channelConfig) (*adapter, *[]inboundCapture) {
	t.Helper()
	var got []inboundCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channel/inbound" {
			var in inboundCapture
			_ = json.NewDecoder(r.Body).Decode(&in)
			got = append(got, in)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	a := &adapter{
		mgr:      NewManager(srv.URL, "test-token"),
		channels: map[string]servedChannel{},
		reported: map[string]string{},
		clients:  map[string]*Telegram{},
	}
	for name, cfg := range channels {
		a.channels[name] = servedChannel{cfg: cfg, token: "bot-token"}
	}
	return a, &got
}

func mustUpdate(t *testing.T, raw string) tgUpdate {
	t.Helper()
	var u tgUpdate
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

// TestChatIDAndApproverFiltering is the MIRROR of signal-telegram's test of
// the same name. Both adapters run these two rules against their own contract
// listing, because the router deliberately carries no channel policy. Keep the
// tables in step — drift shows up in production as messages silently ignored.
func TestChatIDAndApproverFiltering(t *testing.T) {
	a, got := testAdapter(t, map[string]channelConfig{
		"telegram-ops": {ChatID: "-1001", Approvers: []int64{42, 43}},
	})
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"approved sender in the matching chat", `{"update_id":1,"message":{"text":"go","chat":{"id":-1001},"from":{"id":42},"is_topic_message":true,"message_thread_id":7}}`, true},
		{"other approved sender", `{"update_id":2,"message":{"text":"go","chat":{"id":-1001},"from":{"id":43},"is_topic_message":true,"message_thread_id":7}}`, true},
		{"unapproved sender is dropped", `{"update_id":3,"message":{"text":"go","chat":{"id":-1001},"from":{"id":99},"is_topic_message":true,"message_thread_id":7}}`, false},
		{"missing sender with approvers set is dropped", `{"update_id":4,"message":{"text":"go","chat":{"id":-1001},"is_topic_message":true,"message_thread_id":7}}`, false},
		{"unknown chat is dropped", `{"update_id":5,"message":{"text":"go","chat":{"id":-2002},"from":{"id":42},"is_topic_message":true,"message_thread_id":7}}`, false},
		{"empty text is dropped", `{"update_id":6,"message":{"text":"","chat":{"id":-1001},"from":{"id":42},"is_topic_message":true,"message_thread_id":7}}`, false},
		{"non-message update is dropped", `{"update_id":7}`, false},
		{"general surface belongs to the signal adapter", `{"update_id":8,"message":{"text":"go","chat":{"id":-1001},"from":{"id":42}}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(*got)
			a.dispatch(context.Background(), mustUpdate(t, tc.raw))
			delivered := len(*got) > before
			if delivered != tc.want {
				t.Fatalf("delivered = %v, want %v", delivered, tc.want)
			}
		})
	}
}

// TestNoApproversMeansAnyone: an empty approver list is "the whole chat", not
// "nobody".
func TestNoApproversMeansAnyone(t *testing.T) {
	a, got := testAdapter(t, map[string]channelConfig{"telegram-ops": {ChatID: "-1001"}})
	a.dispatch(context.Background(), mustUpdate(t,
		`{"update_id":1,"message":{"text":"go","chat":{"id":-1001},"from":{"id":99},"is_topic_message":true,"message_thread_id":7}}`))
	if len(*got) != 1 {
		t.Fatal("empty approvers must accept anyone in the chat")
	}
}

// TestTopicMessageCarriesItsThreadID: the thread id is what makes this a
// CONTINUATION — /channel/inbound now rejects a message without one.
func TestTopicMessageCarriesItsThreadID(t *testing.T) {
	a, got := testAdapter(t, map[string]channelConfig{"telegram-ops": {ChatID: "-1001"}})
	a.dispatch(context.Background(), mustUpdate(t,
		`{"update_id":1,"message":{"text":"more","chat":{"id":-1001},"from":{"id":1},"is_topic_message":true,"message_thread_id":4242}}`))
	if len(*got) != 1 {
		t.Fatal("topic message should be delivered")
	}
	in := (*got)[0]
	if in.ThreadID == nil || *in.ThreadID != "4242" {
		t.Fatalf("threadId = %v, want 4242", in.ThreadID)
	}
	if in.Channel != "telegram-ops" {
		t.Fatalf("channel = %q", in.Channel)
	}
}

// TestOffsetRoundTripsThroughTheStateAPI: the router delegates persistence
// here, and this adapter writes it to the Channel annotation via the contract.
func TestOffsetRoundTripsThroughTheStateAPI(t *testing.T) {
	var putPath, putValue string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "900"})
		case http.MethodPut:
			putPath = r.URL.Path
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			putValue = in["value"]
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	a := &adapter{
		mgr:      NewManager(srv.URL, "test-token"),
		channels: map[string]servedChannel{"telegram-ops": {cfg: channelConfig{ChatID: "-1001"}}},
		reported: map[string]string{},
		clients:  map[string]*Telegram{},
	}
	h := a.handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/offset", nil))
	var out map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out["value"] != "900" {
		t.Fatalf("GET /offset = %v, want 900", out)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/offset",
		strings.NewReader(`{"value":"901"}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT /offset = %d, want 204", rec.Code)
	}
	if putValue != "901" {
		t.Fatalf("persisted %q, want 901", putValue)
	}
	if putPath != "/channel/state/telegram-ops/telegram-offset" {
		t.Fatalf("persisted to %q", putPath)
	}
}

// TestAlreadyClosedTopicIsNotAFailure pins what the LIVE Bot API returns for a
// redelivered close-topic op. Telegram answers "Bad Request:
// TOPIC_NOT_MODIFIED" when the topic is already archived — TOPIC_CLOSED is a
// different case (posting into a closed topic). Treating either as a failure
// would make every at-least-once redelivery look broken.
func TestAlreadyClosedTopicIsNotAFailure(t *testing.T) {
	tolerated := []string{
		"telegram closeForumTopic: Bad Request: TOPIC_NOT_MODIFIED",
		"telegram closeForumTopic: Bad Request: TOPIC_CLOSED",
	}
	for _, msg := range tolerated {
		if !alreadyClosed(errors.New(msg)) {
			t.Errorf("must tolerate %q", msg)
		}
	}
	for _, msg := range []string{
		"telegram closeForumTopic: Bad Request: chat not found",
		"telegram closeForumTopic: Forbidden: bot is not a member",
	} {
		if alreadyClosed(errors.New(msg)) {
			t.Errorf("must report %q as a failure", msg)
		}
	}
}

// delete-conversation: un-archive → post → re-archive.
//
// Every step is forced by the transport. A CLOSED forum topic refuses
// sendMessage, so the tombstone cannot be posted without reopening first; and
// leaving the topic open afterwards would invite replies into a conversation
// that no longer exists, which the manager drops because the thread maps to
// nothing.
func TestDeleteConversationReopensPostsAndClosesAgain(t *testing.T) {
	var calls []string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, strings.TrimPrefix(r.URL.Path, "/botbot-token/"))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"message_thread_id":42}}`))
	}))
	defer tg.Close()

	a, _ := testAdapter(t, map[string]channelConfig{"tg": {ChatID: "-100"}})
	a.clients["bot-token"] = &Telegram{Token: "bot-token", HTTP: routeTo(tg.URL)}

	tid := "42"
	msg := Message{Kind: MsgNotice, Body: "This conversation has been deleted."}
	_, opErr := a.execute(context.Background(), &Op{
		Kind: "delete-conversation", Channel: "tg", Conversation: "conv-1",
		ThreadID: &tid, Message: &msg,
	})
	if opErr != "" {
		t.Fatalf("delete-conversation: %s", opErr)
	}
	want := []string{"reopenForumTopic", "sendMessage", "closeForumTopic"}
	if len(calls) != len(want) {
		t.Fatalf("want %v, got %v", want, calls)
	}
	for i, c := range want {
		if calls[i] != c {
			t.Fatalf("call %d: want %s, got %s (all: %v)", i, c, calls[i], calls)
		}
	}
	// The topic itself must SURVIVE: the transcript above the tombstone is what
	// a person scrolls back to after an incident.
	for _, c := range calls {
		if c == "deleteForumTopic" {
			t.Fatal("the forum topic must not be deleted")
		}
	}
}

// A thread id is required: without one there is nothing to mark.
func TestDeleteConversationWithoutAThreadIsReported(t *testing.T) {
	a, _ := testAdapter(t, map[string]channelConfig{"tg": {ChatID: "-100"}})
	a.clients["bot-token"] = &Telegram{Token: "bot-token", HTTP: http.DefaultClient}
	if _, opErr := a.execute(context.Background(), &Op{
		Kind: "delete-conversation", Channel: "tg", Conversation: "conv-1",
	}); opErr == "" {
		t.Fatal("a delete-conversation with no thread id must be reported, not silently ignored")
	}
}

// routeTo sends every Bot API call to a stub server. The client builds the
// api.telegram.org URL itself, so the interception is at the transport.
func routeTo(base string) *http.Client {
	u, _ := url.Parse(base)
	return &http.Client{Transport: rewrite{host: u.Host, scheme: u.Scheme}}
}

type rewrite struct{ host, scheme string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Host, req.URL.Scheme = r.host, r.scheme
	return http.DefaultTransport.RoundTrip(req)
}
