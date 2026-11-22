// Member pool scoping (separate-member-slot-pool, task 4.1): the same
// admission/FIFO/eviction mechanics capacity_test.go already pins for the
// conversations pool, now proven independent of a second, equally-real
// members pool — against the real API server, through
// ConversationReconciler.Reconcile exactly as capacity_test.go drives it.
//
// A member conversation is created directly here, CausedBy and the
// agentops.dev/caused-by label set by hand — the same shortcut
// TestCoordinateReconcileBackstopRoutesAMissedMemberResult and its siblings
// in coordinate_test.go take whenever a test is about something other than
// `invoke` itself. The nested-Coordinator scenarios that DO need the real
// `invoke` path (design D-7) are nestedcoordinator_test.go's job.
package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// mkMemberConv creates a Coordinator member conversation directly — one task
// input, so (nothing here ever consumes it) it stays NeedsWorker=true for
// the whole test once admitted, exactly like mkChanConv's plain
// conversations. No channels: a caused conversation binds none
// (coordination-loop).
func mkMemberConv(t *testing.T, name, parent, profile string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	conv.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: "worker"}
	conv.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	conv.Spec.Title = name
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	return conv
}

// markConversationIdle marks every one of a conversation's own inputs
// PROCESSED, so the next reconcile prunes them from spec and
// dispatch.NeedsWorker turns false — the same "nothing inflight, nothing
// queued" state a finished run leaves behind (design D-7), produced here
// directly rather than through a fabricated runtime report, since none of
// these tests are about reporting a run — only about what an idle
// conversation's held pod is allowed to cost once it exists.
func markConversationIdle(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	ids := make([]string, 0, len(conv.Spec.Inputs))
	for _, in := range conv.Spec.Inputs {
		ids = append(ids, in.ID)
	}
	conv.Status.ProcessedInputIDs = ids
	if err := k8sClient.Status().Patch(ctx, &conv, patch); err != nil {
		t.Fatal(err)
	}
}

// TestFullConversationsPoolStillAdmitsAMember: the two pools are truly
// independent (design D-1/D-2) — filling the conversations pool to its own
// ceiling must never block a member's own, separate ceiling.
func TestFullConversationsPoolStillAdmitsAMember(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-a")
	rc := reconcilerWithPoolCaps(nil, 1, 1)

	active := mkChanConv(t, "pool-a-conv-active", "prof-pool-a")
	reconcileWith(t, rc, active.Name)
	if !podExists(t, "pool-a-conv-active") {
		t.Fatal("setup: the conversations pool's own slot must be taken")
	}

	mkMemberConv(t, "pool-a-member", "pool-a-conv-active", "prof-pool-a")
	reconcileWith(t, rc, "pool-a-member")
	if !podExists(t, "pool-a-member") {
		t.Fatal("a full conversations pool must not block an admission into the members pool")
	}
}

// TestFullMembersPoolStillAdmitsAConversation is the symmetric case: a full
// members pool must never block an ordinary conversation's own ceiling.
func TestFullMembersPoolStillAdmitsAConversation(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-b")
	rc := reconcilerWithPoolCaps(nil, 1, 1)

	mkMemberConv(t, "pool-b-member-active", "pool-b-root", "prof-pool-b")
	reconcileWith(t, rc, "pool-b-member-active")
	if !podExists(t, "pool-b-member-active") {
		t.Fatal("setup: the members pool's own slot must be taken")
	}

	mkChanConv(t, "pool-b-conv", "prof-pool-b")
	reconcileWith(t, rc, "pool-b-conv")
	if !podExists(t, "pool-b-conv") {
		t.Fatal("a full members pool must not block an admission into the conversations pool")
	}
}

// TestSixthMemberPendsAtTheMembersCeiling: beyond the members pool's own
// cap, a member pends exactly as a conversation does today
// (TestOverCapConversationProvisionsNothing) — the SAME admission rule, now
// scoped to the other pool.
func TestSixthMemberPendsAtTheMembersCeiling(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-c")
	rc := reconcilerWithPoolCaps(nil, 100, 5)

	for i := 1; i <= 5; i++ {
		name := "pool-c-member-" + string(rune('a'+i-1))
		mkMemberConv(t, name, "pool-c-root", "prof-pool-c")
		reconcileWith(t, rc, name)
		if !podExists(t, name) {
			t.Fatalf("setup: member %d of 5 must be admitted, filling the members pool", i)
		}
	}

	mkMemberConv(t, "pool-c-member-sixth", "pool-c-root", "prof-pool-c")
	reconcileWith(t, rc, "pool-c-member-sixth")
	if got := phaseOf(t, "pool-c-member-sixth"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("a sixth member against a cap of 5 must pend, got phase %q", got)
	}
	if podExists(t, "pool-c-member-sixth") {
		t.Fatal("a Pending member must have no runtime pod")
	}
}

