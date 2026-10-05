package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// attachRunCalls is how per-run model/tool-call diagnostics reach the
// browser (docs/contracts.md's "What the run did: turns[] and toolCalls[]" —
// the manager writes this telemetry to the activity feed ONLY, never to the
// Conversation, so the console derives Run.turns/toolCalls from the same
// events array it already serves).
func TestAttachRunCallsGroupsHopsByRunID(t *testing.T) {
	runs := []Run{{RunID: "r1"}, {RunID: "r2"}}
	events := []ActivityEvent{
		{
			Kind: "model.call", RunID: "r1",
			Data: map[string]string{"model": "claude-sonnet-5", "tokensIn": "100", "tokensOut": "40", "stopReason": "end_turn"},
		},
		{
			Kind: "tool.call", RunID: "r1", LatencyMs: 250,
			Data: map[string]string{"tool": "Bash", "server": "", "resultBytes": "512"},
		},
		{
			// A different run's hop must never bleed into r1's.
			Kind: "model.call", RunID: "r2",
			Data: map[string]string{"model": "claude-sonnet-5"},
		},
		{
			// No RunID at all: not attributable to any run, must be ignored.
			Kind: "model.call", Data: map[string]string{"model": "stray"},
		},
	}

	got := attachRunCalls(runs, events)

	r1 := got[0]
	if len(r1.Turns) != 1 || r1.Turns[0].Model != "claude-sonnet-5" || r1.Turns[0].StopReason != "end_turn" {
		t.Fatalf("r1 turns wrong: %+v", r1.Turns)
	}
	if r1.Turns[0].TokensIn == nil || *r1.Turns[0].TokensIn != 100 {
		t.Fatalf("r1 tokensIn wrong: %+v", r1.Turns[0].TokensIn)
	}
	if r1.Turns[0].TokensOut == nil || *r1.Turns[0].TokensOut != 40 {
		t.Fatalf("r1 tokensOut wrong: %+v", r1.Turns[0].TokensOut)
	}
	if r1.Turns[0].CacheReadTokens != nil {
		t.Fatalf("an absent cacheReadTokens must stay nil, got %v", r1.Turns[0].CacheReadTokens)
	}
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Tool != "Bash" || r1.ToolCalls[0].Server != "" {
		t.Fatalf("r1 tool calls wrong: %+v", r1.ToolCalls)
	}
	if r1.ToolCalls[0].DurationMs == nil || *r1.ToolCalls[0].DurationMs != 250 {
		t.Fatalf("r1 durationMs wrong: %+v", r1.ToolCalls[0].DurationMs)
	}
	if r1.ToolCalls[0].ResultBytes == nil || *r1.ToolCalls[0].ResultBytes != 512 {
		t.Fatalf("r1 resultBytes wrong: %+v", r1.ToolCalls[0].ResultBytes)
	}

	r2 := got[1]
	if len(r2.Turns) != 1 || len(r2.ToolCalls) != 0 {
		t.Fatalf("r2 must carry only its own turn, got turns=%+v toolCalls=%+v", r2.Turns, r2.ToolCalls)
	}
}

// A run with no matching activity hops keeps both fields absent — the work
// contract's own "both lists are optional" rule, carried through unchanged.
func TestAttachRunCallsLeavesAnUnmatchedRunEmpty(t *testing.T) {
	runs := []Run{{RunID: "solo"}}
	got := attachRunCalls(runs, []ActivityEvent{{Kind: "model.call", RunID: "other-run", Data: map[string]string{"model": "x"}}})
	if len(got[0].Turns) != 0 || len(got[0].ToolCalls) != 0 {
		t.Fatalf("an unmatched run must carry neither, got %+v", got[0])
	}
}

// No activity events at all (a fresh console, or a run outside the window)
// must not panic and must return the runs unchanged.
func TestAttachRunCallsIsANoOpWithNoEvents(t *testing.T) {
	runs := []Run{{RunID: "solo", Result: "done"}}
	got := attachRunCalls(runs, nil)
	if len(got) != 1 || got[0].Result != "done" || got[0].Turns != nil || got[0].ToolCalls != nil {
		t.Fatalf("want the runs passed through untouched, got %+v", got)
	}
}

// reportedCount and reportedLatency both treat an absent or malformed fact
// as absent, never as zero — the SAME convention the work contract states
// for the manager's own report (docs/contracts.md: "A fact the runtime
// cannot determine is OMITTED, never sent as zero").
func TestReportedCountTreatsMissingOrMalformedAsAbsent(t *testing.T) {
	data := map[string]string{"present": "42", "negative": "-1", "garbage": "nope"}
	if got := reportedCount(data, "present"); got == nil || *got != 42 {
		t.Fatalf("a clean value must parse through, got %v", got)
	}
	if got := reportedCount(data, "missing"); got != nil {
		t.Fatalf("an absent key must stay nil, got %v", got)
	}
	if got := reportedCount(data, "negative"); got != nil {
		t.Fatalf("a negative count must be treated as untrusted, got %v", got)
	}
	if got := reportedCount(data, "garbage"); got != nil {
		t.Fatalf("an unparseable value must be treated as untrusted, got %v", got)
	}
}

func TestReportedLatencyTreatsZeroAsAbsent(t *testing.T) {
	if got := reportedLatency(0); got != nil {
		t.Fatalf("zero must read as not-reported, got %v", got)
	}
	if got := reportedLatency(-5); got != nil {
		t.Fatalf("a negative latency must read as not-reported, got %v", got)
	}
	if got := reportedLatency(250); got == nil || *got != 250 {
		t.Fatalf("a positive latency must pass through, got %v", got)
	}
}

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
