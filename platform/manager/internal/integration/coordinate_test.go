// The /coordinate/* surface end to end (tasks 2.6-2.11 of coordinated-agents):
// invoke, close, escalate and the budget edges, against a REAL API server —
// which is what actually proves the new CRD fields (InputOrigin's `member`
// kind and `entry`, ConversationStatus's `escalationMessage` and
// `routedToParent`, the `agentops.dev/caused-by` label) round-trip through
// the schema the fake client in internal/chat's own unit tests never
// validates.
package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

func postCoordinateReq(t *testing.T, srv *httpapi.Server, path, token string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return adapterReq(srv, "POST", path, body, token)
}

// reconcileConversation drives one ConversationReconciler pass over a named
// conversation, wired with the SAME Ops queue and Router the http server
// uses, so ops enqueued by an HTTP verb and ops enqueued by the reconciler
// land in the one place a test can drain them from.
func reconcileConversation(t *testing.T, srv *httpapi.Server, name string) {
	t.Helper()
	rc := &controller.ConversationReconciler{
		Client: k8sClient, Scheme: scheme, MaxActiveConversations: 100,
		Ops: srv.Ops, Router: srv.Router, Runtime: srv.Runtime,
	}
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatalf("reconcile %s: %v", name, err)
	}
}

// decodeCoordinateInvokeResponse checks a /coordinate/invoke response for a
// created member and returns its decoded body.
func decodeCoordinateInvokeResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("invoke: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "created" || out["member"] == "" {
		t.Fatalf("want a created member, got %+v", out)
	}
	return out
}

// assertMemberCausedByInvoke checks that a member conversation created by
// /coordinate/invoke round-trips its provenance through the real schema.
func assertMemberCausedByInvoke(t *testing.T, member *agentopsv1alpha1.Conversation, rootName string) {
	t.Helper()
	if member.Spec.CausedBy == nil || member.Spec.CausedBy.Parent != rootName || member.Spec.CausedBy.Entry != "worker" {
		t.Fatalf("causedBy must round-trip through the real schema: %+v", member.Spec.CausedBy)
	}
	if member.Labels[agentopsv1alpha1.LabelCausedBy] != rootName {
		t.Fatalf("the caused-by label must be set for the close cascade to find it: %+v", member.Labels)
	}
	if len(member.Spec.Inputs) != 1 || member.Spec.Inputs[0].Origin == nil ||
		member.Spec.Inputs[0].Origin.Kind != agentopsv1alpha1.OriginMember {
		t.Fatalf("the invoked task's origin must be the NEW `member` kind, admitted by the real schema: %+v", member.Spec.Inputs)
	}
}

func TestCoordinateInvokeCreatesAMemberAndRoutesItsResultBack(t *testing.T) {
	mkProfile(t, "prof-invoke-co")
	mkCapability(t, "cap-invoke-worker", "prof-invoke-co")
	reconcileCapability(t, "cap-invoke-worker")
	mkCoordinator(t, "co-invoke", nil, nil, "prof-invoke-co", nil)
	co := reconcileCoordinator(t, "co-invoke")
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-invoke-worker"},
	}}
	if err := k8sClient.Update(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	reconcileCoordinator(t, "co-invoke")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-invoke-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-invoke-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-invoke"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-invoke", root.Name)
	rec := postCoordinateReq(t, srv, "/coordinate/invoke", token,
		map[string]any{"conversation": root.Name, "agent": "worker", "task": "look into the disk usage"})
	out := decodeCoordinateInvokeResponse(t, rec)
	member := getConv(t, out["member"])
	assertMemberCausedByInvoke(t, member, root.Name)

	// Report the member's work done, and the fast path in /work/done must route
	// the result to the parent as an input, admitted by the real CRD schema.
	member.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", DispatchedAt: metav1.Now()}
	if err := k8sClient.Status().Update(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	rec2 := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "disk is at 40%, nothing to worry about",
	}, srv.AdapterToken)
	if rec2.Code != 200 {
		t.Fatalf("work/done: %d %s", rec2.Code, rec2.Body.String())
	}
	gotRoot := getConv(t, root.Name)
	if len(gotRoot.Spec.Inputs) != 1 || gotRoot.Spec.Inputs[0].Payload != "disk is at 40%, nothing to worry about" {
		t.Fatalf("the member's result must land on the parent, got %+v", gotRoot.Spec.Inputs)
	}
	gotMember := getConv(t, member.Name)
	if len(gotMember.Status.Runs) != 1 || !gotMember.Status.Runs[0].RoutedToParent {
		t.Fatalf("the run must be marked routed once the append lands: %+v", gotMember.Status.Runs)
	}
}

func TestCoordinateCloseCascadesThroughTheRealAPI(t *testing.T) {
	mkProfile(t, "prof-close-co")
	mkCoordinator(t, "co-close", nil, nil, "prof-close-co", nil)
	reconcileCoordinator(t, "co-close")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-close-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-close-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-close"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "co-close-member", ns
	member.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-close-co"}
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	member.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: root.Name}
	if err := k8sClient.Create(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	grandchild := &agentopsv1alpha1.Conversation{}
	grandchild.Name, grandchild.Namespace = "co-close-grandchild", ns
	grandchild.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-close-co"}
	grandchild.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: member.Name, Entry: "helper"}
	grandchild.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: member.Name}
	if err := k8sClient.Create(context.Background(), grandchild); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-close", root.Name)
	rec := postCoordinateReq(t, srv, "/coordinate/close", token,
		map[string]any{"conversation": root.Name, "target": member.Name, "reason": "resolved: done"})
	if rec.Code != 200 {
		t.Fatalf("close: %d %s", rec.Code, rec.Body.String())
	}
	if got := getConv(t, member.Name); got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("the direct member must close, got %s", got.Status.Phase)
	}
	if got := getConv(t, grandchild.Name); got.Status.Phase != agentopsv1alpha1.ConversationClosed ||
		got.Status.CloseReason != "resolved: done" {
		t.Fatalf("closing the member must cascade to its OWN member with the same reason, got %+v", got.Status)
	}
}

