package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	commandbusmemory "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

func testChannel(name, typ string) *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name = name
	ch.Spec.Adapter = typ
	return ch
}

func testConv(name string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name = name
	c.UID = types.UID("uid-" + name)
	c.Spec.Title = "t: " + name
	return c
}

// finishEnsureTopic's two error branches -- an adapter that reported it
// could not create a topic at all, and one that reported success but no
// thread id -- were untouched by any test before this: neither TopicReady
// condition it writes had ever been asserted.
func TestTryFinishEnsureTopicRecordsAnAdapterError(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	err := q.finishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{Error: "rate limited"})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err=%v, want an error naming the adapter's own message", err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(got.Status.Conditions, ConditionTopicReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "AdapterError" {
		t.Fatalf("condition = %+v, want False/AdapterError", cond)
	}
	if !strings.Contains(cond.Message, "rate limited") {
		t.Fatalf("message = %q, want the adapter's reason", cond.Message)
	}
}

func TestTryFinishEnsureTopicRecordsAMissingThreadID(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	err := q.finishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{})
	if err == nil || !strings.Contains(err.Error(), "without a thread id") {
		t.Fatalf("err=%v, want an error naming the missing thread id", err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(got.Status.Conditions, ConditionTopicReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "AdapterError" {
		t.Fatalf("condition = %+v, want False/AdapterError", cond)
	}
}

// A brand new thread — no binding at all for the channel yet — is appended
// rather than updated in place. This is distinct from the claim-only
// placeholder case finishEnsureTopic also treats as "new": here there is
// nothing on the conversation for this channel to begin with.
func TestTryFinishEnsureTopicAppendsABindingWhenNoneExistedAtAll(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	err := q.finishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{ThreadID: "t1"})
	if err != nil {
		t.Fatalf("err=%v, want success", err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	binding := got.Status.Thread("c1")
	if binding == nil || binding.ThreadID != "t1" || !binding.ReadTracked {
		t.Fatalf("binding = %+v, want a new tracked thread t1", binding)
	}
	if binding.Claim != nil {
		t.Fatalf("a completed op must not leave a claim behind: %+v", binding.Claim)
	}
}

// A REOPEN: the channel already carries an archived thread, and whatever the
// adapter returns becomes the thread now, with the claim cleared the same way
// the brand-new path does.
func TestTryFinishEnsureTopicReestablishesAnArchivedThread(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "old-thread"}}
	conv.Status.ThreadsArchived = []string{"c1"}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	err := q.finishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{ThreadID: "new-thread"})
	if err != nil {
		t.Fatalf("err=%v, want success", err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	binding := got.Status.Thread("c1")
	if binding == nil || binding.ThreadID != "new-thread" {
		t.Fatalf("binding = %+v, want the re-established thread id", binding)
	}
	if binding.Claim != nil {
		t.Fatalf("a completed reopen must not leave a claim behind: %+v", binding.Claim)
	}
	for _, c := range got.Status.ThreadsArchived {
		if c == "c1" {
			t.Fatalf("a re-established thread must no longer be archived: %+v", got.Status.ThreadsArchived)
		}
	}
}

// markUndelivered is the observable half of a failed run-reply send: the
// binding alone says a thread looks empty for a reason, from the object
// itself.
func TestMarkUndeliveredRecordsTheRunOnTheChannelsBinding(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err != nil {
		t.Fatal(err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if binding := got.Status.Thread("c1"); binding == nil || binding.UndeliveredReply != "run-1" {
		t.Fatalf("binding = %+v, want UndeliveredReply = run-1", binding)
	}
}

// No binding for the channel yet means there is nothing to mark against —
// markUndelivered must not invent one, unlike setClaim.
func TestMarkUndeliveredIsANoOpWithNoBindingYet(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err != nil {
		t.Fatal(err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Threads) != 0 {
		t.Fatalf("no binding must be created: %+v", got.Status.Threads)
	}
}

// A conversation gone by the time the mark is attempted (deleted between the
// send and its completion) is tolerated, exactly as markThreadArchived
// tolerates it.
func TestMarkUndeliveredToleratesAMissingConversation(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err != nil {
		t.Fatalf("a missing conversation must not error: %v", err)
	}
}

// A non-conflict error writing the undelivered marker is returned rather
// than retried — and Complete only logs it, since Complete's own res.Error
// branch has already decided to Fail the bus task regardless.
func TestMarkUndeliveredReturnsANonConflictPatchError(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1"}}
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
				obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return boom
			},
		}).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry(), Bus: testBus()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err == nil || !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the patch error returned, not retried", err)
	}

	// Complete must not panic when the mark itself fails -- the bus task is
	// Failed regardless, from Complete's own res.Error branch.
	ch := testChannel("c1", "slack")
	q.EnqueueRunReply(context.Background(), ch, crRec(testConv("conv-1")), "run-1", nil, AnswerMessage("a", "s"))
	q.Claim("slack")
	q.Complete(context.Background(), op.ID, OpResult{Error: "sendMessage: Too Many Requests"})
}

// Every retry conflicting is the abandoned-after-5-attempts case: markUndelivered
// gives up and names both the run and the channel.
func TestMarkUndeliveredGivesUpAfterRepeatedConflicts(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
				obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return apierrors.NewConflict(schema.GroupResource{Resource: "conversations"}, "conv-1", errors.New("stale"))
			},
		}).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	err := q.markUndelivered(context.Background(), op)
	if err == nil || !strings.Contains(err.Error(), "conflict") || !strings.Contains(err.Error(), "run-1") {
		t.Fatalf("err = %v, want a conflict error naming the run", err)
	}
}

