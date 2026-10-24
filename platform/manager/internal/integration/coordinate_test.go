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

// TestCoordinateReconcileBackstopRoutesAMissedMemberResult exercises
// deliverMemberResults directly, rather than through /work/done's fast path:
// a run recorded with no RoutedToParent marker (a manager restart between the
// two, or a conflict the fast path gave up retrying) must still reach its
// parent on the next ordinary reconcile.
func TestCoordinateReconcileBackstopRoutesAMissedMemberResult(t *testing.T) {
	mkProfile(t, "prof-backstop-co")
	mkCoordinator(t, "co-backstop", nil, nil, "prof-backstop-co", nil)
	reconcileCoordinator(t, "co-backstop")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-backstop-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-backstop-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-backstop"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "co-backstop-member", ns
	member.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-backstop-co"}
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	member.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: root.Name}
	if err := k8sClient.Create(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	member.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "r1", Status: "succeeded", Result: "backstop delivered this",
	}}
	if err := k8sClient.Status().Update(context.Background(), member); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	reconcileConversation(t, srv, member.Name)

	gotRoot := getConv(t, root.Name)
	if len(gotRoot.Spec.Inputs) != 1 || gotRoot.Spec.Inputs[0].Payload != "backstop delivered this" {
		t.Fatalf("the backstop must append the missed result to the parent, got %+v", gotRoot.Spec.Inputs)
	}
	gotMember := getConv(t, member.Name)
	if len(gotMember.Status.Runs) != 1 || !gotMember.Status.Runs[0].RoutedToParent {
		t.Fatalf("the backstop must mark the run routed once the append lands: %+v", gotMember.Status.Runs)
	}

	// A second reconcile must be a no-op: the run is already marked routed, so
	// appendInputIdempotent's own id check must never fire again.
	reconcileConversation(t, srv, member.Name)
	gotRootAgain := getConv(t, root.Name)
	if len(gotRootAgain.Spec.Inputs) != 1 {
		t.Fatalf("a routed run must not be re-appended, got %+v", gotRootAgain.Spec.Inputs)
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

// TestCoordinateEscalateDeliversTheDigestThroughARealReconcile pins
// coordinator-unconditional-channels at the full-stack level: the root's
// channel is ALREADY bound before any `escalate` call (exactly as creation
// now binds it), so its topic exists and the digest posts into that
// already-open thread — escalate never (re)binds `ChannelRefs`.
func TestCoordinateEscalateDeliversTheDigestThroughARealReconcile(t *testing.T) {
	mkProfile(t, "prof-esc-co")
	mkChannel(t, "esc-desk", "esc-ta")
	mkCoordinator(t, "co-esc", nil, []string{"esc-desk"}, "prof-esc-co", nil)
	reconcileCoordinator(t, "co-esc")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-esc-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-esc-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-esc"}
	root.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc-desk"}}
	root.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc-desk"}}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()

	// The topic exists BEFORE any escalate call — the channel was bound at
	// creation, not by escalating.
	reconcileConversation(t, srv, root.Name)
	opRec := adapterReq(srv, "GET", "/channel/ops?adapter=esc-ta&contract=2&wait=0", nil, srv.AdapterToken)
	var topicOp chat.Op
	if err := json.Unmarshal(opRec.Body.Bytes(), &topicOp); err != nil || topicOp.Kind != chat.OpEnsureTopic {
		t.Fatalf("want an ensure-topic op before any escalation, got %d %s", opRec.Code, opRec.Body.String())
	}
	doneRec := adapterReq(srv, "POST", "/channel/ops/"+topicOp.ID+"/done",
		map[string]any{"threadId": "esc-thread-1"}, srv.AdapterToken)
	if doneRec.Code != 200 {
		t.Fatalf("completing ensure-topic: %d %s", doneRec.Code, doneRec.Body.String())
	}

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
		t.Fatalf("escalate must leave the already-bound channel exactly as it was, got %v", got.Spec.ChannelRefs)
	}

	// The thread already existed, so the digest is queued as an ordinary send
	// op into it — never a second ensure-topic.
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

// A deadline still AHEAD must not return early (unlike the passed one above):
// the rest of the pass runs, and capRequeue only bounds whichever requeue it
// picked so the reconciler is guaranteed to revisit no later than the
// deadline itself.
func TestCoordinateFutureDeadlineCapsTheReconcileRequeue(t *testing.T) {
	mkProfile(t, "prof-dl-cap")
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "dl-cap-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-dl-cap"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	future := metav1.NewTime(metav1.Now().Add(5 * time.Second))
	root.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 5, Deadline: &future}
	if err := k8sClient.Status().Update(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	rc := &controller.ConversationReconciler{
		Client: k8sClient, Scheme: scheme, MaxActiveConversations: 100,
		Ops: srv.Ops, Router: srv.Router, Runtime: srv.Runtime,
	}
	res, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: root.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter <= 0 || res.RequeueAfter > 5*time.Second {
		t.Fatalf("the requeue must be capped to the still-ahead deadline, got %v", res.RequeueAfter)
	}
}

// TestCoordinateMemberResultsReachTheAlreadyBoundThreadBeforeAndAfterEscalation
// is task 3.4's fake-chat integration coverage beyond the digest itself
// (already covered above), REWRITTEN for coordinator-unconditional-channels:
// the root's channel is bound and its thread open from the START, so a
// member result delivered BEFORE any `escalate` call reaches it immediately
// — there is no backlog left for a fence to hold back — and `escalate`'s own
// digest, and a later member result, land in that same thread afterwards. A
// person's reply on the thread still lands as an ordinary root input, through
// the real API and reconciler, not the fake client internal/chat's own unit
// tests use.
func TestCoordinateMemberResultsReachTheAlreadyBoundThreadBeforeAndAfterEscalation(t *testing.T) {
	root := mkEsc3Root(t)
	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-esc3", root.Name)

	// The thread already exists before anything else happens — bound at
	// creation, not by escalating.
	openEsc3Thread(t, srv, root.Name)

	// Invoke and finish BEFORE any escalate call: this member's result must
	// reach the thread right away, with no fence to hold it.
	rec := postCoordinateReq(t, srv, "/coordinate/invoke", token,
		map[string]any{"conversation": root.Name, "agent": "worker", "task": "look into the disk usage"})
	member := decodeCoordinateInvokeResponse(t, rec)["member"]
	finishMemberRun(t, srv, member, "r1", "early result, delivered at once")
	reconcileConversation(t, srv, root.Name)
	earlyOp := nextEsc3SendOp(t, srv, "the early member result to reach the thread immediately")
	if !strings.Contains(earlyOp.Message.Body, "early result, delivered at once") {
		t.Fatalf("the send op must carry the early member result, got %+v", earlyOp.Message)
	}
	if rec := adapterReq(srv, "POST", "/channel/ops/"+earlyOp.ID+"/done", nil, srv.AdapterToken); rec.Code != 200 {
		t.Fatalf("completing the early result send: %d %s", rec.Code, rec.Body.String())
	}

	// escalate posts its digest into the SAME thread — no second ensure-topic.
	rec = postCoordinateReq(t, srv, "/coordinate/escalate", token,
		map[string]any{"conversation": root.Name, "message": "three members failed, see the tree"})
	if rec.Code != 200 {
		t.Fatalf("escalate: %d %s", rec.Code, rec.Body.String())
	}
	reconcileConversation(t, srv, root.Name)
	digestOp := nextEsc3SendOp(t, srv, "the digest queued as a send op")
	if !strings.Contains(digestOp.Message.Body, "three members failed") {
		t.Fatalf("the digest itself must carry the escalate message, got %+v", digestOp.Message)
	}
	if rec := adapterReq(srv, "POST", "/channel/ops/"+digestOp.ID+"/done", nil, srv.AdapterToken); rec.Code != 200 {
		t.Fatalf("completing the digest send: %d %s", rec.Code, rec.Body.String())
	}

	// A second member run, AFTER escalation, reaches the thread exactly as
	// the first one did — escalating changed nothing about delivery.
	finishMemberRun(t, srv, member, "r2", "disk is at 90%, needs attention")
	reconcileConversation(t, srv, root.Name)
	resultOp := nextEsc3SendOp(t, srv, "the later member result to reach the thread")
	if !strings.Contains(resultOp.Message.Body, "disk is at 90%") {
		t.Fatalf("the send op must carry the member's later result, got %+v", resultOp.Message)
	}

	// A person's reply on that thread is an ordinary root input.
	if rec := adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "esc3-desk", "threadId": "esc3-thread-1", "text": "who is handling this?"},
		srv.AdapterToken); rec.Code != 202 {
		t.Fatalf("channel/inbound: %d %s", rec.Code, rec.Body.String())
	}
	assertRootHasInput(t, root.Name, "who is handling this?")
}

