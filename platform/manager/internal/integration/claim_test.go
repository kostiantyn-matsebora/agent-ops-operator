// durable-chat-ops-broker: the claim a leader writes on a channel binding
// before dispatching its ensure-topic op, and the two ways that claim is
// cleared again — staleness (the SAME leader's own claim gone quiet too
// long) and a holder mismatch (a FORMER leader's claim, cleared at once,
// never waited out).
package integration

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// reconcilerAs is reconcilerWithOps with an explicit replica identity and
// claim staleness bound, for the claim tests below.
func reconcilerAs(ops *chat.OpQueue, identity string, staleness time.Duration) *controller.ConversationReconciler {
	return &controller.ConversationReconciler{
		Client:                 k8sClient,
		Scheme:                 scheme,
		MaxActiveConversations: 100,
		Ops:                    ops,
		ReplicaIdentity:        identity,
		ClaimStaleness:         staleness,
		Runtime: runtimepod.Config{
			Image: "busybox:stub", ServiceAccount: "default",
			ControlURL: "http://manager:8080", IdleTTLMinutes: 1,
		},
	}
}

func claimConv(t *testing.T, name, profile, channel string) *agentopsv1alpha1.Conversation {
	t.Helper()
	ctx := context.Background()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: channel}}
	conv.Spec.Title = "claim test"
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	return conv
}

// A leader's first pass over an unbound channel writes a claim naming itself,
// and dispatches the ensure-topic op (durable-chat-ops-broker: "The leader
// claims an op").
func TestEnsureTopicsWritesClaimOnFirstDispatch(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-1")
	mkChannel(t, "chan-claim-1", "tg-claim-1")
	conv := claimConv(t, "claim-conv-1", "prof-claim-1", "chan-claim-1")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a", 0)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}

	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &after); err != nil {
		t.Fatal(err)
	}
	binding := after.Status.Thread("chan-claim-1")
	if binding == nil || binding.Claim == nil || binding.Claim.Holder != "leader-a" {
		t.Fatalf("expected a claim naming leader-a: %+v", binding)
	}
	if binding.ThreadID != "" {
		t.Fatalf("no adapter has completed the op yet: %+v", binding)
	}
	if op := ops.Claim("tg-claim-1"); op == nil || op.Kind != chat.OpEnsureTopic {
		t.Fatalf("ensure-topic op expected on the queue: %+v", op)
	}
}

// The SAME leader reconciling again while its own claim is still fresh does
// not rewrite it — the op is genuinely in flight with an adapter, and
// touching the claim again would only race a write against nothing
// ("A legitimately slow claim is left alone").
func TestEnsureTopicsLeavesALiveClaimAlone(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-2")
	mkChannel(t, "chan-claim-2", "tg-claim-2")
	conv := claimConv(t, "claim-conv-2", "prof-claim-2", "chan-claim-2")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a", time.Hour)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	var first agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &first)
	claimedAt := first.Status.Thread("chan-claim-2").Claim.ClaimedAt

	time.Sleep(50 * time.Millisecond)
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	var second agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &second)
	binding := second.Status.Thread("chan-claim-2")
	if binding == nil || binding.Claim == nil {
		t.Fatalf("claim must still stand: %+v", binding)
	}
	if !binding.Claim.ClaimedAt.Time.Equal(claimedAt.Time) {
		t.Fatalf("a live claim must not be rewritten: first=%s second=%s", claimedAt, binding.Claim.ClaimedAt)
	}
}

// A claim older than the staleness bound, with no thread yet, is cleared and
// the op is retried ("A stale claim is retried") — even by the SAME replica
// that wrote it, because the bound exists precisely for a legitimately slow
// in-flight call that turned out not to come back.
func TestEnsureTopicsRetriesAStaleClaim(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-3")
	mkChannel(t, "chan-claim-3", "tg-claim-3")
	conv := claimConv(t, "claim-conv-3", "prof-claim-3", "chan-claim-3")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a", 10*time.Millisecond)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	var first agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &first)
	staleAt := first.Status.Thread("chan-claim-3").Claim.ClaimedAt

	// metav1.Time round-trips through the API server at WHOLE-SECOND
	// precision (ThreadBinding.ReadAt's own comment documents the same
	// truncation), so the gap has to clear a full second to be observable
	// here — the staleness bound itself (10ms) is unaffected, since it is
	// compared against time.Now() in the reconciler's own process.
	time.Sleep(1200 * time.Millisecond)
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	var second agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &second)
	binding := second.Status.Thread("chan-claim-3")
	if binding == nil || binding.Claim == nil {
		t.Fatalf("a fresh claim must replace the stale one: %+v", binding)
	}
	if !binding.Claim.ClaimedAt.Time.After(staleAt.Time) {
		t.Fatalf("stale claim must be refreshed: before=%s after=%s", staleAt, binding.Claim.ClaimedAt)
	}
}

