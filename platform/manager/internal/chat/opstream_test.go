package chat

import (
	"context"
	"errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"strings"
	"testing"
)

// A second OpQueue over the same bus resumes the claims the first made, after
// the command id of the last op the consumer acknowledged, and completes them.
func TestClaimsResumeAfterACommandIDAndCompleteOnASecondQueue(t *testing.T) {
	bus := testBus()
	first := &OpQueue{Registry: NewRegistry(), Bus: bus}
	ctx := context.Background()
	a, b := testChannel("ui-a", "console"), testChannel("ui-b", "console")
	tid := "t"
	first.EnqueueInputDelivery(ctx, a, crRec(testConv("c1")), "i", &tid, Notice("1"))
	first.EnqueueInputDelivery(ctx, b, crRec(testConv("c2")), "i", &tid, Notice("2"))
	first.EnqueueInputDelivery(ctx, a, crRec(testConv("c3")), "i", &tid, Notice("3"))

	var claimed []SequencedOp
	for {
		so, ok := first.ClaimSequenced("console")
		if !ok {
			break
		}
		claimed = append(claimed, so)
	}
	if len(claimed) != 3 {
		t.Fatalf("claimed %d ops, want 3", len(claimed))
	}
	ids := map[string]bool{}
	for _, so := range claimed {
		if so.Op.ID == "" || ids[so.Op.ID] {
			t.Fatalf("command ids must be non-empty and distinct across channels: %+v", claimed)
		}
		ids[so.Op.ID] = true
	}

	second := &OpQueue{
		Registry: NewRegistry(), Bus: bus,
		ChannelsFor: func(context.Context, string) []string { return []string{"ui-a", "ui-b"} },
	}
	all := second.Resume(ctx, "console")
	if len(all) != 3 {
		t.Fatalf("resume with no cursor returned %d ops, want all 3 unacknowledged", len(all))
	}
	// Log order is the order the ops were enqueued, across both channels.
	for i, want := range []string{"1", "2", "3"} {
		if got := all[i].Op.Message.Body; got != want {
			t.Fatalf("resume must return ops in log order, op %d carries %q, want %q", i, got, want)
		}
	}
	if all[0].ReclaimAfter <= 0 || all[0].ReclaimAfter > ReclaimAfter {
		t.Fatalf("a replayed op reports what its lease has left, got %v", all[0].ReclaimAfter)
	}

	// Acknowledged on the replica that did NOT claim it.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	if got := second.Resume(ctx, "console"); len(got) != 2 {
		t.Fatalf("an acknowledged op must leave the resume set, got %d", len(got))
	}
	// An acknowledgement of an op nothing runs under is the tolerated duplicate.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	// And a completion with no prior Resume at all still finds the bus's lease.
	third := &OpQueue{Registry: NewRegistry(), Bus: bus, ChannelsFor: second.ChannelsFor}
	third.noteChannel("ui-a", "console")
	third.noteChannel("ui-b", "console")
	third.Complete(ctx, all[1].Op.ID, OpResult{})
	if got := third.Resume(ctx, "console"); len(got) != 1 {
		t.Fatalf("a bare acknowledgement on a replica that never saw the claim must settle it, left %d", len(got))
	}
}

// settleFailingBus is a bus whose acknowledgement always fails.
type settleFailingBus struct{ commandbus.Bus }

func (settleFailingBus) Settle(context.Context, string, string) error {
	return errors.New("bus unreachable")
}

// A failed acknowledgement is logged, never swallowed, and the op is not
// reported settled: it will be redelivered when its lease lapses.
func TestCompleteLogsAFailedSettle(t *testing.T) {
	bus := testBus()
	q := &OpQueue{Registry: NewRegistry(), Bus: settleFailingBus{bus}}
	var lines []string
	ctx := logr.NewContext(context.Background(), funcr.New(func(prefix, args string) { lines = append(lines, args) }, funcr.Options{}))
	tid := "t"
	q.EnqueueInputDelivery(ctx, testChannel("ui-a", "console"), crRec(testConv("c1")), "i", &tid, Notice("1"))
	so, ok := q.ClaimSequenced("console")
	if !ok {
		t.Fatal("nothing claimed")
	}
	q.Complete(ctx, so.Op.ID, OpResult{})
	if len(lines) != 1 || !strings.Contains(lines[0], "bus unreachable") {
		t.Fatalf("log = %q, want one line carrying the settle error", lines)
	}
	if q.Settled(so.Op.ID) {
		t.Fatal("an op whose acknowledgement failed must not read as settled")
	}
}
