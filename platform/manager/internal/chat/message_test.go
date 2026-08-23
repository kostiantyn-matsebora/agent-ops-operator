package chat

import (
	"context"
	"strings"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// transportMarkup is what must never appear in anything internal/ produces.
// These are Telegram's dialect specifically — the manager used to emit them,
// and every other surface had to un-parse them.
var transportMarkup = []string{"<b>", "</b>", "<i>", "<code>", "<pre>", "&lt;", "&gt;", "&amp;", "parse_mode"}

// assertNoMarkup fails when a message the manager composed carries transport
// markup. Run across EVERY message-producing path rather than the ones this
// change happened to touch — the leak was a habit, and a habit comes back.
func assertNoMarkup(t *testing.T, where string, m Message) {
	t.Helper()
	for _, field := range []string{m.Body, m.Title, m.Origin, m.Sender, m.Source, m.Pipeline} {
		for _, bad := range transportMarkup {
			if strings.Contains(field, bad) {
				t.Fatalf("%s: manager composed transport markup %q in %q", where, bad, field)
			}
		}
	}
}

func TestManagerComposesNoTransportMarkup(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  Message
	}{
		{"notice", Notice("🔧 On it…")},
		{"warn", Warn("⚠️ Usage: `/ops <task>`")},
		{"answer", AnswerMessage("**done** — restarted `api`", StatusSucceeded)},
		{"relay", RelayMessage("telegram", "kim", "and the disks?")},
		{"signal", SignalMessage("prod-oncall", "vm-alerts", "DiskFull", "conv-in-1",
			map[string]string{"namespace": "prod"}, "node/1 root filesystem at 97%")},
	} {
		assertNoMarkup(t, tc.name, tc.msg)
	}
}

// Every op the queue hands an adapter carries STRUCTURE, never rendered text.
// This is the property the contract version exists to protect: an adapter
// reading a `text` field would find nothing.
func TestOpsCarryStructuredPayloads(t *testing.T) {
	ctx := context.Background()
	q := &OpQueue{Registry: NewRegistry()}
	ch := testChannel("c1", "slack")

	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{Source: "vm-alerts", Kind: "alert"})
	op := q.Claim("slack")
	if op == nil || op.Topic == nil {
		t.Fatalf("ensure-topic must carry a descriptor: %+v", op)
	}
	if op.Topic.Conversation != "conv-1" || op.Topic.Source != "vm-alerts" || op.Topic.Title != "t: conv-1" {
		t.Fatalf("descriptor: %+v", op.Topic)
	}

	q.EnqueueMessage(ctx, ch, nil, Warn("careful"))
	op = q.Claim("slack")
	if op == nil || op.Message == nil || op.Message.Kind != MsgNotice || op.Message.Level != NoticeWarn {
		t.Fatalf("send must carry a typed message: %+v", op)
	}
}

// Input cards are sends with STABLE ids, so the reconciler re-deriving them on
// every pass posts once. Ordinary sends keep fresh ids, because a user who asks
// twice is owed two answers.
func TestInputCardDedupsButOrdinarySendsDoNot(t *testing.T) {
	ctx := context.Background()
	q := &OpQueue{Registry: NewRegistry()}
	ch := testChannel("c1", "slack")
	thread := "t-1"
	card := SignalMessage("", "vm-alerts", "DiskFull", "", nil, "boom")

	q.EnqueueInputDelivery(ctx, ch, "conv-1", "in-1", &thread, card)
	q.EnqueueInputDelivery(ctx, ch, "conv-1", "in-1", &thread, card) // reconcile repeat
	first := q.Claim("slack")
	if first == nil || first.ID != InputOpID("conv-1", "in-1", "c1") {
		t.Fatalf("input card id: %+v", first)
	}
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("duplicate input card not deduped: %+v", again)
	}
	// still deduped AFTER completion — this is the half a fresh-id send lacks,
	// and without it every reconcile would repost the alert
	q.Complete(ctx, first.ID, OpResult{})
	q.EnqueueInputDelivery(ctx, ch, "conv-1", "in-1", &thread, card)
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("input card reposted after completion: %+v", again)
	}
	// a DIFFERENT input on the same conversation is a different card
	q.EnqueueInputDelivery(ctx, ch, "conv-1", "in-2", &thread, card)
	if op := q.Claim("slack"); op == nil {
		t.Fatal("a second input must get its own card")
	}
	// ordinary sends are never suppressed
	q.EnqueueMessage(ctx, ch, &thread, Notice("same words"))
	q.EnqueueMessage(ctx, ch, &thread, Notice("same words"))
	if q.Claim("slack") == nil || q.Claim("slack") == nil {
		t.Fatal("repeated notices must both be delivered")
	}
}

