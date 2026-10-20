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

// tryFinishEnsureTopic's two error branches -- an adapter that reported it
// could not create a topic at all, and one that reported success but no
// thread id -- were untouched by any test before this: neither TopicReady
// condition it writes had ever been asserted.
func TestTryFinishEnsureTopicRecordsAnAdapterError(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	done, err := q.tryFinishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{Error: "rate limited"})
	if !done || err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("done=%v err=%v, want an error naming the adapter's own message", done, err)
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
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	done, err := q.tryFinishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{})
	if !done || err == nil || !strings.Contains(err.Error(), "without a thread id") {
		t.Fatalf("done=%v err=%v, want an error naming the missing thread id", done, err)
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
// placeholder case tryFinishEnsureTopic also treats as "new": here there is
// nothing on the conversation for this channel to begin with.
func TestTryFinishEnsureTopicAppendsABindingWhenNoneExistedAtAll(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	done, err := q.tryFinishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{ThreadID: "t1"})
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want success", done, err)
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
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	done, err := q.tryFinishEnsureTopic(context.Background(),
		&Op{Conversation: "conv-1", Channel: "c1"}, OpResult{ThreadID: "new-thread"})
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want success", done, err)
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
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

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
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

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
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err != nil {
		t.Fatalf("a missing conversation must not error: %v", err)
	}
}

// A non-conflict error writing the undelivered marker is returned rather
// than retried — and Complete only logs it, since the send-failure itself
// was already released for re-derivation regardless.
func TestMarkUndeliveredReturnsANonConflictPatchError(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1"}}
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, cli client.Client, subResourceName string,
				obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return boom
			},
		}).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	if err := q.markUndelivered(context.Background(), op); err == nil || !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the patch error returned, not retried", err)
	}

	// Complete must not panic when the mark itself fails -- the send-failure's
	// own release already happened, so this is purely the observable half.
	ch := testChannel("c1", "slack")
	q.EnqueueRunReply(context.Background(), ch, testConv("conv-1"), "run-1", nil, AnswerMessage("a", "s"))
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
			SubResourcePatch: func(ctx context.Context, cli client.Client, subResourceName string,
				obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return apierrors.NewConflict(schema.GroupResource{Resource: "conversations"}, "conv-1", errors.New("stale"))
			},
		}).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	op := &Op{ID: RunReplyOpID("conv-1", "c1", "run-1"), Conversation: "conv-1", Channel: "c1"}
	err := q.markUndelivered(context.Background(), op)
	if err == nil || !strings.Contains(err.Error(), "conflict") || !strings.Contains(err.Error(), "run-1") {
		t.Fatalf("err = %v, want a conflict error naming the run", err)
	}
}

func TestEnsureTopicDedupsByConversation(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{})
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{}) // reconcile-driven repeat
	if op := q.Claim("slack"); op == nil || op.ID != "topic:conv-1:uid-conv-1:c1" || op.Kind != OpEnsureTopic {
		t.Fatalf("first claim: %+v", op)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("duplicate ensure-topic not deduped: %+v", op)
	}
	// still deduped while claimed
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{})
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
func TestEnsureTopicDoesNotDedupAcrossDifferentConversationsSharingAName(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	first := testConv("chat-echo-console")
	q.EnqueueEnsureTopic(ctx, ch, first, TopicDescriptor{})
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("first conversation's ensure-topic never queued")
	}
	// Marks it settled directly, as Complete would, without needing a fake
	// client wired for tryFinishEnsureTopic's CR patch — irrelevant here.
	q.mu.Lock()
	q.recent[op.ID] = true
	q.mu.Unlock()

	// A second, distinct object (different UID) reuses the exact same name.
	second := testConv("chat-echo-console")
	second.UID = types.UID("uid-chat-echo-console-reborn")
	q.EnqueueEnsureTopic(ctx, ch, second, TopicDescriptor{})
	if op := q.Claim("slack"); op == nil {
		t.Fatal("a conversation reusing a deleted one's name must still get its own ensure-topic queued")
	}
}

func TestClaimIsFIFOAndTypeScoped(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	slack := testChannel("s", "slack")
	teams := testChannel("m", "teams")
	q.EnqueueMessage(ctx, slack, nil, nil, Notice("one"))
	q.EnqueueMessage(ctx, slack, nil, nil, Notice("two"))
	q.EnqueueMessage(ctx, teams, nil, nil, Notice("other"))
	if op := q.Claim("slack"); op == nil || op.Message.Body != "one" {
		t.Fatalf("fifo: %+v", op)
	}
	if op := q.Claim("slack"); op == nil || op.Message.Body != "two" {
		t.Fatalf("fifo second: %+v", op)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("teams op leaked into slack: %+v", op)
	}
	if op := q.Claim("teams"); op == nil || op.Message.Body != "other" {
		t.Fatalf("teams claim: %+v", op)
	}
}

