package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// The rule is read off the manager's: an input it will not post to a thread
// BECAUSE THE PERSON TYPED IT is exactly the one the console must render.
func TestTypedByAPerson(t *testing.T) {
	cases := []struct {
		kind, signalKind string
		want             bool
	}{
		{"channel", "", true},      // typed into a thread, or a command
		{"signal", "chat", true},   // typed on a general surface — the composer
		{"signal", "alert", false}, // an alert: the manager posts its card
		{"signal", "job", false},   // likewise
		{"signal", "task", false},  // posted as a card too
		{"", "", false},            // predates provenance: never guess
		{"unknown", "chat", false}, // an origin kind we do not know
	}
	for _, c := range cases {
		if got := typedByAPerson(c.kind, c.signalKind); got != c.want {
			t.Errorf("typedByAPerson(%q,%q) = %v, want %v", c.kind, c.signalKind, got, c.want)
		}
	}
}

func convObject(name, uid string, inputs ...map[string]any) *Object {
	spec, _ := json.Marshal(map[string]any{"inputs": inputs})
	return &Object{
		Kind:     "conversations",
		Metadata: Metadata{Name: name, UID: uid},
		Spec:     spec,
	}
}

func chatInput(id, payload, at string) map[string]any {
	return map[string]any{
		"id": id, "type": "task", "payload": payload, "receivedAt": at,
		"origin": map[string]any{"kind": "signal", "name": "console", "signalKind": "chat"},
	}
}

// The bug this fixes: a conversation started from the composer whose transcript
// began at the agent's answer, with the question that caused it missing.
func TestOpeningMessageIsRendered(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	obj := convObject("task-abc", "uid-1", chatInput("in-1", "turn on the kitchen lights", "2026-08-21T18:12:24Z"))
	ti.RememberOrigination("turn on the kitchen lights", "komatoo@pm.me")
	ti.record(obj)

	msgs := tr.Thread("console-uid-1")
	if len(msgs) != 1 {
		t.Fatalf("expected the typed message, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "turn on the kitchen lights" || msgs[0].Kind != MsgLocal {
		t.Fatalf("unexpected message: %+v", msgs[0])
	}
	// Named, not anonymous: the reply two lines below it in the same thread
	// carries this address, and one thread must not name one person two ways.
	if msgs[0].Sender != "komatoo@pm.me" {
		t.Fatalf("the starter's identity must be attached, got %q", msgs[0].Sender)
	}
	// Its own timestamp, so it sorts BEFORE the answer it caused rather than
	// after, which is how it would land if it borrowed "now".
	if msgs[0].At != "2026-08-21T18:12:24Z" {
		t.Fatalf("the message must keep the input's time, got %q", msgs[0].At)
	}
	// Nothing to confirm: the manager already has it.
	if msgs[0].Pending {
		t.Fatal("a recorded input is not a pending local message")
	}
}

// One conversation produces many watch events. The message renders once.
func TestTypedInputIsRecordedOnce(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	obj := convObject("task-abc", "uid-1", chatInput("in-1", "hello", "2026-08-21T18:12:24Z"))
	for i := 0; i < 5; i++ {
		ti.record(obj)
	}
	if msgs := tr.Thread("console-uid-1"); len(msgs) != 1 {
		t.Fatalf("expected one message after five events, got %d", len(msgs))
	}
}

// An alert's card is the MANAGER's to post. Rendering it here too would double
// it on the console and nowhere else.
func TestSignalInputsAreLeftToTheManager(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	ti.record(convObject("task-abc", "uid-1", map[string]any{
		"id": "in-1", "type": "task", "payload": "NodeNotReady on worker-3",
		"origin": map[string]any{"kind": "signal", "name": "cluster-events", "signalKind": "alert"},
	}))
	if msgs := tr.Thread("console-uid-1"); len(msgs) != 0 {
		t.Fatalf("an alert must not be rendered here, got %+v", msgs)
	}
}

// An input with no origin predates provenance and cannot be told from an alert.
func TestInputWithoutOriginIsSkipped(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	ti.record(convObject("task-abc", "uid-1", map[string]any{
		"id": "in-1", "type": "task", "payload": "who knows where this came from",
	}))
	if msgs := tr.Thread("console-uid-1"); len(msgs) != 0 {
		t.Fatalf("an origin-less input must be skipped, got %+v", msgs)
	}
}

// A hint is CONSUMED, so two people typing the same words cannot have the
// second message attributed to the first.
func TestSenderHintIsConsumed(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	ti.RememberOrigination("same words", "alice@example.com")
	ti.record(convObject("task-a", "uid-a", chatInput("in-1", "same words", "2026-08-21T18:12:24Z")))
	ti.record(convObject("task-b", "uid-b", chatInput("in-2", "same words", "2026-08-21T18:13:24Z")))

	first := tr.Thread("console-uid-a")
	second := tr.Thread("console-uid-b")
	if len(first) != 1 || first[0].Sender != "alice@example.com" {
		t.Fatalf("the first message keeps the hint: %+v", first)
	}
	if len(second) != 1 || second[0].Sender != "" {
		t.Fatalf("the second must not borrow the first's identity: %+v", second)
	}
}

// A message typed somewhere else — Telegram, another console — has no hint and
// stays unattributed rather than borrowing the viewer's name.
func TestUnknownSenderStaysEmpty(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)
	ti.record(convObject("task-abc", "uid-1", chatInput("in-1", "from telegram", "2026-08-21T18:12:24Z")))
	msgs := tr.Thread("console-uid-1")
	if len(msgs) != 1 || msgs[0].Sender != "" {
		t.Fatalf("expected an unattributed message, got %+v", msgs)
	}
}

