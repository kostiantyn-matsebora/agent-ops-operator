package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

func fakeClaimClient(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithRuntimeObjects(objs...)
}

const claimTestNS = "ns-a"

// claimStore is the kube backend over the fake client: the unit tests seed
// Conversation objects the way the apiserver holds them and the reconciler
// reaches them through the store, as it does in the manager.
func claimStore(c client.WithWatch) state.Collection[state.ConversationRecord] {
	coll, err := kube.Open(c, claimTestNS, kube.ConversationLayout())
	if err != nil {
		panic(err)
	}
	return coll
}

// claimRunsCollection is the Runs half of the same backend.
func claimRunsCollection(c client.WithWatch) state.Appended[state.ConversationRecord, state.Run] {
	runs, err := kube.OpenAppended(c, claimTestNS, kube.RunsLayout())
	if err != nil {
		panic(err)
	}
	return runs
}

func claimRec(t *testing.T, store state.Collection[state.ConversationRecord], name string) *state.ConversationRecord {
	t.Helper()
	rec, _, err := store.Get(context.Background(), state.Key(name))
	if err != nil {
		t.Fatalf("read record %q: %v", name, err)
	}
	return &rec
}

func claimRuns(t *testing.T, runs state.Appended[state.ConversationRecord, state.Run], name string) []state.Run {
	t.Helper()
	got, err := runs.List(context.Background(), state.Key(name))
	if err != nil {
		t.Fatalf("read runs of %q: %v", name, err)
	}
	return got
}

func claimTestConv(name string) *agentopsv1alpha1.Conversation {
	return &agentopsv1alpha1.Conversation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: claimTestNS},
	}
}

// async-command-bus design D-11: status.threads[].claim is retired.
// ensureTopics dispatches unconditionally on every pass; it is
// EnqueueEnsureTopic's own stable per-conversation×channel CommandID that
// the BUS dedups a resubmission against, not a reconciler-level claim. A
// second pass before the first op is even claimed must not have dispatched
// a second one, and the binding must never carry a claim at all.
func TestEnsureTopicsDispatchesUnconditionallyDedupedByTheBus(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, ReplicaIdentity: "me", Ops: &chat.OpQueue{Registry: chat.NewRegistry(), Bus: memory.Open()}}

	rec := claimRec(t, r.Conversations, "conv-1")
	if pending, err := r.ensureTopics(context.Background(), rec); err != nil || !pending {
		t.Fatalf("pending=%v err=%v, want pending on the first pass", pending, err)
	}
	if pending, err := r.ensureTopics(context.Background(), rec); err != nil || !pending {
		t.Fatalf("pending=%v err=%v, want still pending on the second pass (no thread landed yet)", pending, err)
	}

	if r.Ops.Claim("slack") == nil {
		t.Fatal("the first pass's op must be claimable")
	}
	if op := r.Ops.Claim("slack"); op != nil {
		t.Fatalf("a second pass must not have dispatched a second op — the bus must have deduped it: %+v", op)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: claimTestNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if binding := got.Status.Thread("c1"); binding != nil && binding.Claim != nil {
		t.Fatalf("status.threads[].claim must never be written, got %+v", binding.Claim)
	}
}

// Even a DIFFERENT replica identity reconciling makes no difference now —
// there is no claim ownership left to contest. Both passes dispatch through
// the SAME bus, which is what dedups them, independent of who is "the
// leader".
func TestEnsureTopicsDispatchIsIndifferentToReplicaIdentity(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).Build()
	bus := memory.Open()
	ops := &chat.OpQueue{Registry: chat.NewRegistry(), Bus: bus}

	leaderA := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, ReplicaIdentity: "leader-a", Ops: ops}
	leaderB := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, ReplicaIdentity: "leader-b", Ops: ops}

	rec := claimRec(t, leaderA.Conversations, "conv-1")
	if _, err := leaderA.ensureTopics(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if _, err := leaderB.ensureTopics(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	if ops.Claim("slack") == nil {
		t.Fatal("one op must be claimable, from whichever replica dispatched it")
	}
	if op := ops.Claim("slack"); op != nil {
		t.Fatalf("the second replica's pass must not have added a second op: %+v", op)
	}
}

// ensureTopics carries the archived thread's old id as a hint on reopen —
// an adapter that honours it continues where the thread left off.
func TestEnsureTopicsCarriesThePreviousThreadIDOnReopen(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "old-thread"}}
	conv.Status.ThreadsArchived = []string{"c1"}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, ReplicaIdentity: "me", Ops: &chat.OpQueue{Registry: chat.NewRegistry(), Bus: memory.Open()}}

	pending, err := r.ensureTopics(context.Background(), claimRec(t, r.Conversations, "conv-1"))
	if err != nil || !pending {
		t.Fatalf("pending=%v err=%v, want pending", pending, err)
	}

	op := r.Ops.Claim("slack")
	if op == nil || op.Topic == nil || op.Topic.PreviousThreadID != "old-thread" {
		t.Fatalf("op = %+v, want a topic descriptor naming the previous thread", op)
	}
}