// TestEvictionNeverTakesAConversationsPodToFreeAMembersSlot pins design D-3:
// a members-pool admission that needs to evict may only evict a member's
// own pod. An idle, evictable CONVERSATION sitting in the other pool must
// survive untouched, even though it is the only evictable pod anywhere.
func TestEvictionNeverTakesAConversationsPodToFreeAMembersSlot(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-d")
	rc := reconcilerWithPoolCaps(nil, 1, 1)

	idleConv := mkChanConv(t, "pool-d-conv-idle", "prof-pool-d")
	reconcileWith(t, rc, idleConv.Name)
	if !podExists(t, "pool-d-conv-idle") {
		t.Fatal("setup: the idle conversation must hold the conversations pool's own slot")
	}
	markConversationIdle(t, "pool-d-conv-idle")
	reconcileWith(t, rc, "pool-d-conv-idle") // prune: NeedsWorker now false, pod untouched

	busyMember := mkMemberConv(t, "pool-d-member-busy", "pool-d-root", "prof-pool-d")
	reconcileWith(t, rc, busyMember.Name)
	if !podExists(t, "pool-d-member-busy") {
		t.Fatal("setup: the busy member must hold the members pool's own slot")
	}

	mkMemberConv(t, "pool-d-member-second", "pool-d-root", "prof-pool-d")
	reconcileWith(t, rc, "pool-d-member-second")

	if got := phaseOf(t, "pool-d-member-second"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("a second member against a full, all-busy members pool must pend, got phase %q", got)
	}
	if !podExists(t, "pool-d-conv-idle") {
		t.Fatal("eviction must never cross pools: the idle CONVERSATION's pod must survive a members-pool admission")
	}
	if !podExists(t, "pool-d-member-busy") {
		t.Fatal("the busy member's pod must survive too — nothing in its own pool was evictable")
	}
}

// TestEvictionNeverTakesAMembersPodToFreeAConversationsSlot is the symmetric
// case: an idle, evictable MEMBER must survive a conversations-pool
// admission that cannot be satisfied from its own pool.
func TestEvictionNeverTakesAMembersPodToFreeAConversationsSlot(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-e")
	rc := reconcilerWithPoolCaps(nil, 1, 1)

	idleMember := mkMemberConv(t, "pool-e-member-idle", "pool-e-root", "prof-pool-e")
	reconcileWith(t, rc, idleMember.Name)
	if !podExists(t, "pool-e-member-idle") {
		t.Fatal("setup: the idle member must hold the members pool's own slot")
	}
	markConversationIdle(t, "pool-e-member-idle")
	reconcileWith(t, rc, "pool-e-member-idle")

	busyConv := mkChanConv(t, "pool-e-conv-busy", "prof-pool-e")
	reconcileWith(t, rc, busyConv.Name)
	if !podExists(t, "pool-e-conv-busy") {
		t.Fatal("setup: the busy conversation must hold the conversations pool's own slot")
	}

	mkChanConv(t, "pool-e-conv-second", "prof-pool-e")
	reconcileWith(t, rc, "pool-e-conv-second")

	if got := phaseOf(t, "pool-e-conv-second"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("a second conversation against a full, all-busy conversations pool must pend, got phase %q", got)
	}
	if !podExists(t, "pool-e-member-idle") {
		t.Fatal("eviction must never cross pools: the idle MEMBER's pod must survive a conversations-pool admission")
	}
	if !podExists(t, "pool-e-conv-busy") {
		t.Fatal("the busy conversation's pod must survive too — nothing in its own pool was evictable")
	}
}

// TestYoungerMemberIsNotHeldBehindAnOlderPendingConversation: FIFO is scoped
// per pool (design D-2), never global creation order — a member created
// after an older, still-Pending conversation must still be admitted at once
// when its own (empty) pool has room.
func TestYoungerMemberIsNotHeldBehindAnOlderPendingConversation(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-pool-f")
	rc := reconcilerWithPoolCaps(nil, 1, 1)

	busyConv := mkChanConv(t, "pool-f-conv-busy", "prof-pool-f")
	reconcileWith(t, rc, busyConv.Name)
	if !podExists(t, "pool-f-conv-busy") {
		t.Fatal("setup: the conversations pool's own slot must be taken")
	}

	olderConv := mkChanConv(t, "pool-f-conv-older", "prof-pool-f")
	reconcileWith(t, rc, olderConv.Name)
	if got := phaseOf(t, "pool-f-conv-older"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("setup: the older conversation must pend behind the full conversations pool, got %q", got)
	}

	youngerMember := mkMemberConv(t, "pool-f-member-younger", "pool-f-root", "prof-pool-f")
	reconcileWith(t, rc, youngerMember.Name)
	if !podExists(t, "pool-f-member-younger") {
		t.Fatal("a younger member must not wait behind an older PENDING conversation of the other pool")
	}
}
