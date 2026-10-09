package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// sanitizeLog exists to stop an error carrying user-controlled content (a
// task's text, a manager-relayed reason) from forging a second log line —
// gosecurity:S5145.
func TestSanitizeLogStripsControlCharacters(t *testing.T) {
	err := errors.New("boom\n[console] FAKE line\r\n")
	got := sanitizeLog(err)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("control characters survived: %q", got)
	}
	if !strings.Contains(got, "boom") || !strings.Contains(got, "FAKE line") {
		t.Fatalf("sanitizeLog dropped the message content, not just the control characters: %q", got)
	}
}

func TestSanitizeLogLeavesOrdinaryTextUnchanged(t *testing.T) {
	err := errors.New("origination failed: pipeline not ready")
	if got := sanitizeLog(err); got != err.Error() {
		t.Fatalf("got %q, want unchanged %q", got, err.Error())
	}
}

// handleConversation is the one place a browser sees a run's model/tool-call
// diagnostics (item 6 of the chat-shaped-conversations QA pass): they are
// derived from the SAME activity events the detail view already carries as
// "events", never read off the Conversation itself (docs/contracts.md).
func TestHandleConversationAttachesPerRunDiagnosticsFromActivity(t *testing.T) {
	conv := convWithRuns("conv-diag", "", [2]string{"the answer", tEarly})
	api, _, _, _ := apiWithOptions(t, "tok", true, conv)
	api.activity.add(ActivityEvent{
		Cursor: "0000000000000001", Kind: "model.call", Conversation: "conv-diag", RunID: "r0",
		Data: map[string]string{"model": "claude-sonnet-5", "tokensIn": "123", "tokensOut": "45", "stopReason": "end_turn"},
	})
	api.activity.add(ActivityEvent{
		Cursor: "0000000000000002", Kind: "tool.call", Conversation: "conv-diag", RunID: "r0", LatencyMs: 80,
		Data: map[string]string{"tool": "Read", "resultBytes": "200"},
	})
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Conversation ConversationSummary `json:"conversation"`
		Events       []ActivityEvent     `json:"events"`
	}
	getJSON(t, h, "/api/conversations/conv-diag", &out)

	if len(out.Events) != 2 {
		t.Fatalf("the raw events must still be served too, got %d", len(out.Events))
	}
	if len(out.Conversation.Runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(out.Conversation.Runs))
	}
	run := out.Conversation.Runs[0]
	if len(run.Turns) != 1 || run.Turns[0].Model != "claude-sonnet-5" || run.Turns[0].StopReason != "end_turn" {
		t.Fatalf("run.turns wrong: %+v", run.Turns)
	}
	if run.Turns[0].TokensIn == nil || *run.Turns[0].TokensIn != 123 {
		t.Fatalf("run.turns[0].tokensIn wrong: %+v", run.Turns[0].TokensIn)
	}
	if len(run.ToolCalls) != 1 || run.ToolCalls[0].Tool != "Read" {
		t.Fatalf("run.toolCalls wrong: %+v", run.ToolCalls)
	}
	if run.ToolCalls[0].DurationMs == nil || *run.ToolCalls[0].DurationMs != 80 {
		t.Fatalf("run.toolCalls[0].durationMs wrong: %+v", run.ToolCalls[0].DurationMs)
	}
	if run.ToolCalls[0].ResultBytes == nil || *run.ToolCalls[0].ResultBytes != 200 {
		t.Fatalf("run.toolCalls[0].resultBytes wrong: %+v", run.ToolCalls[0].ResultBytes)
	}
}

// Item #15: a Coordinator root binds no channel until it escalates
// (invariants.md), so it has no ConsoleThread — and the transcript must still
// show the triggering signal and the coordinator's own reasoning turns,
// derived straight from its runs, exactly as a joined conversation's would.
func TestHandleConversationShowsATranscriptForAnUnjoinedCoordinatorRoot(t *testing.T) {
	root := obj("conversations", "coord-root", "1",
		`{"profileRef":{"name":"ops"},"coordinatorRef":{"name":"triage"}}`,
		`{"phase":"Idle","runs":[`+
			`{"runId":"r0","status":"succeeded","result":"first reasoning turn","finishedAt":"`+tEarly+`",`+
			`"inputs":[{"id":"in0","text":"cluster is unhealthy","receivedAt":"`+tEarly+`"}]},`+
			`{"runId":"r1","status":"succeeded","result":"second reasoning turn","finishedAt":"`+tLate+`"}`+
			`]}`)
	api, _, _, _ := apiWithOptions(t, "tok", true, root)
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Transcript []Message `json:"transcript"`
	}
	getJSON(t, h, "/api/conversations/coord-root", &out)

	if len(out.Transcript) != 3 {
		t.Fatalf("want signal + 2 reasoning turns with no console thread, got %d: %+v", len(out.Transcript), out.Transcript)
	}
	if out.Transcript[0].Kind != MsgSignal {
		t.Fatalf("the triggering signal must still open the transcript: %+v", out.Transcript[0])
	}
	if out.Transcript[1].Text != "first reasoning turn" || out.Transcript[2].Text != "second reasoning turn" {
		t.Fatalf("the coordinator's own reasoning turns must appear in order: %+v", out.Transcript)
	}
}

// Item #19: the join hint must name the COORDINATOR a root actually belongs
// to, never a generic "the Pipeline that originated it" — that text was wrong
// on every Coordinator-originated conversation, live, twice.
func TestJoinHintNamesTheCoordinatorForAnUnjoinedRoot(t *testing.T) {
	root := obj("conversations", "coord-root", "1",
		`{"profileRef":{"name":"ops"},"coordinatorRef":{"name":"triage"}}`, `{"phase":"Idle"}`)
	api, _, _, _ := apiWithOptions(t, "tok", true, root)
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		JoinHint *struct {
			Reason string `json:"reason"`
			Fix    string `json:"fix"`
		} `json:"joinHint"`
	}
	getJSON(t, h, "/api/conversations/coord-root", &out)

	if out.JoinHint == nil {
		t.Fatal("an unjoined root must carry a joinHint")
	}
	if !strings.Contains(out.JoinHint.Fix, "Coordinator triage") {
		t.Fatalf("fix must name the Coordinator, not a Pipeline: %q", out.JoinHint.Fix)
	}
	if strings.Contains(out.JoinHint.Fix, "the Pipeline that originated it") {
		t.Fatalf("must not fall back to the Pipeline wording for a Coordinator root: %q", out.JoinHint.Fix)
	}
}