func TestEnsureTopicDedupsByConversation(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueEnsureTopic(ctx, ch, crRec(testConv("conv-1")), TopicDescriptor{})
	q.EnqueueEnsureTopic(ctx, ch, crRec(testConv("conv-1")), TopicDescriptor{}) // reconcile-driven repeat
	if op := q.Claim("slack"); op == nil || op.ID != "topic:conv-1:uid-conv-1:c1" || op.Kind != OpEnsureTopic {
		t.Fatalf("first claim: %+v", op)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("duplicate ensure-topic not deduped: %+v", op)
	}
	// still deduped while claimed
	q.EnqueueEnsureTopic(ctx, ch, crRec(testConv("conv-1")), TopicDescriptor{})
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("re-enqueue while claimed must dedup: %+v", op)
	}
}

// AllocateName reuses a name once its holder is deleted (namewords.go), so a
// brand-new Conversation object can share an exact name with one whose
// ensure-topic already completed in this same manager process. Measured
// live: the smoke suite's "chat-echo-console" recreated across two
// sequential tests never got a thread id, because a name-derived-only id
// collided with the first conversation's entry in the completed-ops dedup
// window, silently dropping the second's own enqueue forever.
//
// The fix is structural — the id embeds conv.ID (the object's own UID), so
// a reused NAME under a different UID is never the same CommandID to begin
// with — this test pins that by marking the first id settled directly
// (the local mirror is whitebox-writable, same package, mirroring the old
// test's own direct `q.recent[...] = true`) and showing the second
// conversation's own, structurally distinct id is unaffected either way.
func TestEnsureTopicDoesNotDedupAcrossDifferentConversationsSharingAName(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	first := testConv("chat-echo-console")
	q.EnqueueEnsureTopic(ctx, ch, crRec(first), TopicDescriptor{})
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("first conversation's ensure-topic never queued")
	}
	// Settle the UNDERLYING bus task, not just the local mirror: the first
	// and second conversation share the SAME Key (conversation NAME ×
	// channel — outKey does not know about conv.ID), so a task left
	// Running here would block the second one's claim regardless of its
	// own, structurally distinct CommandID (design D-2's per-key
	// serialization, same queue).
	settleClaimed(t, q, op.ID)

	// A second, distinct object (different UID) reuses the exact same name.
	second := testConv("chat-echo-console")
	second.UID = types.UID("uid-chat-echo-console-reborn")
	q.EnqueueEnsureTopic(ctx, ch, crRec(second), TopicDescriptor{})
	if op := q.Claim("slack"); op == nil {
		t.Fatal("a conversation reusing a deleted one's name must still get its own ensure-topic queued")
	}
}