func TestReclaimAfterTimeout(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	q.EnqueueMessage(ctx, testChannel("s", "slack"), nil, nil, Notice("hello"))
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("claim")
	}
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("claimed op must not be claimable: %+v", again)
	}
	// age the claim past ReclaimAfter (dead adapter)
	q.mu.Lock()
	q.ops[op.ID].claimedAt = time.Now().Add(-ReclaimAfter - time.Minute)
	q.mu.Unlock()
	if again := q.Claim("slack"); again == nil || again.ID != op.ID {
		t.Fatalf("timed-out claim not requeued: %+v", again)
	}
}

func TestCompleteIsIdempotentForSends(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
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
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	msg := AnswerMessage("done", "succeeded")

	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg)
	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg) // reconcile-driven repeat
	op := q.Claim("slack")
	if op == nil || op.ID != "send:conv-1:c1:run-1" {
		t.Fatalf("first claim: %+v", op)
	}
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("duplicate reply not deduped: %+v", dup)
	}
	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg) // re-enqueue while claimed
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("re-enqueue while claimed must dedup: %+v", dup)
	}
	// An ack for the same conversation is EPHEMERAL and must still get through.
	q.EnqueueMessage(ctx, ch, nil, &tid, Notice("working on it"))
	if ack := q.Claim("slack"); ack == nil || ack.Message.Body != "working on it" {
		t.Fatalf("an ack must never be suppressed by reply dedup: %+v", ack)
	}
}

func TestCloseTopicOpIsEnqueuedPerChannelAndSettlesOnce(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	slack := testChannel("c1", "slack")
	teams := testChannel("c2", "teams")

	q.EnqueueCloseTopic(ctx, slack, "9876", testConv("conv-1"))
	q.EnqueueCloseTopic(ctx, teams, "T-42", testConv("conv-1"))
	// the finalizer re-derives the op on every pass until it lets go
	q.EnqueueCloseTopic(ctx, slack, "9876", testConv("conv-1"))

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
	q.EnqueueCloseTopic(ctx, slack, "9876", testConv("conv-1"))
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("completed close-topic was regenerated: %+v", again)
	}
}

// A failed close-topic is now RE-DERIVABLE, which is the reverse of what this
// test used to pin — and the reversal is the point of the two-stage lifecycle.
//
// close-topic was the one op not derivable from CR state only because it was
// enqueued while the object was disappearing: there was nothing left to record
// against, so a failure had to be terminal or deletion would wedge. Closing no
// longer deletes, so the object survives, status.threadsArchived records what
// actually got archived, and a failure is simply an archive still owed.
//
// The deleting path keeps its old protection from the finalizer's grace, not
// from this op being terminal.
func TestFailedCloseTopicIsReDerivable(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueCloseTopic(ctx, ch, "9876", testConv("conv-1"))
	op := q.Claim("slack")
	q.Complete(ctx, op.ID, OpResult{Error: "closeForumTopic: chat not found"})
	if q.Pending(op.ID) {
		t.Fatal("failed close-topic must not stay pending")
	}
	q.EnqueueCloseTopic(ctx, ch, "9876", testConv("conv-1"))
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed close-topic must be re-derivable: the thread is still owed an archive")
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

func TestInProcessProviderServesRegisteredType(t *testing.T) {
	reg := NewRegistry()
	fake := &fakeProvider{}
	reg.Register("web", func(ctx context.Context, channelName string) (Provider, error) { return fake, nil })
	q := &OpQueue{Registry: reg}
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
		t.Fatalf("in-process op leaked to adapter queue: %+v", op)
	}
}

