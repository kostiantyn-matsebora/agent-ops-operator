package main

import (
	"encoding/json"
	"testing"
)

func testAdapter(sources map[string]sourceConfig) *adapter {
	a := &adapter{sources: map[string]servedSource{}, reported: map[string]string{}}
	for name, cfg := range sources {
		a.sources[name] = servedSource{cfg: cfg}
	}
	return a
}

func mustUpdate(t *testing.T, raw string) tgUpdate {
	t.Helper()
	var u tgUpdate
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

// TestNormalizeChatSignal pins the normalized shape the manager depends on:
// the chat lane, a per-update fingerprint, and the reserved labels that let a
// reply find its way back to the surface the message came from.
func TestNormalizeChatSignal(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":77,"message":{"text":"check the disk",
		"chat":{"id":-1001},"from":{"id":42,"username":"operator"}}}`)

	source, sig, ok := a.normalize(upd)
	if !ok {
		t.Fatal("general-surface message should originate a signal")
	}
	if source != "tg-chat" {
		t.Fatalf("source = %q, want tg-chat", source)
	}
	if sig.Kind != "chat" {
		t.Fatalf("kind = %q, want chat", sig.Kind)
	}
	if sig.Fingerprint != "tg-77" {
		t.Fatalf("fingerprint = %q, want tg-77", sig.Fingerprint)
	}
	if sig.Payload != "check the disk" {
		t.Fatalf("payload = %q", sig.Payload)
	}
	if sig.Labels[labelChannel] != "telegram-ops" {
		t.Fatalf("%s = %q, want telegram-ops", labelChannel, sig.Labels[labelChannel])
	}
	if sig.Labels[labelSender] != "operator" {
		t.Fatalf("%s = %q, want operator", labelSender, sig.Labels[labelSender])
	}
}

// TestSenderFallsBackToID: not every Telegram user has a username.
func TestSenderFallsBackToID(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{"tg-chat": {ChatID: "-1001", Channel: "c"}})
	upd := mustUpdate(t, `{"update_id":1,"message":{"text":"hi","chat":{"id":-1001},"from":{"id":42}}}`)
	_, sig, ok := a.normalize(upd)
	if !ok {
		t.Fatal("should originate")
	}
	if sig.Labels[labelSender] != "42" {
		t.Fatalf("%s = %q, want 42", labelSender, sig.Labels[labelSender])
	}
}

// TestRepeatedTextGetsDistinctFingerprints: cooldown collapses on fingerprint,
// so identical text must NOT collapse — a human repeating a request is not a
// duplicate alert.
func TestRepeatedTextGetsDistinctFingerprints(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{"tg-chat": {ChatID: "-1001", Channel: "c"}})
	first := mustUpdate(t, `{"update_id":1,"message":{"text":"same","chat":{"id":-1001}}}`)
	second := mustUpdate(t, `{"update_id":2,"message":{"text":"same","chat":{"id":-1001}}}`)
	_, a1, _ := a.normalize(first)
	_, a2, _ := a.normalize(second)
	if a1.Fingerprint == a2.Fingerprint {
		t.Fatalf("identical text collapsed on fingerprint %q", a1.Fingerprint)
	}
}

// TestChatIDAndApproverFiltering is the shared-behavior test named in the plan:
// channel-telegram runs the SAME two rules on the continuation side, and the
// duplication is deliberate (the router holds no channel policy). Keep this
// table and channel-telegram's identical so drift shows up here, not as
// messages silently ignored in production.
func TestChatIDAndApproverFiltering(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops", Approvers: []int64{42, 43}},
	})
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"approved sender in the matching chat", `{"update_id":1,"message":{"text":"go","chat":{"id":-1001},"from":{"id":42}}}`, true},
		{"other approved sender", `{"update_id":2,"message":{"text":"go","chat":{"id":-1001},"from":{"id":43}}}`, true},
		{"unapproved sender is dropped", `{"update_id":3,"message":{"text":"go","chat":{"id":-1001},"from":{"id":99}}}`, false},
		{"missing sender with approvers set is dropped", `{"update_id":4,"message":{"text":"go","chat":{"id":-1001}}}`, false},
		{"unknown chat is dropped", `{"update_id":5,"message":{"text":"go","chat":{"id":-2002},"from":{"id":42}}}`, false},
		{"empty text is dropped", `{"update_id":6,"message":{"text":"","chat":{"id":-1001},"from":{"id":42}}}`, false},
		{"non-message update is dropped", `{"update_id":7}`, false},
		{"topic message belongs to the channel adapter", `{"update_id":8,"message":{"text":"go","chat":{"id":-1001},"from":{"id":42},"is_topic_message":true}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := a.normalize(mustUpdate(t, tc.raw))
			if ok != tc.want {
				t.Fatalf("originated = %v, want %v", ok, tc.want)
			}
		})
	}
}