// A binding with no ThreadID yet — carrying only a Claim, which a
// pre-upgrade replica could still have written before async-command-bus
// design D-11 retired it (this reconciler itself never writes one any
// more; the field here models leftover state, and is otherwise ignored).
// deliverRunReplies must not treat it as a topic to deliver into: doing so
// would hand the adapter an empty thread id, which parks the reply on a
// channel-level pseudo-thread and marks it delivered PERMANENTLY — the real
// thread ensureTopics creates moments later would never receive the answer.
func TestDeliverRunRepliesSkipsAClaimOnlyPlaceholder(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{
		Channel: "c1",
		Claim:   &agentopsv1alpha1.OpClaim{Holder: "me", ClaimedAt: metav1.Time{Time: time.Now()}},
	}}
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "run-1", Status: "succeeded", Result: "the answer", DeliveryTracked: true,
	}}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry(), Bus: memory.Open()}}

	rec := claimRec(t, r.Conversations, "conv-1")
	if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Runs, "conv-1")); err != nil {
		t.Fatal(err)
	}

	if op := r.Ops.Claim("slack"); op != nil {
		t.Fatalf("a claim-only binding must not be delivered into: %+v", op)
	}
	cond := rec.FindCondition(ConditionDeliveryPending)
	if cond == nil || cond.Status != state.ConditionTrue {
		t.Fatalf("condition = %+v, want DeliveryPending=True — the answer is still owed", cond)
	}
}

// Once the thread is real, the same run delivers normally.
func TestDeliverRunRepliesDeliversOnceTheThreadIsReal(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1"}}
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "run-1", Status: "succeeded", Result: "the answer", DeliveryTracked: true,
	}}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry(), Bus: memory.Open()}}

	rec := claimRec(t, r.Conversations, "conv-1")
	if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Runs, "conv-1")); err != nil {
		t.Fatal(err)
	}

	if r.Ops.Claim("slack") == nil {
		t.Fatal("a bound real thread must get the reply enqueued")
	}
}

// markDelivered writes the run marker and then the thread's note; a crash
// between them leaves a delivered run with a note still naming it. The
// reconciler backstop — the only pass that visits an already-delivered run —
// must settle that note, without sending again, and must leave a note naming a
// different (genuinely owed) run alone.
func TestDeliverRunRepliesSettlesANoteLeftByACrashAfterTheRunMarker(t *testing.T) {
	for _, tc := range []struct {
		name, note, wantNote string
	}{
		{"note names the delivered run", "run-1", ""},
		{"note names a different owed run", "run-2", "run-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := claimTestConv("conv-1")
			conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
			conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1", UndeliveredReply: tc.note}}
			conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
				RunID: "run-1", Status: "succeeded", Result: "the answer", DeliveryTracked: true,
				Delivered: []string{"c1"},
			}}
			ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
			ch.Spec.Adapter = "slack"
			c := fakeClaimClient(t, conv, ch).Build()
			r := &ConversationReconciler{Client: c, Conversations: claimStore(c), Runs: claimRunsCollection(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry(), Bus: memory.Open()}}

			rec := claimRec(t, r.Conversations, "conv-1")
			if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Runs, "conv-1")); err != nil {
				t.Fatal(err)
			}

			if op := r.Ops.Claim("slack"); op != nil {
				t.Fatalf("a delivered run must not be sent again: %+v", op)
			}
			if b := claimRec(t, r.Conversations, "conv-1").Thread("c1"); b == nil || b.UndeliveredReply != tc.wantNote {
				t.Fatalf("note = %+v, want %q", b, tc.wantNote)
			}
		})
	}
}
