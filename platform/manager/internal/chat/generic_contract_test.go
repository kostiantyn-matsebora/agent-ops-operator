package chat

import (
	"context"
	"fmt"
	"testing"
	"time"

	commandbusmemory "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// The get / ack / done-or-failed contract with a delivery TTL is the default for
// EVERY adapter (design D-17). Nothing here is registered, declared or special-
// cased for the made-up adapter: a new channel adapter needs no manager change.
const madeUpAdapter = "zz-never-heard-of-it"

func TestAnyNewAdapterGetsTheFullGenericContractWithNoCodeChange(t *testing.T) {
	ctx := context.Background()
	backing := commandbusmemory.New()
	q := &OpQueue{Registry: NewRegistry(), Bus: backing.Bus(), ReceiptAdapters: []string{"console"}}
	ch := testChannel("zz-chan", madeUpAdapter)
	tid := "t"
	var ids []string
	for i := 0; i < 3; i++ {
		conv := crRec(testConv(fmt.Sprintf("conv-%d", i)))
		q.EnqueueInputDelivery(ctx, ch, conv, "in", &tid, Notice(fmt.Sprint(i)))
		ids = append(ids, InputOpID(conv.Name, "in", "zz-chan"))
	}

	// GET changes nothing, and a reader that died is replaced by one that reads
	// the same messages.
	if got := fmt.Sprint(readIDs(q, madeUpAdapter)); got != fmt.Sprint(ids) {
		t.Fatalf("read = %s, want %v", got, ids)
	}
	reborn := &OpQueue{Registry: NewRegistry(), Bus: q.Bus, ChannelsFor: func(context.Context, string) []string { return []string{"zz-chan"} }}
	if got := fmt.Sprint(readIDs(reborn, madeUpAdapter)); got != fmt.Sprint(ids) {
		t.Fatalf("a new reader after a dead one reads %s, want %v", got, ids)
	}

	// ACK hands over, starts the stable kind's delivery TTL, and is never repeated.
	receipt, err := q.Acknowledge(ctx, madeUpAdapter, ids[0])
	if err != nil || receipt.Consumed || receipt.TTL < 9*time.Minute {
		t.Fatalf("ack = %+v %v, want the stable delivery TTL and no consumption", receipt, err)
	}
	if got := fmt.Sprint(readIDs(q, madeUpAdapter)); got != fmt.Sprint(ids[1:]) {
		t.Fatalf("after the ack the next read is %s, want %v", got, ids[1:])
	}

	// DONE settles; FAILED is final; no report by the TTL is failed by the backstop.
	if _, err := q.Acknowledge(ctx, madeUpAdapter, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Acknowledge(ctx, madeUpAdapter, ids[2]); err != nil {
		t.Fatal(err)
	}
	q.Complete(ctx, ids[1], OpResult{})
	if !q.Settled(ids[1]) {
		t.Fatal("done must settle")
	}
	backing.Advance(11 * time.Minute)
	dead, err := q.Bus.DeadLetters(ctx, "out.zz-chan")
	if err != nil || len(dead) != 2 {
		t.Fatalf("the two unreported messages are failed by the backstop: %v %v", dead, err)
	}
	if got := readIDs(q, madeUpAdapter); len(got) != 0 {
		t.Fatalf("a failed message was sent again: %v", got)
	}
	q.Complete(ctx, ids[0], OpResult{}) // a late report changes nothing
	if q.Settled(ids[0]) {
		t.Fatal("a report after the TTL must not settle a failed message")
	}
}

// The console is an exception by CONFIGURATION alone: the same adapter name with
// no entry in ReceiptAdapters gets the generic contract, and an arbitrary name
// in it gets the receipt behaviour.
func TestTheReceiptExceptionIsPurelyConfigured(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		adapter  string
		receipt  []string
		consumed bool
	}{
		{"console", nil, false},
		{"console", []string{"console"}, true},
		{madeUpAdapter, []string{madeUpAdapter}, true},
		{madeUpAdapter, []string{"console"}, false},
	} {
		q := &OpQueue{Registry: NewRegistry(), Bus: testBus(), ReceiptAdapters: tc.receipt}
		tid := "t"
		conv := crRec(testConv("c1"))
		q.EnqueueInputDelivery(ctx, testChannel("ch", tc.adapter), conv, "in", &tid, Notice("x"))
		r, err := q.Acknowledge(ctx, tc.adapter, InputOpID("c1", "in", "ch"))
		if err != nil || r.Consumed != tc.consumed {
			t.Errorf("adapter %q receipt set %v: ack = %+v %v, want consumed=%v", tc.adapter, tc.receipt, r, err, tc.consumed)
		}
		if tc.consumed != q.Settled(InputOpID("c1", "in", "ch")) {
			t.Errorf("adapter %q receipt set %v: settled at the ack = %v, want %v", tc.adapter, tc.receipt, !tc.consumed, tc.consumed)
		}
	}
}
