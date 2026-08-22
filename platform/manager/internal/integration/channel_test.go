// Adapter-contract integration tests: inbound routing through the shared
// Router, ensure-topic ops landing string thread ids, the pending-topic
// dispatch gate, duplicate completion tolerance, and bearer auth.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

func mkChannel(t *testing.T, name, typ string) {
	t.Helper()
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, ns
	ch.Spec.Adapter = typ
	ch.Spec.Config = &runtime.RawExtension{Raw: []byte(`{"chatId":"-100","pollingEnabled":true}`)}
	if err := k8sClient.Create(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

func adapterReq(srv *httpapi.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestChannelAuthRequired(t *testing.T) {
	srv := apiServer()
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&contract=2&wait=0", nil, ""); rec.Code != 401 {
		t.Fatalf("missing token: %d", rec.Code)
	}
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&contract=2&wait=0", nil, "wrong"); rec.Code != 401 {
		t.Fatalf("wrong token: %d", rec.Code)
	}
	srv.AdapterToken = ""
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&contract=2&wait=0", nil, "anything"); rec.Code != 503 {
		t.Fatalf("unconfigured auth must 503: %d", rec.Code)
	}
}

func TestInboundCreatesConversationAndAckOp(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-inb")
	mkChannel(t, "chan-inb", "tg-inb")
	mkChatSource(t, "src-inb", "chan-inb")
	// a command addresses a PIPELINE — it originates the conversation, so it
	// supplies both the profile and the capabilities. Origination arrives on
	// the SIGNAL path now: the channel carries conversations, it never starts
	// one.
	mkPipeline(t, "inb-pipe", []string{"src-inb"}, []string{"chan-inb"}, "prof-inb")
	reconcilePipeline(t, "inb-pipe")
	srv := apiServer()

	// command naming a known pipeline -> task conversation
	rec := chatSignal(t, srv, "src-inb", "chan-inb", "/inb-pipe check the nodes")
	if rec.Code != 200 {
		t.Fatalf("inbound: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list)
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		c := &list.Items[i]
		if c.BoundTo("chan-inb") {
			conv = c
		}
	}
	if conv == nil {
		t.Fatal("task conversation not created")
	}
	if conv.Spec.ProfileRef.Name != "prof-inb" || len(conv.Spec.Inputs) != 1 ||
		conv.Spec.Inputs[0].Type != agentopsv1alpha1.InputTask ||
		conv.Spec.Inputs[0].Payload != "check the nodes" {
		t.Fatalf("conversation shape: %+v", conv.Spec)
	}

	// /agents listing comes back as a send op on the adapter queue
	rec = chatSignal(t, srv, "src-inb", "chan-inb", "/agents")
	if rec.Code != 200 {
		t.Fatalf("agents inbound: %d", rec.Code)
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-inb&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("expected queued send op: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	// the listing names PIPELINES: they are what a message can address, and
	// what carries the capabilities the conversation will get. Listing profiles
	// would advertise names a user cannot actually address.
	if op.Kind != chat.OpSend || !strings.Contains(opBody(op), "/inb-pipe") {
		t.Fatalf("agents send op must list addressable pipelines: %+v", op)
	}
	if strings.Contains(opBody(op), "/prof-inb") {
		t.Fatalf("listing must not advertise profile names: %q", opBody(op))
	}
}

func TestEnsureTopicRoundTripAndDispatchGate(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-topic")
	mkChannel(t, "chan-topic", "tg-topic")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "topic-1", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-topic"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-topic"}}
	conv.Spec.Title = "needs a topic"
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}

	// pending topic gates dispatch (inputs stay queued)
	rec := adapterReq(srv, "GET", "/work?convo=topic-1&wait=0", nil, "")
	if rec.Code != 204 {
		t.Fatalf("dispatch must wait for the thread id: %d %s", rec.Code, rec.Body.String())
	}

	// reconcile enqueues the ensure-topic op on the shared queue
	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "topic-1"}}); err != nil {
		t.Fatal(err)
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-topic&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("ensure-topic op expected: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpEnsureTopic || op.Conversation != "topic-1" ||
		op.Topic == nil || op.Topic.Title != "needs a topic" {
		t.Fatalf("op: %+v", op)
	}

	// adapter completes with a non-numeric thread id (string id space)
	rec = adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: "1712345678.000200"}, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("done: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "topic-1"}, &after)
	if tid := after.ThreadFor("chan-topic"); tid == nil || *tid != "1712345678.000200" {
		t.Fatalf("thread binding not landed: %+v", after.Status)
	}

	// duplicate completion is tolerated
	rec = adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: "9999"}, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("duplicate done: %d", rec.Code)
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "topic-1"}, &after)
	if tid := after.ThreadFor("chan-topic"); *tid != "1712345678.000200" {
		t.Fatalf("duplicate done overwrote thread binding: %v", *tid)
	}

	// gate lifted — the unit dispatches with the string thread id
	rec = adapterReq(srv, "GET", "/work?convo=topic-1&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch after topic: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID    string  `json:"runId"`
		ThreadID *string `json:"threadId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	if unit.ThreadID == nil || *unit.ThreadID != "1712345678.000200" {
		t.Fatalf("unit threadId: %v", unit.ThreadID)
	}

	// finish the run so the conversation goes idle (keeps the shared pod pool
	// evictable for later tests)
	rec = adapterReq(srv, "POST", "/work/done",
		map[string]any{"convo": "topic-1", "runId": unit.RunID, "status": "succeeded", "result": "ok"}, "")
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
}

func TestThreadedReplyQueuesInputWithBusyAck(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-reply")
	mkChannel(t, "chan-reply", "tg-reply")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "reply-1", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-reply"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-reply"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "chan-reply", ThreadID: "4242"}}
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", InputIDs: []string{"x"}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-reply", "threadId": "4242", "text": "also check the disks"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("threaded inbound: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "reply-1"}, &after)
	if len(after.Spec.Inputs) != 1 || after.Spec.Inputs[0].Type != agentopsv1alpha1.InputReply {
		t.Fatalf("reply input expected: %+v", after.Spec.Inputs)
	}

	// busy ack queued for the adapter, addressed to the thread
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-reply&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("busy ack op expected: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || op.ThreadID == nil || *op.ThreadID != "4242" || !strings.Contains(opBody(op), "Noted") {
		t.Fatalf("busy ack: %+v", op)
	}
}

func TestAdapterStateAndChannelListing(t *testing.T) {
	mkChannel(t, "chan-state", "tg-state")
	srv := apiServer()

	rec := adapterReq(srv, "GET", "/channel/channels?adapter=tg-state", nil, "test-adapter-token")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"chan-state"`) || !strings.Contains(rec.Body.String(), `"chatId":"-100"`) {
		t.Fatalf("channel listing: %d %s", rec.Code, rec.Body.String())
	}

	if rec = adapterReq(srv, "PUT", "/channel/state/chan-state/offset", map[string]string{"value": "1044"}, "test-adapter-token"); rec.Code != 200 {
		t.Fatalf("state put: %d %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/channel/state/chan-state/offset", nil, "test-adapter-token")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"1044"`) {
		t.Fatalf("state get: %d %s", rec.Code, rec.Body.String())
	}
}

// opBody is the message body an op would render from, or "" when the op carries
// no message. Assertions are on MEANING — the markup belongs to the adapter.
func opBody(op chat.Op) string {
	if op.Message == nil {
		return ""
	}
	return op.Message.Body
}

// The claim window is ADVERTISED, not guessed. An adapter that sleeps out a
// transport's backpressure must finish inside it or the manager reclaims the op
// and a second claimant posts the same message twice — an inequality spanning
// two dependency-free modules, which is exactly the kind of constant that drifts
// the first time someone tunes ReclaimAfter.
func TestClaimedOpAdvertisesTheReclaimInterval(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-reclaim")
	mkChannel(t, "chan-reclaim", "tg-reclaim")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-reclaim", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-reclaim"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-reclaim"}}
	conv.Spec.Title = "needs a topic"
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "conv-reclaim") })

	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "conv-reclaim"}}); err != nil {
		t.Fatal(err)
	}
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-reclaim&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("expected a claimed op: %d", rec.Code)
	}
	var got struct {
		ID                  string `json:"id"`
		Kind                string `json:"kind"`
		ReclaimAfterSeconds int    `json:"reclaimAfterSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if want := int(chat.ReclaimAfter.Seconds()); got.ReclaimAfterSeconds != want {
		t.Fatalf("reclaimAfterSeconds=%d, want %d", got.ReclaimAfterSeconds, want)
	}
	// the op itself still marshals as before — the field is ADDITIVE
	if got.ID == "" || got.Kind != string(chat.OpEnsureTopic) {
		t.Fatalf("the op payload changed shape: %+v", got)
	}
}