// TestNoApproversMeansAnyone: an empty approver list is "the whole chat", not
// "nobody".
func TestNoApproversMeansAnyone(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{"tg-chat": {ChatID: "-1001", Channel: "c"}})
	_, _, ok := a.normalize(mustUpdate(t,
		`{"update_id":1,"message":{"text":"go","chat":{"id":-1001},"from":{"id":99}}}`))
	if !ok {
		t.Fatal("empty approvers must accept anyone in the chat")
	}
}

func TestTitleTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	if got := title(long); len([]rune(got)) != 61 {
		t.Fatalf("title length = %d, want 61 (60 + ellipsis)", len([]rune(got)))
	}
	if got := title("first line\nsecond line"); got != "first line" {
		t.Fatalf("title = %q, want first line only", got)
	}
}

// ---- the transport-local spelling ---------------------------------------
//
// Telegram command names admit only [a-z0-9_], so a hyphenated Pipeline is
// registered under an underscored spelling and the menu inserts THAT. It has to
// be reversed before anything leaves this adapter.

func TestReverseSpellingIsExactAndBoundedToTheCommandWord(t *testing.T) {
	for in, want := range map[string]string{
		"/k8s_observe check the disk": "/k8s-observe check the disk",
		"/k8s_observe":                "/k8s-observe",
		"/pipelines":                  "/pipelines",
		// The TASK is the person's own text and is never rewritten — only the
		// command word is ours.
		"/k8s_observe grep foo_bar": "/k8s-observe grep foo_bar",
		"not a command at all":      "not a command at all",
		"tell me about foo_bar":     "tell me about foo_bar",
	} {
		if got := reverseSpelling(in); got != want {
			t.Errorf("reverseSpelling(%q) = %q, want %q", in, got, want)
		}
	}
}

// The mapping is injective BY CONSTRUCTION: a Kubernetes object name is a
// DNS-1123 subdomain and cannot contain an underscore, so every `_` in a
// command word is one we introduced. That is what lets this adapter and
// channel-telegram each hold one line and no shared state.
func TestMenuSpellingReachesTheManagerAsTheRealName(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":91,"message":{"message_id":5,"text":"/k8s_observe check the disk",
		"chat":{"id":-1001},"from":{"id":42,"username":"operator"}}}`)
	_, sig, ok := a.normalize(upd)
	if !ok {
		t.Fatal("menu-inserted command should originate")
	}
	if sig.Payload != "/k8s-observe check the disk" {
		t.Fatalf("payload = %q — the alternate spelling escaped the adapter", sig.Payload)
	}
}

// The message handle is what lets a reply say which message it answers.
func TestChatSignalCarriesTheMessageHandle(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":78,"message":{"message_id":314,"text":"who is on call?",
		"chat":{"id":-1001},"from":{"id":42,"username":"operator"}}}`)
	_, sig, _ := a.normalize(upd)
	if sig.Labels[labelMessage] != "314" {
		t.Fatalf("%s = %q, want 314", labelMessage, sig.Labels[labelMessage])
	}

	// Optional: an update with no handle simply loses the linkage.
	upd = mustUpdate(t, `{"update_id":79,"message":{"text":"hi","chat":{"id":-1001}}}`)
	_, sig, _ = a.normalize(upd)
	if _, ok := sig.Labels[labelMessage]; ok {
		t.Fatalf("handle invented from nothing: %v", sig.Labels)
	}
}

// ---- selections ----------------------------------------------------------

const selectionUpdate = `{"update_id":80,"callback_query":{"id":"cb-1","data":"p:k8s-observe",
	"from":{"id":42,"username":"operator"},
	"message":{"message_id":9,"chat":{"id":-1001},
		"reply_to_message":{"message_id":8,"text":"who is on call?"}}}}`

// ONE TAP SENDS WHAT THEY ALREADY TYPED. The offer was posted as a reply to the
// person's own message, so Telegram still holds that text — nothing is retained
// on either side between the offer and the selection.
func TestSelectionCarriesTheOriginalMessage(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	source, sig, ok := a.normalize(mustUpdate(t, selectionUpdate))
	if !ok {
		t.Fatal("a selection should originate")
	}
	if source != "tg-chat" || sig.Kind != "chat" {
		t.Fatalf("source=%q kind=%q", source, sig.Kind)
	}
	if sig.Payload != "/k8s-observe who is on call?" {
		t.Fatalf("payload = %q — the original was not carried forward", sig.Payload)
	}
	if sig.Fingerprint != "tg-cb-cb-1" {
		t.Fatalf("fingerprint = %q", sig.Fingerprint)
	}
	if sig.Labels[labelMessage] != "8" {
		t.Fatalf("selection must link to the ORIGINAL message, got %q", sig.Labels[labelMessage])
	}
	if sig.Labels[labelSender] != "operator" {
		t.Fatalf("sender = %q", sig.Labels[labelSender])
	}
}

