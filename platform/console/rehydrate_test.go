package main

import (
	"strings"
	"testing"
)

func agentMsg(id, text, at string) Message {
	return Message{ID: id, Thread: "c1", Kind: MsgAgent, Text: text, At: at}
}

// THE REGRESSION. Rehydrating only when the buffer was empty meant that the
// moment a reader typed a reply, their own message made the buffer non-empty
// and the whole history disappeared.
func TestReplyingDoesNotEraseHistory(t *testing.T) {
	runs := []Run{
		{RunID: "r1", Result: "first answer", FinishedAt: "2026-08-21T06:00:00Z"},
		{RunID: "r2", Result: "second answer", FinishedAt: "2026-08-21T06:05:00Z"},
	}
	// A restarted console: buffer holds ONLY the message just typed.
	live := []Message{
		{ID: "local-1", Thread: "c1", Kind: MsgLocal, Text: "what about the disk?",
			At: "2026-08-21T06:10:00Z", Pending: true},
	}

	got := mergeTranscript("c1", "console", live, runs)
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3 (two answers + the reply): %+v", len(got), got)
	}
	if got[0].Text != "first answer" || got[1].Text != "second answer" {
		t.Fatalf("history missing or out of order: %+v", got)
	}
	if got[2].Kind != MsgLocal {
		t.Fatalf("the reply must be last: %+v", got[2])
	}
}

// An answer held by BOTH the live buffer and the durable record appears once.
func TestNoDuplicateWhenBufferAlreadyHasTheAnswer(t *testing.T) {
	runs := []Run{{RunID: "r1", Result: "the answer", FinishedAt: "2026-08-21T06:00:00Z"}}
	live := []Message{agentMsg("op-1", "the answer", "2026-08-21T06:00:01Z")}

	got := mergeTranscript("c1", "console", live, runs)
	if len(got) != 1 {
		t.Fatalf("expected one copy, got %d: %+v", len(got), got)
	}
	// The LIVE one wins — it is the message the console actually observed.
	if got[0].ID != "op-1" {
		t.Fatalf("id = %q, want the live message to be kept", got[0].ID)
	}
}

// An agent that genuinely answered the same thing twice produced two runs and
// must show two lines. A set-based dedup would silently drop one.
func TestRepeatedIdenticalAnswersBothSurvive(t *testing.T) {
	runs := []Run{
		{RunID: "r1", Result: "same", FinishedAt: "2026-08-21T06:00:00Z"},
		{RunID: "r2", Result: "same", FinishedAt: "2026-08-21T06:05:00Z"},
	}
	if got := mergeTranscript("c1", "console", nil, runs); len(got) != 2 {
		t.Fatalf("got %d, want 2 — identical answers are still two answers", len(got))
	}
	// One already live: exactly one more should be added.
	live := []Message{agentMsg("op-1", "same", "2026-08-21T06:00:01Z")}
	if got := mergeTranscript("c1", "console", live, runs); len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
}

// Messages the buffer alone holds must never be dropped by the merge — an ack
// is recorded nowhere, and a message whose run has not completed yet is not in
// the record either.
func TestBufferOnlyMessagesSurvive(t *testing.T) {
	live := []Message{
		{ID: "s1", Thread: "c1", Kind: MsgSignal, Text: "NodeNotReady", At: "2026-08-21T05:59:00Z"},
		{ID: "rel", Thread: "c1", Kind: MsgRelay, Sender: "home-ops/operator", Text: "have a look",
			At: "2026-08-21T06:01:00Z"},
	}
	runs := []Run{{RunID: "r1", Result: "diagnosed", FinishedAt: "2026-08-21T06:02:00Z"}}

	got := mergeTranscript("c1", "console", live, runs)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	kinds := []string{got[0].Kind, got[1].Kind, got[2].Kind}
	want := []string{MsgSignal, MsgRelay, MsgAgent}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("order = %v, want %v", kinds, want)
		}
	}
}

// A restarted console with nothing typed yet: the whole durable history shows.
func TestEmptyBufferShowsFullHistory(t *testing.T) {
	runs := []Run{
		{RunID: "r1", Result: "one", FinishedAt: "2026-08-21T06:00:00Z"},
		{RunID: "r2", Result: "two", FinishedAt: "2026-08-21T06:05:00Z"},
	}
	got := mergeTranscript("c1", "console", nil, runs)
	if len(got) != 2 || got[0].Text != "one" || got[1].Text != "two" {
		t.Fatalf("got %+v", got)
	}
	for _, m := range got {
		if m.Kind != MsgAgent {
			t.Fatalf("kind = %q, want agent — these ARE what the agent said", m.Kind)
		}
		if m.Thread != "c1" {
			t.Fatalf("thread = %q", m.Thread)
		}
	}
}

func TestResultlessRunsContributeNothing(t *testing.T) {
	got := mergeTranscript("c1", "console", nil, []Run{
		{RunID: "r1", Status: "failed", Result: ""},
		{RunID: "r2", Result: "said something", FinishedAt: "2026-08-21T06:00:00Z"},
	})
	if len(got) != 1 || got[0].Text != "said something" {
		t.Fatalf("got %+v", got)
	}
}

func TestFallsBackToStartedAt(t *testing.T) {
	got := mergeTranscript("c1", "console", nil, []Run{
		{RunID: "r1", Result: "x", StartedAt: "2026-08-21T06:00:00Z"},
	})
	if got[0].At != "2026-08-21T06:00:00Z" {
		t.Fatalf("at = %q, want the start time when there is no finish time", got[0].At)
	}
}

