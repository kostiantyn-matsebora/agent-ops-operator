package main

import (
	"context"
	"testing"
)

// B3: delete-conversation and close-topic without a threadId must complete
// with an explicit error — never a silent no-op the manager reads as done.

func TestDeleteConversationWithoutThreadIDErrors(t *testing.T) {
	f := newFakeManager(t)
	adapter, transcripts, _ := consoleUnderTest(t, f)

	msg := OpMessage{Kind: "notice", Body: "This conversation has been deleted."}
	threadID, opErr := adapter.execute(context.Background(), &Op{
		ID: "gone:conv-1:console", Kind: "delete-conversation", Conversation: "conv-1", Message: &msg,
	})
	if opErr == "" {
		t.Fatal("a delete-conversation with no threadId must report an error, not succeed silently")
	}
	if threadID != "" {
		t.Fatalf("an errored op reports no thread id: %q", threadID)
	}
	if len(transcripts.Thread("")) != 0 {
		t.Fatal("nothing must be appended without a thread to append to")
	}
}

func TestCloseTopicWithoutThreadIDErrors(t *testing.T) {
	f := newFakeManager(t)
	adapter, _, _ := consoleUnderTest(t, f)

	_, opErr := adapter.execute(context.Background(), &Op{
		ID: "close:conv-2:console", Kind: "close-topic", Conversation: "conv-2",
	})
	if opErr == "" {
		t.Fatal("a close-topic with no threadId must report an error, not succeed silently")
	}
}

// Both verbs still behave exactly as before WHEN a threadId is present — this
// is a validation ADDED in front of the existing behaviour, not a replacement
// of it. TestReopenMakesTheConsoleThreadLiveAgain and
// TestDeleteConversationLeavesAReadableArchivedTranscript (bulkdelete_test.go)
// already pin the threadId-present path for both ops.

// B5: ensure-topic always answers `console-<uid>`, resolving a cache miss
// through the watch client rather than falling back to a name-based id.

func TestEnsureTopicResolvesUIDOnACacheMiss(t *testing.T) {
	f := newFakeManager(t)
	cache := staticCache() // empty: the conversation is NOT in the watch cache yet
	src := cache.src.(*fakeSource)
	src.gets = map[string]*Object{
		"conversations/late-arrival": obj("conversations", "late-arrival", "5", "{}", "{}"),
	}
	tr := NewTranscripts()
	adapter := NewAdapter(NewManager(f.server.URL, "token"), cache, tr, "console")

	threadID, opErr := adapter.execute(context.Background(), &Op{
		ID: "topic:late-arrival:console", Kind: "ensure-topic", Conversation: "late-arrival",
	})
	if opErr != "" {
		t.Fatalf("a resolvable cache miss must not error: %s", opErr)
	}
	if want := "console-uid-late-arrival"; threadID != want {
		t.Fatalf("threadID = %q, want %q", threadID, want)
	}
	// The resolved object converges into the cache, so a second ensure-topic
	// for the same conversation does not need another live fetch.
	if cache.Get("conversations", "late-arrival") == nil {
		t.Fatal("a live-fetched object must be applied to the cache")
	}
}

func TestEnsureTopicErrorsWhenTheUIDCannotBeResolved(t *testing.T) {
	f := newFakeManager(t)
	cache := staticCache() // empty, and the fake source answers every Get with 404
	tr := NewTranscripts()
	adapter := NewAdapter(NewManager(f.server.URL, "token"), cache, tr, "console")

	threadID, opErr := adapter.execute(context.Background(), &Op{
		ID: "topic:gone:console", Kind: "ensure-topic", Conversation: "gone",
	})
	if opErr == "" {
		t.Fatal("an unresolvable conversation must error, so the manager's redelivery retries")
	}
	if threadID != "" {
		t.Fatalf("an errored ensure-topic reports no thread id: %q", threadID)
	}
}

// The old name-based fallback must never be produced again, present or absent
// from the cache.
func TestEnsureTopicNeverProducesTheRetiredNameBasedID(t *testing.T) {
	f := newFakeManager(t)
	conv := obj("conversations", "task-xyz", "1", "{}", "{}")
	conv.Metadata.UID = "" // a conversation the cache has seen but with no UID yet
	adapter, _, _ := consoleUnderTest(t, f, conv)

	_, opErr := adapter.execute(context.Background(), &Op{
		ID: "topic:task-xyz:console", Kind: "ensure-topic", Conversation: "task-xyz",
	})
	if opErr == "" {
		t.Fatal("a UID-less conversation must error rather than fall back to a name-based id")
	}
}