// The whole point: the typed message and the answer read in the order they
// happened, in one thread.
func TestTypedMessageMergesBeforeItsAnswer(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)
	thread := "console-uid-1"

	ti.record(convObject("task-abc", "uid-1", chatInput("in-1", "turn on the AC", "2026-08-21T18:12:24Z")))
	merged := mergeTranscript(thread, tr.Thread(thread), []Run{{
		RunID: "r1", Result: "done", FinishedAt: "2026-08-21T18:13:14Z",
	}})
	if len(merged) != 2 {
		t.Fatalf("expected question then answer, got %+v", merged)
	}
	if merged[0].Text != "turn on the AC" || merged[1].Text != "done" {
		t.Fatalf("out of order: %+v", merged)
	}
	_ = time.Now
	_ = context.Background
}

// The manager consumes the address deciding who answers, so an addressed task
// arrives as the REST. Rendering that shows a message starting mid-sentence —
// which is what "/ha-control - turn the AC on" looked like as "- turn the AC
// on". The console posted the whole thing and is the only component that has it.
func TestAddressedTaskRendersAsTyped(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	typed := "/ha-control - turn air conditioner in cabinet on"
	ti.RememberOrigination(typed, "komatoo@pm.me")
	// What the manager stores: the rest, with the address gone.
	ti.record(convObject("task-abc", "uid-1",
		chatInput("in-1", "- turn air conditioner in cabinet on", "2026-08-21T18:26:59Z")))

	msgs := tr.Thread("console-uid-1")
	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %+v", msgs)
	}
	if msgs[0].Text != typed {
		t.Fatalf("the message must read as typed:\n got %q\nwant %q", msgs[0].Text, typed)
	}
	if msgs[0].Sender != "komatoo@pm.me" {
		t.Fatalf("sender = %q", msgs[0].Sender)
	}
}

func TestAddressedRest(t *testing.T) {
	cases := map[string]string{
		"/ha-control turn the AC on":   "turn the AC on",
		"/ha-control:agent do a thing": "do a thing",
		"/ha-control":                  "", // addresses nobody in particular
		"turn the AC on":               "", // not addressed at all
		"":                             "",
	}
	for in, want := range cases {
		if got := addressedRest(in); got != want {
			t.Errorf("addressedRest(%q) = %q, want %q", in, got, want)
		}
	}
}

// Consuming either key drops both, so one origination cannot attribute two
// messages.
func TestBothHintKeysAreConsumedTogether(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	ti.RememberOrigination("/ha-control do it", "alice@example.com")
	ti.record(convObject("task-a", "uid-a", chatInput("in-1", "do it", "2026-08-21T18:12:24Z")))
	// A second conversation carrying the FULL text must not reuse the hint.
	ti.record(convObject("task-b", "uid-b", chatInput("in-2", "/ha-control do it", "2026-08-21T18:13:24Z")))

	if second := tr.Thread("console-uid-b"); len(second) != 1 || second[0].Sender != "" {
		t.Fatalf("the hint must be spent: %+v", second)
	}
}

// A reply typed into an open conversation is already on screen: `Send` puts it
// there, and the input it becomes is the same message, not a second one.
// Appending it produced two bubbles — one attributed, one anonymous — for one
// thing the person said once.
func TestReplyIsNotDuplicatedByItsInput(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)
	thread := "console-uid-1"

	// Typed in this console.
	tr.AppendLocal("local:1", thread, "komatoo@pm.me", "is tuya integration working properly?")
	// The same text arrives as an input on the next watch event.
	ti.record(convObject("task-abc", "uid-1",
		chatInput("in-1", "is tuya integration working properly?", "2026-08-21T18:29:46Z")))

	msgs := tr.Thread(thread)
	if len(msgs) != 1 {
		t.Fatalf("one message said once must render once, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Sender != "komatoo@pm.me" {
		t.Fatalf("the attributed bubble is the one that survives: %+v", msgs[0])
	}
	if msgs[0].Pending {
		t.Fatal("adopting an input confirms the bubble")
	}
}

// Saying the same thing twice is two messages, and two inputs. One bubble must
// not stand for both.
func TestTwoIdenticalRepliesKeepTwoBubbles(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)
	thread := "console-uid-1"

	tr.AppendLocal("local:1", thread, "komatoo@pm.me", "yes")
	tr.AppendLocal("local:2", thread, "komatoo@pm.me", "yes")
	ti.record(convObject("task-abc", "uid-1",
		chatInput("in-1", "yes", "2026-08-21T18:29:46Z"),
		chatInput("in-2", "yes", "2026-08-21T18:29:50Z")))

	if msgs := tr.Thread(thread); len(msgs) != 2 {
		t.Fatalf("expected two bubbles, got %d: %+v", len(msgs), msgs)
	}
}

// A conversation STARTED from the composer has no bubble to adopt — nothing
// typed it into an open thread — so the input is what puts it on screen.
func TestOpeningMessageStillAppendsWithNothingToAdopt(t *testing.T) {
	tr := NewTranscripts()
	a := &Adapter{cache: NewCache(nil, nil)}
	ti := NewTypedInputs(a.cache, tr, a)

	ti.record(convObject("task-abc", "uid-1", chatInput("in-1", "turn the AC on", "2026-08-21T18:12:24Z")))
	if msgs := tr.Thread("console-uid-1"); len(msgs) != 1 {
		t.Fatalf("expected the opening message, got %+v", msgs)
	}
}
