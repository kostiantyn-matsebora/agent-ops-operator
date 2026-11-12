package conformance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// Scenarios of the hand-over contract (design D-17), walked for every queue that
// declares a DeliveryTTL, on both backends. A consumer READS a message, which
// changes nothing, ACKNOWLEDGES it, which hands it over for the delivery TTL,
// and then reports it done or failed.

var handoverScenarios = []struct {
	name string
	run  func(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue)
}{
	{"reading changes nothing and a read message is offered again", testHandoverReadChangesNothing},
	{"an acknowledged message is never offered again", testHandoverAckIsFinal},
	{"an acknowledgement is idempotent across handles and replicas", testHandoverAckIsIdempotent},
	{"a message with no report at its delivery TTL is failed, never redelivered", testHandoverTTLFails},
	{"done removes the message and its entries", testHandoverDoneRemoves},
	{"a failure is final", testHandoverFailureIsFinal},
	{"a message waits behind an unfinished message of its key", testHandoverKeyOrder},
	{"acknowledging what is not offered is refused", testHandoverAckRefusals},
	{"a racing acknowledgement hands over once", testHandoverRacingAck},
}

func countEntries(b Backing, typ entrylog.Type) int {
	n := 0
	for _, e := range b.Entries() {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func peekAll(t *testing.T, ctx context.Context, bus commandbus.Bus, queue commandbus.Queue) []commandbus.Task {
	t.Helper()
	got, err := bus.Peek(ctx, queue, 16)
	mustNoErr(t, err, "Peek")
	return got
}

func testHandoverReadChangesNothing(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	first, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	_, err = bus.Submit(ctx, req(queue, d.Kind, "cmd-b", "key-b"))
	mustNoErr(t, err, "Submit")
	before := entriesEventually(t, b, 2)

	for i := 0; i < 3; i++ {
		got := peekAll(t, ctx, b.Bus(), queue)
		if len(got) != 2 || got[0].UID != first.UID || got[0].Lease != nil || got[0].State != commandbus.StateQueued {
			t.Fatalf("read %d = %+v, want both messages, oldest first, unclaimed", i, got)
		}
	}
	if after := b.Entries(); len(after) != len(before) {
		t.Fatalf("reading appended %d entries; a read must change nothing", len(after)-len(before))
	}
	if limited, _ := bus.Peek(ctx, queue, 1); len(limited) != 1 {
		t.Fatalf("a limit of 1 returned %d messages", len(limited))
	}
}

func testHandoverAckIsFinal(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	acked, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter")
	mustNoErr(t, err, "Acknowledge")
	if acked.State != commandbus.StateRunning || acked.Lease == nil {
		t.Fatalf("an acknowledged message is held: %+v", acked)
	}
	if got := peekAll(t, ctx, bus, queue); len(got) != 0 {
		t.Fatalf("an acknowledged message was offered again: %+v", got)
	}
	// Nothing the clock does before the delivery TTL hands it out again.
	b.Advance(d.Policy.DeliveryTTL - time.Second)
	if got := peekAll(t, ctx, bus, queue); len(got) != 0 {
		t.Fatalf("an acknowledged message was offered before its delivery TTL: %+v", got)
	}
}

func testHandoverAckIsIdempotent(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	first, second := b.Bus(), b.Bus()
	_, err := first.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	one, err := first.Acknowledge(ctx, queue, "cmd-a", "replica-1")
	mustNoErr(t, err, "Acknowledge")
	two, err := second.Acknowledge(ctx, queue, "cmd-a", "replica-2")
	mustNoErr(t, err, "Acknowledge through a second handle")
	if two.UID != one.UID || two.Lease == nil || two.Lease.Token != one.Lease.Token {
		t.Fatalf("a repeated acknowledgement must read the first lease back: %+v vs %+v", one.Lease, two.Lease)
	}
	if n := countEntries(b, entrylog.TypeClaimed); n != 1 {
		t.Fatalf("the message was handed over %d times, want once", n)
	}
	// The report is accepted through the handle that did not acknowledge.
	mustNoErr(t, second.Settle(ctx, one.UID, one.Lease.Token), "Settle through the second handle")
}

func testHandoverTTLFails(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	acked, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter")
	mustNoErr(t, err, "Acknowledge")

	b.Advance(d.Policy.DeliveryTTL + time.Second)
	got, err := bus.GetTask(ctx, acked.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateDead || got.Code != commandbus.CodeUnavailable {
		t.Fatalf("a message with no report at its delivery TTL must be failed, got %s code %s", got.State, got.Code)
	}
	if offered := peekAll(t, ctx, bus, queue); len(offered) != 0 {
		t.Fatalf("a failed message was offered again: %+v", offered)
	}
	// A late report from the adapter that gave up on the wire is refused.
	if err := bus.Settle(ctx, acked.UID, acked.Lease.Token); !errors.Is(err, commandbus.ErrLeaseLost) {
		t.Fatalf("a report after the TTL = %v, want ErrLeaseLost", err)
	}
	dead, err := bus.DeadLetters(ctx, queue)
	mustNoErr(t, err, "DeadLetters")
	if len(dead) != 1 || dead[0].UID != acked.UID {
		t.Fatalf("the failed message is a dead letter, got %+v", dead)
	}
}

func testHandoverDoneRemoves(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	acked, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter")
	mustNoErr(t, err, "Acknowledge")
	mustNoErr(t, bus.Settle(ctx, acked.UID, acked.Lease.Token), "Settle")
	b.Compact()
	if _, err := bus.GetTask(ctx, acked.UID); !errors.Is(err, commandbus.ErrNotFound) {
		t.Fatalf("a consumed message is removed, GetTask = %v", err)
	}
	if left := b.Entries(); len(left) != 0 {
		t.Fatalf("the consumed message's entries remain: %v", entryTypes(left))
	}
	if got := peekAll(t, ctx, bus, queue); len(got) != 0 {
		t.Fatalf("a consumed message was offered: %+v", got)
	}
}

func testHandoverFailureIsFinal(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	acked, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter")
	mustNoErr(t, err, "Acknowledge")
	// A code that would retry anywhere else.
	mustNoErr(t, bus.Fail(ctx, acked.UID, acked.Lease.Token, commandbus.CodeUnavailable, "the surface refused"), "Fail")
	got, err := bus.GetTask(ctx, acked.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateDead {
		t.Fatalf("a failure is final, state = %s", got.State)
	}
	b.Advance(time.Hour)
	if offered := peekAll(t, ctx, bus, queue); len(offered) != 0 {
		t.Fatalf("a failed message was offered again: %+v", offered)
	}
}

func testHandoverKeyOrder(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-1", "same-key"))
	mustNoErr(t, err, "Submit")
	_, err = bus.Submit(ctx, req(queue, d.Kind, "cmd-2", "same-key"))
	mustNoErr(t, err, "Submit")
	if got := peekAll(t, ctx, bus, queue); len(got) != 1 || got[0].CommandID != "cmd-1" {
		t.Fatalf("only the first message of a key is offered, got %+v", got)
	}
	if _, err := bus.Acknowledge(ctx, queue, "cmd-2", "adapter"); !errors.Is(err, commandbus.ErrNotOffered) {
		t.Fatalf("acknowledging a message behind its key = %v, want ErrNotOffered", err)
	}
	first, err := bus.Acknowledge(ctx, queue, "cmd-1", "adapter")
	mustNoErr(t, err, "Acknowledge")
	if got := peekAll(t, ctx, bus, queue); len(got) != 0 {
		t.Fatalf("the second message is not offered while the first is unreported: %+v", got)
	}
	mustNoErr(t, bus.Settle(ctx, first.UID, first.Lease.Token), "Settle")
	if got := peekAll(t, ctx, bus, queue); len(got) != 1 || got[0].CommandID != "cmd-2" {
		t.Fatalf("the second message is offered once the first is done, got %+v", got)
	}
}

func testHandoverAckRefusals(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	if _, err := bus.Acknowledge(ctx, queue, "never-submitted", "adapter"); !errors.Is(err, commandbus.ErrNotFound) {
		t.Fatalf("acknowledging an unknown message = %v, want ErrNotFound", err)
	}
	_, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	acked, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter")
	mustNoErr(t, err, "Acknowledge")
	mustNoErr(t, bus.Fail(ctx, acked.UID, acked.Lease.Token, commandbus.CodeInternal, "no"), "Fail")
	if _, err := bus.Acknowledge(ctx, queue, "cmd-a", "adapter"); !errors.Is(err, commandbus.ErrNotOffered) {
		t.Fatalf("acknowledging a failed message = %v, want ErrNotOffered", err)
	}
}

func testHandoverRacingAck(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	_, err := b.Bus().Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	const racers = 6
	tokens := make([]string, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := b.Bus().Acknowledge(ctx, queue, "cmd-a", "racer")
			if err != nil || got.Lease == nil {
				t.Errorf("racer %d: %v", i, err)
				return
			}
			tokens[i] = got.Lease.Token
		}(i)
	}
	wg.Wait()
	for _, tok := range tokens {
		if tok != tokens[0] {
			t.Fatalf("racing acknowledgements saw different leases: %v", tokens)
		}
	}
	if n := countEntries(b, entrylog.TypeClaimed); n != 1 {
		t.Fatalf("a racing acknowledgement handed over %d times, want once", n)
	}
}

func testAcknowledgeNeedsHandover(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	_, err := bus.Submit(ctx, req("conv", commandbus.KindDefault, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit")
	if _, err := bus.Acknowledge(ctx, "conv", "cmd-a", "adapter"); !errors.Is(err, commandbus.ErrInvalidRequest) {
		t.Fatalf("acknowledging on a queue with no delivery TTL = %v, want ErrInvalidRequest", err)
	}
}
