// Nested Coordinators and the members pool (separate-member-slot-pool,
// task 4.2, design D-7): a member that is itself a Coordinator's own root
// must not deadlock its own children once the members pool is full.
//
// Every member here is created through the REAL `/coordinate/invoke` path
// (design D-F) — the same mechanism TestCoordinateInvokeCreatesAMemberAndRoutesItsResultBack
// in coordinate_test.go proves end to end — never by hand-crafting a
// CausedBy conversation, because this file is specifically about the
// Coordinator semantics invoke itself enforces (resolving the acting
// Coordinator, nesting a CoordinatorRef member, the cycle check), not just
// about pool bookkeeping. memberpool_test.go's mkMemberConv shortcut is for
// tests where that mechanism is incidental; D-7 is the opposite case.
package integration

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// invokeChild drives the real /coordinate/invoke path from callerName —
// authenticated as acting for callerCoordinatorRef, exactly the token a
// running agent's own `invoke` tool call would present — and returns the
// created member's name.
func invokeChild(t *testing.T, srv *httpapi.Server, callerName, callerCoordinatorRef, agent, task string) string {
	t.Helper()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, callerCoordinatorRef, callerName)
	rec := postCoordinateReq(t, srv, "/coordinate/invoke", token,
		map[string]any{"conversation": callerName, "agent": agent, "task": task})
	out := decodeCoordinateInvokeResponse(t, rec)
	member := out["member"]
	t.Cleanup(func() { cleanupConversation(t, member) })
	return member
}

// markMemberDispatched simulates a runtime claiming a member's one pending
// task as its current run, carrying the task's own input id as
// Inflight.InputIDs so a later /work/done also consumes it. This must
// happen BEFORE the member calls `invoke`: `/coordinate/invoke`'s own
// stampMove (coordinate.go, design D-A) only marks a move on an EXISTING
// inflight entry, and `/work/done`'s move-per-turn judge (moveperturn.go,
// coordination-loop: "every root turn ends in a move") reads an un-moved
// turn on a coordinatorRef-carrying conversation as a MISS — appending a
// corrective input that would leave PendingInputs non-empty, so the member
// would never go idle however its run is reported.
func markMemberDispatched(t *testing.T, name, runID string) {
	t.Helper()
	ctx := context.Background()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(conv.Spec.Inputs))
	for _, in := range conv.Spec.Inputs {
		ids = append(ids, in.ID)
	}
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: runID, InputIDs: ids, DispatchedAt: metav1.Now()}
	if err := k8sClient.Status().Update(ctx, &conv); err != nil {
		t.Fatal(err)
	}
}

// reportMemberRunDone is the other half of markMemberDispatched: the real
// /work/done path, called once the member's own `invoke` of its child has
// already stamped Moved — so the run is recorded as a move, the pending
// task input is consumed (Inflight.InputIDs), and nothing is left queued.
// dispatch.NeedsWorker is false from here on: a pod the member still holds
// is EVICTABLE (design D-7's "the parent's run ends").
func reportMemberRunDone(t *testing.T, srv *httpapi.Server, name, runID string) {
	t.Helper()
	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": convoID(t, name), "runId": runID, "status": "succeeded", "result": "invoked its own child, then finished",
	}, srv.AdapterToken)
	if rec.Code != 200 {
		t.Fatalf("work/done for %s: %d %s", name, rec.Code, rec.Body.String())
	}
}

// mkNestingCoordinator creates a Coordinator naming ONE agents[] entry
// "child", nesting another Coordinator when nextCoordinator is non-empty or
// invoking capabilityRef (capName) otherwise — mirroring
// TestInvokeMemberCreatesANestedCoordinatorMember's (chat/coordinate_test.go)
// shape against the real API server.
func mkNestingCoordinator(t *testing.T, name, profile, nextCoordinator, capName string) {
	t.Helper()
	mkCoordinator(t, name, nil, nil, profile, nil)
	co := reconcileCoordinator(t, name)
	entry := agentopsv1alpha1.CoordinatorAgentEntry{Name: "child", Description: "the next level of the chain"}
	if nextCoordinator != "" {
		entry.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: nextCoordinator}
	} else {
		entry.CapabilityRef = &agentopsv1alpha1.ObjectRef{Name: capName}
	}
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{entry}
	if err := k8sClient.Update(context.Background(), co); err != nil {
		t.Fatal(err)
	}
}