// mkEsc3Root builds the coordinator, its one member capability and the root
// conversation the member-result-delivery test drives.
func mkEsc3Root(t *testing.T) *agentopsv1alpha1.Conversation {
	t.Helper()
	mkProfile(t, "prof-esc3-co")
	// A member's result routes onto the root as a pending input, so — unlike
	// the escalate-only tests above — this root actually gets admitted and
	// dispatched, and that needs a resolvable runtime image. Named for this
	// test rather than "default": that name is a shared fixture no other test
	// in this suite expects to exist as a real AgentRuntime CR.
	mkRuntime(t, "esc3-rt", "example/agent:esc3", "")
	mkCapability(t, "cap-esc3-worker", "prof-esc3-co")
	reconcileCapability(t, "cap-esc3-worker")
	mkChannel(t, "esc3-desk", "esc3-ta")
	mkCoordinator(t, "co-esc3", nil, []string{"esc3-desk"}, "prof-esc3-co", nil)
	co := reconcileCoordinator(t, "co-esc3")
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-esc3-worker"},
	}}
	if err := k8sClient.Update(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	reconcileCoordinator(t, "co-esc3")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-esc3-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-esc3-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-esc3"}
	root.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "esc3-rt"}
	root.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc3-desk"}}
	root.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc3-desk"}}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return root
}

