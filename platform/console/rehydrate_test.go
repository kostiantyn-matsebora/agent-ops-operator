package main

import (
	"strings"
	"testing"
)

func agentMsg(id, text, at string) Message {
	return Message{ID: id, Thread: "c1", Kind: MsgAgent, Text: text, At: at, runID: runIDOf(id)}
}

// replyOp is the id the manager gives a run's reply — `send:<conv>:<channel>:
// <runId>`. Tests that used to hand-write "op-1" now use this, because the
// merge matches a buffered answer to its durable run BY ID.
func replyOp(runID string) string { return "send:c1:console:" + runID }

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

	got := mergeTranscript("c1", "console", live, runs, ConversationSummary{})
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
//
// MATCHED BY RUN ID, NOT BY TEXT. The buffer holds `body`, which the manager
// flattens from the blocks it parsed, while the record holds the raw text the
// agent printed — so the two differ by design and a text match would double
// every structured answer. It did, on a live install.
func TestNoDuplicateWhenBufferAlreadyHasTheAnswer(t *testing.T) {
	runs := []Run{{RunID: "r1", Result: "<title>\nthe answer\n</title>", FinishedAt: "2026-08-21T06:00:00Z"}}
	// Deliberately DIFFERENT text: the flattened form of the same answer.
	live := []Message{agentMsg(replyOp("r1"), "**the answer**", "2026-08-21T06:00:01Z")}

	got := mergeTranscript("c1", "console", live, runs, ConversationSummary{})
	if len(got) != 1 {
		t.Fatalf("expected one copy, got %d: %+v", len(got), got)
	}
	// The LIVE one wins — it is the message the console actually observed, and
	// the only one carrying blocks.
	if got[0].ID != replyOp("r1") {
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
	if got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{}); len(got) != 2 {
		t.Fatalf("got %d, want 2 — identical answers are still two answers", len(got))
	}
	// One already live: exactly one more should be added. Two runs answering
	// identically are two IDS, so neither is collapsed — the case the old
	// text multiset existed for, handled by identity instead.
	live := []Message{agentMsg(replyOp("r1"), "same", "2026-08-21T06:00:01Z")}
	if got := mergeTranscript("c1", "console", live, runs, ConversationSummary{}); len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
}

// Messages the buffer alone holds must never be dropped by the merge — an ack
// is recorded nowhere, and a message whose run has not completed yet is not in
// the record either.
func TestBufferOnlyMessagesSurvive(t *testing.T) {
	live := []Message{
		{ID: "s1", Thread: "c1", Kind: MsgSignal, Text: "NodeNotReady", At: "2026-08-21T05:59:00Z"},
		{ID: "rel", Thread: "c1", Kind: MsgRelay, Sender: "home-ops/kostya", Text: "have a look",
			At: "2026-08-21T06:01:00Z"},
	}
	runs := []Run{{RunID: "r1", Result: "diagnosed", FinishedAt: "2026-08-21T06:02:00Z"}}

	got := mergeTranscript("c1", "console", live, runs, ConversationSummary{})
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
	got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
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
	}, ConversationSummary{})
	if len(got) != 1 || got[0].Text != "said something" {
		t.Fatalf("got %+v", got)
	}
}

func TestFallsBackToStartedAt(t *testing.T) {
	got := mergeTranscript("c1", "console", nil, []Run{
		{RunID: "r1", Result: "x", StartedAt: "2026-08-21T06:00:00Z"},
	}, ConversationSummary{})
	if got[0].At != "2026-08-21T06:00:00Z" {
		t.Fatalf("at = %q, want the start time when there is no finish time", got[0].At)
	}
}

// Ids must be stable, or every poll would look like new messages arriving.
func TestReconstructedIdsAreStable(t *testing.T) {
	runs := []Run{{RunID: "r1", Result: "x", FinishedAt: "2026-08-21T06:00:00Z"}}
	a := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
	b := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
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
	got := mergeTranscript("c1", "console", live, nil, ConversationSummary{})
	if len(got) != 3 || got[1].ID != "b" {
		t.Fatalf("order disturbed: %+v", got)
	}
}

