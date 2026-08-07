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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
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
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&wait=0", nil, ""); rec.Code != 401 {
		t.Fatalf("missing token: %d", rec.Code)
	}
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&wait=0", nil, "wrong"); rec.Code != 401 {
		t.Fatalf("wrong token: %d", rec.Code)
	}
	srv.AdapterToken = ""
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-auth&wait=0", nil, "anything"); rec.Code != 503 {
		t.Fatalf("unconfigured auth must 503: %d", rec.Code)
	}
}

func TestInboundCreatesConversationAndAckOp(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-inb")
	mkChannel(t, "chan-inb", "tg-inb")
	srv := apiServer()

	// command for a known profile -> task conversation
	rec := adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-inb", "text": "/prof-inb check the nodes"}, "test-adapter-token")
	if rec.Code != 202 {
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
	rec = adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-inb", "text": "/agents"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("agents inbound: %d", rec.Code)
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-inb&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("expected queued send op: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || !strings.Contains(op.Text, "/prof-inb") {
		t.Fatalf("agents send op: %+v", op)
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
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-topic&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("ensure-topic op expected: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpEnsureTopic || op.Conversation != "topic-1" || op.Title != "needs a topic" {
		t.Fatalf("op: %+v", op)
	}

	// adapter completes with a non-numeric thread id (string id space)
	rec = adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: "1234567890.000200"}, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("done: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "topic-1"}, &after)
	if tid := after.ThreadFor("chan-topic"); tid == nil || *tid != "1234567890.000200" {
		t.Fatalf("thread binding not landed: %+v", after.Status)
	}

	// duplicate completion is tolerated
	rec = adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: "9999"}, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("duplicate done: %d", rec.Code)
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "topic-1"}, &after)
	if tid := after.ThreadFor("chan-topic"); *tid != "1234567890.000200" {
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
	if unit.ThreadID == nil || *unit.ThreadID != "1234567890.000200" {
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
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-reply&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("busy ack op expected: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpSend || op.ThreadID == nil || *op.ThreadID != "4242" || !strings.Contains(op.Text, "Noted") {
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
