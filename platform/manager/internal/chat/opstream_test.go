package chat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The console's ops are read from a retained feed (design D-16): no claim, no
// lease, no acknowledgement, a cursor of command ids.

func newFeedQueue() *OpQueue {
	return &OpQueue{Registry: NewRegistry(), Bus: testBus(), FeedAdapters: []string{"console"}}
}

func nextOp(t *testing.T, ch <-chan *Op) *Op {
	t.Helper()
	select {
	case op, ok := <-ch:
		if !ok {
			t.Fatal("feed closed")
		}
		return op
	case <-time.After(5 * time.Second):
		t.Fatal("no op within 5s")
	}
	return nil
}

func bodies(t *testing.T, ch <-chan *Op, n int) []string {
	t.Helper()
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, nextOp(t, ch).Message.Body)
	}
	return out
}

func enqueueNotices(q *OpQueue, n int) []string {
	var ids []string
	for i := 0; i < n; i++ {
		conv := crRec(testConv(fmt.Sprintf("c%d", i)))
		tid := "t"
		q.EnqueueInputDelivery(context.Background(), testChannel("ui", "console"), conv, "in", &tid, Notice(fmt.Sprint(i)))
		ids = append(ids, InputOpID(conv.Name, "in", "ui"))
	}
	return ids
}

func TestFeedReplaysAfterACursorInLogOrderThenPushes(t *testing.T) {
	q := newFeedQueue()
	ids := enqueueNotices(q, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := q.OpenFeed(ctx, "console", ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(bodies(t, ch, 2)); got != "[2 3]" {
		t.Fatalf("replay after the cursor = %s, want [2 3]", got)
	}
	tid := "t"
	q.EnqueueInputDelivery(ctx, testChannel("ui", "console"), crRec(testConv("later")), "in", &tid, Notice("4"))
	if got := nextOp(t, ch).Message.Body; got != "4" {
		t.Fatalf("a new op is pushed after the replay, got %q", got)
	}
}

func TestFeedUnknownCursorReplaysTheRetainedLog(t *testing.T) {
	q := newFeedQueue()
	enqueueNotices(q, 3)
	for _, cursor := range []string{"", "not-an-op"} {
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := q.OpenFeed(ctx, "console", cursor)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(bodies(t, ch, 3)); got != "[0 1 2]" {
			t.Fatalf("cursor %q replays %s, want the whole log", cursor, got)
		}
		cancel()
	}
}

// Two connections, and one that never reads, each leave the others whole: a
// connection holds no claim.
func TestFeedConnectionsStrandNothing(t *testing.T) {
	q := newFeedQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := q.OpenFeed(ctx, "console", ""); err != nil { // dead: never read
		t.Fatal(err)
	}
	a, _ := q.OpenFeed(ctx, "console", "")
	b, _ := q.OpenFeed(ctx, "console", "")
	enqueueNotices(q, 3)
	for name, ch := range map[string]<-chan *Op{"a": a, "b": b} {
		if got := fmt.Sprint(bodies(t, ch, 3)); got != "[0 1 2]" {
			t.Fatalf("connection %s received %s", name, got)
		}
	}
}

// The feed does not touch the long-poll's queue: nothing is claimable.
func TestFeedOpsAreNotClaimable(t *testing.T) {
	q := newFeedQueue()
	enqueueNotices(q, 2)
	if op := q.Claim("console"); op != nil {
		t.Fatalf("a feed op must not be claimable: %+v", op)
	}
}

func TestTelegramQueueKeepsClaimAndAcknowledgement(t *testing.T) {
	q := newFeedQueue()
	ctx := context.Background()
	tid := "t"
	q.EnqueueInputDelivery(ctx, testChannel("tg", "telegram"), crRec(testConv("c1")), "in", &tid, Notice("hi"))
	op := q.Claim("telegram")
	if op == nil {
		t.Fatal("telegram op not claimable")
	}
	if !q.Pending(op.ID) {
		t.Fatal("a claimed op is pending until acknowledged")
	}
	q.Complete(ctx, op.ID, OpResult{})
	if !q.Settled(op.ID) || q.Claim("telegram") != nil {
		t.Fatal("an acknowledged op settles and is not delivered again")
	}
}

func TestFeedEnsureTopicCarriesTheThreadTheManagerAssigned(t *testing.T) {
	q := newFeedQueue()
	conv := crRec(testConv("conv-1"))
	q.EnqueueEnsureTopic(context.Background(), testChannel("ui", "console"), conv, TopicDescriptor{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := q.OpenFeed(ctx, "console", "")
	op := nextOp(t, ch)
	if op.Kind != OpEnsureTopic || op.ThreadID == nil || *op.ThreadID != "console-uid-conv-1" {
		t.Fatalf("ensure-topic must carry console-<uid>, got %+v", op)
	}
}

func TestFeedDeliveryBindsTheThreadOnceAndSettles(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithObjects(conv).Build()
	q := newFeedQueue()
	q.Conversations = newTestStore(c)
	q.EnqueueEnsureTopic(context.Background(), testChannel("ui", "console"), crRec(conv), TopicDescriptor{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := q.OpenFeed(ctx, "console", "")
	op := nextOp(t, ch)
	if !q.Pending(op.ID) {
		t.Fatal("an undelivered op is pending")
	}
	q.Delivered(ctx, op)
	var got agentopsv1alpha1.Conversation
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if b := got.Status.Thread("ui"); b == nil || b.ThreadID != "console-uid-conv-1" {
		t.Fatalf("binding = %+v, want console-uid-conv-1", b)
	}
	if q.Pending(op.ID) || !q.Settled(op.ID) {
		t.Fatal("a delivered op settles")
	}
	// A replay delivers the same frame again: the effect does not repeat.
	got.Status.Threads = nil
	if err := c.Status().Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	q.Delivered(ctx, op)
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Thread("ui") != nil {
		t.Fatal("a replayed entry ran its side effect twice")
	}
}

// A delete's tombstone is durable once appended: the finalizer is never held
// waiting for a console to connect.
func TestFeedDeleteConversationIsNotPending(t *testing.T) {
	q := newFeedQueue()
	q.EnqueueDeleteConversation(context.Background(), testChannel("ui", "console"), "console-x", crRec(testConv("c1")))
	if q.Pending(DeleteConversationOpID("c1", "ui")) {
		t.Fatal("a feed delete-conversation must not be pending")
	}
}

// A close-topic replayed after the conversation was reopened must not archive
// the live thread.
func TestReplayedCloseTopicDoesNotArchiveAReopenedConversation(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "ui", ThreadID: "console-x"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithObjects(conv).Build()
	q := newFeedQueue()
	q.Conversations = newTestStore(c)
	q.EnqueueCloseTopic(context.Background(), testChannel("ui", "console"), "console-x", crRec(conv))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := q.OpenFeed(ctx, "console", "")
	q.Delivered(ctx, nextOp(t, ch)) // the conversation is open: nothing to archive
	var got agentopsv1alpha1.Conversation
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.ThreadsArchived) != 0 {
		t.Fatalf("an open conversation's thread was archived: %v", got.Status.ThreadsArchived)
	}
}
