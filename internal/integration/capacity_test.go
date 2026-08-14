// Capacity and close: the admission gate, the Pending phase, FIFO promotion,
// the pending-backlog bound at ingest, and what deleting a conversation does to
// its chat threads.
//
// envtest runs no kubelet, so a created runtime pod stays Pending forever —
// which is exactly the "active conversation" the cap counts. That makes these
// tests deterministic without any scheduler.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// mkChanConv creates a conversation with one task input, bound to the named
// channels, and registers cleanup.
func mkChanConv(t *testing.T, name, profile string, channels ...string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	for _, ch := range channels {
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: ch})
	}
	conv.Spec.Title = name
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	return conv
}

func reconcileWith(t *testing.T, rc *controller.ConversationReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}})
	if err != nil {
		t.Fatalf("reconcile %s: %v", name, err)
	}
	return res
}

func phaseOf(t *testing.T, name string) agentopsv1alpha1.ConversationPhase {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return conv.Status.Phase
}

func podExists(t *testing.T, conv string) bool {
	t.Helper()
	var pod corev1.Pod
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "agentops-conv-" + conv}, &pod)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

// TestOverCapConversationProvisionsNothing: the point of the Pending phase is
// that nothing is provisioned — not the pod, and above all not the chat topic.
// A thousand signals must not become a thousand threads before anyone has
// looked at the first one.
func TestOverCapConversationProvisionsNothing(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-cap")
	mkChannel(t, "chan-cap", "tg-cap")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)

	mkChanConv(t, "cap-first", "prof-cap", "chan-cap")
	reconcileWith(t, rc, "cap-first")
	if !podExists(t, "cap-first") {
		t.Fatal("the first conversation must be admitted and get a pod")
	}

	mkChanConv(t, "cap-second", "prof-cap", "chan-cap")
	reconcileWith(t, rc, "cap-second")

	if got := phaseOf(t, "cap-second"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("over-cap conversation phase = %q, want Pending", got)
	}
	if podExists(t, "cap-second") {
		t.Fatal("a Pending conversation must have no runtime pod")
	}
	var cm corev1.ConfigMap
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: controller.MCPConfigMapName("cap-second")}, &cm)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("a Pending conversation must have no MCP ConfigMap: %v", err)
	}
	// no ensure-topic op for cap-second: the only ops queued are the first
	// conversation's topic and the queued notice
	for {
		rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-cap&contract=2&wait=0", nil, "test-adapter-token")
		if rec.Code != 200 {
			break
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		if op.Kind == chat.OpEnsureTopic && op.Conversation == "cap-second" {
			t.Fatal("a Pending conversation must not get an ensure-topic op")
		}
	}
}

