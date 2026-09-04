package chat

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

func TestEnsureTopicDedupsByConversation(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{})
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"), TopicDescriptor{}) // reconcile-driven repeat
	if op := q.Claim("slack"); op == nil || op.ID != "topic:conv-1:c1" || op.Kind != OpEnsureTopic {
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

func TestClaimIsFIFOAndTypeScoped(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	slack := testChannel("s", "slack")
	teams := testChannel("m", "teams")
	q.EnqueueMessage(ctx, slack, nil, Notice("one"))
	q.EnqueueMessage(ctx, slack, nil, Notice("two"))
	q.EnqueueMessage(ctx, teams, nil, Notice("other"))
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
	q.EnqueueMessage(ctx, testChannel("s", "slack"), nil, Notice("hello"))
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
	q.EnqueueMessage(ctx, testChannel("s", "slack"), nil, Notice("hello"))
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

	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg)
	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg) // reconcile-driven repeat
	op := q.Claim("slack")
	if op == nil || op.ID != "send:conv-1:c1:run-1" {
		t.Fatalf("first claim: %+v", op)
	}
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("duplicate reply not deduped: %+v", dup)
	}
	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg) // re-enqueue while claimed
	if dup := q.Claim("slack"); dup != nil {
		t.Fatalf("re-enqueue while claimed must dedup: %+v", dup)
	}
	// An ack for the same conversation is EPHEMERAL and must still get through.
	q.EnqueueMessage(ctx, ch, &tid, Notice("working on it"))
	if ack := q.Claim("slack"); ack == nil || ack.Message.Body != "working on it" {
		t.Fatalf("an ack must never be suppressed by reply dedup: %+v", ack)
	}
}

func TestCloseTopicOpIsEnqueuedPerChannelAndSettlesOnce(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	slack := testChannel("c1", "slack")
	teams := testChannel("c2", "teams")

	q.EnqueueCloseTopic(ctx, slack, "9876", "conv-1")
	q.EnqueueCloseTopic(ctx, teams, "T-42", "conv-1")
	// the finalizer re-derives the op on every pass until it lets go
	q.EnqueueCloseTopic(ctx, slack, "9876", "conv-1")

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
	q.EnqueueCloseTopic(ctx, slack, "9876", "conv-1")
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
	q.EnqueueCloseTopic(ctx, ch, "9876", "conv-1")
	op := q.Claim("slack")
	q.Complete(ctx, op.ID, OpResult{Error: "closeForumTopic: chat not found"})
	if q.Pending(op.ID) {
		t.Fatal("failed close-topic must not stay pending")
	}
	q.EnqueueCloseTopic(ctx, ch, "9876", "conv-1")
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
	q.EnqueueMessage(context.Background(), testChannel("w", "web"), nil, Notice("hi there"))

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

	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no reply queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})
	if q.Pending(op.ID) {
		t.Fatal("a failed send must not stay pending")
	}

	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg) // the reconciler backstop
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
	q.EnqueueRunReply(ctx, ch, "conv-1", "run-1", &tid, msg)
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
	card := SignalMessage("k8s-ops", "cluster-events", "PodCrashLooping", "i1", nil, "the alert body")

	q.EnqueueInputDelivery(ctx, ch, "conv-1", "i1", &tid, card)
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no card queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: Too Many Requests: retry after 30"})

	q.EnqueueInputDelivery(ctx, ch, "conv-1", "i1", &tid, card)
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

	q.EnqueueDeleteConversation(ctx, ch, "9876", "conv-1")
	op := q.Claim("slack")
	if op == nil {
		t.Fatal("no delete-conversation queued")
	}
	q.Complete(ctx, op.ID, OpResult{Error: "sendMessage: chat not found"})

	q.EnqueueDeleteConversation(ctx, ch, "9876", "conv-1")
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("a failed delete-conversation is TERMINAL and must stay suppressed: %+v", again)
	}
}
