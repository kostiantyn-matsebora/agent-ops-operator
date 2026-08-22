package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// A conversation thread should read as: the EVENT, then the work, then the
// answer. Before this, an alert thread was a topic title, then silence, then
// the agent's interpretation — and if the agent hung or died, the thread never
// said what had happened at all.

// drainOps claims every queued op for an adapter and returns them in order.
func drainOps(t *testing.T, srv *httpapi.Server, adapter string) []chat.Op {
	t.Helper()
	var out []chat.Op
	for i := 0; i < 50; i++ {
		rec := adapterReq(srv, "GET", "/channel/ops?adapter="+adapter+"&contract=2&wait=0", nil, testMasterToken)
		if rec.Code != 200 {
			return out
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		out = append(out, op)
	}
	return out
}

// cardsFor returns the input cards among a batch of ops.
func cardsFor(ops []chat.Op) []chat.Op {
	var out []chat.Op
	for _, op := range ops {
		if op.Kind == chat.OpSend && op.Message != nil && op.Message.Kind == chat.MsgSignal {
			out = append(out, op)
		}
	}
	return out
}

// bindThread completes ensure-topic ops ALREADY CLAIMED by the caller, so the
// conversation gains a thread. It takes the drained ops rather than draining
// again: claiming an op without completing it leaves it pending, and the
// reconciler's re-enqueue then dedups against it forever — which is exactly how
// a real adapter wedges a conversation, and worth not reproducing in a helper.
func bindThread(t *testing.T, srv *httpapi.Server, ops []chat.Op, threadID string) {
	t.Helper()
	for _, op := range ops {
		if op.Kind != chat.OpEnsureTopic {
			continue
		}
		rec := adapterReq(srv, "POST", "/channel/ops/"+op.ID+"/done",
			chat.OpResult{ThreadID: threadID}, testMasterToken)
		if rec.Code != 200 {
			t.Fatalf("complete ensure-topic: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func convForProfile(t *testing.T, profile string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == profile {
			return &list.Items[i]
		}
	}
	return nil
}

// 6.4 / 6.11 — an alert-originated conversation posts its card, naming the
// source and the INFERRED pipeline, before any run completes.
func TestAlertConversationPostsItsCard(t *testing.T) {
	mkProfile(t, "prof-card")
	mkChannel(t, "chan-card", "tg-card")
	mkSignalSource(t, "src-card", "am-card", "")
	mkPipeline(t, "card-pipe", []string{"src-card"}, []string{"chan-card"}, "prof-card")
	reconcilePipeline(t, "card-pipe")
	srv := apiServer()

	rec := postSignal(t, srv.Handler(), testMasterToken, "src-card", []map[string]any{{
		"fingerprint": "card-1", "labels": map[string]string{"alertname": "CardAlert", "namespace": "prod"},
		"payload": "node/1 root filesystem at 97%",
	}})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-card")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	// no thread yet: nothing may be posted, and nothing may be LOST either
	reconcileWithOps(t, srv, conv.Name)
	ops := drainOps(t, srv, "tg-card")
	if n := len(cardsFor(ops)); n != 0 {
		t.Fatalf("a card before the thread binding would be dropped by fan-out, got %d", n)
	}

	bindThread(t, srv, ops, "9001")
	reconcileWithOps(t, srv, conv.Name)

	cards := cardsFor(drainOps(t, srv, "tg-card"))
	if len(cards) != 1 {
		t.Fatalf("want exactly one card once the thread exists, got %d", len(cards))
	}
	card := cards[0].Message
	if card.Source != "src-card" {
		t.Fatalf("card must name its source: %+v", card)
	}
	if card.Pipeline != "card-pipe" {
		t.Fatalf("card must name the INFERRED pipeline: %+v", card)
	}
	if !strings.Contains(card.Body, "root filesystem at 97%") {
		t.Fatalf("card must carry the event payload: %q", card.Body)
	}
	if card.Labels["alertname"] != "CardAlert" || card.Labels["namespace"] != "prod" {
		t.Fatalf("card must carry the signal labels for the adapter to render: %+v", card.Labels)
	}
	if card.InputRef == "" {
		t.Fatalf("card must cite where the full event lives: %+v", card)
	}
	if cards[0].ThreadID == nil || *cards[0].ThreadID != "9001" {
		t.Fatalf("card must land in the conversation thread: %+v", cards[0].ThreadID)
	}

	// 6.5 — repeated reconciles post exactly one card per bound channel
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	if n := len(cardsFor(drainOps(t, srv, "tg-card"))); n != 0 {
		t.Fatalf("reconcile reposted the card %d time(s) — the stable op id is not deduping", n)
	}
}

// 6.6 — a chat message is not delivered back to the surface it was typed on.
// The rule is per DESTINATION and reads the origin SURFACE, so the chat lane
// needs no clause of its own: here the only bound channel IS that surface, and
// its transport shows a person their own message.
func TestChatSignalIsNotDeliveredToItsOwnSurface(t *testing.T) {
	mkProfile(t, "prof-nocard")
	mkChannel(t, "chan-nocard", "tg-nocard")
	mkChatSource(t, "src-nocard", "chan-nocard")
	mkPipeline(t, "nocard-pipe", []string{"src-nocard"}, []string{"chan-nocard"}, "prof-nocard")
	reconcilePipeline(t, "nocard-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-nocard", "chan-nocard", "why is api down?"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-nocard")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	if in := conv.Spec.Inputs[0]; in.Origin == nil || in.Origin.SignalKind != "chat" {
		t.Fatalf("a chat input records its lane so it can be excluded: %+v", in.Origin)
	}

	reconcileWithOps(t, srv, conv.Name)
	bindThread(t, srv, drainOps(t, srv, "tg-nocard"), "7001")
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	for _, op := range drainOps(t, srv, "tg-nocard") {
		if op.Message != nil && (op.Message.Kind == chat.MsgSignal || op.Message.Kind == chat.MsgRelay) {
			t.Fatalf("a message must not go back to the surface that displayed it: %+v", op.Message)
		}
	}
}

// 6.7 — a posted task explains itself, so its topic does not appear in chat
// with no stated cause.
func TestTaskSignalPostsItsCard(t *testing.T) {
	mkProfile(t, "prof-taskcard")
	mkChannel(t, "chan-taskcard", "tg-taskcard")
	mkSignalSource(t, "src-taskcard", "api-taskcard", "")
	mkPipeline(t, "taskcard-pipe", []string{"src-taskcard"}, []string{"chan-taskcard"}, "prof-taskcard")
	reconcilePipeline(t, "taskcard-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "src-taskcard", "tc-1", "roll the api deployment", nil); rec.Code != 200 {
		t.Fatalf("task signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-taskcard")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	reconcileWithOps(t, srv, conv.Name)
	bindThread(t, srv, drainOps(t, srv, "tg-taskcard"), "8001")
	reconcileWithOps(t, srv, conv.Name)

	cards := cardsFor(drainOps(t, srv, "tg-taskcard"))
	if len(cards) != 1 || !strings.Contains(cards[0].Message.Body, "roll the api deployment") {
		t.Fatalf("a posted task must state its cause in the thread: %+v", cards)
	}
}

// 6.11 (second half) — an input predating provenance is delivered nowhere, so
// upgrading does not fill every open thread with history.
func TestInputWithoutOriginIsDeliveredNowhere(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-legacy")
	mkChannel(t, "chan-legacy", "tg-legacy")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "legacy-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-legacy"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-legacy"}}
	// no Origin: exactly the shape every input had before this change
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{
		ID: "in-old", Type: agentopsv1alpha1.InputAlert, Payload: "an old alert",
	}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	reconcileWithOps(t, srv, conv.Name)
	bindThread(t, srv, drainOps(t, srv, "tg-legacy"), "6001")
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	for _, op := range drainOps(t, srv, "tg-legacy") {
		if op.Message != nil && (op.Message.Kind == chat.MsgSignal || op.Message.Kind == chat.MsgRelay) {
			t.Fatalf("pre-provenance inputs must deliver nothing: %+v", op.Message)
		}
	}
}

// 6.9 — an adapter that declares no contract version, or an old one, is refused
// with a 400 naming the replacement. Without this it would poll happily and
// post empty messages forever.
func TestOpsEndpointRequiresTheContractVersion(t *testing.T) {
	mkChannel(t, "chan-contract", "tg-contract")
	srv := apiServer()

	rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-contract&wait=0", nil, testMasterToken)
	if rec.Code != 400 {
		t.Fatalf("an undeclared contract must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), chat.ContractVersion) {
		t.Fatalf("the refusal must name the expected version: %s", rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-contract&contract=1&wait=0", nil, testMasterToken)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `contract \"1\"`) {
		t.Fatalf("an outdated contract must be refused by name: %d %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-contract&contract="+chat.ContractVersion+"&wait=0",
		nil, testMasterToken)
	if rec.Code != 204 {
		t.Fatalf("the current contract must be served: %d %s", rec.Code, rec.Body.String())
	}
}

// 6.8 — two bound channels each receive their own copy of one semantic answer,
// so each adapter renders it independently.
func TestAnswerFansOutAsOneSemanticMessagePerChannel(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-fanout")
	mkChannel(t, "chan-fan-a", "tg-fan-a")
	mkChannel(t, "chan-fan-b", "tg-fan-b")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "fanout-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-fanout"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-fan-a"}, {Name: "chan-fan-b"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{
		{Channel: "chan-fan-a", ThreadID: "a1"}, {Channel: "chan-fan-b", ThreadID: "b1"},
	}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-1", "status": "succeeded", "result": "**restarted** `api`",
	}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
	for _, adapter := range []string{"tg-fan-a", "tg-fan-b"} {
		ops := drainOps(t, srv, adapter)
		if len(ops) != 1 || ops[0].Message == nil {
			t.Fatalf("%s: want one message, got %+v", adapter, ops)
		}
		m := ops[0].Message
		if m.Kind != chat.MsgAnswer || m.Status != "succeeded" || m.Body != "**restarted** `api`" {
			t.Fatalf("%s: each channel gets the same SEMANTIC answer to render itself: %+v", adapter, m)
		}
	}

	// a failed run is a notice, not an empty answer
	if rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-2", "status": "failed",
	}, testMasterToken); rec.Code != 200 {
		t.Fatalf("failed work done: %d", rec.Code)
	}
	ops := drainOps(t, srv, "tg-fan-a")
	if len(ops) != 1 || ops[0].Message.Kind != chat.MsgNotice || ops[0].Message.Level != chat.NoticeWarn {
		t.Fatalf("a failed run must report as a warning notice: %+v", ops)
	}
}

// reconcileWithOps runs one reconcile against the SAME op queue the API server
// serves, so cards enqueued by the reconciler are claimable through
// /channel/ops exactly as an adapter would see them.
func reconcileWithOps(t *testing.T, srv *httpapi.Server, name string) {
	t.Helper()
	if _, err := reconcilerWithOps(srv.Ops).Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
}