// TestQueuedNoticeIsSentOncePerEntry: a pending conversation has no thread, so
// the only place to say "waiting" is the channel's general surface — and saying
// it on every 30-second reconcile would be worse than not saying it.
func TestQueuedNoticeIsSentOncePerEntry(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-notice")
	mkChannel(t, "chan-notice", "tg-notice")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)

	mkChanConv(t, "notice-first", "prof-notice", "chan-notice")
	reconcileWith(t, rc, "notice-first")
	mkChanConv(t, "notice-second", "prof-notice", "chan-notice")
	reconcileWith(t, rc, "notice-second")
	reconcileWith(t, rc, "notice-second") // still waiting; must not repeat itself
	reconcileWith(t, rc, "notice-second")

	notices := 0
	for {
		rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-notice&contract=2&wait=0", nil, "test-adapter-token")
		if rec.Code != 200 {
			break
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		if op.Kind == chat.OpSend && op.ThreadID == nil {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("want exactly one queued notice on the general surface, got %d", notices)
	}
}

// TestFreedSlotAdmitsOldestPending: closing the active conversation frees its
// slot, the OLDEST waiter takes it, and the promoted conversation then goes
// down the ordinary path — topic first, then pod.
//
// The names invert the alphabet on purpose (fifo-z-older was created first):
// a pass therefore proves ordering by creation time, not by the name tiebreak
// that only settles conversations created within the same second.
func TestFreedSlotAdmitsOldestPending(t *testing.T) {
	clearRuntimePods(t)
	ctx := context.Background()
	mkProfile(t, "prof-fifo")
	mkChannel(t, "chan-fifo", "tg-fifo")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)
	rc.CloseTopicGrace = time.Millisecond

	active := mkChanConv(t, "fifo-active", "prof-fifo", "chan-fifo")
	reconcileWith(t, rc, "fifo-active")
	if !podExists(t, "fifo-active") {
		t.Fatal("setup: the first conversation should hold the only slot")
	}

	mkChanConv(t, "fifo-z-older", "prof-fifo", "chan-fifo")
	time.Sleep(1100 * time.Millisecond) // creation timestamps are second-granular
	mkChanConv(t, "fifo-a-newer", "prof-fifo", "chan-fifo")
	reconcileWith(t, rc, "fifo-z-older")
	reconcileWith(t, rc, "fifo-a-newer")
	if phaseOf(t, "fifo-z-older") != agentopsv1alpha1.ConversationPending ||
		phaseOf(t, "fifo-a-newer") != agentopsv1alpha1.ConversationPending {
		t.Fatal("both over-cap conversations should be Pending")
	}

	// close the active conversation: the CR goes, and with it the pod and the slot
	if err := k8sClient.Delete(ctx, active); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, rc, "fifo-active")
	if podExists(t, "fifo-active") {
		t.Fatal("closing must release the runtime pod")
	}

	// the newer one still loses: the one free slot belongs to the older
	reconcileWith(t, rc, "fifo-a-newer")
	if got := phaseOf(t, "fifo-a-newer"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("newer conversation jumped the queue: phase %q", got)
	}
	if podExists(t, "fifo-a-newer") {
		t.Fatal("newer conversation took the freed slot")
	}

	reconcileWith(t, rc, "fifo-z-older")
	if got := phaseOf(t, "fifo-z-older"); got == agentopsv1alpha1.ConversationPending {
		t.Fatal("the oldest pending conversation was not admitted")
	}
	if !podExists(t, "fifo-z-older") {
		t.Fatal("admitted conversation must get a runtime pod")
	}
	if op := findOp(t, srv, "tg-fifo", chat.OpEnsureTopic, "fifo-z-older"); op == nil {
		t.Fatal("an admitted conversation must get its ensure-topic op")
	}
}

// TestPodDeletionWakesOldestPending pins the watch that fills a freed slot
// without waiting for the requeue backstop. Owns(&Pod{}) cannot do it: it
// routes the event to the pod's own conversation, which is the one that no
// longer needs the slot.
func TestPodDeletionWakesOldestPending(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-wake")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)

	mkChanConv(t, "wake-active", "prof-wake")
	reconcileWith(t, rc, "wake-active")
	mkChanConv(t, "wake-pending", "prof-wake")
	reconcileWith(t, rc, "wake-pending")
	if phaseOf(t, "wake-pending") != agentopsv1alpha1.ConversationPending {
		t.Fatal("setup: second conversation should be Pending")
	}

	var pod corev1.Pod
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "agentops-conv-wake-active"}, &pod); err != nil {
		t.Fatal(err)
	}
	reqs := rc.MapRuntimePodToPending(context.Background(), &pod)
	found := false
	for _, r := range reqs {
		if r.Name == "wake-pending" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a deleted runtime pod must wake the oldest pending conversation, got %+v", reqs)
	}
}