// An answer recorded on the conversation but not yet on the thread is now
// VISIBLE. The 2026-08-13 incident ran four and a half hours with 22 threads
// empty and the operator reporting itself healthy, because nothing said the
// replies were owed.
func TestDeliveryPendingSurfacesAnOwedReply(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-owed")
	mkChannel(t, "chan-owed", "tg-owed")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-owed", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-owed"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-owed"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "conv-owed") })
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "chan-owed", ThreadID: "42", ReadTracked: true}}
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "run-1", Status: "succeeded", Result: "the answer", DeliveryTracked: true,
	}}
	if err := k8sClient.Status().Update(ctx, conv); err != nil {
		t.Fatal(err)
	}

	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "conv-owed"}}); err != nil {
		t.Fatal(err)
	}
	cond := conversationCondition(t, "conv-owed", controller.ConditionDeliveryPending)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("an undelivered answer must be visible: %+v", cond)
	}
	if !strings.Contains(cond.Message, "chan-owed") {
		t.Fatalf("the condition must name where delivery is owed: %q", cond.Message)
	}

	// The adapter takes it and succeeds; the next reconcile clears the condition.
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-owed&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("expected the reply op: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID), chat.OpResult{}, "test-adapter-token"); rec.Code != 200 {
		t.Fatalf("done: %d", rec.Code)
	}
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "conv-owed"}}); err != nil {
		t.Fatal(err)
	}
	if cond := conversationCondition(t, "conv-owed", controller.ConditionDeliveryPending); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("a delivered answer must clear the condition: %+v", cond)
	}
}

func conversationCondition(t *testing.T, name, condType string) *metav1.Condition {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	return apimeta.FindStatusCondition(conv.Status.Conditions, condType)
}