// Both "one" and "two" are general-surface notices (crRec(nil)) on the SAME
// channel, so they share one Key — design D-2's per-key serialization means
// "two" is not claimable until "one" settles, unlike the old in-memory
// queue's single FIFO list with no such rule. Completing between claims is
// what proves FIFO order now, not a pair of claims held open at once.
func TestClaimIsFIFOAndTypeScoped(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	slack := testChannel("s", "slack")
	teams := testChannel("m", "teams")
	q.EnqueueMessage(ctx, slack, crRec(nil), nil, Notice("one"))
	q.EnqueueMessage(ctx, slack, crRec(nil), nil, Notice("two"))
	q.EnqueueMessage(ctx, teams, crRec(nil), nil, Notice("other"))

	first := q.Claim("slack")
	if first == nil || first.Message.Body != "one" {
		t.Fatalf("fifo: %+v", first)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("a second op sharing the first's Key must not be claimable before it settles: %+v", op)
	}
	q.Complete(ctx, first.ID, OpResult{})

	second := q.Claim("slack")
	if second == nil || second.Message.Body != "two" {
		t.Fatalf("fifo second: %+v", second)
	}
	q.Complete(ctx, second.ID, OpResult{})

	if op := q.Claim("slack"); op != nil {
		t.Fatalf("teams op leaked into slack: %+v", op)
	}
	if op := q.Claim("teams"); op == nil || op.Message.Body != "other" {
		t.Fatalf("teams claim: %+v", op)
	}
}

// Claim must return promptly when nothing is claimable — handleChannelOps
// (server.go) already owns the long-poll wait, calling Claim once a
// second; a Claim that itself blocked for any real duration would stack on
// top of that wait instead of standing in for it (design D-2 moved the
// lease-lapse scan the old in-memory queue hand-rolled into the bus
// itself, which is what makes a short, bounded Claim correct now).
func TestClaimIsNonBlockingOnAnEmptyQueue(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	start := time.Now()
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("empty queue claimed something: %+v", op)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Claim on an empty queue took %v, want well under a second", elapsed)
	}
}

// The bus's own lease now owns reclaim-after-timeout (design D-2) — the old
// in-memory queue hand-rolled a scan over its own claimedAt field, which
// this rewrite deletes outright. A lapsed lease is simulated by fast-
// forwarding the Backing's clock past its own LeaseDuration, never by
// poking a local field that no longer has anything to do with reclaim.
func TestReclaimAfterLeaseLapse(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ctx := context.Background()
	// A STABLE op, not a plain EnqueueMessage: out.ephemeral's own 2-minute
	// TTL (design D-5) is shorter than the default 5-minute lease, so an
	// ephemeral op left claimed that long EXPIRES on the very reap that
	// would otherwise return it to Retrying (reapLocked applies both
	// checks in sequence against the one "now"), rather than ever coming
	// back for reclaim. out.stable declares no TTL, which is what this
	// test needs to isolate the lease-lapse mechanism from TTL expiry.
	q.EnqueueCloseTopic(ctx, testChannel("s", "slack"), "9876", crRec(testConv("conv-1")))
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("claim")
	}
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("claimed op must not be claimable: %+v", again)
	}
	// TWO rounds, not one: the bus reaps a lapsed lease lazily, on the next
	// call that touches the store, and schedules its OWN retry backoff
	// measured from THAT call's clock — not from the moment it actually
	// lapsed. So the first Claim after advancing past the lease merely
	// turns Running into Retrying-with-a-fresh-backoff; a second advance
	// (and a second Claim) is what actually crosses it.
	backing.Advance(backing.LeaseDuration() + time.Second)
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("want the first post-lapse claim to still be behind its own fresh backoff: %+v", again)
	}
	backing.Advance(6 * time.Minute) // past out.stable's own 5s-5m backoff ceiling
	if again := q.Claim("slack"); again == nil || again.ID != op.ID {
		t.Fatalf("lapsed lease not reclaimed: %+v", again)
	}
}