// TestBacklogBoundRefusesCreation: an unbounded Pending queue would reproduce
// the original complaint one level down. Window reuse is NOT gated by it — the
// bound stops new objects, not new inputs.
func TestBacklogBoundRefusesCreation(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-backlog")
	mkSignalSource(t, "src-backlog", "sig-backlog", "")
	mkPipeline(t, "pipe-backlog", []string{"src-backlog"}, nil, "prof-backlog")
	reconcilePipeline(t, "pipe-backlog")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 0) // nothing may be admitted: everything queues

	// first signal opens a conversation and the reconciler parks it Pending
	if rec := postSignal(t, srv.Handler(), testMasterToken, "src-backlog", []map[string]any{{
		"fingerprint": "fp-b1", "labels": map[string]string{"alertname": "One"}, "payload": "one",
	}}); rec.Code != 200 {
		t.Fatalf("first signal: %d %s", rec.Code, rec.Body.String())
	}
	first := onlyConvFor(t, "prof-backlog")
	t.Cleanup(func() { cleanupConversation(t, first) })
	reconcileWith(t, rc, first)
	if got := phaseOf(t, first); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("setup: phase %q, want Pending", got)
	}
	// pin the bound at whatever is pending right now, so the test is about the
	// bound and not about what earlier tests happened to leave behind
	srv.MaxQueuedConversations = pendingCount(t)

	// backlog is full: a signal with a NEW signature is refused with a reason
	rec := postSignal(t, srv.Handler(), testMasterToken, "src-backlog", []map[string]any{{
		"fingerprint": "fp-b2", "labels": map[string]string{"alertname": "Two"}, "payload": "two",
	}})
	if rec.Code != 200 {
		t.Fatalf("second signal: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Queued        int    `json:"queued"`
		Conversations int    `json:"conversations"`
		Reason        string `json:"reason"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Reason == "" || out.Conversations != 0 || out.Queued != 0 {
		t.Fatalf("full backlog must report a drop reason: %s", rec.Body.String())
	}
	if n := convCountFor(t, "prof-backlog"); n != 1 {
		t.Fatalf("a refused signal created a conversation: %d exist", n)
	}

	// but window reuse still lands on the existing pending conversation
	if rec := postSignal(t, srv.Handler(), testMasterToken, "src-backlog", []map[string]any{{
		"fingerprint": "fp-b3", "labels": map[string]string{"alertname": "One"}, "payload": "one again",
	}}); rec.Code != 200 {
		t.Fatalf("reuse signal: %d %s", rec.Code, rec.Body.String())
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: first}, &conv); err != nil {
		t.Fatal(err)
	}
	if len(conv.Spec.Inputs) != 2 {
		t.Fatalf("window reuse must keep appending to a pending conversation: %d inputs", len(conv.Spec.Inputs))
	}
}

// TestCloseArchivesThreadsThenReleases: deletion by any means archives the
// bound threads first, and a silent adapter can delay that but never wedge it.
// Renamed and re-aimed: DELETION now sends delete-conversation, not
// close-topic. The two say different things — a closed conversation's thread is
// archived and may come back, a deleted one's is neither — and a conversation
// being deleted gets one operation, never both, so no adapter has to work out
// whether a pair means one ending or two.
func TestDeletionTellsThreadsThenReleases(t *testing.T) {
	clearRuntimePods(t)
	ctx := context.Background()
	mkProfile(t, "prof-close")
	mkChannel(t, "chan-close-a", "tg-close-a")
	mkChannel(t, "chan-close-b", "tg-close-b")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 5)
	rc.CloseTopicGrace = time.Millisecond // no test waits two minutes

	conv := mkChanConv(t, "close-1", "prof-close", "chan-close-a", "chan-close-b")
	reconcileWith(t, rc, "close-1") // adds the finalizer, enqueues the topics

	// bind both threads, as completed ensure-topic ops would
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "close-1"}, conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{
		{Channel: "chan-close-a", ThreadID: "9876"},
		{Channel: "chan-close-b", ThreadID: "T-42"},
	}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	if err := k8sClient.Delete(ctx, conv); err != nil {
		t.Fatal(err)
	}
	// the finalizer holds the object while the ops go out
	var held agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "close-1"}, &held); err != nil {
		t.Fatalf("the close-topics finalizer must hold the object: %v", err)
	}
	reconcileWith(t, rc, "close-1")

	for _, tc := range []struct{ adapter, thread string }{{"tg-close-a", "9876"}, {"tg-close-b", "T-42"}} {
		op := findOp(t, srv, tc.adapter, chat.OpDeleteConversation, "close-1")
		if op == nil {
			t.Fatalf("%s: no delete-conversation op reached the adapter", tc.adapter)
		}
		if op.ThreadID == nil || *op.ThreadID != tc.thread {
			t.Fatalf("%s: delete-conversation op %+v", tc.adapter, op)
		}
		if op.Message == nil {
			t.Fatalf("%s: it must carry the notice the thread is owed", tc.adapter)
		}
		// and NOT close-topic: one lifecycle event, one operation
		if stale := findOp(t, srv, tc.adapter, chat.OpCloseTopic, "close-1"); stale != nil {
			t.Fatalf("%s: deletion must not also send close-topic", tc.adapter)
		}
	}

	// nobody completes them; the grace expires and deletion proceeds anyway
	reconcileWith(t, rc, "close-1")
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "close-1"}, &held)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("a silent adapter must not wedge deletion: err=%v finalizers=%v", err, held.Finalizers)
	}
	if podExists(t, "close-1") {
		t.Fatal("closing must release the runtime pod (and with it the capacity slot)")
	}
}

// TestCloseCompletesFinalizerOnAdapterAck: the happy path releases as soon as
// every bound thread reports archived, without waiting out the grace.
// Same re-aim: the operation the finalizer waits on is delete-conversation.
func TestDeletionCompletesFinalizerOnAdapterAck(t *testing.T) {
	clearRuntimePods(t)
	ctx := context.Background()
	mkProfile(t, "prof-close-ack")
	mkChannel(t, "chan-close-ack", "tg-close-ack")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 5) // grace left at the shipped two minutes

	conv := mkChanConv(t, "close-ack-1", "prof-close-ack", "chan-close-ack")
	reconcileWith(t, rc, "close-ack-1")
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "close-ack-1"}, conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "chan-close-ack", ThreadID: "555"}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Delete(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, rc, "close-ack-1")

	op := findOp(t, srv, "tg-close-ack", chat.OpDeleteConversation, "close-ack-1")
	if op == nil {
		t.Fatal("no delete-conversation op reached the adapter")
	}
	// an EMPTY body is how an adapter says "archived"
	rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID), nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("empty-body completion: %d %s", rec.Code, rec.Body.String())
	}

	reconcileWith(t, rc, "close-ack-1")
	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "close-ack-1"}, &after); !apierrors.IsNotFound(err) {
		t.Fatalf("archived threads must release the finalizer at once: err=%v finalizers=%v", err, after.Finalizers)
	}
}

// TestDefaultIdleTTLReachesThePod pins the shipped default: at 1 minute a
// finished conversation releases its slot promptly, which is what makes a cap
// of 5 workable. A per-runtime override still wins (TestAgentRuntimeSelection).
func TestDefaultIdleTTLReachesThePod(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-ttl")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 5)
	mkChanConv(t, "ttl-1", "prof-ttl")
	reconcileWith(t, rc, "ttl-1")

	var pod corev1.Pod
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "agentops-conv-ttl-1"}, &pod); err != nil {
		t.Fatal(err)
	}
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "RUNTIME_IDLE_TTL_M" {
			if e.Value != "1" {
				t.Fatalf("default RUNTIME_IDLE_TTL_M = %q, want \"1\"", e.Value)
			}
			return
		}
	}
	t.Fatal("RUNTIME_IDLE_TTL_M not set on the runtime pod")
}

// findOp drains an adapter's queue until it sees the op it is looking for, so
// a test asserting on one kind is not tripped by another kind queued earlier.
func findOp(t *testing.T, srv *httpapi.Server, adapter string, kind chat.OpKind, conversation string) *chat.Op {
	t.Helper()
	for {
		rec := adapterReq(srv, "GET", "/channel/ops?adapter="+adapter+"&contract=2&wait=0", nil, testMasterToken)
		if rec.Code != 200 {
			return nil
		}
		var op chat.Op
		if err := json.Unmarshal(rec.Body.Bytes(), &op); err != nil {
			t.Fatal(err)
		}
		if op.Kind == kind && (conversation == "" || op.Conversation == conversation) {
			return &op
		}
	}
}

// onlyConvFor returns the single conversation using a profile.
func onlyConvFor(t *testing.T, profile string) string {
	t.Helper()
	names := convNamesFor(t, profile)
	if len(names) != 1 {
		t.Fatalf("want exactly one conversation for %s, got %v", profile, names)
	}
	return names[0]
}

// pendingCount is the namespace-wide pending backlog the manager would see.
func pendingCount(t *testing.T) int {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := range list.Items {
		if list.Items[i].Status.Phase == agentopsv1alpha1.ConversationPending {
			n++
		}
	}
	return n
}

func convCountFor(t *testing.T, profile string) int {
	t.Helper()
	return len(convNamesFor(t, profile))
}

func convNamesFor(t *testing.T, profile string) []string {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	var names []string
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == profile && list.Items[i].DeletionTimestamp.IsZero() {
			names = append(names, list.Items[i].Name)
		}
	}
	return names
}
