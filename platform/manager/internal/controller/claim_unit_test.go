package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
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
func claimStore(c client.WithWatch) state.Store { return kube.NewStore(c, claimTestNS) }

func claimRec(t *testing.T, store state.Store, name string) *state.ConversationRecord {
	t.Helper()
	rec, _, err := store.Get(context.Background(), state.Key(name))
	if err != nil {
		t.Fatalf("read record %q: %v", name, err)
	}
	return &rec
}

func claimRuns(t *testing.T, store state.Store, name string) []state.Run {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), state.Key(name))
	if err != nil {
		t.Fatalf("read runs of %q: %v", name, err)
	}
	return runs
}

func claimTestConv(name string) *agentopsv1alpha1.Conversation {
	return &agentopsv1alpha1.Conversation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: claimTestNS},
	}
}

// claimStaleness defers to the configured bound when one is set, and to
// DefaultClaimStalenessSeconds — never zero — when it is not.
func TestClaimStalenessFallsBackToTheDefaultWhenUnset(t *testing.T) {
	r := &ConversationReconciler{}
	if got, want := r.claimStaleness(), DefaultClaimStalenessSeconds*time.Second; got != want {
		t.Fatalf("claimStaleness() = %v, want the default %v", got, want)
	}

	r.ClaimStaleness = 5 * time.Minute
	if got, want := r.claimStaleness(), 5*time.Minute; got != want {
		t.Fatalf("claimStaleness() = %v, want the configured %v", got, want)
	}
}

// No binding at all for the channel: claimEnsureTopic claims it and writes
// this replica's identity.
func TestClaimEnsureTopicClaimsWhenNoBindingExists(t *testing.T) {
	conv := claimTestConv("conv-1")
	c := fakeClaimClient(t, conv).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err != nil || !claimed {
		t.Fatalf("claimed=%v err=%v, want claimed", claimed, err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: claimTestNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	binding := got.Status.Thread("c1")
	if binding == nil || binding.Claim == nil || binding.Claim.Holder != "me" {
		t.Fatalf("binding = %+v, want a fresh claim held by me", binding)
	}
}

// This leader's own claim, still inside the staleness bound, is left alone:
// the op is genuinely in flight and nothing new is dispatched.
func TestClaimEnsureTopicSkipsALiveClaimHeldByThisLeader(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{
		Channel: "c1",
		Claim:   &agentopsv1alpha1.OpClaim{Holder: "me", ClaimedAt: metav1.Time{Time: time.Now()}},
	}}
	c := fakeClaimClient(t, conv).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err != nil || claimed {
		t.Fatalf("claimed=%v err=%v, want not claimed", claimed, err)
	}
}

// A claim held by a DIFFERENT identity is, by construction, a former
// leader's — this leader reclaims it at once rather than waiting it out.
func TestClaimEnsureTopicReclaimsAFormerLeadersClaim(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{
		Channel: "c1",
		Claim:   &agentopsv1alpha1.OpClaim{Holder: "old-leader", ClaimedAt: metav1.Time{Time: time.Now()}},
	}}
	c := fakeClaimClient(t, conv).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err != nil || !claimed {
		t.Fatalf("claimed=%v err=%v, want claimed (former leader's claim)", claimed, err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: claimTestNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if binding := got.Status.Thread("c1"); binding == nil || binding.Claim.Holder != "me" {
		t.Fatalf("binding = %+v, want the claim overwritten to me", binding)
	}
}

// This leader's OWN claim, past the staleness bound with no thread yet, is
// treated as abandoned and reclaimed — the slow-answering case, not a second
// writer.
func TestClaimEnsureTopicReclaimsItsOwnStaleClaim(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{
		Channel: "c1",
		Claim:   &agentopsv1alpha1.OpClaim{Holder: "me", ClaimedAt: metav1.Time{Time: time.Now().Add(-5 * time.Minute)}},
	}}
	c := fakeClaimClient(t, conv).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me", ClaimStaleness: time.Minute}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err != nil || !claimed {
		t.Fatalf("claimed=%v err=%v, want claimed (stale)", claimed, err)
	}
}