func TestCompleteIsIdempotentForSends(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	q.EnqueueMessage(ctx, testChannel("s", "slack"), nil, nil, Notice("hello"))
	op := q.Claim("slack")
	q.Complete(ctx, op.ID, OpResult{})
	if q.Pending(op.ID) {
		t.Fatal("op still pending after complete")
	}
	q.Complete(ctx, op.ID, OpResult{}) // duplicate done — must not panic or resurrect
	q.Complete(ctx, "never-existed", OpResult{})
	if q.Pending(op.ID) {
		t.Fatal("duplicate complete resurrected op")
	}
}

// A run reply is DERIVED, so its id must round-trip: completion maps the op
// back to the run it delivers with nothing but the id.
func TestRunReplyOpIDRoundTrips(t *testing.T) {
	id := RunReplyOpID("conv-1", "chan-a", "run-7")
	conv, channel, run, ok := ParseRunReplyOpID(id)
	if !ok || conv != "conv-1" || channel != "chan-a" || run != "run-7" {
		t.Fatalf("round trip: %q -> %q/%q/%q ok=%v", id, conv, channel, run, ok)
	}
	// A run id containing ':' keeps its tail — object names cannot, so the
	// REMAINDER is the run rather than the third field.
	if _, _, run, _ := ParseRunReplyOpID(RunReplyOpID("c", "ch", "a:b")); run != "a:b" {
		t.Fatalf("run id with a colon lost its tail: %q", run)
	}
	// Ephemeral sends share the prefix and must NOT be mistaken for replies —
	// marking one delivered would attribute an ack to a run.
	for _, other := range []string{"send:1a", "topic:conv-1:chan-a", "input:conv-1:in-1:chan-a", ""} {
		if _, _, _, ok := ParseRunReplyOpID(other); ok {
			t.Fatalf("%q must not parse as a run reply", other)
		}
	}
}

// Unlike an ack, a reply is re-derived on every reconcile: it must dedup while
// pending and after completion, or a conversation reposts its answer forever.
func TestRunReplyDedupsWhilePendingAndAfterCompletion(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	msg := AnswerMessage("done", "succeeded")

	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg)
	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg) // reconcile-driven repeat
	op := q.Claim("slack")
	if op == nil || op.ID != "send:conv-1:c1:run-1" {
		t.Fatalf("first claim: %+v", op)
	}
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("duplicate reply not deduped: %+v", dup)
	}
	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg) // re-enqueue while claimed
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("re-enqueue while claimed must dedup: %+v", dup)
	}
	// An ack for the same conversation is EPHEMERAL and must still get through.
	q.EnqueueMessage(ctx, ch, crRec(nil), &tid, Notice("working on it"))
	if ack := q.Claim("slack"); ack == nil || ack.Message.Body != "working on it" {
		t.Fatalf("an ack must never be suppressed by reply dedup: %+v", ack)
	}
}

func TestCloseTopicOpIsEnqueuedPerChannelAndSettlesOnce(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	slack := testChannel("c1", "slack")
	teams := testChannel("c2", "teams")

	q.EnqueueCloseTopic(ctx, slack, "9876", crRec(testConv("conv-1")))
	q.EnqueueCloseTopic(ctx, teams, "T-42", crRec(testConv("conv-1")))
	// the finalizer re-derives the op on every pass until it lets go
	q.EnqueueCloseTopic(ctx, slack, "9876", crRec(testConv("conv-1")))

	op := q.Claim("slack")
	if op == nil || op.ID != CloseTopicOpID("conv-1", "c1") || op.Kind != OpCloseTopic {
		t.Fatalf("close-topic claim: %+v", op)
	}
	if op.ThreadID == nil || *op.ThreadID != "9876" || op.Conversation != "conv-1" {
		t.Fatalf("close-topic payload: %+v", op)
	}
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("re-enqueue while pending must dedup: %+v", dup)
	}
	// delivery is by adapter name: the sibling channel's op is its own
	if other := q.Claim("teams"); other == nil || other.ID != CloseTopicOpID("conv-1", "c2") {
		t.Fatalf("sibling channel op: %+v", other)
	}

	// empty completion body == archived
	q.Complete(ctx, op.ID, OpResult{})
	if q.Pending(op.ID) {
		t.Fatal("close-topic still pending after completion")
	}
	if !q.Settled(op.ID) {
		t.Fatal("completed close-topic must report settled — the finalizer reads this")
	}
	// and it is NOT regenerated: a completed close-topic stays completed
	// (within the dedup window)
	q.EnqueueCloseTopic(ctx, slack, "9876", crRec(testConv("conv-1")))
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("completed close-topic was regenerated: %+v", again)
	}
}