// Ids must be stable, or every poll would look like new messages arriving.
func TestReconstructedIdsAreStable(t *testing.T) {
	runs := []Run{{RunID: "r1", Result: "x", FinishedAt: "2026-08-21T06:00:00Z"}}
	a := mergeTranscript("c1", "console", nil, runs)
	b := mergeTranscript("c1", "console", nil, runs)
	if a[0].ID != b[0].ID {
		t.Fatalf("ids unstable: %q vs %q", a[0].ID, b[0].ID)
	}
}

// A message with an unusable timestamp must not be flung to one end of the
// thread — it keeps its position.
func TestUnparseableTimestampsKeepTheirPlace(t *testing.T) {
	live := []Message{
		agentMsg("a", "first", "2026-08-21T06:00:00Z"),
		{ID: "b", Thread: "c1", Kind: MsgAck, Text: "no timestamp", At: ""},
		agentMsg("c", "third", "2026-08-21T06:02:00Z"),
	}
	got := mergeTranscript("c1", "console", live, nil)
	if len(got) != 3 || got[1].ID != "b" {
		t.Fatalf("order disturbed: %+v", got)
	}
}

func TestNothingAtAllIsEmpty(t *testing.T) {
	if len(mergeTranscript("c1", "console", nil, nil)) != 0 {
		t.Fatal("no buffer and no runs means no transcript")
	}
}

// THE BUG THIS CHANGE EXISTS FOR: a conversation started from the composer read
// as an answer with no question, because the message that opened it was never
// delivered here and its queue entry was pruned before anything could look.
func TestConversationReadsQuestionThenAnswer(t *testing.T) {
	runs := []Run{{
		RunID: "r1", Result: "it was OOMKilled", FinishedAt: "2026-08-21T06:00:05Z",
		Inputs: []RecordedInput{{
			ID: "in-1", Text: "why is api down?", Surface: "console",
			Sender: "operator@example.com", ReceivedAt: "2026-08-21T06:00:00Z",
		}},
	}}
	got := mergeTranscript("c1", "console", nil, runs)
	if len(got) != 2 {
		t.Fatalf("got %d, want the question and the answer: %+v", len(got), got)
	}
	if got[0].Text != "why is api down?" || got[1].Text != "it was OOMKilled" {
		t.Fatalf("a thread reads question then answer: %+v", got)
	}
	if got[0].Kind != MsgLocal || got[0].Sender != "operator@example.com" {
		t.Fatalf("a message typed here is attributed to whoever typed it: %+v", got[0])
	}
}

// A message already on screen and the record entry naming it are ONE message.
func TestRecordedInputAlreadyLiveIsNotDoubled(t *testing.T) {
	live := []Message{{
		ID: "local-1", Thread: "c1", Kind: MsgLocal, Text: "what about the disk?",
		At: "2026-08-21T06:10:00Z", recordID: "in-7",
	}}
	runs := []Run{{
		RunID: "r1", Result: "it is fine", FinishedAt: "2026-08-21T06:10:05Z",
		Inputs: []RecordedInput{{ID: "in-7", Text: "what about the disk?",
			Surface: "console", ReceivedAt: "2026-08-21T06:10:00Z"}},
	}}
	got := mergeTranscript("c1", "console", live, runs)
	if len(got) != 2 {
		t.Fatalf("got %d, want one message and one answer: %+v", len(got), got)
	}
	if got[0].ID != "local-1" {
		t.Fatalf("the live bubble is the one the reader is looking at: %+v", got[0])
	}
}

// A message typed on ANOTHER surface stays attributed to its remote sender when
// it is read back from the record, exactly as its live relay was.
func TestRecordedRelayKeepsItsRemoteSender(t *testing.T) {
	runs := []Run{{
		RunID: "r1", Result: "looking", FinishedAt: "2026-08-21T06:00:05Z",
		Inputs: []RecordedInput{{ID: "in-2", Text: "have a look", Surface: "home-ops",
			Sender: "operator", ReceivedAt: "2026-08-21T06:00:00Z"}},
	}}
	got := mergeTranscript("c1", "console", nil, runs)
	if got[0].Kind != MsgRelay || got[0].Sender != "home-ops/operator" {
		t.Fatalf("somebody else's words keep their attribution: %+v", got[0])
	}
}

// An event no surface displayed reads as the event that woke the agent, not as
// somebody's message.
func TestRecordedEventReadsAsASignal(t *testing.T) {
	runs := []Run{{
		RunID: "r1", Result: "restarted it", FinishedAt: "2026-08-21T06:00:05Z",
		Inputs: []RecordedInput{{ID: "in-3", Text: "disk at 99%",
			Type: "alert", ReceivedAt: "2026-08-21T06:00:00Z"}},
	}}
	got := mergeTranscript("c1", "console", nil, runs)
	if got[0].Kind != MsgSignal || got[0].Sender != "" {
		t.Fatalf("an event has no speaker: %+v", got[0])
	}
}

// A fragment says so. Presenting the beginning of a payload as the whole of it
// would be the quiet kind of wrong.
func TestTruncatedRecordSaysSo(t *testing.T) {
	runs := []Run{{
		RunID: "r1", Result: "ok", FinishedAt: "2026-08-21T06:00:05Z",
		Inputs: []RecordedInput{{ID: "in-4", Text: "xxxx", Truncated: true,
			ReceivedAt: "2026-08-21T06:00:00Z"}},
	}}
	got := mergeTranscript("c1", "console", nil, runs)
	if !strings.Contains(got[0].Text, "truncated") {
		t.Fatalf("a fragment must be presented as one: %q", got[0].Text)
	}
}
