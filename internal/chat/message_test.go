package chat

import (
	"context"
	"strings"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
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

	q.EnqueueInputCard(ctx, ch, "conv-1", "in-1", &thread, card)
	q.EnqueueInputCard(ctx, ch, "conv-1", "in-1", &thread, card) // reconcile repeat
	first := q.Claim("slack")
	if first == nil || first.ID != InputCardOpID("conv-1", "in-1", "c1") {
		t.Fatalf("input card id: %+v", first)
	}
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("duplicate input card not deduped: %+v", again)
	}
	// still deduped AFTER completion — this is the half a fresh-id send lacks,
	// and without it every reconcile would repost the alert
	q.Complete(ctx, first.ID, OpResult{})
	q.EnqueueInputCard(ctx, ch, "conv-1", "in-1", &thread, card)
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("input card reposted after completion: %+v", again)
	}
	// a DIFFERENT input on the same conversation is a different card
	q.EnqueueInputCard(ctx, ch, "conv-1", "in-2", &thread, card)
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

// The posting rule is a property of the input, so a new input type inherits
// correct behavior instead of defaulting to whichever branch someone forgot.
func TestPostToChannelsRule(t *testing.T) {
	sig := func(kind string) *agentopsv1alpha1.InputItem {
		return &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
			Kind: agentopsv1alpha1.OriginSignal, Name: "src", SignalKind: kind}}
	}
	for kind, want := range map[string]bool{"alert": true, "job": true, "task": true, "chat": false} {
		if got := sig(kind).PostToChannels(); got != want {
			t.Fatalf("kind %q: PostToChannels() = %v, want %v", kind, got, want)
		}
	}
	channel := &agentopsv1alpha1.InputItem{Origin: &agentopsv1alpha1.InputOrigin{
		Kind: agentopsv1alpha1.OriginChannel, Name: "home-ops"}}
	if channel.PostToChannels() {
		t.Fatal("a channel-originated input is an echo and must not be posted")
	}
	// pre-upgrade inputs: posting them would spray history into every open
	// thread on the first reconcile after upgrade
	if (&agentopsv1alpha1.InputItem{}).PostToChannels() {
		t.Fatal("an input with no recorded origin must not be posted")
	}
}