// A failed close-topic is now RETRIED BY THE BUS, with backoff (design D-5:
// out.stable's 8 attempts, 5s→5m jittered) — the reverse of what this test
// used to pin against the old queue's own one-shot "release the dedup entry
// and hope the next reconcile pass re-derives it, right now" behaviour. The
// reversal is the point of task 6: the bus's attempts+backoff is what
// replaces that hand-rolled release (see Complete's own doc comment, and
// the 2026-08-13 incident it names). Advancing the clock past the backoff
// is what makes the SAME task claimable again — a re-enqueue alone would
// not, since the bus itself is now the thing deciding when a retry is due.
func TestFailedCloseTopicIsReDerivable(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueCloseTopic(ctx, ch, "9876", crRec(testConv("conv-1")))
	op := q.Claim("slack")
	q.Complete(ctx, op.ID, OpResult{Error: "closeForumTopic: chat not found"})
	if !q.Pending(op.ID) {
		t.Fatal("a failed close-topic with attempts remaining must still be pending (retrying)")
	}
	if q.Settled(op.ID) {
		t.Fatal("a retrying close-topic must not report settled yet")
	}

	backing.Advance(6 * time.Minute) // past out.stable's own 5s-5m backoff ceiling
	q.EnqueueCloseTopic(ctx, ch, "9876", crRec(testConv("conv-1"))) // the reconciler's own re-derive
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed close-topic must be re-derivable once its backoff elapses: the thread is still owed an archive")
	}
	if again.ID != CloseTopicOpID("conv-1", "c1") {
		t.Fatalf("re-derived op must keep the stable id: %+v", again)
	}
}

type fakeProvider struct {
	mu    sync.Mutex
	sends []string
}

func (f *fakeProvider) EnsureTopic(ctx context.Context, topic TopicDescriptor) (string, error) {
	return "T-1", nil
}

// An in-process provider RENDERS like any adapter — it receives the semantic
// message, not text — so this fake records what it would have composed.
func (f *fakeProvider) Send(ctx context.Context, threadID *string, msg Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, msg.Body)
	return nil
}

