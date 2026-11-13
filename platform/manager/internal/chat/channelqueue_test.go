package chat

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// claimStuckFirst polls until it holds the stuck channel's first op, which it
// never acknowledges. A live op taken first is acknowledged normally.
func claimStuckFirst(t *testing.T, q *OpQueue, ctx context.Context) *Op {
	t.Helper()
	var held *Op
	for held == nil || held.Channel != "stuck" {
		if held = q.Claim("telegram"); held == nil {
			t.Fatal("the stuck channel's op never became claimable")
		}
		if held.Channel == "live" {
			q.Complete(ctx, held.ID, OpResult{})
		}
	}
	return held
}

// One queue per Channel resource (design D-10): each op travels on
// out.<channel>, ordered by its conversation, however many Channels one
// adapter serves.
func TestEachChannelHasItsOwnQueue(t *testing.T) {
	bus := testBus()
	q := &OpQueue{Registry: NewRegistry(), Bus: bus}
	ctx := context.Background()
	q.EnqueueMessage(ctx, testChannel("home-telegram", "telegram"), crRec(testConv("conv-1")), nil, Notice("hi"))
	q.EnqueueMessage(ctx, testChannel("work-telegram", "telegram"), nil, nil, Notice("general"))

	for queue, key := range map[commandbus.Queue]commandbus.Key{
		"out.home-telegram": "conv-1",
		"out.work-telegram": generalKey,
	} {
		st, err := bus.Stats(ctx, queue, commandbus.KindOutEphemeral)
		if err != nil || st.Pending != 1 || st.OldestPendingKey != key {
			t.Fatalf("%s: stats %+v err %v, want one pending task keyed %q", queue, st, err, key)
		}
	}
	if st, _ := bus.Stats(ctx, "out.telegram", commandbus.KindOutEphemeral); st.Pending != 0 {
		t.Fatalf("nothing may travel on a queue named for the adapter: %+v", st)
	}
}

// Two Channels of one adapter type: one never acknowledges, the other still
// gets its entries, in order, and the adapter's one poll drains both.
func TestAChannelThatNeverAcknowledgesDelaysNoOtherChannelOfTheAdapter(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	stuck, live := testChannel("stuck", "telegram"), testChannel("live", "telegram")
	conv := crRec(testConv("conv-1"))
	tid := "t"

	q.EnqueueInputDelivery(ctx, stuck, conv, "a", &tid, Notice("a"))
	q.EnqueueInputDelivery(ctx, stuck, conv, "b", &tid, Notice("b"))
	for _, id := range []string{"1", "2", "3"} {
		q.EnqueueInputDelivery(ctx, live, conv, id, &tid, Notice(id))
	}

	// The poll takes stuck's first op and its consumer never acknowledges.
	held := claimStuckFirst(t, q, ctx)

	var got []string
	for _, op := range drainCompleting(q, "telegram") {
		if op.Channel == "stuck" {
			t.Fatalf("stuck's second op is serialized behind its unacknowledged first: %+v", op)
		}
		got = append(got, op.ID)
	}
	live3 := []string{InputOpID("conv-1", "1", "live"), InputOpID("conv-1", "2", "live"), InputOpID("conv-1", "3", "live")}
	if len(got) == 0 || got[len(got)-1] != live3[2] {
		t.Fatalf("live's entries must all arrive, got %v", got)
	}
	// In order among themselves, whichever were taken before stuck's claim.
	if !sort.StringsAreSorted(got) {
		t.Fatalf("live's entries out of order: %v", got)
	}
	if !q.Pending(held.ID) {
		t.Fatal("the unacknowledged op must stay outstanding")
	}
}

// The adapter's poll is fair across its channels: a deep backlog on one never
// starves the next.
func TestClaimRotatesAcrossAnAdaptersChannels(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	tid := "t"
	for i, conv := range []string{"conv-a", "conv-b", "conv-c", "conv-d"} {
		for _, ch := range []string{"one", "two"} {
			q.EnqueueInputDelivery(ctx, testChannel(ch, "telegram"), crRec(testConv(conv)), string(rune('0'+i)), &tid, Notice("x"))
		}
	}
	seen := map[string]int{}
	for range 4 { // four polls, none acknowledged
		op := q.Claim("telegram")
		if op == nil {
			t.Fatal("op missing")
		}
		seen[op.Channel]++
	}
	if seen["one"] != 2 || seen["two"] != 2 {
		t.Fatalf("four polls over two equally deep channels must alternate, got %v", seen)
	}
}

// /api/queues stays per ADAPTER: the channels' queues aggregate.
func TestStatsAggregateAnAdaptersChannelQueues(t *testing.T) {
	q := &OpQueue{Registry: NewRegistry(), Bus: testBus()}
	ctx := context.Background()
	q.EnqueueMessage(ctx, testChannel("one", "telegram"), crRec(testConv("conv-1")), nil, Notice("a"))
	q.EnqueueMessage(ctx, testChannel("two", "telegram"), crRec(testConv("conv-2")), nil, Notice("b"))
	q.EnqueueMessage(ctx, testChannel("desk", "console"), crRec(testConv("conv-3")), nil, Notice("c"))

	byAdapter := map[string]QueueStat{}
	for _, st := range q.Stats() {
		byAdapter[st.Adapter] = st
	}
	if len(byAdapter) != 2 || byAdapter["telegram"].Queued != 2 || byAdapter["console"].Queued != 1 {
		t.Fatalf("stats must report per adapter, got %+v", byAdapter)
	}
}

// A replica that never enqueued learns a channel's adapter from the bus feed
// (the queue name gives the channel, the task payload the adapter), so a poll
// reaching it is served.
func TestAReplicaLearnsAChannelFromTheFeed(t *testing.T) {
	bus := testBus()
	producer := &OpQueue{Registry: NewRegistry(), Bus: bus}
	replica := &OpQueue{Registry: NewRegistry(), Bus: bus}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = replica.Run(ctx) }()
	defer func() { cancel(); <-done }()
	time.Sleep(50 * time.Millisecond) // the feed starts from now

	producer.EnqueueMessage(ctx, testChannel("one", "telegram"), crRec(testConv("conv-1")), nil, Notice("hi"))

	deadline := time.Now().Add(3 * time.Second)
	for {
		if op := replica.Claim("telegram"); op != nil {
			if op.Channel != "one" {
				t.Fatalf("claimed %+v", op)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a replica that never enqueued never learned the channel from the feed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ChannelsFor lets a freshly restarted replica claim before any feed event.
func TestClaimUsesTheChannelResolver(t *testing.T) {
	bus := testBus()
	producer := &OpQueue{Registry: NewRegistry(), Bus: bus}
	producer.EnqueueMessage(context.Background(), testChannel("one", "telegram"), crRec(testConv("conv-1")), nil, Notice("hi"))

	fresh := &OpQueue{Registry: NewRegistry(), Bus: bus,
		ChannelsFor: func(_ context.Context, adapter string) []string {
			if adapter == "telegram" {
				return []string{"one"}
			}
			return nil
		}}
	if op := fresh.Claim("telegram"); op == nil || op.Channel != "one" {
		t.Fatalf("claim via the resolver = %+v", op)
	}
	if op := fresh.Claim("console"); op != nil {
		t.Fatalf("another adapter's poll must not see it: %+v", op)
	}
}
