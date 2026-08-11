// Pipeline + multi-channel conversation tests: wiring validation, source
// SHARING, per-channel topic ensure, the at-least-one dispatch gate, forced
// result delivery, reply/ack fan-out, and attributed cross-channel relay.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

func mkPipeline(t *testing.T, name string, sources, channels []string, profile string) {
	t.Helper()
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = name, ns
	for _, s := range sources {
		p.Spec.SignalSourceRefs = append(p.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	for _, c := range channels {
		p.Spec.ChannelRefs = append(p.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	p.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func reconcilePipeline(t *testing.T, name string) *agentopsv1alpha1.Pipeline {
	t.Helper()
	rc := &controller.PipelineReconciler{Client: k8sClient}
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func reconcileSignalSource(t *testing.T, name string) *agentopsv1alpha1.SignalSource {
	t.Helper()
	rc := &controller.SignalSourceReconciler{Client: k8sClient}
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &src); err != nil {
		t.Fatal(err)
	}
	return &src
}

// cleanupConversation force-removes a conversation and its runtime pod so the
// active-conversation cap stays predictable for later tests. The close-topics
// finalizer is stripped rather than driven: without a reconciler running here
// the object would sit Terminating forever and the name would stay taken.
func cleanupConversation(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = runtimepod.PodName(name), ns
	_ = k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0))
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	_ = k8sClient.Delete(ctx, conv)
	dropCloseFinalizer(t, name)
}

// dropCloseFinalizer releases a deleting conversation in tests that never run
// the finalize path themselves.
func dropCloseFinalizer(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		return
	}
	if len(conv.Finalizers) == 0 {
		return
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Finalizers = nil
	_ = k8sClient.Patch(ctx, &conv, patch)
}

func TestPipelineValidationAndSharing(t *testing.T) {
	mkProfile(t, "prof-pipe")
	mkChannel(t, "pipe-chan-a", "pipe-ta")
	mkSignalSource(t, "pipe-src", "pipe-sig", "")

	// all refs resolve → Ready
	mkPipeline(t, "pipe-ok", []string{"pipe-src"}, []string{"pipe-chan-a"}, "prof-pipe")
	if p := reconcilePipeline(t, "pipe-ok"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("Ready expected: %+v", p.Status.Conditions)
	}

	// dangling reference → Ready=False naming it
	mkPipeline(t, "pipe-dangling", []string{"no-such-source"}, []string{"pipe-chan-a"}, "prof-pipe")
	p := reconcilePipeline(t, "pipe-dangling")
	ready := apimeta.FindStatusCondition(p.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "no-such-source") {
		t.Fatalf("dangling ref not surfaced: %+v", p.Status.Conditions)
	}

	// A SECOND pipeline on the same source is a valid configuration, not a
	// conflict: sources are shareable exactly as channels are, and a signal
	// there opens a conversation on each. Both stay Ready, and no
	// SourceConflict condition is written on either — the condition type is
	// gone, so a stale one would have to be invented to appear.
	mkPipeline(t, "pipe-second", []string{"pipe-src"}, []string{"pipe-chan-a"}, "prof-pipe")
	p = reconcilePipeline(t, "pipe-second")
	if !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("second pipeline on a shared source must be Ready: %+v", p.Status.Conditions)
	}
	if c := apimeta.FindStatusCondition(p.Status.Conditions, "SourceConflict"); c != nil {
		t.Fatalf("no SourceConflict condition may be written: %+v", c)
	}

	// A condition a PREVIOUS manager wrote must be CLEARED, not merely left
	// unwritten: deleting the writer does not delete what it already wrote, so
	// on upgrade the stale one would sit there forever next to Ready=True.
	stale := p.DeepCopy()
	apimeta.SetStatusCondition(&stale.Status.Conditions, metav1.Condition{
		Type: "SourceConflict", Status: metav1.ConditionTrue, Reason: "SourceConflict",
		Message: "sources already claimed by older pipelines: pipe-src (by pipe-ok)",
	})
	if err := k8sClient.Status().Patch(context.Background(), stale, client.MergeFrom(p)); err != nil {
		t.Fatal(err)
	}
	p = reconcilePipeline(t, "pipe-second")
	if c := apimeta.FindStatusCondition(p.Status.Conditions, "SourceConflict"); c != nil {
		t.Fatalf("a stale SourceConflict from an older manager must be removed: %+v", c)
	}
	if p := reconcilePipeline(t, "pipe-ok"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("first pipeline must stay Ready: %+v", p.Status.Conditions)
	}

	// the source names BOTH servers on Wired — the count is what tells an
	// operator how many conversations one signal will open
	src := reconcileSignalSource(t, "pipe-src")
	wired := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionWired)
	if wired == nil || wired.Status != "True" ||
		!strings.Contains(wired.Message, "pipe-ok") || !strings.Contains(wired.Message, "pipe-second") {
		t.Fatalf("Wired must name every serving pipeline: %+v", wired)
	}

	// channels may be shared between pipelines — no conflict
	mkPipeline(t, "pipe-sharechan", nil, []string{"pipe-chan-a"}, "prof-pipe")
	if p := reconcilePipeline(t, "pipe-sharechan"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("channel sharing must not conflict: %+v", p.Status.Conditions)
	}
}

func TestMultiChannelConversationMirroring(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-mc")
	mkChannel(t, "mc-a", "mc-ta")
	mkChannel(t, "mc-b", "mc-tb")
	mkChatSource(t, "mc-src", "mc-a")
	mkPipeline(t, "mc-pipe", []string{"mc-src"}, []string{"mc-a", "mc-b"}, "prof-mc")
	reconcilePipeline(t, "mc-pipe")

	srv := apiServer()
	// chat originating on channel A → conversation bound to BOTH channels
	rec := chatSignal(t, srv, "mc-src", "mc-a", "/mc-pipe mirror me")
	if rec.Code != 200 {
		t.Fatalf("inbound: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].BoundTo("mc-a") && list.Items[i].BoundTo("mc-b") {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("conversation not bound to both pipeline channels")
	}
	defer cleanupConversation(t, conv.Name)

	// gate: no bindings yet → no dispatch
	if rec := adapterReq(srv, "GET", "/work?convo="+conv.Name+"&wait=0", nil, ""); rec.Code != 204 {
		t.Fatalf("gate before any binding: %d", rec.Code)
	}

	// reconcile enqueues one ensure-topic per channel
	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}
	claimTopic := func(chanType, threadID string) {
		t.Helper()
		rec := adapterReq(srv, "GET", "/channel/ops?adapter="+chanType+"&contract=2&wait=0", nil, "test-adapter-token")
		if rec.Code != 200 {
			t.Fatalf("%s ensure-topic expected: %d", chanType, rec.Code)
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		if op.Kind != chat.OpEnsureTopic || op.Conversation != conv.Name {
			t.Fatalf("op: %+v", op)
		}
		if rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
			chat.OpResult{ThreadID: threadID}, "test-adapter-token"); rec.Code != 200 {
			t.Fatalf("done: %d", rec.Code)
		}
	}
	// complete only channel A first — at-least-one gate lifts
	claimTopic("mc-ta", "111")
	rec = adapterReq(srv, "GET", "/work?convo="+conv.Name+"&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch after first binding: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID      string  `json:"runId"`
		ThreadID   *string `json:"threadId"`
		PromptText string  `json:"promptText"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	// multi-channel: no runtime thread id; delivery is the operator's job
	if unit.ThreadID != nil {
		t.Fatalf("multi-channel unit must carry no thread id: %v", *unit.ThreadID)
	}
	if !strings.Contains(unit.PromptText, "printed answer IS the deliverable") {
		t.Fatalf("result delivery wording missing: %.200s", unit.PromptText)
	}
	if !strings.Contains(unit.PromptText, "Do not attempt to send chat messages yourself") {
		t.Fatalf("prompt must forbid agent-side posting: %.200s", unit.PromptText)
	}

	// channel B's topic lands late — binding still recorded
	claimTopic("mc-tb", "222")
	var bound agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &bound)
	if bound.ThreadFor("mc-a") == nil || bound.ThreadFor("mc-b") == nil {
		t.Fatalf("expected both bindings: %+v", bound.Status.Threads)
	}

	// run completes → manager fans the result out to BOTH channels' threads
	rec = adapterReq(srv, "POST", "/work/done",
		map[string]any{"convo": conv.Name, "runId": unit.RunID, "status": "succeeded", "result": "the mirrored answer"}, "")
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
	expectSend := func(chanType, threadID, contains string) chat.Op {
		t.Helper()
		rec := adapterReq(srv, "GET", "/channel/ops?adapter="+chanType+"&contract=2&wait=0", nil, "test-adapter-token")
		if rec.Code != 200 {
			t.Fatalf("%s send expected: %d", chanType, rec.Code)
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		if op.Kind != chat.OpSend || op.ThreadID == nil || *op.ThreadID != threadID || !strings.Contains(opBody(op), contains) {
			t.Fatalf("%s send op: %+v", chanType, op)
		}
		return op
	}
	expectSend("mc-ta", "111", "the mirrored answer")
	expectSend("mc-tb", "222", "the mirrored answer")

	// threaded reply on channel B → same conversation; relay to A (attributed),
	// acks on both
	rec = adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "mc-b", "threadId": "222", "text": "and the disks?", "sender": "kostya"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("threaded reply: %d %s", rec.Code, rec.Body.String())
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &bound)
	n := len(bound.Spec.Inputs)
	if n == 0 || bound.Spec.Inputs[n-1].Type != agentopsv1alpha1.InputReply {
		t.Fatalf("reply input expected: %+v", bound.Spec.Inputs)
	}
	// channel A: first the attributed relay, then the ack (FIFO). The ack is
	// the busy variant — the processed task input is pruned only on reconcile,
	// so the conversation still counts as busy.
	//
	// Attribution is STRUCTURED now: origin and sender are fields, not a "💬
	// <b>…</b>: " prefix the manager composed. Each adapter decides how to mark
	// somebody else's words, so asserting on markup here would be asserting on
	// a decision the manager no longer makes.
	relay := expectSend("mc-ta", "111", "and the disks?")
	if relay.Message.Kind != chat.MsgRelay ||
		relay.Message.Origin != "mc-b" || relay.Message.Sender != "kostya" {
		t.Fatalf("relay attribution: %+v", relay.Message)
	}
	expectSend("mc-ta", "111", "Noted")
	// origin channel B: ack only, never a relay of its own message
	ack := expectSend("mc-tb", "222", "Noted")
	if ack.Message.Kind == chat.MsgRelay {
		t.Fatalf("origin channel got a relay: %+v", ack.Message)
	}
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=mc-tb&contract=2&wait=0", nil, "test-adapter-token"); rec.Code != 204 {
		t.Fatalf("unexpected extra op on origin channel: %d", rec.Code)
	}
}

func TestSignalRoutingBindsPipelineFirst(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-sigpipe")
	mkChannel(t, "sp-chan-a", "sp-ta")
	mkChannel(t, "sp-chan-b", "sp-tb")
	// the source carries no wiring — profile and channels come from the pipeline
	mkSignalSource(t, "sp-src", "sp-sig", "")
	mkPipeline(t, "sp-pipe", []string{"sp-src"}, []string{"sp-chan-a", "sp-chan-b"}, "prof-sigpipe")
	reconcilePipeline(t, "sp-pipe")

	h := apiServer().Handler()
	rec := postSignal(t, h, testMasterToken, "sp-src", []map[string]any{{
		"fingerprint": "sp-1", "labels": map[string]string{"alertname": "PipeAlert"}, "payload": "boom",
	}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].BoundTo("sp-chan-a") {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("pipeline-bound alert conversation not created")
	}
	defer cleanupConversation(t, conv.Name)
	if !conv.BoundTo("sp-chan-b") || conv.Spec.ProfileRef.Name != "prof-sigpipe" {
		t.Fatalf("pipeline-first binding wrong: %+v", conv.Spec)
	}
}

func TestBrokenChannelNeverDeadlocks(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-broken")
	mkChannel(t, "bk-good", "bk-tg")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "bk-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-broken"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "bk-good"}, {Name: "bk-missing"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "go"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	defer cleanupConversation(t, "bk-conv")

	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "bk-conv"}}); err != nil {
		t.Fatal(err)
	}
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=bk-tg&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("good channel's ensure-topic expected: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: "777"}, "test-adapter-token"); rec.Code != 200 {
		t.Fatalf("done: %d", rec.Code)
	}
	// one binding is enough — the dangling channel never blocks dispatch
	rec = adapterReq(srv, "GET", "/work?convo=bk-conv&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch with one broken channel: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID string `json:"runId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	if rec := adapterReq(srv, "POST", "/work/done",
		map[string]any{"convo": "bk-conv", "runId": unit.RunID, "status": "succeeded", "result": "done"}, ""); rec.Code != 200 {
		t.Fatalf("work done: %d", rec.Code)
	}
}

// Delivery belongs to the operator for EVERY conversation, not just mirrored
// ones. Before this, a single-channel conversation posted nothing on its own —
// it relied on the agent curling the transport itself (Channel.delivery
// mode:agent), which meant the agent knew the transport and the runtime held
// the channel's credentials.
func TestSingleChannelResultIsDeliveredByTheOperator(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-sc")
	mkChannel(t, "sc-chan", "sc-tg")
	mkChatSource(t, "sc-src", "sc-chan")
	mkPipeline(t, "sc-pipe", []string{"sc-src"}, []string{"sc-chan"}, "prof-sc")
	reconcilePipeline(t, "sc-pipe")
	srv := apiServer()

	rec := chatSignal(t, srv, "sc-src", "sc-chan", "/sc-pipe single surface")
	if rec.Code != 200 {
		t.Fatalf("inbound: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].BoundTo("sc-chan") {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	// reconcile enqueues the ensure-topic op; the adapter reports the thread
	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=sc-tg&contract=2&wait=0", nil, "test-adapter-token")
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpEnsureTopic {
		t.Fatalf("expected ensure-topic: %+v", op)
	}
	if rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		map[string]any{"threadId": "555"}, "test-adapter-token"); rec.Code != 200 {
		t.Fatalf("topic done: %d", rec.Code)
	}

	rec = adapterReq(srv, "GET", "/work?convo="+conv.Name+"&pod=p-sc&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID      string `json:"runId"`
		PromptText string `json:"promptText"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	if !strings.Contains(unit.PromptText, "Do not attempt to send chat messages yourself") {
		t.Fatalf("single-channel prompt must forbid agent-side posting: %.200s", unit.PromptText)
	}

	rec = adapterReq(srv, "POST", "/work/done",
		map[string]any{"convo": conv.Name, "runId": unit.RunID, "status": "succeeded", "result": "the single answer"}, "")
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=sc-tg&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("operator must post the result itself: %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || op.ThreadID == nil || *op.ThreadID != "555" || !strings.Contains(opBody(op), "the single answer") {
		t.Fatalf("result send op: %+v", op)
	}
}
