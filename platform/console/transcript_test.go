package main

import "testing"

// countUnread's derivation table: a message counts by KIND alone, never by
// re-deriving "is this mine" from the sender — transcriptKind already folded
// that into MsgLocal before this function ever sees a message.
func TestCountUnreadByKind(t *testing.T) {
	const (
		before = "2026-08-13T10:00:00Z"
		after  = "2026-08-13T11:00:00Z"
	)
	cases := []struct {
		kind  string
		count bool
	}{
		{MsgSignal, true},
		{MsgAgent, true},
		{MsgRelay, true},
		{MsgAck, false},
		{MsgLocal, false},
	}
	for _, tc := range cases {
		msgs := []Message{{Kind: tc.kind, At: after}}
		got, _, _ := countUnread(msgs, before)
		want := 0
		if tc.count {
			want = 1
		}
		if got != want {
			t.Fatalf("%s: count=%d, want %d", tc.kind, got, want)
		}
	}
}

// Two counted messages after the watermark count twice, and the newest
// counted message is reported however many uncounted messages sit above it.
func TestCountUnreadCountsEveryCountedMessageAndTracksTheNewest(t *testing.T) {
	msgs := []Message{
		{Kind: MsgSignal, At: "2026-08-13T10:00:00Z", Text: "an alert"},
		{Kind: MsgAgent, At: "2026-08-13T10:05:00Z", Text: "the answer"},
		{Kind: MsgAck, At: "2026-08-13T10:06:00Z", Text: "🔧 On it…"},
	}
	count, newest, ok := countUnread(msgs, "")
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
	if !ok || newest.Kind != MsgAgent || newest.Text != "the answer" {
		t.Fatalf("newest counted message wrong: ok=%v newest=%+v", ok, newest)
	}
}

// An empty watermark counts from the beginning — a thread never read has
// nothing to be "after".
func TestCountUnreadWithNoWatermarkCountsEverything(t *testing.T) {
	msgs := []Message{{Kind: MsgAgent, At: "2026-08-13T10:00:00Z"}}
	count, _, ok := countUnread(msgs, "")
	if count != 1 || !ok {
		t.Fatalf("count=%d ok=%v, want 1/true", count, ok)
	}
}

// A message already read (at or before the watermark) does not count, even
// though it is still the newest counted message on the thread.
func TestCountUnreadMessageAtTheWatermarkIsRead(t *testing.T) {
	msgs := []Message{{Kind: MsgAgent, At: "2026-08-13T10:00:00Z"}}
	count, newest, ok := countUnread(msgs, "2026-08-13T10:00:00Z")
	if count != 0 {
		t.Fatalf("a message at the watermark must not count: %d", count)
	}
	if !ok || newest.Kind != MsgAgent {
		t.Fatalf("the newest counted message is reported whether or not it counts: ok=%v newest=%+v", ok, newest)
	}
}

// A console user's own message never counts: transcriptKind already mapped
// it to MsgLocal, regardless of whose words they were.
func TestCountUnreadExcludesLocalMessages(t *testing.T) {
	msgs := []Message{{Kind: MsgLocal, At: "2026-08-13T11:00:00Z", Sender: "kim"}}
	count, _, ok := countUnread(msgs, "2026-08-13T10:00:00Z")
	if count != 0 || ok {
		t.Fatalf("a local message must never count or become the newest counted message: count=%d ok=%v", count, ok)
	}
}

// inputIDOf must recover the WHOLE input id even when that id is itself
// colon-delimited — internal/chat/coordinate.go's member-result dedup ids
// ("member:<conversation>:<runId>", "escalate:<conversation>:<nanotime>") are
// exactly this shape. Splitting on every ':' and demanding 4 parts silently
// returned "" for these (item #15 QA, bug 2): the live buffer's copy of a
// member's result then carried recordID "", so mergeTranscript could never
// recognize it as the same message as the durable record's, and the result
// rendered twice.
func TestInputIDOfRecoversAColonDelimitedInputID(t *testing.T) {
	cases := []struct {
		name, opID, want string
	}{
		{"plain id", "input:coord-root:in-1a2b3c:console", "in-1a2b3c"},
		{"member dedup id", "input:coord-root:member:member-xw4h5:01998abc:console", "member:member-xw4h5:01998abc"},
		{"escalate dedup id", "input:coord-root:escalate:member-xw4h5:abc123:console", "escalate:member-xw4h5:abc123"},
		{"not an input op", "send:coord-root:console:r1", ""},
		{"malformed", "input:onlyoneconstituent", ""},
	}
	for _, tc := range cases {
		if got := inputIDOf(tc.opID); got != tc.want {
			t.Errorf("%s: inputIDOf(%q) = %q, want %q", tc.name, tc.opID, got, tc.want)
		}
	}
}