// finishMemberRun marks the member inflight under runID and reports it done.
func finishMemberRun(t *testing.T, srv *httpapi.Server, member, runID, result string) {
	t.Helper()
	memberConv := getConv(t, member)
	memberConv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: runID, DispatchedAt: metav1.Now()}
	if err := k8sClient.Status().Update(context.Background(), memberConv); err != nil {
		t.Fatal(err)
	}
	if rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": member, "runId": runID, "status": "succeeded", "result": result,
	}, srv.AdapterToken); rec.Code != 200 {
		t.Fatalf("work/done: %d %s", rec.Code, rec.Body.String())
	}
}

// openEsc3Thread opens the escalation thread, exactly as
// TestCoordinateEscalateDeliversTheDigestThroughARealReconcile does.
func openEsc3Thread(t *testing.T, srv *httpapi.Server, root string) {
	t.Helper()
	reconcileConversation(t, srv, root)
	topicRec := adapterReq(srv, "GET", "/channel/ops?adapter=esc3-ta&contract=2&wait=0", nil, srv.AdapterToken)
	var topicOp chat.Op
	if err := json.Unmarshal(topicRec.Body.Bytes(), &topicOp); err != nil || topicOp.Kind != chat.OpEnsureTopic {
		t.Fatalf("want an ensure-topic op, got %d %s", topicRec.Code, topicRec.Body.String())
	}
	if rec := adapterReq(srv, "POST", "/channel/ops/"+topicOp.ID+"/done",
		map[string]any{"threadId": "esc3-thread-1"}, srv.AdapterToken); rec.Code != 200 {
		t.Fatalf("completing ensure-topic: %d %s", rec.Code, rec.Body.String())
	}
	reconcileConversation(t, srv, root)
}

// nextEsc3SendOp reads the next queued op for the esc3 adapter and requires a send.
func nextEsc3SendOp(t *testing.T, srv *httpapi.Server, want string) chat.Op {
	t.Helper()
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=esc3-ta&contract=2&wait=0", nil, srv.AdapterToken)
	var op chat.Op
	if err := json.Unmarshal(rec.Body.Bytes(), &op); err != nil || op.Kind != chat.OpSend {
		t.Fatalf("want %s, got %d %s", want, rec.Code, rec.Body.String())
	}
	return op
}

// assertRootHasInput requires a pending input on the conversation carrying payload.
func assertRootHasInput(t *testing.T, name, payload string) {
	t.Helper()
	got := getConv(t, name)
	for _, in := range got.Spec.Inputs {
		if in.Payload == payload {
			return
		}
	}
	t.Fatalf("the reply must land as a root input, got %+v", got.Spec.Inputs)
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
