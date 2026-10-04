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
