package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
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

func TestFailedCloseTopicIsTerminal(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	ch := testChannel("c1", "slack")
	q.EnqueueCloseTopic(ctx, ch, "9876", "conv-1")
	op := q.Claim("slack")
	// an adapter that cannot archive reports it; deletion must still proceed,
	// so the op is not eligible for regeneration the way ensure-topic is
	q.Complete(ctx, op.ID, OpResult{Error: "closeForumTopic: chat not found"})
	if q.Pending(op.ID) {
		t.Fatal("failed close-topic must not stay pending")
	}
	q.EnqueueCloseTopic(ctx, ch, "9876", "conv-1")
	if again := q.Claim("slack"); again != nil {
		t.Fatalf("failed close-topic was regenerated: %+v", again)
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
