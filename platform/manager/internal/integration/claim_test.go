// async-command-bus design D-11: status.threads[].claim — the leader-only
// dispatch placeholder durable-chat-ops-broker introduced — is retired.
// ensureTopics dispatches unconditionally on every pass now; it is
// chat.OpQueue.EnqueueEnsureTopic's own stable per-conversation×channel
// CommandID that the BUS dedups a resubmission against. /channel/ops
// answers from ANY replica sharing that bus, with no Lease read and no
// leader-only refusal.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	commandbusmemory "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// reconcilerAs is reconcilerWithOps with an explicit replica identity, for
// the dispatch tests below. The identity gates nothing any more (D-11
// retired the claim it used to write) — named anyway, for readability, and
// because TestEnsureTopicsDispatchIsIndifferentToReplicaIdentity below
// exists specifically to pin that indifference.
func reconcilerAs(ops *chat.OpQueue, identity string) *controller.ConversationReconciler {
	return &controller.ConversationReconciler{
		Client:        k8sClient,
		Conversations: stores.Conversations, Runs: stores.Runs, Blobs: stores.Blobs, Namespace: ns,
		Scheme:                 scheme,
		MaxActiveConversations: 100,
		Ops:                    ops,
		ReplicaIdentity:        identity,
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

// A first pass over an unbound channel dispatches the ensure-topic op
// unconditionally and writes no claim of any kind — no ThreadBinding exists
// at all until the op completes (bindThread), since nothing stamps a
// claim-only placeholder there any more.
func TestEnsureTopicsDispatchesOnFirstPassWithNoClaimWritten(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-1")
	mkChannel(t, "chan-claim-1", "tg-claim-1")
	conv := claimConv(t, "claim-conv-1", "prof-claim-1", "chan-claim-1")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a")
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}

	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &after); err != nil {
		t.Fatal(err)
	}
	if binding := after.Status.Thread("chan-claim-1"); binding != nil {
		t.Fatalf("no binding (claim or otherwise) until the op completes, got %+v", binding)
	}
	if op := ops.Claim("tg-claim-1"); op == nil || op.Kind != chat.OpEnsureTopic {
		t.Fatalf("ensure-topic op expected on the queue: %+v", op)
	}
}

// A second pass before the first op is even claimed must not dispatch a
// second one — the bus dedups the resubmission against the same stable
// CommandID, not a reconciler-level claim.
func TestEnsureTopicsResubmissionDedupsOnTheBus(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-2")
	mkChannel(t, "chan-claim-2", "tg-claim-2")
	conv := claimConv(t, "claim-conv-2", "prof-claim-2", "chan-claim-2")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a")
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := rc.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	if op := ops.Claim("tg-claim-2"); op == nil {
		t.Fatal("the first pass's op must be claimable")
	}
	if op := ops.Claim("tg-claim-2"); op != nil {
		t.Fatalf("a second reconcile pass must not have dispatched a second op: %+v", op)
	}
}

// A DIFFERENT replica identity reconciling the same conversation makes no
// difference — there is no claim ownership left to contest, only the SAME
// bus deduping both passes' dispatch, indifferent to who ran it.
func TestEnsureTopicsDispatchIsIndifferentToReplicaIdentity(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-4")
	mkChannel(t, "chan-claim-4", "tg-claim-4")
	conv := claimConv(t, "claim-conv-4", "prof-claim-4", "chan-claim-4")

	ops := testOps()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}

	leaderA := reconcilerAs(ops, "leader-a")
	if _, err := leaderA.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	// A fresh reconciler under a different identity, sharing the SAME ops
	// (and so the same bus) — modelling a new leader taking over.
	leaderB := reconcilerAs(ops, "leader-b")
	if _, err := leaderB.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	if op := ops.Claim("tg-claim-4"); op == nil {
		t.Fatal("one op must be claimable, from whichever replica dispatched it")
	}
	if op := ops.Claim("tg-claim-4"); op != nil {
		t.Fatalf("the second replica's pass must not have added a second op: %+v", op)
	}
}

