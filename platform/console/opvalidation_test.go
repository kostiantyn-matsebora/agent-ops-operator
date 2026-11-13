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

// The manager assigns the console thread's id when it enqueues ensure-topic and
// nothing travels back: the console uses the id it is given, and an ensure-topic
// that carries none is an error it logs rather than an id it invents.

func TestEnsureTopicUsesTheThreadIDTheManagerAssigned(t *testing.T) {
	f := newFakeManager(t)
	tr := NewTranscripts()
	adapter := NewAdapter(NewManager(f.server.URL, "token"), staticCache(), tr, "console")
	tid := "console-uid-late-arrival"

	threadID, opErr := adapter.execute(context.Background(), &Op{
		ID: "topic:late-arrival:console", Kind: "ensure-topic", Conversation: "late-arrival", ThreadID: &tid,
	})
	if opErr != "" || threadID != tid {
		t.Fatalf("threadID = %q (%q), want the assigned %q", threadID, opErr, tid)
	}
}

func TestEnsureTopicWithoutAnAssignedThreadIDErrors(t *testing.T) {
	f := newFakeManager(t)
	adapter, _, _ := consoleUnderTest(t, f)
	for _, tid := range []*string{nil, new(string)} {
		threadID, opErr := adapter.execute(context.Background(), &Op{
			ID: "topic:gone:console", Kind: "ensure-topic", Conversation: "gone", ThreadID: tid,
		})
		if opErr == "" || threadID != "" {
			t.Fatalf("an ensure-topic with no thread id must error and report none: %q %q", threadID, opErr)
		}
	}
}
