package chat

import (
	"context"
	"testing"
)

// ClaimSequenced carries the task's Order: it rises with enqueue order across
// the queues of every channel the adapter serves, and a second OpQueue over the
// same bus resumes the claims the first made and completes them.
func TestSequencedClaimsResumeAndCompleteOnASecondQueue(t *testing.T) {
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
	bySeq := map[uint64]bool{}
	for _, so := range claimed {
		if so.Sequence == 0 || bySeq[so.Sequence] {
			t.Fatalf("sequences must be non-zero and distinct across channels: %+v", claimed)
		}
		bySeq[so.Sequence] = true
	}

	second := &OpQueue{
		Registry: NewRegistry(), Bus: bus,
		ChannelsFor: func(context.Context, string) []string { return []string{"ui-a", "ui-b"} },
	}
	all := second.Resume(ctx, "console", 0)
	if len(all) != 3 {
		t.Fatalf("resume from 0 returned %d ops, want all 3 unacknowledged", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Sequence >= all[i].Sequence {
			t.Fatalf("resume must return oldest sequence first: %+v", all)
		}
	}
	if got := second.Resume(ctx, "console", all[1].Sequence); len(got) != 1 || got[0].Op.ID != all[2].Op.ID {
		t.Fatalf("resume past the second returned %+v, want only the third", got)
	}
	if all[0].ReclaimAfter <= 0 || all[0].ReclaimAfter > ReclaimAfter {
		t.Fatalf("a replayed op reports what its lease has left, got %v", all[0].ReclaimAfter)
	}

	// Acknowledged on the replica that did NOT claim it.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	if got := second.Resume(ctx, "console", 0); len(got) != 2 {
		t.Fatalf("an acknowledged op must leave the resume set, got %d", len(got))
	}
	// An acknowledgement of an op nothing runs under is the tolerated duplicate.
	second.Complete(ctx, all[0].Op.ID, OpResult{})
	// And a completion with no prior Resume at all still finds the bus's lease.
	third := &OpQueue{Registry: NewRegistry(), Bus: bus, ChannelsFor: second.ChannelsFor}
	third.noteChannel("ui-a", "console")
	third.noteChannel("ui-b", "console")
	third.Complete(ctx, all[1].Op.ID, OpResult{})
	if got := third.Resume(ctx, "console", 0); len(got) != 1 {
		t.Fatalf("a bare acknowledgement on a replica that never saw the claim must settle it, left %d", len(got))
	}
}
