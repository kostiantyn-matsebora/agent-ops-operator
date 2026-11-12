package chat

import (
	"context"
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
	all := second.Resume(ctx, "console", "")
	if len(all) != 3 {
		t.Fatalf("resume with no cursor returned %d ops, want all 3 unacknowledged", len(all))
	}
	// Log order is the order the ops were enqueued, across both channels.
	for i, want := range []string{"1", "2", "3"} {
		if got := all[i].Op.Message.Body; got != want {
			t.Fatalf("resume must return ops in log order, op %d carries %q, want %q", i, got, want)
		}
	}
	if got := second.Resume(ctx, "console", all[1].Op.ID); len(got) != 1 || got[0].Op.ID != all[2].Op.ID {
		t.Fatalf("resume after the second returned %+v, want only the third", got)
	}
	if got := second.Resume(ctx, "console", "an-id-no-op-ever-had"); len(got) != 3 {
		t.Fatalf("an unknown cursor must resume from the oldest unacknowledged op, got %d ops", len(got))
	}
	if all[0].ReclaimAfter <= 0 || all[0].ReclaimAfter > ReclaimAfter {
		t.Fatalf("a replayed op reports what its lease has left, got %v", all[0].ReclaimAfter)
	}

	// Acknowledged on the replica that did NOT claim it.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	if got := second.Resume(ctx, "console", ""); len(got) != 2 {
		t.Fatalf("an acknowledged op must leave the resume set, got %d", len(got))
	}
	// An acknowledgement of an op nothing runs under is the tolerated duplicate.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	// And a completion with no prior Resume at all still finds the bus's lease.
	third := &OpQueue{Registry: NewRegistry(), Bus: bus, ChannelsFor: second.ChannelsFor}
	third.noteChannel("ui-a", "console")
	third.noteChannel("ui-b", "console")
	third.Complete(ctx, all[1].Op.ID, OpResult{})
	if got := third.Resume(ctx, "console", ""); len(got) != 1 {
		t.Fatalf("a bare acknowledgement on a replica that never saw the claim must settle it, left %d", len(got))
	}
}
