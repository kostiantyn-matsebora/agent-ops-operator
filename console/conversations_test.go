package main

import "testing"

// A Conversation held by its close-topics finalizer is still in the watch cache
// and still listed. It must not read as an ordinary open conversation: without
// this the list looks unchanged after a successful close, the operator concludes
// the close failed, and closes again.
func TestSummarizeReportsAClosingConversation(t *testing.T) {
	open := obj("conversations", "open", "1", `{"profileRef":{"name":"ops"}}`, `{"phase":"Idle"}`)
	if summarize(open, nil, "console").Closing {
		t.Fatal("an ordinary conversation must not report closing")
	}

	closing := obj("conversations", "closing", "1", `{"profileRef":{"name":"ops"}}`, `{"phase":"Idle"}`)
	closing.Metadata.DeletionTimestamp = "2026-08-12T10:00:00Z"
	if !summarize(closing, nil, "console").Closing {
		t.Fatal("a deleted conversation held by its finalizer must report closing")
	}
}
