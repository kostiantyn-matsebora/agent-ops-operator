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
	ch.Spec.Type = typ
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
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"))
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1")) // reconcile-driven repeat
	if op := q.Claim("slack"); op == nil || op.ID != "topic:conv-1" || op.Kind != OpEnsureTopic {
		t.Fatalf("first claim: %+v", op)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("duplicate ensure-topic not deduped: %+v", op)
	}
	// still deduped while claimed
	q.EnqueueEnsureTopic(ctx, ch, testConv("conv-1"))
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("re-enqueue while claimed must dedup: %+v", op)
	}
}

func TestClaimIsFIFOAndTypeScoped(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	slack := testChannel("s", "slack")
	teams := testChannel("m", "teams")
	q.EnqueueSend(ctx, slack, nil, "one")
	q.EnqueueSend(ctx, slack, nil, "two")
	q.EnqueueSend(ctx, teams, nil, "other")
	if op := q.Claim("slack"); op == nil || op.Text != "one" {
		t.Fatalf("fifo: %+v", op)
	}
	if op := q.Claim("slack"); op == nil || op.Text != "two" {
		t.Fatalf("fifo second: %+v", op)
	}
	if op := q.Claim("slack"); op != nil {
		t.Fatalf("teams op leaked into slack: %+v", op)
	}
	if op := q.Claim("teams"); op == nil || op.Text != "other" {
		t.Fatalf("teams claim: %+v", op)
	}
}

func TestReclaimAfterTimeout(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry()}
	ctx := context.Background()
	q.EnqueueSend(ctx, testChannel("s", "slack"), nil, "hello")
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
	q.EnqueueSend(ctx, testChannel("s", "slack"), nil, "hello")
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

type fakeProvider struct {
	mu    sync.Mutex
	sends []string
}

func (f *fakeProvider) EnsureTopic(ctx context.Context, title string) (string, error) {
	return "T-1", nil
}

func (f *fakeProvider) Send(ctx context.Context, threadID *string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, text)
	return nil
}

func TestInProcessProviderServesRegisteredType(t *testing.T) {
	reg := NewRegistry()
	fake := &fakeProvider{}
	reg.Register("web", func(ctx context.Context, channelName string) (Provider, error) { return fake, nil })
	q := &OpQueue{Registry: reg}
	q.EnqueueSend(context.Background(), testChannel("w", "web"), nil, "hi there")

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