// In-process delivery is now a real consumer.Pool, started by Run (design
// D-10's "in-process providers become a consumer consuming out.<type>"),
// never a one-shot goroutine fired from inside enqueue — so this test
// drives Run itself, in the background, for the fake registered type.
func TestInProcessProviderServesRegisteredType(t *testing.T) {
	reg := NewRegistry()
	fake := &fakeProvider{}
	reg.Register("web", func(ctx context.Context, channelName string) (Provider, error) { return fake, nil })
	q := &OpQueue{Registry: reg, Bus: testBus()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = q.Run(ctx) }()

	q.EnqueueMessage(context.Background(), testChannel("w", "web"), nil, nil, Notice("hi there"))

	deadline := time.Now().Add(3 * time.Second)
	for {
		fake.mu.Lock()
		n := len(fake.sends)
		fake.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("in-process provider never received the send")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if op := q.Claim("web"); op != nil {
		t.Fatalf("in-process op leaked to the external adapter queue: %+v", op)
	}
	cancel()
	<-done
}

// A close that is RETRIED must say goodbye once, not once per attempt.
//
// Found by a live smoke: the CRD in that cluster did not yet allow phase
// `Closed`, so every close attempt posted its farewell and then failed its
// status write, and the reconciler retried for seven hours. Four farewells
// landed in one thread. The farewell still goes first — a thread that stops
// with no word is indistinguishable from a fault — so the fix is a stable id.
func TestFarewellIsPostedOncePerClose(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	conv := testConv("conv-1")
	tid := "9876"

	for i := 0; i < 3; i++ {
		q.EnqueueFarewell(ctx, ch, crRec(conv), &tid, Notice("goodbye"))
	}
	ops := drainOps(q, "slack")
	if len(ops) != 1 {
		t.Fatalf("a retried close must post ONE farewell, got %d", len(ops))
	}
	// Complete it: design D-2 blocks the NEXT op sharing this conversation×
	// channel Key (the reopen's own farewell, below) until this one settles.
	for _, op := range ops {
		q.Complete(ctx, op.ID, OpResult{})
	}

	// ...but a conversation closed, REOPENED and closed again is owed a second.
	conv.Status.Reopens = 1
	q.EnqueueFarewell(ctx, ch, crRec(conv), &tid, Notice("goodbye again"))
	if ops := drainOps(q, "slack"); len(ops) != 1 {
		t.Fatalf("a close after a reopen must post its own farewell, got %d", len(ops))
	}
}

// settleClaimed Settles id's underlying bus task directly, bypassing
// Complete's own side-effect handlers — for a test that only needs the Key
// this op occupies released (design D-2's per-key serialization: an
// unsettled task blocks every later one sharing its Key, regardless of
// Kind), not the full Complete behaviour (which needs a wired
// Conversations store for ensure-topic/close-topic/run-reply kinds).
func settleClaimed(t *testing.T, q *OpQueue, id string) {
	t.Helper()
	q.mu.Lock()
	entry, ok := q.claimed[id]
	if ok {
		delete(q.claimed, id)
	}
	q.mu.Unlock()
	if !ok {
		t.Fatalf("settleClaimed: %q is not currently claimed", id)
	}
	if err := q.Bus.Settle(context.Background(), entry.uid, entry.token); err != nil {
		t.Fatalf("settleClaimed: %v", err)
	}
	q.markFromState(id, commandbus.StateSucceeded)
}

func drainOps(q *OpQueue, adapter string) []*Op {
	var out []*Op
	for {
		op := q.Claim(adapter)
		if op == nil {
			return out
		}
		out = append(out, op)
	}
}

// THE 2026-08-13 REGRESSION, IN ITS NEW SHAPE. The old queue had no retry of
// its own, so a rejected send released its stable id by hand so the NEXT
// reconcile pass could re-derive it; this rewrite replaces that release with
// the bus's own attempts+backoff (design D-5, out.stable: 8 attempts, 5s→5m
// jittered) — a failed send is RETRIED, not silently dropped, and the
// answer in status.runs[].result is never stranded either way. Advancing
// the clock is what stands in for "the backoff elapsed", the same way it
// does for close-topic above.
func TestFailedSendIsReDerivable(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	msg := AnswerMessage("the answer", "succeeded")

	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no reply queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})
	if !q.Pending(op.ID) {
		t.Fatal("a failed send with attempts remaining must still be pending (retrying), not dropped")
	}

	backing.Advance(6 * time.Minute)
	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg) // the reconciler backstop
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed send must be re-derivable once its backoff elapses: the answer is still owed to the thread")
	}
	if again.ID != op.ID {
		t.Fatalf("re-derived reply must keep the stable id: %s != %s", again.ID, op.ID)
	}

	// ...and a SUCCESSFUL one still dedups, or the backstop would repost every
	// answer on every reconcile.
	q.Complete(ctx, again.ID, OpResult{})
	q.EnqueueRunReply(ctx, ch, crRec(testConv("conv-1")), "run-1", &tid, msg)
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("a delivered reply must stay deduped: %+v", dup)
	}
}

// The same rule on the card path, pinned independently: a card and a reply are
// both stable sends, but they are derived by different passes, and one covering
// the other is exactly the assumption that let this bug hide.
func TestFailedInputCardIsReDerivable(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	card := SignalMessage("k8s-ops", "cluster-events", "PodCrashLooping", "i1", nil, "the alert body", agentopsv1alpha1.OriginSignal)

	q.EnqueueInputDelivery(ctx, ch, crRec(testConv("conv-1")), "i1", &tid, card)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no card queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})

	backing.Advance(6 * time.Minute)
	q.EnqueueInputDelivery(ctx, ch, crRec(testConv("conv-1")), "i1", &tid, card)
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed input card must be re-derivable: the thread never opened with its event")
	}
	if again.ID != op.ID {
		t.Fatalf("re-derived card must keep the stable id: %s != %s", again.ID, op.ID)
	}
}

