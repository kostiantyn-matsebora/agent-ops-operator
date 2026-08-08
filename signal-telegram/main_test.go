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