// runIDOf mirrors the manager's own ParseRunReplyOpID (internal/chat/ops.go):
// the run id is the REMAINDER after the third ':', never assumed colon-free,
// so the two sides of this op id cannot drift apart on what a run id may
// contain.
func TestRunIDOfRecoversAColonDelimitedRunID(t *testing.T) {
	cases := []struct {
		name, opID, want string
	}{
		{"plain id", "send:coord-root:console:r1", "r1"},
		{"colon-bearing run id", "send:coord-root:console:part1:part2", "part1:part2"},
		{"not a send op", "input:coord-root:in-1:console", ""},
	}
	for _, tc := range cases {
		if got := runIDOf(tc.opID); got != tc.want {
			t.Errorf("%s: runIDOf(%q) = %q, want %q", tc.name, tc.opID, got, tc.want)
		}
	}
}

// END TO END: a member-result delivery op, whose id embeds the coordination
// dedup id, must still correlate with the durable record through AppendOp —
// this is the fix verified through the real entry point rather than the
// helper alone.
func TestAppendOpSetsRecordIDForAColonBearingInputID(t *testing.T) {
	tr := NewTranscripts()
	// "input:" + conversation + ":" + inputID + ":" + channel — the same shape
	// internal/chat/ops.go's InputOpID builds. No OriginKind this console
	// renders embeds a colon today — the only ones that do (OriginMember's
	// "member:..."/"escalate:..." dedup ids) are suppressed below before
	// correlation ever runs — but inputIDOf is a general-purpose correlation
	// helper and must recover an id shaped like this regardless.
	opID := "input:coord-root:member:member-xw4h5:01998abc:console"
	tr.AppendOp(opID, "coord-root", &OpMessage{Kind: "signal", Source: "alerts", Body: "the ha report"}, "console")
	got := tr.Thread("coord-root")
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].recordID != "member:member-xw4h5:01998abc" {
		t.Fatalf("recordID = %q, want the full dedup id preserved", got[0].recordID)
	}
}

// A member's result (or the task invoke handed down) must never render as a
// visible bubble on the console — item #15 QA, bug 2. The invoke card
// ThreadPane.tsx builds, straight from the member's own conversation, already
// shows the exchange in full; this is the LIVE-path half of that suppression
// (rehydrate.go's mergeTranscript is the durable-record half, tested there).
func TestAppendOpSuppressesAMemberOriginSignalCard(t *testing.T) {
	tr := NewTranscripts()
	opID := "input:coord-root:member:member-xw4h5:01998abc:console"
	handled := tr.AppendOp(opID, "coord-root", &OpMessage{
		Kind: "signal", Source: "member-xw4h5", OriginKind: "member", Body: "the ha report",
	}, "console")
	if !handled {
		t.Fatal("a suppressed op must still report handled, so the caller completes it normally")
	}
	if got := tr.Thread("coord-root"); len(got) != 0 {
		t.Fatalf("a member-origin signal must render nowhere on the console: %+v", got)
	}
}

// Delivery to OTHER adapters is untouched — OriginKind is read nowhere but
// here, so a signal that is not a "member" origin renders exactly as before,
// origin kind set or not.
func TestAppendOpRendersAnOrdinarySignalWithAnyOriginKind(t *testing.T) {
	tr := NewTranscripts()
	tr.AppendOp("input:c1:in-1:console", "c1",
		&OpMessage{Kind: "signal", Source: "cluster-events", OriginKind: "signal", Body: "disk at 99%"}, "console")
	if got := tr.Thread("c1"); len(got) != 1 || got[0].Kind != MsgSignal {
		t.Fatalf("an ordinary signal origin must render as always: %+v", got)
	}
}