// A conversation gone by the time the claim is attempted is tolerated.
func TestClaimEnsureTopicToleratesAMissingConversation(t *testing.T) {
	c := fakeClaimClient(t).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err != nil || claimed {
		t.Fatalf("claimed=%v err=%v, want not claimed, no error", claimed, err)
	}
}

// A non-conflict error claiming the binding is returned rather than
// retried.
func TestClaimEnsureTopicReturnsANonConflictPatchError(t *testing.T) {
	conv := claimTestConv("conv-1")
	boom := errors.New("boom")
	c := fakeClaimClient(t, conv).WithInterceptorFuncs(interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
			obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return boom
		},
	}).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err == nil || !errors.Is(err, boom) || claimed {
		t.Fatalf("claimed=%v err=%v, want the patch error returned, not retried", claimed, err)
	}
}

// Every retry conflicting is the abandoned-after-5-attempts case.
func TestClaimEnsureTopicGivesUpAfterRepeatedConflicts(t *testing.T) {
	conv := claimTestConv("conv-1")
	c := fakeClaimClient(t, conv).WithInterceptorFuncs(interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
			obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return apierrors.NewConflict(schema.GroupResource{Resource: "conversations"}, "conv-1", errors.New("stale"))
		},
	}).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me"}

	claimed, err := r.claimEnsureTopic(context.Background(), "conv-1", "c1")
	if err == nil || claimed {
		t.Fatalf("claimed=%v err=%v, want a conflict-exhausted error", claimed, err)
	}
}

// An error claiming the binding (here, every patch conflicting) is surfaced
// through ensureTopics as its own firstErr, and does not stop the channel
// loop from running (there being only one channel here, nothing further to
// observe beyond the error itself).
func TestEnsureTopicsSurfacesAClaimError(t *testing.T) {
	conv := claimTestConv("conv-1")
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	ch := &agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: claimTestNS}}
	ch.Spec.Adapter = "slack"
	c := fakeClaimClient(t, conv, ch).WithInterceptorFuncs(interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
			obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return apierrors.NewConflict(schema.GroupResource{Resource: "conversations"}, "conv-1", errors.New("stale"))
		},
	}).Build()
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me", Ops: &chat.OpQueue{Registry: chat.NewRegistry()}}

	if _, err := r.ensureTopics(context.Background(), claimRec(t, r.Store, "conv-1")); err == nil {
		t.Fatal("want the claim error surfaced")
	}
	if op := r.Ops.Claim("slack"); op != nil {
		t.Fatalf("a channel whose claim errored must not be dispatched: %+v", op)
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
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, ReplicaIdentity: "me", Ops: &chat.OpQueue{Registry: chat.NewRegistry()}}

	pending, err := r.ensureTopics(context.Background(), claimRec(t, r.Store, "conv-1"))
	if err != nil || !pending {
		t.Fatalf("pending=%v err=%v, want pending", pending, err)
	}

	op := r.Ops.Claim("slack")
	if op == nil || op.Topic == nil || op.Topic.PreviousThreadID != "old-thread" {
		t.Fatalf("op = %+v, want a topic descriptor naming the previous thread", op)
	}
}

// A claim-only placeholder binding (durable-chat-ops-broker's own
// setClaim, written before ensure-topic completes) carries no ThreadID.
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
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry()}}

	rec := claimRec(t, r.Store, "conv-1")
	if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Store, "conv-1")); err != nil {
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
	r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry()}}

	rec := claimRec(t, r.Store, "conv-1")
	if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Store, "conv-1")); err != nil {
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
			r := &ConversationReconciler{Client: c, Store: claimStore(c), Namespace: claimTestNS, Ops: &chat.OpQueue{Registry: chat.NewRegistry()}}

			rec := claimRec(t, r.Store, "conv-1")
			if err := r.deliverRunReplies(context.Background(), rec, claimRuns(t, r.Store, "conv-1")); err != nil {
				t.Fatal(err)
			}

			if op := r.Ops.Claim("slack"); op != nil {
				t.Fatalf("a delivered run must not be sent again: %+v", op)
			}
			if b := claimRec(t, r.Store, "conv-1").Thread("c1"); b == nil || b.UndeliveredReply != tc.wantNote {
				t.Fatalf("note = %+v, want %q", b, tc.wantNote)
			}
		})
	}
}