func TestCoordinateEscalateDeliversTheDigestThroughARealReconcile(t *testing.T) {
	mkProfile(t, "prof-esc-co")
	mkChannel(t, "esc-desk", "esc-ta")
	mkCoordinator(t, "co-esc", nil, []string{"esc-desk"}, "prof-esc-co", nil)
	reconcileCoordinator(t, "co-esc")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-esc-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-esc-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-esc"}
	root.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc-desk"}}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-esc", root.Name)
	rec := postCoordinateReq(t, srv, "/coordinate/escalate", token,
		map[string]any{"conversation": root.Name, "message": "three members failed, see the tree"})
	if rec.Code != 200 {
		t.Fatalf("escalate: %d %s", rec.Code, rec.Body.String())
	}
	got := getConv(t, root.Name)
	if got.Status.EscalatedAt == nil || got.Status.EscalationMessage != "three members failed, see the tree" {
		t.Fatalf("escalate must stamp the digest, admitted by the real schema: %+v", got.Status)
	}
	if len(got.Spec.ChannelRefs) != 1 || got.Spec.ChannelRefs[0].Name != "esc-desk" {
		t.Fatalf("escalate must bind the snapshotted channel, got %v", got.Spec.ChannelRefs)
	}

	// The reconciler creates the topic, and once the adapter reports a thread
	// id the digest is queued as the opening message.
	reconcileConversation(t, srv, root.Name)
	opRec := adapterReq(srv, "GET", "/channel/ops?adapter=esc-ta&contract=2&wait=0", nil, srv.AdapterToken)
	var topicOp chat.Op
	if err := json.Unmarshal(opRec.Body.Bytes(), &topicOp); err != nil || topicOp.Kind != chat.OpEnsureTopic {
		t.Fatalf("want an ensure-topic op, got %d %s", opRec.Code, opRec.Body.String())
	}
	doneRec := adapterReq(srv, "POST", "/channel/ops/"+topicOp.ID+"/done",
		map[string]any{"threadId": "esc-thread-1"}, srv.AdapterToken)
	if doneRec.Code != 200 {
		t.Fatalf("completing ensure-topic: %d %s", doneRec.Code, doneRec.Body.String())
	}
	reconcileConversation(t, srv, root.Name)
	msgRec := adapterReq(srv, "GET", "/channel/ops?adapter=esc-ta&contract=2&wait=0", nil, srv.AdapterToken)
	var msgOp chat.Op
	if err := json.Unmarshal(msgRec.Body.Bytes(), &msgOp); err != nil || msgOp.Kind != chat.OpSend {
		t.Fatalf("want the digest queued as a send op, got %d %s", msgRec.Code, msgRec.Body.String())
	}
	if !strings.Contains(msgOp.Message.Body, "three members failed") {
		t.Fatalf("the send op must carry the digest, got %+v", msgOp.Message)
	}
}

// The DEADLINE budget edge is the reconciler's own (design D-E): a
// Coordinator-rooted conversation past its snapshotted deadline is closed
// budget-exceeded on the very next reconcile, whatever else is happening on
// it — no request has to arrive to trigger it.
func TestCoordinateDeadlineClosesOnReconcile(t *testing.T) {
	mkProfile(t, "prof-dl-co")
	mkCoordinator(t, "co-dl", nil, nil, "prof-dl-co", nil)
	reconcileCoordinator(t, "co-dl")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-dl-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-dl-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-dl"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	past := metav1.NewTime(metav1.Now().Add(-time.Minute))
	root.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 5, Deadline: &past}
	if err := k8sClient.Status().Update(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	reconcileConversation(t, srv, root.Name)

	got := getConv(t, root.Name)
	// This is the UNCAUSED root, so budget-exceeded ESCALATES it rather than
	// closing it (design D-D) — it stays open, carrying the digest.
	if got.Status.EscalatedAt == nil {
		t.Fatalf("a passed deadline must escalate the uncaused root, got %+v", got.Status)
	}
	if !strings.Contains(got.Status.EscalationMessage, "deadline") {
		t.Fatalf("the digest must say why, got %q", got.Status.EscalationMessage)
	}
}

func TestCoordinateInvokeRefusesAChannelReaderTokenThroughTheRealAPI(t *testing.T) {
	mkProfile(t, "prof-chread-co")
	mkChannel(t, "chread-desk", "chread-ta")
	mkCoordinator(t, "co-chread", nil, nil, "prof-chread-co", nil)
	reconcileCoordinator(t, "co-chread")
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-chread-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-chread-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-chread"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	token := chat.DeriveChannelReaderToken(srv.AdapterToken, "chread-desk")
	rec := postCoordinateReq(t, srv, "/coordinate/invoke", token,
		map[string]any{"conversation": root.Name, "agent": "worker", "task": "do it"})
	if rec.Code != 401 {
		t.Fatalf("a channel-reader token must be refused for invoke, got %d %s", rec.Code, rec.Body.String())
	}
}