// The delivery rule is a property of the input and the DESTINATION, so a new
// input type inherits correct behavior instead of defaulting to whichever
// branch someone forgot — and no lane needs a clause of its own.
func TestDeliverToRule(t *testing.T) {
	sig := func(kind string) *agentopsv1alpha1.InputItem {
		return &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
			Kind: agentopsv1alpha1.OriginSignal, Name: "src", SignalKind: kind}}
	}
	// An event nobody typed entered on no surface, so every bound channel is
	// owed it — chat included, which used to be the one exception.
	for _, kind := range []string{"alert", "job", "task", "chat"} {
		if !sig(kind).DeliverTo("telegram", "", true) {
			t.Fatalf("kind %q: an input no surface displayed must reach every channel", kind)
		}
	}
	// A chat message typed on a surface that echoes: everywhere but there.
	chat := sig("chat")
	if chat.DeliverTo("telegram", "telegram", true) {
		t.Fatal("the surface that displayed a message must not be sent it again")
	}
	if !chat.DeliverTo("console", "telegram", true) {
		t.Fatal("a channel that never showed the message is owed it")
	}
	// A surface that renders only what it is sent receives its own users'
	// messages, because echoing is a fact about the transport.
	if !chat.DeliverTo("console", "console", false) {
		t.Fatal("a viewer that does not echo must receive its own users' messages")
	}
	channel := &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
		Kind: agentopsv1alpha1.OriginChannel, Name: "home-ops"}}
	if channel.DeliverTo("home-ops", "home-ops", true) {
		t.Fatal("a reply typed in a thread must not be posted back into it")
	}
	if !channel.DeliverTo("console", "home-ops", true) {
		t.Fatal("a reply typed elsewhere is new to every other bound channel")
	}
	// pre-upgrade inputs: delivering them would spray history into every open
	// thread on the first reconcile after upgrade
	if (&agentopsv1alpha1.InputItem{}).DeliverTo("console", "", true) {
		t.Fatal("an input with no recorded origin must not be delivered")
	}
}

// The origin SURFACE is resolved in one place for both lanes, so a chat signal
// and a channel reply cannot drift apart.
func TestOriginSurfaceAndSender(t *testing.T) {
	chatSignal := &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
		Kind: agentopsv1alpha1.OriginSignal, Name: "tg-chat", SignalKind: "chat"}}
	labels := map[string]string{
		agentopsv1alpha1.LabelChatChannel: "telegram",
		agentopsv1alpha1.LabelChatSender:  "alice",
	}
	if got := chatSignal.OriginSurface(labels); got != "telegram" {
		t.Fatalf("chat signal surface = %q, want the channel it was typed on", got)
	}
	if got := chatSignal.OriginSender(labels); got != "alice" {
		t.Fatalf("chat signal sender = %q, want alice", got)
	}
	if !chatSignal.TypedByAPerson() {
		t.Fatal("a chat signal is somebody's words")
	}
	alert := &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
		Kind: agentopsv1alpha1.OriginSignal, Name: "vm-alerts", SignalKind: "alert"}}
	if alert.OriginSurface(nil) != "" {
		t.Fatal("an alert entered on no surface")
	}
	if alert.TypedByAPerson() {
		t.Fatal("an alert is an event, not somebody's words")
	}
	reply := &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
		Kind: agentopsv1alpha1.OriginChannel, Name: "console", Sender: "bob@example.com"}}
	if got := reply.OriginSurface(nil); got != "console" {
		t.Fatalf("channel surface = %q, want the channel itself", got)
	}
	if got := reply.OriginSender(nil); got != "bob@example.com" {
		t.Fatalf("channel sender = %q, want the recorded sender", got)
	}
}

// THE MANAGER RETURNS THE AGENT'S TEXT UNCHANGED.
//
// It used to parse the grammar and replace the body with a flattened form. Both
// are gone: the adapters parse, and a body the manager rewrote could not be the
// same characters a viewer reads back from `status.runs[].result`.
func TestAgentTextIsPassedThroughUnaltered(t *testing.T) {
	raw := "<title>\nDisk filling\n</title>\n<details>\nthe long tail\n</details>"

	if got := AnswerMessage(raw, StatusSucceeded).Body; got != raw {
		t.Errorf("an answer was rewritten:\n got  %q\n want %q", got, raw)
	}
	// A failed run that explained itself leaves as a notice, and that body is
	// the agent's too.
	failed := RunReplyMessage(&agentopsv1alpha1.RunStatus{Status: "failed", Result: raw})
	if failed.Kind != MsgNotice || failed.Level != NoticeWarn {
		t.Fatalf("a failed run with an explanation is a warn notice: %+v", failed)
	}
	if failed.Body != raw {
		t.Errorf("the explanation was rewritten:\n got  %q\n want %q", failed.Body, raw)
	}
	// A signal is a card, and its payload is never touched either.
	typed := "why won't\n<details>\nwork in my docs?\n</details>"
	if got := SignalMessage("p", "src", "", "", nil, typed).Body; got != typed {
		t.Errorf("a signal payload was rewritten:\n got  %q\n want %q", got, typed)
	}
}
