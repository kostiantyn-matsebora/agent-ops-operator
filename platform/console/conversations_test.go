package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A Conversation held by its close-topics finalizer is still in the watch cache
// and still listed. It must not read as an ordinary open conversation: without
// this the list looks unchanged after a successful close, the operator concludes
// the close failed, and closes again.
func TestSummarizeReportsAConversationBeingDeleted(t *testing.T) {
	open := obj("conversations", "open", "1", `{"profileRef":{"name":"ops"}}`, `{"phase":"Idle"}`)
	if summarize(open, nil, nil, "console", "", nil).Deleting {
		t.Fatal("an ordinary conversation must not report closing")
	}

	closing := obj("conversations", "closing", "1", `{"profileRef":{"name":"ops"}}`, `{"phase":"Idle"}`)
	closing.Metadata.DeletionTimestamp = "2026-08-12T10:00:00Z"
	if !summarize(closing, nil, nil, "console", "", nil).Deleting {
		t.Fatal("a deleted conversation held by its finalizer must report closing")
	}
}

// A dispatch stamps lastActivity AFTER the answer it dispatched for — the
// false-unread bug the proposal names. The read report still covers it
// (design D-D), so opening after the dispatch stamp reports once rather than
// skipping (because it read only the answer's time) and leaving the
// dispatch's own stamp unread forever.
func TestReadReportTimeCoversADispatchStampedAfterTheAnswer(t *testing.T) {
	conv := convWithRuns("dispatch-after-answer", "", [2]string{"the fix", tEarly})
	conv.Status = json.RawMessage(strings.Replace(string(conv.Status),
		`"lastActivity":"`+tEarly+`"`, `"lastActivity":"`+tLate+`"`, 1))
	s := summarize(conv, nil, nil, "console", "", nil)
	if got := s.readReportTime(); got != tLate {
		t.Fatalf("readReportTime must cover a later dispatch stamp, got %q want %q", got, tLate)
	}
}
