// Conversation object names, end to end through /signal/inbound:
// conversation-naming/spec.md's own alert/job/chat/task scenarios, plus
// the caption-less fallback to the claiming SignalSource's name.
package integration

import (
	"context"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// findConversationByTitle is a test-local lookup — the naming tests each
// create exactly one conversation carrying a distinctive title.
func findConversationByTitle(t *testing.T, title string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	for i := range list.Items {
		if list.Items[i].Spec.Title == title {
			return &list.Items[i]
		}
	}
	t.Fatalf("no conversation titled %q", title)
	return nil
}

// conversation-naming/spec.md: "An alert conversation's name hints at the
// alert" — CamelCase and punctuation both split into words.
func TestAlertConversationNameHintsAtTheAlert(t *testing.T) {
	mkProfile(t, "prof-name-alert")
	mkSignalSource(t, "src-name-alert", "am-name-alert", "")
	mkPipeline(t, "name-alert-pipe", []string{"src-name-alert"}, nil, "prof-name-alert")
	reconcilePipeline(t, "name-alert-pipe")

	rec := postSignal(t, apiServer().Handler(), testMasterToken, "src-name-alert", []map[string]any{
		{"fingerprint": "name-alert-1", "labels": map[string]string{"alertname": "x"},
			"title": "NodeDown — ns-prod"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := findConversationByTitle(t, "NodeDown — ns-prod")
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	if conv.Name != "alert-node-down-ns" {
		t.Fatalf("name = %q, want alert-node-down-ns", conv.Name)
	}
}

// conversation-naming/spec.md: the job-kind path shares the same word
// source as alert (the title already derived for spec.Title).
func TestJobConversationNameHintsAtTheTitle(t *testing.T) {
	mkProfile(t, "prof-name-job")
	mkSignalSource(t, "src-name-job", "cron-name-job", "")
	mkPipeline(t, "name-job-pipe", []string{"src-name-job"}, nil, "prof-name-job")
	reconcilePipeline(t, "name-job-pipe")

	rec := postSignal(t, apiServer().Handler(), testMasterToken, "src-name-job", []map[string]any{
		{"fingerprint": "name-job-1", "labels": map[string]string{"alertname": "nightly"},
			"title": "Nightly backup of prod-db", "payload": "run it", "kind": "job"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := findConversationByTitle(t, "Nightly backup of prod-db")
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	if conv.Name != "job-nightly-backup-prod" {
		t.Fatalf("name = %q, want job-nightly-backup-prod", conv.Name)
	}
}

// conversation-naming/spec.md: "A caption-less chat message still slugs" —
// a chat-kind signal with no title and no significant payload word falls
// back to the claiming SignalSource's own name.
func TestChatConversationWithNoCaptionNamesTheSource(t *testing.T) {
	mkProfile(t, "prof-name-chat-empty")
	mkChannel(t, "chan-name-chat-empty", "telegram")
	mkChatSource(t, "ops-room", "chan-name-chat-empty")
	mkPipeline(t, "name-chat-empty-pipe", []string{"ops-room"}, []string{"chan-name-chat-empty"}, "prof-name-chat-empty")
	reconcilePipeline(t, "name-chat-empty-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "ops-room", "chan-name-chat-empty", ""); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-name-chat-empty")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	t.Cleanup(func() { cleanupConversation(t, convs[0].Name) })
	if convs[0].Name != "chat-ops-room" {
		t.Fatalf("name = %q, want chat-ops-room", convs[0].Name)
	}
}

// A sticker/voice-note caption that IS text but carries no significant
// WORD (only punctuation/emoji) must fall back the same way, even though
// titleForGroup's own payload-derived title is non-empty.
func TestChatConversationWithSymbolOnlyCaptionNamesTheSource(t *testing.T) {
	mkProfile(t, "prof-name-chat-sym")
	mkChannel(t, "chan-name-chat-sym", "telegram")
	mkChatSource(t, "ops-room-two", "chan-name-chat-sym")
	mkPipeline(t, "name-chat-sym-pipe", []string{"ops-room-two"}, []string{"chan-name-chat-sym"}, "prof-name-chat-sym")
	reconcilePipeline(t, "name-chat-sym-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "ops-room-two", "chan-name-chat-sym", "👍"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-name-chat-sym")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	t.Cleanup(func() { cleanupConversation(t, convs[0].Name) })
	if convs[0].Name != "chat-ops-room-two" {
		t.Fatalf("name = %q, want chat-ops-room-two", convs[0].Name)
	}
	// The title itself is untouched by the name's own fallback — it still
	// carries the emoji caption, not the source name.
	if !strings.Contains(convs[0].Spec.Title, "👍") {
		t.Fatalf("title must still come from the message: %q", convs[0].Spec.Title)
	}
}

// conversation-naming/spec.md: a chat message that DOES carry words names
// from them, never the source — same shape as TestChatSignalOriginatesWithClaimingPipeline's
// title assertion, pinned here for the object NAME instead.
func TestChatConversationWithWordsNamesFromTheMessage(t *testing.T) {
	mkProfile(t, "prof-name-chat-words")
	mkChannel(t, "chan-name-chat-words", "telegram")
	mkChatSource(t, "src-name-chat-words", "chan-name-chat-words")
	mkPipeline(t, "name-chat-words-pipe", []string{"src-name-chat-words"}, []string{"chan-name-chat-words"}, "prof-name-chat-words")
	reconcilePipeline(t, "name-chat-words-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-name-chat-words", "chan-name-chat-words", "why is the api pod crashlooping?"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-name-chat-words")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	t.Cleanup(func() { cleanupConversation(t, convs[0].Name) })
	if convs[0].Name != "chat-why-api-pod" {
		t.Fatalf("name = %q, want chat-why-api-pod", convs[0].Name)
	}
}