// A close that is RETRIED must say goodbye once, not once per attempt.
//
// Found by a live smoke: the CRD in that cluster did not yet allow phase
// `Closed`, so every close attempt posted its farewell and then failed its
// status write, and the reconciler retried for seven hours. Four farewells
// landed in one thread. The farewell still goes first — a thread that stops
// with no word is indistinguishable from a fault — so the fix is a stable id.
func TestFarewellIsPostedOncePerClose(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	conv := testConv("conv-1")
	tid := "9876"

	for i := 0; i < 3; i++ {
		q.EnqueueFarewell(ctx, ch, conv, &tid, Notice("goodbye"))
	}
	if ops := len(drainOps(q, "slack")); ops != 1 {
		t.Fatalf("a retried close must post ONE farewell, got %d", ops)
	}

	// ...but a conversation closed, REOPENED and closed again is owed a second.
	conv.Status.Reopens = 1
	q.EnqueueFarewell(ctx, ch, conv, &tid, Notice("goodbye again"))
	if ops := len(drainOps(q, "slack")); ops != 1 {
		t.Fatalf("a close after a reopen must post its own farewell, got %d", ops)
	}
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

// THE 2026-08-13 REGRESSION TEST. A rejected send left its stable id in the
// completed window, so every reconcile-driven re-derivation deduped against it
// and the answer — already a fact in status.runs[].result — could never be
// enqueued again. No restart helped: the re-derivation was what was suppressed.
func TestFailedSendIsReDerivable(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	msg := AnswerMessage("the answer", "succeeded")

	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no reply queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})
	if q.Pending(op.ID) {
		t.Fatal("a failed send must not stay pending")
	}

	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg) // the reconciler backstop
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed send must be re-derivable: the answer is still owed to the thread")
	}
	if again.ID != op.ID {
		t.Fatalf("re-derived reply must keep the stable id: %s != %s", again.ID, op.ID)
	}

	// ...and a SUCCESSFUL one still dedups, or the backstop would repost every
	// answer on every reconcile.
	q.Complete(ctx, again.ID, OpResult{})
	q.EnqueueRunReply(ctx, ch, testConv("conv-1"), "run-1", &tid, msg)
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("a delivered reply must stay deduped: %+v", dup)
	}
}

// The same rule on the card path, pinned independently: a card and a reply are
// both stable sends, but they are derived by different passes, and one covering
// the other is exactly the assumption that let this bug hide.
func TestFailedInputCardIsReDerivable(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	tid := "t1"
	card := SignalMessage("k8s-ops", "cluster-events", "PodCrashLooping", "i1", nil, "", "the alert body", agentopsv1alpha1.OriginSignal)

	q.EnqueueInputDelivery(ctx, ch, testConv("conv-1"), "i1", &tid, card)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no card queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})

	q.EnqueueInputDelivery(ctx, ch, testConv("conv-1"), "i1", &tid, card)
	again := q.Claim("slack")
	if again == nil {
		t.Fatal("a failed input card must be re-derivable: the thread never opened with its event")
	}
	if again.ID != op.ID {
		t.Fatalf("re-derived card must keep the stable id: %s != %s", again.ID, op.ID)
	}
}

// delete-conversation is the ONE exemption, and for a reason the others lack:
// its Conversation is being deleted, so there is no object to re-derive from
// and no marker to gain. The finalizer's grace releases regardless.
func TestFailedDeleteConversationIsNotReDerivable(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")

	q.EnqueueDeleteConversation(ctx, ch, "9876", testConv("conv-1"))
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no delete-conversation queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: chat not found"})

	q.EnqueueDeleteConversation(ctx, ch, "9876", testConv("conv-1"))
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("a failed delete-conversation is TERMINAL and must stay suppressed: %+v", again)
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
	q := &OpQueue{Registry: NewRegistry()}
	ch := testChannel("c1", "slack")

	withID := testConv("conv-with-id")
	withID.Spec.ConversationID = "corr-77"

	q.EnqueueEnsureTopic(ctx, ch, withID, TopicDescriptor{Source: "vm-alerts", Kind: "alert"})
	if op := q.Claim("slack"); op == nil || op.ConversationID != "corr-77" {
		t.Fatalf("ensure-topic must carry the conversationId: %+v", op)
	}

	tid := "t-1"
	q.EnqueueCloseTopic(ctx, ch, tid, withID)
	if op := q.Claim("slack"); op == nil || op.ConversationID != "corr-77" || op.Conversation != "conv-with-id" {
		t.Fatalf("close-topic must carry both the name and the conversationId: %+v", op)
	}

	q.EnqueueMessage(ctx, ch, withID, &tid, Notice("hello"))
	if op := q.Claim("slack"); op == nil || op.ConversationID != "corr-77" {
		t.Fatalf("a message bound to a conversation must carry its conversationId: %+v", op)
	}

	// no conversationId to adopt — legacy conversation, never backfilled
	legacy := testConv("conv-legacy")
	q.EnqueueEnsureTopic(ctx, ch, legacy, TopicDescriptor{Source: "vm-alerts", Kind: "alert"})
	if op := q.Claim("slack"); op == nil || op.ConversationID != "" {
		t.Fatalf("a legacy conversation's op must carry no conversationId: %+v", op)
	}

	// no conversation at all — a general-surface notice
	q.EnqueueMessage(ctx, ch, nil, nil, Notice("unwired source dropped a signal"))
	if op := q.Claim("slack"); op == nil || op.ConversationID != "" || op.Conversation != "" {
		t.Fatalf("a conversation-less notice must carry neither name nor conversationId: %+v", op)
	}
}