// delete-conversation is declared NEVER PARKED (design D-5) — a deliberate
// reversal of the old queue's own "stays suppressed forever after one
// failure" behaviour (there was no object left to re-derive from, so a
// permanent release-once-and-done was the only safety net available). The
// bus retries it 3 times with a fixed 5s backoff; once genuinely Dead, the
// zero park cool-off means the VERY NEXT re-derive is immediately
// resubmittable — the finalizer's own grace period, not this op's dedup
// state, is what eventually gives up (OpDeleteConversation's own doc
// comment).
func TestFailedDeleteConversationRetriesThenResubmitsImmediatelyOnceDead(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	q.EnqueueDeleteConversation(ctx, ch, "9876", crRec(testConv("conv-1")))
	for i := 0; i < 3; i++ {
		op := q.Claim("slack")
		if op == nil {
			t.Fatalf("attempt %d: no delete-conversation queued", i+1)
		}
		q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: chat not found"})
		backing.Advance(6 * time.Second) // past the fixed 5s backoff
		q.EnqueueDeleteConversation(ctx, ch, "9876", crRec(testConv("conv-1")))
	}
	// Three attempts exhausted: the task is now Dead. Never parked, so the
	// re-derive inside the loop's own last iteration already produced a
	// fresh, immediately-claimable task.
	if again := q.Claim("slack"); again == nil {
		t.Fatal("a dead delete-conversation (never parked) must be immediately resubmittable")
	}
}

// Every op leaving the queue for a known conversation carries that
// conversation's own conversationId BESIDE its name (channel-adapter-contract
// OPS-9) — additive, so a conformant adapter that does not recognise the
// field keeps working. Exercises the three distinct call shapes: a method
// that already took the whole Conversation (EnsureTopic), one changed from a
// bare name to the Conversation (CloseTopic), and EnqueueMessage's own nilable
// conv, which must stay empty with no conversation to source it from.
func TestOpsCarryTheOwningConversationID(t *testing.T) {
	ctx := context.Background()
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ch := testChannel("c1", "slack")

	withID := testConv("conv-with-id")
	withID.Spec.ConversationID = "corr-77"

	// Every one of the next three ops shares conv-with-id × c1 as its Key
	// (design D-2: Key is conversation × channel, the SAME for every op
	// kind on that pair) — each must settle before the next is claimable.
	q.EnqueueEnsureTopic(ctx, ch, crRec(withID), TopicDescriptor{Source: "vm-alerts", Kind: "alert"})
	op := q.Claim("slack")
	if op == nil || op.ConversationID != "corr-77" {
		t.Fatalf("ensure-topic must carry the conversationId: %+v", op)
	}
	settleClaimed(t, q, op.ID)

	tid := "t-1"
	q.EnqueueCloseTopic(ctx, ch, tid, crRec(withID))
	op = q.Claim("slack")
	if op == nil || op.ConversationID != "corr-77" || op.Conversation != "conv-with-id" {
		t.Fatalf("close-topic must carry both the name and the conversationId: %+v", op)
	}
	settleClaimed(t, q, op.ID)

	q.EnqueueMessage(ctx, ch, crRec(withID), &tid, Notice("hello"))
	if op := q.Claim("slack"); op == nil || op.ConversationID != "corr-77" {
		t.Fatalf("a message bound to a conversation must carry its conversationId: %+v", op)
	}

	// no conversationId to adopt — legacy conversation, never backfilled
	legacy := testConv("conv-legacy")
	q.EnqueueEnsureTopic(ctx, ch, crRec(legacy), TopicDescriptor{Source: "vm-alerts", Kind: "alert"})
	if op := q.Claim("slack"); op == nil || op.ConversationID != "" {
		t.Fatalf("a legacy conversation's op must carry no conversationId: %+v", op)
	}

	// no conversation at all — a general-surface notice
	q.EnqueueMessage(ctx, ch, crRec(nil), nil, Notice("unwired source dropped a signal"))
	if op := q.Claim("slack"); op == nil || op.ConversationID != "" || op.Conversation != "" {
		t.Fatalf("a conversation-less notice must carry neither name nor conversationId: %+v", op)
	}
}

