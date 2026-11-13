package chat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	commandbusmemory "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// The adapter delivery contract (design D-17): read without change, acknowledge
// as hand-over, then done or failed. The console's acknowledgement is its
// receipt, and the manager consumes the message at it.

func newReceiptQueue() *OpQueue {
	return &OpQueue{Registry: NewRegistry(), Bus: testBus(), ReceiptAdapters: []string{"console"}}
}

func enqueueNotices(q *OpQueue, adapter string, n int) []string {
	var ids []string
	for i := 0; i < n; i++ {
		conv := crRec(testConv(fmt.Sprintf("c%d", i)))
		tid := "t"
		q.EnqueueInputDelivery(context.Background(), testChannel("ui", adapter), conv, "in", &tid, Notice(fmt.Sprint(i)))
		ids = append(ids, InputOpID(conv.Name, "in", "ui"))
	}
	return ids
}

func readIDs(q *OpQueue, adapter string) []string {
	var ids []string
	for _, op := range q.Read(context.Background(), adapter, 16) {
		ids = append(ids, op.ID)
	}
	return ids
}

func TestReadChangesNothingAndAReadMessageIsOfferedAgain(t *testing.T) {
	q := newReceiptQueue()
	ids := enqueueNotices(q, "telegram", 3)
	for i := 0; i < 3; i++ {
		if got := fmt.Sprint(readIDs(q, "telegram")); got != fmt.Sprint(ids) {
			t.Fatalf("read %d = %s, want %v in order", i, got, ids)
		}
	}
	// An adapter that died after reading, a new one reads the same.
	restarted := &OpQueue{Registry: NewRegistry(), Bus: q.Bus, ChannelsFor: func(context.Context, string) []string { return []string{"ui"} }}
	if got := fmt.Sprint(readIDs(restarted, "telegram")); got != fmt.Sprint(ids) {
		t.Fatalf("a restarted adapter reads %s, want everything unacknowledged %v", got, ids)
	}
	if q.Pending(ids[0]) != true {
		t.Fatal("an unacknowledged message is pending")
	}
}

func TestAcknowledgedMessageIsNeverSentAgain(t *testing.T) {
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus()}
	ids := enqueueNotices(q, "telegram", 2)
	ctx := context.Background()
	receipt, err := q.Acknowledge(ctx, "telegram", ids[0])
	if err != nil || receipt.TTL <= 0 || receipt.Consumed {
		t.Fatalf("acknowledge = %+v %v, want a delivery TTL and no consumption", receipt, err)
	}
	if got := fmt.Sprint(readIDs(q, "telegram")); got != fmt.Sprint(ids[1:]) {
		t.Fatalf("after the acknowledgement the next read is %s, want only %v", got, ids[1:])
	}
	// Neither a repeated acknowledgement nor time re-sends it.
	if again, err := q.Acknowledge(ctx, "telegram", ids[0]); err != nil || again.TTL <= 0 {
		t.Fatalf("a repeated acknowledgement is idempotent: %+v %v", again, err)
	}
	backing.Advance(time.Minute)
	if got := fmt.Sprint(readIDs(q, "telegram")); got != fmt.Sprint(ids[1:]) {
		t.Fatalf("an acknowledged message was offered again: %s", got)
	}
}

func TestAcknowledgingWhatIsGoneOrNotOfferedIsRefused(t *testing.T) {
	q := newReceiptQueue()
	ctx := context.Background()
	if _, err := q.Acknowledge(ctx, "telegram", "never"); err != ErrOpGone {
		t.Fatalf("unknown message = %v, want ErrOpGone", err)
	}
	conv := crRec(testConv("c1"))
	tid := "t"
	ch := testChannel("tg", "telegram")
	q.EnqueueInputDelivery(ctx, ch, conv, "a", &tid, Notice("1"))
	q.EnqueueInputDelivery(ctx, ch, conv, "b", &tid, Notice("2"))
	// Same conversation: the second waits behind the first.
	if _, err := q.Acknowledge(ctx, "telegram", InputOpID("c1", "b", "tg")); err != ErrOpNotOffered {
		t.Fatalf("a message behind its conversation's unfinished one = %v, want ErrOpNotOffered", err)
	}
}