// An unrecoverable original is not an error to report — this adapter holds no
// Telegram credential. The addressed command goes out with no task and the
// manager answers with its own usage reply, on the surface they are looking at.
func TestSelectionWithNoRecoverableOriginalAsksForTheTask(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":81,"callback_query":{"id":"cb-2","data":"p:k8s-observe",
		"from":{"id":42},"message":{"message_id":9,"chat":{"id":-1001}}}}`)
	_, sig, ok := a.normalize(upd)
	if !ok {
		t.Fatal("a selection with no original should still reach the manager")
	}
	if sig.Payload != "/k8s-observe" {
		t.Fatalf("payload = %q, want the bare addressed command", sig.Payload)
	}
}

// Controls this adapter did not offer are none of its business.
func TestForeignCallbackDataIsIgnored(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	for _, data := range []string{"", "something-else", "x:k8s-observe"} {
		upd := mustUpdate(t, `{"update_id":82,"callback_query":{"id":"cb-3","data":"`+data+`",
			"message":{"message_id":9,"chat":{"id":-1001}}}}`)
		if _, _, ok := a.normalize(upd); ok {
			t.Fatalf("data %q should not originate", data)
		}
	}
}

// A selection inside a topic is a CONTINUATION and belongs to the channel
// adapter — this one must never turn it into a second conversation.
func TestSelectionInsideATopicIsNotOrigination(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":83,"callback_query":{"id":"cb-4","data":"p:k8s-observe",
		"message":{"message_id":9,"chat":{"id":-1001},"is_topic_message":true}}}`)
	if _, _, ok := a.normalize(upd); ok {
		t.Fatal("in-topic selection must not originate")
	}
}

// Approver filtering applies to whoever TAPPED, not to whoever was offered.
func TestSelectionRespectsApprovers(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops", Approvers: []int64{7}},
	})
	if _, _, ok := a.normalize(mustUpdate(t, selectionUpdate)); ok {
		t.Fatal("unapproved tapper originated a conversation")
	}
}

// ---- answering a prompt --------------------------------------------------
//
// Telegram's command menu SENDS on tap, so `/k8s_ops` arrives bare and the
// manager asks what the task is. The answer has to find its way back to the
// Pipeline — and it does so through Telegram's own reply chain, with nothing
// remembered on either side.

const promptChain = `{"update_id":90,"message":{"message_id":12,"text":"check the disk",
	"chat":{"id":-1001},"from":{"id":42,"username":"operator"},
	"reply_to_message":{"message_id":11,"text":"What should k8s-ops do? Reply with the task.",
		"reply_to_message":{"message_id":10,"text":"/k8s_ops@ExampleOpsBot"}}}}`

func TestReplyToAPromptRebuildsTheAddressedCommand(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	_, sig, ok := a.normalize(mustUpdate(t, promptChain))
	if !ok {
		t.Fatal("a prompted answer should originate")
	}
	// The pipeline comes from the person's OWN command two links up, not from
	// parsing the manager's prose — and the menu spelling is reversed on the way.
	if sig.Payload != "/k8s-ops check the disk" {
		t.Fatalf("payload = %q", sig.Payload)
	}
}

// An ordinary reply inside the general surface is not an answer to a prompt.
func TestOrdinaryReplyIsNotTreatedAsAPrompt(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":91,"message":{"message_id":12,"text":"thanks",
		"chat":{"id":-1001},"reply_to_message":{"message_id":11,"text":"some answer",
			"reply_to_message":{"message_id":10,"text":"what do you think?"}}}}`)
	_, sig, _ := a.normalize(upd)
	if sig.Payload != "thanks" {
		t.Fatalf("payload = %q — an ordinary reply was rewritten", sig.Payload)
	}
}

// A command that ALREADY carried a task was never prompted, so a later reply to
// it must not be prefixed a second time.
func TestAddressedCommandWithATaskIsNotAPrompt(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":92,"message":{"message_id":12,"text":"and the memory?",
		"chat":{"id":-1001},"reply_to_message":{"message_id":11,"text":"here you go",
			"reply_to_message":{"message_id":10,"text":"/k8s_ops check the disk"}}}}`)
	_, sig, _ := a.normalize(upd)
	if sig.Payload != "and the memory?" {
		t.Fatalf("payload = %q", sig.Payload)
	}
}

// One link is not enough: replying straight to a bare command is somebody
// quoting it, not answering a question about it.
func TestReplyDirectlyToACommandIsNotAPrompt(t *testing.T) {
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"},
	})
	upd := mustUpdate(t, `{"update_id":93,"message":{"message_id":12,"text":"check the disk",
		"chat":{"id":-1001},"reply_to_message":{"message_id":10,"text":"/k8s_ops"}}}`)
	_, sig, _ := a.normalize(upd)
	if sig.Payload != "check the disk" {
		t.Fatalf("payload = %q", sig.Payload)
	}
}