// Complete decides Settle vs Fail from TWO independent facts (design D-10):
// the adapter's own res.Error, AND whatever the synchronous side-effect
// handler itself reports. Here the adapter's call SUCCEEDS (res.Error == ""
// and a thread id comes back) but the manager's own Kubernetes write fails —
// the task must still be Failed (so the bus retries it), never silently
// Settled, or the thread binding this op exists to create is lost for good.
func TestCompleteFailsTheTaskWhenTheHandlerWriteFailsDespiteAdapterSuccess(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, cli client.Client, subResourceName string,
				obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return boom
			},
		}).Build()
	q := &OpQueue{Conversations: newTestStore(c), Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	q.EnqueueEnsureTopic(ctx, ch, crRec(conv), TopicDescriptor{})
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no ensure-topic queued")
	}
	q.Complete(ctx, op.ID, OpResult{ThreadID: "t1"}) // the adapter succeeded

	if !q.Pending(op.ID) {
		t.Fatal("a handler-write failure must Fail the task (still pending/retrying), not Settle it")
	}
	if q.Settled(op.ID) {
		t.Fatal("a handler-write failure must not report settled")
	}
}

// Stats() aggregates over the three out.<type> kinds and reports WHICH op
// and WHICH conversation is oldest in each of the queued/claimed buckets —
// the identities commandbus.QueueStats gained for exactly this (task 6's
// own extension to internal/commandbus/stats.go).
func TestStatsReportsPerAdapterIdentities(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	// Different conversations -> different Keys, so both can be queued/
	// claimed independently rather than serializing behind one another.
	// conv-claimed's op is submitted FIRST, so the one Claim call below
	// (oldest-first ordering, design D-2) deterministically takes it,
	// leaving conv-queued's op the only one still sitting in the queue.
	q.EnqueueMessage(ctx, ch, crRec(testConv("conv-claimed")), nil, Notice("claimed"))
	q.EnqueueMessage(ctx, ch, crRec(testConv("conv-queued")), nil, Notice("queued"))

	claimed := q.Claim("slack")
	if claimed == nil || claimed.Conversation != "conv-claimed" {
		t.Fatalf("expected to claim conv-claimed's op (submitted first): %+v", claimed)
	}

	stats := q.Stats()
	var st *QueueStat
	for i := range stats {
		if stats[i].Adapter == "slack" {
			st = &stats[i]
		}
	}
	if st == nil {
		t.Fatalf("no stats for slack: %+v", stats)
	}
	if st.Queued != 1 || st.Claimed != 1 {
		t.Fatalf("Queued/Claimed = %d/%d, want 1/1: %+v", st.Queued, st.Claimed, st)
	}
	if st.OldestQueuedConv != "conv-queued" {
		t.Fatalf("OldestQueuedConv = %q, want conv-queued: %+v", st.OldestQueuedConv, st)
	}
	if st.OldestClaimedConv != "conv-claimed" || st.OldestClaimedOpID != claimed.ID {
		t.Fatalf("OldestClaimedConv/OpID = %q/%q, want conv-claimed/%q: %+v",
			st.OldestClaimedConv, st.OldestClaimedOpID, claimed.ID, st)
	}
}

// An adapter with nothing outstanding is simply absent from Stats() — the
// old in-memory map read the same way, and a reader of /api/queues should
// not have to tell "quiet" from "unheard of" apart.
func TestStatsOmitsAnAdapterWithNothingOutstanding(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	q.EnqueueMessage(ctx, testChannel("s", "slack"), nil, nil, Notice("hi"))
	op := q.Claim("slack")
	q.Complete(ctx, op.ID, OpResult{})

	for _, st := range q.Stats() {
		if st.Adapter == "slack" {
			t.Fatalf("a settled adapter with nothing outstanding must be absent: %+v", st)
		}
	}
}