// TestIdleMemberCoordinatorsSlotIsEvictedForItsOwnChild pins design D-7's
// three central steps: a member that is itself a Coordinator invokes its own
// child, then its own run ends — nothing inflight, nothing queued — which
// makes its pod EVICTABLE; with the members pool otherwise full of BUSY
// members, the child it just invoked, needing a slot, evicts the idle
// parent (D-3: eviction stays inside the pool, which is where both live).
func TestIdleMemberCoordinatorsSlotIsEvictedForItsOwnChild(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-nest-nd")
	mkNestingCoordinator(t, "co-nest-nd-a", "prof-nest-nd", "co-nest-nd-b", "")
	mkCapability(t, "cap-nest-nd-leaf", "prof-nest-nd")
	reconcileCapability(t, "cap-nest-nd-leaf")
	mkNestingCoordinator(t, "co-nest-nd-b", "prof-nest-nd", "", "cap-nest-nd-leaf")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "nest-nd-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-nest-nd"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-nest-nd-a"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, root.Name) })

	srv := apiServer()
	rc := reconcilerWithPoolCaps(nil, 100, 2) // small: one filler + one parent fills it

	filler := mkMemberConv(t, "nest-nd-filler-busy", root.Name, "prof-nest-nd")
	reconcileWith(t, rc, filler.Name)
	if !podExists(t, filler.Name) {
		t.Fatal("setup: the busy filler member must hold one of the members pool's two slots")
	}

	parent := invokeChild(t, srv, root.Name, "co-nest-nd-a", "child", "delegate to b")
	reconcileWith(t, rc, parent)
	if !podExists(t, parent) {
		t.Fatal("setup: the nested-Coordinator member must be admitted, filling the members pool")
	}
	gotParent := getConv(t, parent)
	if gotParent.Spec.CoordinatorRef == nil || gotParent.Spec.CoordinatorRef.Name != "co-nest-nd-b" {
		t.Fatalf("the parent must carry the nested Coordinator's own ref, got %+v", gotParent.Spec.CoordinatorRef)
	}

	// The parent is BUSY (inflight) when it invokes its own child — the same
	// order a real run follows: dispatched, call invoke, then report done.
	// stampMove needs that inflight entry to mark the move.
	markMemberDispatched(t, parent, "r-nd-parent")
	child := invokeChild(t, srv, parent, "co-nest-nd-b", "child", "do the leaf work")
	reportMemberRunDone(t, srv, parent, "r-nd-parent") // the parent's own run ends: idle, evictable

	reconcileWith(t, rc, child)
	if !podExists(t, child) {
		t.Fatal("the child must be admitted by evicting its own idle parent, not left Pending")
	}
	if podExists(t, parent) {
		t.Fatal("the idle parent's pod must be the one evicted")
	}
	if !podExists(t, filler.Name) {
		t.Fatal("the busy filler, in the SAME pool, must survive — only the idle parent was evictable")
	}
}

// mkNestedChain creates the leaf capability and depth nested Coordinators
// co-nest-chain-1..depth, each nesting the next, the last invoking the leaf.
func mkNestedChain(t *testing.T, depth int) {
	t.Helper()
	mkProfile(t, "prof-nest-chain")
	mkCapability(t, "cap-nest-chain-leaf", "prof-nest-chain")
	reconcileCapability(t, "cap-nest-chain-leaf")
	for i := 1; i <= depth; i++ {
		next, leaf := "", ""
		if i < depth {
			next = fmt.Sprintf("co-nest-chain-%d", i+1)
		} else {
			leaf = "cap-nest-chain-leaf"
		}
		mkNestingCoordinator(t, fmt.Sprintf("co-nest-chain-%d", i), "prof-nest-chain", next, leaf)
	}
}

// walkNestedChain invokes each level from its parent and reconciles it,
// failing if any level is left Pending. Level 1's caller is the root, which
// is not pool-counted, so it needs no dispatched/done bookkeeping. Every
// later level is BUSY when it invokes its child, then reports done: idle and
// evictable, which lets a chain deeper than the ceiling keep moving (D-7).
func walkNestedChain(t *testing.T, srv *httpapi.Server, rc *controller.ConversationReconciler, rootName string, depth, membersCap int) []string {
	t.Helper()
	callerName, callerCoordinatorRef := rootName, "co-nest-chain-1"
	var members []string
	for i := 1; i <= depth; i++ {
		runID := fmt.Sprintf("r-chain-%d", i)
		if i > 1 {
			markMemberDispatched(t, callerName, runID)
		}
		member := invokeChild(t, srv, callerName, callerCoordinatorRef, "child", fmt.Sprintf("level %d", i))
		if i > 1 {
			reportMemberRunDone(t, srv, callerName, runID)
		}
		reconcileWith(t, rc, member)
		if !podExists(t, member) {
			t.Fatalf("level %d of %d (members cap %d) must be admitted, not left Pending — the chain stalled",
				i, depth, membersCap)
		}
		members = append(members, member)
		callerName, callerCoordinatorRef = member, fmt.Sprintf("co-nest-chain-%d", i+1)
	}
	return members
}

// TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes: a chain of
// six nested Coordinator members against a ceiling of five still completes
// (design D-7's own worked example): no level is ever left Pending, and the
// pool never holds more pods than its own ceiling at once.
//
// A level does not stay idle for long once its own child finishes: the
// child's result lands back on it as a fresh input (coordination-loop:
// "a member's result becomes an input on its parent"), which makes it
// NeedsWorker again — so it is NOT necessarily the one eviction reclaims.
// Only the deepest level with no finished child yet is reliably idle; this
// test does not pin WHICH level eviction picks, only that the ceiling holds
// and nobody ever stalls behind it.
func TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes(t *testing.T) {
	clearRuntimePods(t)
	const depth = 6
	const membersCap = 5

	mkNestedChain(t, depth)

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "nest-chain-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-nest-chain"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-nest-chain-1"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, root.Name) })

	srv := apiServer()
	rc := reconcilerWithPoolCaps(nil, 100, membersCap)

	members := walkNestedChain(t, srv, rc, root.Name, depth, membersCap)

	alive := 0
	for _, m := range members {
		if podExists(t, m) {
			alive++
		}
	}
	if alive != membersCap {
		t.Fatalf("a chain of %d levels against a members cap of %d must hold exactly %d pods at once once it has run its course, got %d",
			depth, membersCap, membersCap, alive)
	}
}