func TestNothingAtAllIsEmpty(t *testing.T) {
	if len(mergeTranscript("c1", "console", nil, nil, ConversationSummary{})) != 0 {
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
			Sender: "kostya@example.com", ReceivedAt: "2026-08-21T06:00:00Z",
		}},
	}}
	got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
	if len(got) != 2 {
		t.Fatalf("got %d, want the question and the answer: %+v", len(got), got)
	}
	if got[0].Text != "why is api down?" || got[1].Text != "it was OOMKilled" {
		t.Fatalf("a thread reads question then answer: %+v", got)
	}
	if got[0].Kind != MsgLocal || got[0].Sender != "kostya@example.com" {
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
	got := mergeTranscript("c1", "console", live, runs, ConversationSummary{})
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
			Sender: "kostya", ReceivedAt: "2026-08-21T06:00:00Z"}},
	}}
	got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
	if got[0].Kind != MsgRelay || got[0].Sender != "home-ops/kostya" {
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
	got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
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
	// A SIGNAL's fragment is its PAYLOAD, which now travels apart from the card
	// so the browser can collapse it — so the notice rides with the text it is
	// about, not with the heading above it.
	got := mergeTranscript("c1", "console", nil, runs, ConversationSummary{})
	if !strings.Contains(got[0].Payload, "truncated") {
		t.Fatalf("a fragment must be presented as one: %q", got[0].Payload)
	}

	// A message somebody TYPED has no payload — its text is the message, and
	// the notice belongs there.
	typed := recordedMessage("c1", "console", RecordedInput{
		ID: "in-5", Text: "half a sentence", Truncated: true, Surface: "telegram"}, ConversationSummary{})
	if !strings.Contains(typed.Text, "truncated") {
		t.Fatalf("a truncated relay must say so: %q", typed.Text)
	}
}

// A REBUILT SIGNAL IS STILL A CARD.
//
// The record keeps only the payload text, so a reopened conversation showed a
// raw JSON document as prose. What the CR still knows rebuilds the head, and
// the payload moves to the field the browser folds.
func TestRecordedSignalIsACard(t *testing.T) {
	conv := ConversationSummary{Title: "BackOff: jellyfin", Source: "cluster-events", Pipeline: "k8s-ops"}
	in := RecordedInput{ID: "in-1", Text: "{\n  \"reason\": \"BackOff\"\n}", ReceivedAt: "2026-08-23T06:00:00Z"}

	got := recordedMessage("c1", "console", in, conv)
	if got.Kind != MsgSignal {
		t.Fatalf("kind = %q", got.Kind)
	}
	if !strings.Contains(got.Text, "**Source** `cluster-events`") {
		t.Errorf("the source is not on the rebuilt card:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "📣 **BackOff: jellyfin**") {
		t.Errorf("the title is missing:\n%s", got.Text)
	}
	// The payload travels APART, so the browser collapses it instead of
	// printing a JSON document as prose.
	if !strings.Contains(got.Payload, "\"reason\": \"BackOff\"") {
		t.Errorf("payload did not travel separately: %q", got.Payload)
	}
	if strings.Contains(got.Text, "reason") {
		t.Errorf("the payload is still inline in the card:\n%s", got.Text)
	}
}

// A conversation with no title and no recorded source must not panic, and must
// not render an empty card either.
func TestRecordedSignalDegrades(t *testing.T) {
	in := RecordedInput{ID: "in-1", Text: "plain event"}
	got := recordedMessage("c1", "console", in, ConversationSummary{})
	if got.Text != "" {
		t.Errorf("nothing is known, so the card is empty: %q", got.Text)
	}
	if got.Payload != "plain event" {
		t.Errorf("the payload must survive: %q", got.Payload)
	}
}

// THE REBUILT CARD CARRIES ITS LABELS.
//
// They used to live only on the ConversationInput, which is deleted with the
// queue entry, so a card lost its whole label table the moment the console
// restarted. They are on the conversation now, beside the source.
func TestRecordedSignalKeepsItsLabels(t *testing.T) {
	conv := ConversationSummary{
		Title: "OOMKilling: radarr", Source: "cluster-events", Pipeline: "k8s-ops",
		SignalLabels: map[string]string{
			"namespace": "media-center", "severity": "Warning",
			"alertname": "OOMKilling",     // already the title
			"source":    "cluster-events", // already the source line
		},
	}
	got := recordedMessage("c1", "console", RecordedInput{ID: "in-1", Text: "{}"}, conv)

	if !strings.Contains(got.Text, "| `namespace` | media-center |") {
		t.Errorf("the rebuilt card lost its labels:\n%s", got.Text)
	}
	// The SAME suppression the live card applies, so the two do not diverge.
	if strings.Contains(got.Text, "| `alertname` |") || strings.Contains(got.Text, "| `source` |") {
		t.Errorf("a redundant label survived on the rebuilt card:\n%s", got.Text)
	}
}
