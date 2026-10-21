package main

import (
	"context"
	"net/http"
	"testing"
)

// B1 (server half): a thread-less `send` — a channel-level notice with no
// conversation behind it, like an /agents reply — is parked on the
// `channel:<name>` pseudo-thread rather than dropped. This pins that the
// transcript API serves that pseudo-thread back as the channel's general
// feed, so a reader can actually see it.

func TestChannelFeedServesAThreadlessSend(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true)
	h := api.Handler(http.NotFoundHandler())

	threadless := Op{
		ID: "send:agents-listing:1", Channel: "console", Kind: "send",
		Message: &OpMessage{Kind: "answer", Body: "Available: ha-ops, ha-control"},
	}
	threadID, opErr := api.adapter.execute(context.Background(), &threadless)
	if opErr != "" {
		t.Fatalf("a thread-less send must not error: %s", opErr)
	}
	if threadID != "" {
		t.Fatalf("a send reports no thread id: %q", threadID)
	}

	var out struct {
		Channel  string    `json:"channel"`
		Messages []Message `json:"messages"`
	}
	getJSON(t, h, "/api/channel/feed", &out)
	if out.Channel != "console" {
		t.Fatalf("channel = %q, want the served console channel", out.Channel)
	}
	if len(out.Messages) != 1 || out.Messages[0].Text != "Available: ha-ops, ha-control" {
		t.Fatalf("the thread-less send must come back in the general feed: %+v", out.Messages)
	}
}

// With no served channel yet (a fresh console before the first refresh), the
// feed degrades to empty rather than erroring — there is no channel name to
// key a pseudo-thread on.
func TestChannelFeedIsEmptyWithNoServedChannel(t *testing.T) {
	cfg := &Config{Namespace: "agent-ops", AdapterName: "console", UIToken: "tok", WriteEnabled: true}
	cache := staticCache()
	tr := NewTranscripts()
	adapter := NewAdapter(NewManager("http://unused", "tok"), cache, tr, "console")
	api := NewAPI(APIDeps{Cache: cache, Transcripts: tr, Adapter: adapter, Activity: NewActivityWindow(adapter.mgr, 10), Manager: adapter.mgr, Config: cfg})
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Channel  string    `json:"channel"`
		Messages []Message `json:"messages"`
	}
	getJSON(t, h, "/api/channel/feed", &out)
	if out.Channel != "" || len(out.Messages) != 0 {
		t.Fatalf("want an empty feed, got %+v", out)
	}
}