// A claim held by a DIFFERENT replica identity — a former leader — is
// cleared and dispatched again AT ONCE, never waited out, whatever the
// staleness bound says ("A dead leader's claim is not waited out").
func TestEnsureTopicsClearsAFormerLeadersClaimImmediately(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-4")
	mkChannel(t, "chan-claim-4", "tg-claim-4")
	conv := claimConv(t, "claim-conv-4", "prof-claim-4", "chan-claim-4")

	// leader-a claims it, long bound: nothing about this claim is stale.
	ops := testOps()
	leaderA := reconcilerAs(ops, "leader-a", time.Hour)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}
	if _, err := leaderA.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	// leader-b takes over (a fresh process: this reconciler never runs unless
	// its process holds the Lease, so any claim it did not itself write is,
	// by construction, a former leader's).
	leaderB := reconcilerAs(testOps(), "leader-b", time.Hour)
	if _, err := leaderB.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &after)
	binding := after.Status.Thread("chan-claim-4")
	if binding == nil || binding.Claim == nil || binding.Claim.Holder != "leader-b" {
		t.Fatalf("the new leader must claim it immediately: %+v", binding)
	}
}

// Completing the op — success or failure — releases the claim: it is no
// longer in flight either way, and a still-fresh claim must not make the
// next pass wait out the staleness bound before noticing.
func TestCompletingEnsureTopicClearsTheClaim(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-5")
	mkChannel(t, "chan-claim-5", "tg-claim-5")
	conv := claimConv(t, "claim-conv-5", "prof-claim-5", "chan-claim-5")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a", time.Hour)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}
	op := ops.Claim("tg-claim-5")
	if op == nil {
		t.Fatal("ensure-topic op expected")
	}
	ops.Complete(ctx, op.ID, chat.OpResult{ThreadID: "t-claim-5"})

	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &after)
	binding := after.Status.Thread("chan-claim-5")
	if binding == nil || binding.ThreadID != "t-claim-5" {
		t.Fatalf("thread not landed: %+v", binding)
	}
	if binding.Claim != nil {
		t.Fatalf("a completed op must release its claim: %+v", binding.Claim)
	}
}

// A run reply that fails to send is surfaced on the thread binding as
// undeliveredReply, and a later successful send of the SAME run clears it
// (state-durability: "An owed reply is visible on the object").
func TestRunReplyFailureSurfacesUndeliveredReplyAndSuccessClearsIt(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-undelivered")
	mkChannel(t, "chan-undelivered", "tg-undelivered")

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "undelivered-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-undelivered"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-undelivered"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "chan-undelivered", ThreadID: "t-undelivered"}}
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{RunID: "run-1", DeliveryTracked: true, Result: "the answer"}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	ops := testOps()
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "chan-undelivered"}, &ch); err != nil {
		t.Fatal(err)
	}
	tid := "t-undelivered"
	ops.EnqueueRunReply(ctx, &ch, conv.Name, "run-1", &tid, chat.AnswerMessage("the answer", ""))
	op := ops.Claim("tg-undelivered")
	if op == nil {
		t.Fatal("run reply op expected")
	}
	ops.Complete(ctx, op.ID, chat.OpResult{Error: "telegram: rate limited"})

	var afterFail agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &afterFail)
	binding := afterFail.Status.Thread("chan-undelivered")
	if binding == nil || binding.UndeliveredReply != "run-1" {
		t.Fatalf("expected undeliveredReply=run-1: %+v", binding)
	}

	// re-derive and succeed this time
	ops.EnqueueRunReply(ctx, &ch, conv.Name, "run-1", &tid, chat.AnswerMessage("the answer", ""))
	op = ops.Claim("tg-undelivered")
	if op == nil {
		t.Fatal("re-derived run reply op expected")
	}
	ops.Complete(ctx, op.ID, chat.OpResult{})

	var afterSuccess agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &afterSuccess)
	binding = afterSuccess.Status.Thread("chan-undelivered")
	if binding == nil || binding.UndeliveredReply != "" {
		t.Fatalf("a successful delivery must clear undeliveredReply: %+v", binding)
	}
	run := runNamed(t, &afterSuccess, "run-1")
	if !run.DeliveredTo("chan-undelivered") {
		t.Fatalf("run must be recorded delivered: %+v", run)
	}
}

// /channel/ops on a non-leader rejects with 503 rather than answering from an
// OpQueue the leader-gated reconciler never populated on this process
// ("A non-leader rejects a poll").
func TestChannelOpsRejectsNonLeader(t *testing.T) {
	srv, _ := apiServerWithActivity()
	srv.ReplicaIdentity = "replica-b"
	// no Lease object exists: the current leader is unknown, which must be
	// treated the same as "not me" — a replica that cannot tell whether it is
	// the leader cannot safely answer this poll either.
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-leader&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 503 {
		t.Fatalf("non-leader (unknown lease) must reject with 503, got %d: %s", rec.Code, rec.Body.String())
	}

	holder := "replica-b"
	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = httpapi.LeaseName, ns
	lease.Spec.HolderIdentity = &holder
	if err := k8sClient.Create(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), lease) })

	rec = adapterReq(srv, "GET", "/channel/ops?adapter=tg-leader&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 204 {
		t.Fatalf("the current leader must be served normally (empty queue = 204), got %d: %s", rec.Code, rec.Body.String())
	}
}
