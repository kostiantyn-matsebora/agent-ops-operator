package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	// The CANONICAL captured update, shared with the e2e pack (test-only
	// relative read, no go.mod entry): the fake Bot API replays this exact
	// payload and the router forwards it verbatim, so what this test pins is
	// what the pack sends.
	a := testAdapter(map[string]sourceConfig{
		"tg-chat": {ChatID: "-1001234567890", Channel: "telegram-ops"},
	})
	raw, err := os.ReadFile("../../test/fixtures/telegram-update-message.json")
	if err != nil {
		t.Fatal(err)
	}
	upd := mustUpdate(t, string(raw))

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

// ---- the real path -------------------------------------------------------
//
// These drive the ENTRY POINT — an update POSTed to /updates, exactly as the
// router forwards one — and assert what this adapter POSTs to the manager.
// Nothing calls normalize() directly.
//
// The earlier version of these tests did, with a payload shaped the way I
// assumed Telegram sends it. It does not: `reply_to_message` is ONE level deep
// and never carries its own. The tests passed and the feature did not work, so
// they were asserting my assumption rather than the behaviour.
//
// The wire SHAPE is still an assumption these cannot check — only the live
// transport can settle that. What they do check is everything downstream of it.

// postedSignal runs one update through the adapter's HTTP handler and returns
// what reached the manager.
func postedSignal(t *testing.T, cfg map[string]sourceConfig, update string) (string, []Signal, bool) {
	t.Helper()
	var gotSource string
	var gotSignals []Signal
	var reached bool

	mgr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Source  string   `json:"source"`
			Signals []Signal `json:"signals"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &in); err != nil {
			t.Errorf("manager received unparseable body: %v", err)
		}
		gotSource, gotSignals, reached = in.Source, in.Signals, true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mgr.Close()

	a := testAdapter(cfg)
	a.mgr = NewManager(mgr.URL, "tok")

	rec := httptest.NewRecorder()
	a.handleUpdate(rec, httptest.NewRequest("POST", "/updates", strings.NewReader(update)))
	if rec.Code >= 400 {
		t.Fatalf("handler refused the update: %d %s", rec.Code, rec.Body.String())
	}
	return gotSource, gotSignals, reached
}

var opsChat = map[string]sourceConfig{"tg-chat": {ChatID: "-1001", Channel: "telegram-ops"}}

// The shape Telegram actually sends: ONE level of reply, and the question's own
// text is the only place the Pipeline survives.
const promptChain = `{"update_id":90,"message":{"message_id":12,"text":"check the disk",
	"chat":{"id":-1001},"from":{"id":42,"username":"operator"},
	"reply_to_message":{"message_id":11,"from":{"id":9,"is_bot":true,"username":"ExampleOpsBot"},
		"text":"⚠️ 💬 Reply with the task for /k8s_ops."}}}`

func TestPromptedAnswerReachesTheManagerAddressed(t *testing.T) {
	source, sigs, reached := postedSignal(t, opsChat, promptChain)
	if !reached {
		t.Fatal("nothing reached the manager")
	}
	if source != "tg-chat" || len(sigs) != 1 {
		t.Fatalf("source=%q signals=%d", source, len(sigs))
	}
	if sigs[0].Payload != "/k8s-ops check the disk" {
		t.Fatalf("payload = %q — the manager cannot tell which pipeline this is for", sigs[0].Payload)
	}
	if sigs[0].Kind != "chat" {
		t.Fatalf("kind = %q", sigs[0].Kind)
	}
	if sigs[0].Labels[labelChannel] != "telegram-ops" {
		t.Fatalf("reply has nowhere to go: %v", sigs[0].Labels)
	}
}

// A reply to a PERSON is that person's words, whatever they quoted.
func TestReplyToAPersonReachesTheManagerUnchanged(t *testing.T) {
	_, sigs, reached := postedSignal(t, opsChat, `{"update_id":91,"message":{"message_id":12,
		"text":"check the disk","chat":{"id":-1001},"from":{"id":42},
		"reply_to_message":{"message_id":11,"from":{"id":7,"is_bot":false,"username":"dana"},
			"text":"try /k8s_ops for that"}}}`)
	if !reached {
		t.Fatal("nothing reached the manager")
	}
	if sigs[0].Payload != "check the disk" {
		t.Fatalf("payload = %q — a person's quote was treated as a prompt", sigs[0].Payload)
	}
}

// A bot message with no command in it is not a prompt.
func TestReplyToABotMessageWithNoCommandIsUnchanged(t *testing.T) {
	_, sigs, _ := postedSignal(t, opsChat, `{"update_id":92,"message":{"message_id":12,
		"text":"thanks","chat":{"id":-1001},"from":{"id":42},
		"reply_to_message":{"message_id":11,"from":{"id":9,"is_bot":true},
			"text":"the pod restarted twice"}}}`)
	if sigs[0].Payload != "thanks" {
		t.Fatalf("payload = %q", sigs[0].Payload)
	}
}

// A menu tap: the command arrives bare, and must reach the manager as one so it
// can ask for the task.
func TestBareMenuCommandReachesTheManagerBare(t *testing.T) {
	_, sigs, _ := postedSignal(t, opsChat, `{"update_id":93,"message":{"message_id":10,
		"text":"/k8s_ops@ExampleOpsBot","chat":{"id":-1001},"from":{"id":42}}}`)
	if sigs[0].Payload != "/k8s-ops@ExampleOpsBot" {
		t.Fatalf("payload = %q", sigs[0].Payload)
	}
	// The handle is what lets the manager's question be a REPLY to this, which
	// is the whole mechanism the answer travels back through.
	if sigs[0].Labels[labelMessage] != "10" {
		t.Fatalf("no message handle: %v", sigs[0].Labels)
	}
}
