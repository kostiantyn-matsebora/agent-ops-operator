package main

import "testing"

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

	got := mergeTranscript("c1", live, runs)
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

	got := mergeTranscript("c1", live, runs)
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
	if got := mergeTranscript("c1", nil, runs); len(got) != 2 {
		t.Fatalf("got %d, want 2 — identical answers are still two answers", len(got))
	}
	// One already live: exactly one more should be added.
	live := []Message{agentMsg("op-1", "same", "2026-08-21T06:00:01Z")}
	if got := mergeTranscript("c1", live, runs); len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
}

// Non-durable messages the buffer alone holds must never be dropped by the
// merge — the signal card is the whole reason a thread reads as event-then-work.
func TestBufferOnlyMessagesSurvive(t *testing.T) {
	live := []Message{
		{ID: "s1", Thread: "c1", Kind: MsgSignal, Text: "NodeNotReady", At: "2026-08-21T05:59:00Z"},
		{ID: "rel", Thread: "c1", Kind: MsgRelay, Sender: "home-ops/operator", Text: "have a look",
			At: "2026-08-21T06:01:00Z"},
	}
	runs := []Run{{RunID: "r1", Result: "diagnosed", FinishedAt: "2026-08-21T06:02:00Z"}}

	got := mergeTranscript("c1", live, runs)
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
	got := mergeTranscript("c1", nil, runs)
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
	got := mergeTranscript("c1", nil, []Run{
		{RunID: "r1", Status: "failed", Result: ""},
		{RunID: "r2", Result: "said something", FinishedAt: "2026-08-21T06:00:00Z"},
	})
	if len(got) != 1 || got[0].Text != "said something" {
		t.Fatalf("got %+v", got)
	}
}

func TestFallsBackToStartedAt(t *testing.T) {
	got := mergeTranscript("c1", nil, []Run{
		{RunID: "r1", Result: "x", StartedAt: "2026-08-21T06:00:00Z"},
	})
	if got[0].At != "2026-08-21T06:00:00Z" {
		t.Fatalf("at = %q, want the start time when there is no finish time", got[0].At)
	}
}

// Ids must be stable, or every poll would look like new messages arriving.
func TestReconstructedIdsAreStable(t *testing.T) {
	runs := []Run{{RunID: "r1", Result: "x", FinishedAt: "2026-08-21T06:00:00Z"}}
	a := mergeTranscript("c1", nil, runs)
	b := mergeTranscript("c1", nil, runs)
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
	got := mergeTranscript("c1", live, nil)
	if len(got) != 3 || got[1].ID != "b" {
		t.Fatalf("order disturbed: %+v", got)
	}
}

func TestNothingAtAllIsEmpty(t *testing.T) {
	if len(mergeTranscript("c1", nil, nil)) != 0 {
		t.Fatal("no buffer and no runs means no transcript")
	}
}