func TestDoneReportSettlesAndFailedIsFinal(t *testing.T) {
	q := newReceiptQueue()
	ctx := context.Background()
	ids := enqueueNotices(q, "telegram", 2)
	for _, id := range ids {
		if _, err := q.Acknowledge(ctx, "telegram", id); err != nil {
			t.Fatal(err)
		}
	}
	q.Complete(ctx, ids[0], OpResult{})
	if q.Pending(ids[0]) || !q.Settled(ids[0]) {
		t.Fatal("done settles the message")
	}
	q.Complete(ctx, ids[1], OpResult{Error: "the surface refused"})
	if q.Pending(ids[1]) || q.Settled(ids[1]) {
		t.Fatal("failed is final: neither pending nor settled")
	}
	if got := readIDs(q, "telegram"); len(got) != 0 {
		t.Fatalf("a finished message was offered again: %v", got)
	}
}

// A message with no report when its delivery TTL passes is failed by the bus,
// never redelivered, and a late report is ignored.
func TestNoReportByTheDeliveryTTLFailsTheMessage(t *testing.T) {
	backing := commandbusmemory.New()
	bus := backing.Bus()
	q := &OpQueue{Registry: NewRegistry(), Bus: bus}
	ids := enqueueNotices(q, "telegram", 1)
	ctx := context.Background()
	if _, err := q.Acknowledge(ctx, "telegram", ids[0]); err != nil {
		t.Fatal(err)
	}
	backing.Advance(11 * time.Minute) // past the stable kind's 10 minutes
	dead, err := bus.DeadLetters(ctx, "out.ui")
	if err != nil || len(dead) != 1 || dead[0].CommandID != commandbus.CommandID(ids[0]) {
		t.Fatalf("the unreported message is a dead letter: %+v %v", dead, err)
	}
	if got := readIDs(q, "telegram"); len(got) != 0 {
		t.Fatalf("a failed message was offered again: %v", got)
	}
	q.Complete(ctx, ids[0], OpResult{}) // the late report changes nothing
	if q.Settled(ids[0]) {
		t.Fatal("a report after the TTL must not settle a failed message")
	}
}

// An adapter restart after acknowledging does not duplicate: the new process
// reads nothing, and still reports done through the lease the bus holds.
func TestAdapterRestartAfterAckDoesNotDuplicateAndStillReports(t *testing.T) {
	bus := testBus()
	first := &OpQueue{Registry: NewRegistry(), Bus: bus}
	ids := enqueueNotices(first, "telegram", 1)
	ctx := context.Background()
	if _, err := first.Acknowledge(ctx, "telegram", ids[0]); err != nil {
		t.Fatal(err)
	}
	second := &OpQueue{Registry: NewRegistry(), Bus: bus, ChannelsFor: func(context.Context, string) []string { return []string{"ui"} }}
	if got := readIDs(second, "telegram"); len(got) != 0 {
		t.Fatalf("a replica that did not acknowledge offers an acknowledged message: %v", got)
	}
	second.noteChannel("ui", "telegram") // what the adapter's own GET on this replica records
	second.Complete(ctx, ids[0], OpResult{})
	if !second.Settled(ids[0]) {
		t.Fatal("the report reaches the message through another replica")
	}
}

