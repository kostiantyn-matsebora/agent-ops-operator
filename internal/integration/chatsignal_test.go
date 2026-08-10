package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// Origination is signal-only. These tests exercise the path a general-surface
// chat message actually takes: signal-telegram normalizes it and posts
// /signal/inbound, where the Pipeline CLAIMING the chat source decides who
// answers — no channel default, no creation-timestamp tiebreak.

var chatFingerprint int

// chatSignal posts one general-surface message as the chat adapter would.
func chatSignal(t *testing.T, srv *httpapi.Server, source, channel, text string) *httptest.ResponseRecorder {
	t.Helper()
	chatFingerprint++
	return adapterReq(srv, "POST", "/signal/inbound", map[string]any{
		"source": source,
		"signals": []map[string]any{{
			"fingerprint": fmt.Sprintf("tg-%d", chatFingerprint),
			"kind":        "chat",
			"payload":     text,
			"labels":      map[string]string{"agentops.dev/channel": channel},
		}},
	}, "test-adapter-token")
}

// mkChatSource creates a chat SignalSource served by a telegram signal adapter.
func mkChatSource(t *testing.T, name, channel string) {
	t.Helper()
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, ns
	src.Spec.Adapter = "telegram"
	src.Spec.Config = &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{"chatId":"-100","channel":%q}`, channel)),
	}
	if err := k8sClient.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
}

func convsBoundTo(t *testing.T, channel string) []agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	var out []agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].BoundTo(channel) {
			out = append(out, list.Items[i])
		}
	}
	return out
}

// 7.1: a general-surface message opens a conversation with the CLAIMING
// pipeline's profile and channel set, and the source's counter moves.
func TestChatSignalOriginatesWithClaimingPipeline(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-chat")
	mkChannel(t, "chan-chat", "telegram")
	mkChatSource(t, "src-chat", "chan-chat")
	mkPipeline(t, "chat-pipe", []string{"src-chat"}, []string{"chan-chat"}, "prof-chat")
	reconcilePipeline(t, "chat-pipe")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-chat", "chan-chat", "check the disk")
	if rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-chat")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	conv := convs[0]
	if conv.Spec.ProfileRef.Name != "prof-chat" {
		t.Fatalf("profile = %q, want the claiming pipeline's", conv.Spec.ProfileRef.Name)
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].Type != agentopsv1alpha1.InputTask {
		t.Fatalf("chat must take the task lane: %+v", conv.Spec.Inputs)
	}

	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "src-chat"}, &src); err != nil {
		t.Fatal(err)
	}
	if src.Status.ReceivedTotal != 1 {
		t.Fatalf("receivedTotal = %d, want 1", src.Status.ReceivedTotal)
	}
}

// 7.2: an unclaimed chat source drops — and the person who typed is told,
// rather than only a condition changing somewhere they cannot see.
func TestUnclaimedChatSourceDropsAndTellsTheUser(t *testing.T) {
	mkChannel(t, "chan-unwired", "telegram")
	mkChatSource(t, "src-unwired", "chan-unwired")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-unwired", "chan-unwired", "anyone there?")
	if rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	reason, _ := out["reason"].(string)
	if !strings.Contains(reason, "not claimed") {
		t.Fatalf("want a drop reason, got %v", out)
	}
	if n := len(convsBoundTo(t, "chan-unwired")); n != 0 {
		t.Fatalf("unclaimed source must create no conversation, got %d", n)
	}
	// the reason reaches the originating surface as a send op
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("expected a send op carrying the drop reason: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || !strings.Contains(opBody(op), "src-unwired") {
		t.Fatalf("drop reason must name the unwired source: %+v", op)
	}
}

// 7.3: two Ready pipelines share a channel. Resolution is by CLAIM of the
// source, so creation order is irrelevant — this is the timestamp tiebreak
// this change exists to delete.
func TestResolutionIsByClaimNotCreationOrder(t *testing.T) {
	mkProfile(t, "prof-first")
	mkProfile(t, "prof-second")
	mkChannel(t, "chan-shared", "telegram")
	mkChatSource(t, "src-shared", "chan-shared")

	// created FIRST, shares the channel, but claims no chat source
	mkPipeline(t, "pipe-older", nil, []string{"chan-shared"}, "prof-first")
	reconcilePipeline(t, "pipe-older")
	// created SECOND, and it is the one claiming the chat source
	mkPipeline(t, "pipe-newer", []string{"src-shared"}, []string{"chan-shared"}, "prof-second")
	reconcilePipeline(t, "pipe-newer")

	srv := apiServer()
	if rec := chatSignal(t, srv, "src-shared", "chan-shared", "who answers?"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-shared")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	if got := convs[0].Spec.ProfileRef.Name; got != "prof-second" {
		t.Fatalf("profile = %q, want prof-second (the CLAIMANT, not the oldest pipeline)", got)
	}
}

// 7.4: /channel/inbound is reply-only. A missing threadId is refused with a
// message naming the signal path, and an unknown thread is not adopted.
func TestChannelInboundIsReplyOnly(t *testing.T) {
	mkProfile(t, "prof-replyonly")
	mkChannel(t, "chan-replyonly", "telegram")
	mkPipeline(t, "replyonly-pipe", nil, []string{"chan-replyonly"}, "prof-replyonly")
	reconcilePipeline(t, "replyonly-pipe")
	srv := apiServer()

	rec := adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-replyonly", "text": "start something"}, "test-adapter-token")
	if rec.Code != 400 {
		t.Fatalf("missing threadId must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/signal/inbound") {
		t.Fatalf("rejection must name the signal path: %s", rec.Body.String())
	}

	// unknown thread: accepted by the contract, adopted by nothing
	rec = adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-replyonly", "threadId": "9999", "text": "hello?"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("unknown thread: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(convsBoundTo(t, "chan-replyonly")); n != 0 {
		t.Fatalf("unknown thread must not be adopted, got %d conversation(s)", n)
	}
}

// 7.6: a command that only produces a response answers in place and creates
// nothing; an addressed pipeline still opens a conversation.
func TestChatCommandsAnswerWithoutCreatingConversations(t *testing.T) {
	mkProfile(t, "prof-cmd")
	mkChannel(t, "chan-cmd", "telegram")
	mkChatSource(t, "src-cmd", "chan-cmd")
	mkPipeline(t, "cmd-pipe", []string{"src-cmd"}, []string{"chan-cmd"}, "prof-cmd")
	reconcilePipeline(t, "cmd-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-cmd", "chan-cmd", "/agents"); rec.Code != 200 {
		t.Fatalf("/agents: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(convsBoundTo(t, "chan-cmd")); n != 0 {
		t.Fatalf("/agents must create no conversation, got %d", n)
	}
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("/agents must emit a send op: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || !strings.Contains(opBody(op), "/cmd-pipe") {
		t.Fatalf("listing must name addressable pipelines: %+v", op)
	}

	// an addressed pipeline still opens a conversation, on the pipeline named
	if rec := chatSignal(t, srv, "src-cmd", "chan-cmd", "/cmd-pipe check nodes"); rec.Code != 200 {
		t.Fatalf("addressed command: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-cmd")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	if convs[0].Spec.Inputs[0].Payload != "check nodes" {
		t.Fatalf("task payload: %+v", convs[0].Spec.Inputs[0])
	}
}

// A chat conversation is titled by the QUESTION, not by the surface it arrived
// on. Falling back to the source name gave every conversation from one surface
// the same title, which makes a list of them unreadable and search useless.
func TestChatConversationIsTitledByTheMessage(t *testing.T) {
	mkProfile(t, "prof-title")
	mkChannel(t, "chan-title", "telegram")
	mkChatSource(t, "src-title", "chan-title")
	mkPipeline(t, "title-pipe", []string{"src-title"}, []string{"chan-title"}, "prof-title")
	reconcilePipeline(t, "title-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-title", "chan-title",
		"  why   is the api pod   crashlooping? "); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsBoundTo(t, "chan-title")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	title := convs[0].Spec.Title
	if !strings.Contains(title, "why is the api pod crashlooping?") {
		t.Fatalf("title must come from the message, with whitespace collapsed: %q", title)
	}
	if strings.Contains(title, "src-title") {
		t.Fatalf("the source name is not a useful title for a question: %q", title)
	}

	// A long question is bounded, and cut on a RUNE so multi-byte input is not
	// sliced in half.
	long := strings.Repeat("почему ", 40)
	if rec := chatSignal(t, srv, "src-title", "chan-title", long); rec.Code != 200 {
		t.Fatalf("long chat signal: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range convsBoundTo(t, "chan-title") {
		if n := len([]rune(c.Spec.Title)); n > 60 {
			t.Fatalf("title not bounded: %d runes", n)
		}
		if !utf8.ValidString(c.Spec.Title) {
			t.Fatalf("title was cut mid-character: %q", c.Spec.Title)
		}
	}
}

// An ALERT keeps the source name: its payload is a machine document, and the
// source is the useful label there.
func TestAlertConversationKeepsTheSourceTitle(t *testing.T) {
	mkProfile(t, "prof-alerttitle")
	mkSignalSource(t, "src-alerttitle", "am-alerttitle", "")
	mkPipeline(t, "alerttitle-pipe", []string{"src-alerttitle"}, nil, "prof-alerttitle")
	reconcilePipeline(t, "alerttitle-pipe")

	rec := postSignal(t, apiServer().Handler(), testMasterToken, "src-alerttitle", []map[string]any{
		{"fingerprint": "at-1", "labels": map[string]string{"alertname": "DiskFull"}, "payload": "disk is full"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(context.Background(), &list)
	found := false
	for i := range list.Items {
		if strings.Contains(list.Items[i].Spec.Title, "src-alerttitle") {
			found = true
		}
	}
	if !found {
		t.Fatal("an alert conversation should still be titled by its source")
	}
}

// 7.7: cooldown is OFF for chat — repeating yourself is not dedup.
func TestRepeatedChatTextCreatesTwoConversations(t *testing.T) {
	mkProfile(t, "prof-rep")
	mkChannel(t, "chan-rep", "telegram")
	mkChatSource(t, "src-rep", "chan-rep")
	mkPipeline(t, "rep-pipe", []string{"src-rep"}, []string{"chan-rep"}, "prof-rep")
	reconcilePipeline(t, "rep-pipe")
	srv := apiServer()

	for i := 0; i < 2; i++ {
		if rec := chatSignal(t, srv, "src-rep", "chan-rep", "same question"); rec.Code != 200 {
			t.Fatalf("chat signal %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if n := len(convsBoundTo(t, "chan-rep")); n != 2 {
		t.Fatalf("want 2 conversations (no cooldown, no grouping for chat), got %d", n)
	}
}

// 7.8: a chat signal that names no channel is unanswerable — refused at the
// door rather than accepted with its reply silently dropped.
func TestChatSignalWithoutChannelLabelIsRejected(t *testing.T) {
	mkChannel(t, "chan-nolabel", "telegram")
	mkChatSource(t, "src-nolabel", "chan-nolabel")
	srv := apiServer()

	rec := adapterReq(srv, "POST", "/signal/inbound", map[string]any{
		"source": "src-nolabel",
		"signals": []map[string]any{{
			"fingerprint": "tg-nolabel", "kind": "chat", "payload": "hi",
		}},
	}, "test-adapter-token")
	if rec.Code != 400 {
		t.Fatalf("want 400, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agentops.dev/channel") {
		t.Fatalf("rejection must name the missing label: %s", rec.Body.String())
	}
}
