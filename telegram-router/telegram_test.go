package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClassifyUpdate pins the ONE rule this process owns. A forum-topic
// message continues a conversation the manager already knows; anything else
// originates one. Getting this backwards silently turns every reply into a new
// conversation, so it is worth pinning field by field.
func TestClassifyUpdate(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantTopic bool
		wantID    int64
	}{
		{
			name:      "general surface is an origination",
			raw:       `{"update_id":11,"message":{"text":"check the disk","chat":{"id":-100}}}`,
			wantTopic: false, wantID: 11,
		},
		{
			name:      "topic message is a continuation",
			raw:       `{"update_id":12,"message":{"text":"and the memory?","chat":{"id":-100},"is_topic_message":true,"message_thread_id":42}}`,
			wantTopic: true, wantID: 12,
		},
		{
			name:      "explicit false is the general surface",
			raw:       `{"update_id":13,"message":{"text":"hi","is_topic_message":false}}`,
			wantTopic: false, wantID: 13,
		},
		{
			name:      "update without a message is not a topic message",
			raw:       `{"update_id":14}`,
			wantTopic: false, wantID: 14,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := classifyUpdate(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if u.IsTopicMessage != tc.wantTopic {
				t.Fatalf("IsTopicMessage = %v, want %v", u.IsTopicMessage, tc.wantTopic)
			}
			if u.UpdateID != tc.wantID {
				t.Fatalf("UpdateID = %d, want %d", u.UpdateID, tc.wantID)
			}
			if string(u.Raw) != tc.raw {
				t.Fatalf("Raw not preserved verbatim:\n got %s\nwant %s", u.Raw, tc.raw)
			}
		})
	}
}

// TestRouteSendsToTheRightTarget covers the fan-out end to end over real HTTP,
// including that the forwarded body is the update VERBATIM — receiving
// adapters parse the Telegram shape themselves, so anything the router
// re-encodes is a field they could lose.
func TestRouteSendsToTheRightTarget(t *testing.T) {
	var signalGot, channelGot string
	signalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/updates" {
			t.Errorf("signal adapter got %s, want /updates", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		signalGot = string(b)
	}))
	defer signalSrv.Close()
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		channelGot = string(b)
	}))
	defer channelSrv.Close()

	cfg := config{SignalTarget: signalSrv.URL, ChannelTarget: channelSrv.URL}
	r := &router{cfg: cfg, down: NewDownstream()}

	general := `{"update_id":1,"message":{"text":"hello","chat":{"id":-100}}}`
	u, _ := classifyUpdate(json.RawMessage(general))
	r.route(context.Background(), u)
	if signalGot != general {
		t.Fatalf("general-surface update did not reach the signal adapter verbatim: %q", signalGot)
	}
	if channelGot != "" {
		t.Fatalf("general-surface update leaked to the channel adapter: %q", channelGot)
	}

	signalGot, channelGot = "", ""
	topic := `{"update_id":2,"message":{"text":"more","is_topic_message":true,"message_thread_id":7}}`
	u, _ = classifyUpdate(json.RawMessage(topic))
	r.route(context.Background(), u)
	if channelGot != topic {
		t.Fatalf("topic update did not reach the channel adapter verbatim: %q", channelGot)
	}
	if signalGot != "" {
		t.Fatalf("topic update leaked to the signal adapter: %q", signalGot)
	}
}

// TestOffsetDelegation: the router reads and reports the offset through the
// channel adapter, holding no storage of its own.
func TestOffsetDelegation(t *testing.T) {
	var put string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "500"})
		case http.MethodPut:
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			put = in["value"]
		}
	}))
	defer srv.Close()

	d := NewDownstream()
	got, err := d.GetOffset(context.Background(), srv.URL)
	if err != nil || got != "500" {
		t.Fatalf("GetOffset = %q, %v; want 500", got, err)
	}
	if err := d.PutOffset(context.Background(), srv.URL, "501"); err != nil {
		t.Fatal(err)
	}
	if put != "501" {
		t.Fatalf("PutOffset sent %q, want 501", put)
	}
}