func TestReceiptAdapterConsumesAtTheAcknowledgement(t *testing.T) {
	q := newReceiptQueue()
	ids := enqueueNotices(q, "console", 3)
	ctx := context.Background()
	// A dropped connection reads again and is offered every unacknowledged one.
	readIDs(q, "console")
	if got := fmt.Sprint(readIDs(q, "console")); got != fmt.Sprint(ids) {
		t.Fatalf("reconnect reads %s, want %v", got, ids)
	}
	receipt, err := q.Acknowledge(ctx, "console", ids[0])
	if err != nil || !receipt.Consumed {
		t.Fatalf("a receipt adapter's acknowledgement consumes: %+v %v", receipt, err)
	}
	if q.Pending(ids[0]) || !q.Settled(ids[0]) {
		t.Fatal("a consumed message is settled")
	}
	if got := fmt.Sprint(readIDs(q, "console")); got != fmt.Sprint(ids[1:]) {
		t.Fatalf("after one acknowledgement a reconnect reads %s, want only the unacknowledged %v", got, ids[1:])
	}
	// A second acknowledgement finds it gone: harmless.
	if _, err := q.Acknowledge(ctx, "console", ids[0]); err != ErrOpGone {
		t.Fatalf("a duplicate acknowledgement = %v, want ErrOpGone", err)
	}
	// An ordinary adapter is untouched by the console's setting.
	tgConv := crRec(testConv("tg-conv"))
	tid := "t"
	q.EnqueueInputDelivery(ctx, testChannel("tgch", "telegram"), tgConv, "in", &tid, Notice("x"))
	tg := []string{InputOpID(tgConv.Name, "in", "tgch")}
	if r, err := q.Acknowledge(ctx, "telegram", tg[0]); err != nil || r.Consumed {
		t.Fatalf("telegram keeps the three steps: %+v %v", r, err)
	}
}

func TestReceiptEnsureTopicCarriesTheAssignedThreadAndBindsItAtTheAck(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithObjects(conv).Build()
	q := newReceiptQueue()
	q.Conversations = newTestStore(c)
	ctx := context.Background()
	q.EnqueueEnsureTopic(ctx, testChannel("ui", "console"), crRec(conv), TopicDescriptor{})
	ops := q.Read(ctx, "console", 4)
	if len(ops) != 1 || ops[0].Kind != OpEnsureTopic || ops[0].ThreadID == nil || *ops[0].ThreadID != "console-uid-conv-1" {
		t.Fatalf("ensure-topic must carry the thread the manager assigned: %+v", ops)
	}
	if _, err := q.Acknowledge(ctx, "console", ops[0].ID); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if b := got.Status.Thread("ui"); b == nil || b.ThreadID != "console-uid-conv-1" {
		t.Fatalf("binding = %+v, want console-uid-conv-1", b)
	}
}

// The thread id of an ordinary adapter's ensure-topic comes back in the done
// report, never from the manager.
func TestEnsureTopicThreadComesBackInTheDoneReportForOtherAdapters(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithObjects(conv).Build()
	q := newReceiptQueue()
	q.Conversations = newTestStore(c)
	ctx := context.Background()
	q.EnqueueEnsureTopic(ctx, testChannel("tg", "telegram"), crRec(conv), TopicDescriptor{})
	ops := q.Read(ctx, "telegram", 4)
	if len(ops) != 1 || ops[0].ThreadID != nil {
		t.Fatalf("the manager assigns no thread for telegram: %+v", ops)
	}
	if _, err := q.Acknowledge(ctx, "telegram", ops[0].ID); err != nil {
		t.Fatal(err)
	}
	q.Complete(ctx, ops[0].ID, OpResult{ThreadID: "42"})
	var got agentopsv1alpha1.Conversation
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if b := got.Status.Thread("tg"); b == nil || b.ThreadID != "42" {
		t.Fatalf("binding = %+v, want the reported topic 42", b)
	}
}

// A close-topic acknowledged after the conversation was reopened must not
// archive the live thread.
func TestCloseTopicDoesNotArchiveAReopenedConversation(t *testing.T) {
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "ui", ThreadID: "console-x"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).WithObjects(conv).Build()
	q := newReceiptQueue()
	q.Conversations = newTestStore(c)
	ctx := context.Background()
	q.EnqueueCloseTopic(ctx, testChannel("ui", "console"), "console-x", crRec(conv))
	ops := q.Read(ctx, "console", 4)
	if len(ops) != 1 {
		t.Fatalf("ops = %+v", ops)
	}
	if _, err := q.Acknowledge(ctx, "console", ops[0].ID); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(ctx, client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.ThreadsArchived) != 0 {
		t.Fatalf("an open conversation's thread was archived: %v", got.Status.ThreadsArchived)
	}
}