// Completing the op lands the thread id; status.threads[].claim stays nil
// throughout, since nothing ever wrote one.
func TestCompletingEnsureTopicLandsTheThreadIDWithNoClaimEverWritten(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-claim-5")
	mkChannel(t, "chan-claim-5", "tg-claim-5")
	conv := claimConv(t, "claim-conv-5", "prof-claim-5", "chan-claim-5")

	ops := testOps()
	rc := reconcilerAs(ops, "leader-a")
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
		t.Fatalf("status.threads[].claim must never be written, got %+v", binding.Claim)
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

	// A Backing, not plain testOps()/memory.Open(): a failed run reply is
	// now RETRIED by the bus with backoff (design D-5, out.stable) rather
	// than released for an immediate re-derive, so advancing the clock past
	// that backoff is what stands in for "later" below.
	backing := commandbusmemory.New()
	ops := &chat.OpQueue{Conversations: stores.Conversations, Runs: stores.Runs, Registry: chat.NewRegistry(), Bus: backing.Bus()}
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "chan-undelivered"}, &ch); err != nil {
		t.Fatal(err)
	}
	tid := "t-undelivered"
	ops.EnqueueRunReply(ctx, &ch, recOf(t, conv.Name), "run-1", &tid, chat.AnswerMessage("the answer", ""))
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

	// re-derive and succeed this time, once the bus's own backoff elapses
	backing.Advance(6 * time.Minute)
	ops.EnqueueRunReply(ctx, &ch, recOf(t, conv.Name), "run-1", &tid, chat.AnswerMessage("the answer", ""))
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

// /channel/ops answers an empty-queue poll with 204 from ANY replica,
// leader or not, with no Lease even existing — the leader-only refusal
// durable-chat-ops-broker required is gone (async-command-bus design D-11).
func TestChannelOpsAnswers204RegardlessOfLease(t *testing.T) {
	srv, _ := apiServerWithActivity()
	srv.ReplicaIdentity = "replica-b"
	// no Lease object exists at all — formerly "leader unknown", which used
	// to refuse this poll outright. Now irrelevant to this endpoint.
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-leader&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 204 {
		t.Fatalf("an empty queue must answer 204 from any replica, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Two Servers, each its own OpQueue, SHARING one bus — the relationship
// nats gives two real replica processes (a bare memory.Bus would not:
// design D-13 restricts it to one replica). The op one reconciler
// dispatches through the "leader" OpQueue is claimable through the
// "follower" Server's /channel/ops directly — no Lease, no pod lookup, no
// proxy attempt, because the endpoint is now exempt from
// leaderProxyMiddleware entirely (async-command-bus design D-11).
func TestChannelOpsAnswersFromAnyReplicaSharingTheBus(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-bus-shared")
	mkChannel(t, "chan-bus-shared", "tg-shared")
	conv := claimConv(t, "bus-shared-conv", "prof-bus-shared", "chan-bus-shared")

	leaderSrv, _ := apiServerWithActivity()
	followerSrv, _ := apiServerWithActivity()
	followerSrv.Ops = &chat.OpQueue{
		Conversations: stores.Conversations, Runs: stores.Runs,
		Registry: chat.NewRegistry(), Bus: leaderSrv.Ops.Bus,
	}

	rc := reconcilerAs(leaderSrv.Ops, "leader-a")
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(followerSrv, "GET", "/channel/ops?adapter=tg-shared&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("a replica sharing the bus must answer the leader-enqueued op directly, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "ensure-topic") {
		t.Fatalf("the answer must be the leader's own ensure-topic op: %s", got)
	}

	// The follower's OWN OpQueue local state now holds the claim entry, but
	// it is the SAME bus task the leader's reconciler created — claiming
	// again must find nothing left, proving there was only ever one op.
	if followerSrv.Ops.Claim("tg-shared") != nil {
		t.Fatal("the op was already claimed once; a second claim must find nothing left")
	}
}
