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

// finishNestedMemberRun reports a member's one dispatched task done through the
// real /work/done path, carrying the member's own pending task input as
// Inflight.InputIDs so the report also consumes it — the same mechanism
// TestCoordinateInvokeCreatesAMemberAndRoutesItsResultBack uses, extended to
// also clear the input queue. Afterwards the member has nothing queued and
// nothing inflight: dispatch.NeedsWorker is false, and a pod it still holds
// is EVICTABLE (design D-7's "the parent's run ends").
func finishNestedMemberRun(t *testing.T, srv *httpapi.Server, name, runID string) {
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
	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": name, "runId": runID, "status": "succeeded", "result": "invoked its own child, then finished",
	}, srv.AdapterToken)
	if rec.Code != 200 {
		t.Fatalf("work/done for %s: %d %s", name, rec.Code, rec.Body.String())
	}
}

// mkNestingCoordinator creates a Coordinator naming ONE agents[] entry
// "child", nesting another Coordinator when nextCoordinator is non-empty or
// invoking capabilityRef (cap) otherwise — mirroring
// TestInvokeMemberCreatesANestedCoordinatorMember's (chat/coordinate_test.go)
// shape against the real API server.
func mkNestingCoordinator(t *testing.T, name, profile, nextCoordinator, cap string) {
	t.Helper()
	mkCoordinator(t, name, nil, nil, profile, nil)
	co := reconcileCoordinator(t, name)
	entry := agentopsv1alpha1.CoordinatorAgentEntry{Name: "child", Description: "the next level of the chain"}
	if nextCoordinator != "" {
		entry.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: nextCoordinator}
	} else {
		entry.CapabilityRef = &agentopsv1alpha1.ObjectRef{Name: cap}
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

	child := invokeChild(t, srv, parent, "co-nest-nd-b", "child", "do the leaf work")
	finishNestedMemberRun(t, srv, parent, "r-nd-parent") // the parent's own run ends: idle, evictable

	debugParent := getConv(t, parent)
	t.Logf("DEBUG parent after finish: phase=%v inflight=%+v inputs=%+v processed=%v",
		debugParent.Status.Phase, debugParent.Status.Inflight, debugParent.Spec.Inputs, debugParent.Status.ProcessedInputIDs)

	reconcileWith(t, rc, child)
	debugChild := getConv(t, child)
	t.Logf("DEBUG child after reconcile: phase=%v", debugChild.Status.Phase)
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

// TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes: a chain of
// six nested Coordinator members against a ceiling of five still completes
// (design D-7's own worked example) — each level's pod is reclaimed once it
// goes idle, which is what lets the next level run without ever stalling.
func TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes(t *testing.T) {
	clearRuntimePods(t)
	const depth = 6
	const membersCap = 5

	mkProfile(t, "prof-nest-chain")
	mkCapability(t, "cap-nest-chain-leaf", "prof-nest-chain")
	reconcileCapability(t, "cap-nest-chain-leaf")
	for i := 1; i <= depth; i++ {
		name := fmt.Sprintf("co-nest-chain-%d", i)
		if i < depth {
			mkNestingCoordinator(t, name, "prof-nest-chain", fmt.Sprintf("co-nest-chain-%d", i+1), "")
		} else {
			mkNestingCoordinator(t, name, "prof-nest-chain", "", "cap-nest-chain-leaf")
		}
	}

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

	callerName, callerCoordinatorRef := root.Name, "co-nest-chain-1"
	var members []string
	for i := 1; i <= depth; i++ {
		member := invokeChild(t, srv, callerName, callerCoordinatorRef, "child", fmt.Sprintf("level %d", i))
		reconcileWith(t, rc, member)
		if !podExists(t, member) {
			t.Fatalf("level %d of %d (members cap %d) must be admitted, not left Pending — the chain stalled",
				i, depth, membersCap)
		}
		members = append(members, member)
		if len(members) >= 2 {
			// The level just before this one has nothing left to do until this
			// child's result arrives — idle and evictable, exactly what lets a
			// chain deeper than the ceiling keep moving.
			finishNestedMemberRun(t, srv, members[len(members)-2], fmt.Sprintf("r-chain-%d", i-1))
		}
		callerName, callerCoordinatorRef = member, fmt.Sprintf("co-nest-chain-%d", i+1)
	}

	if podExists(t, members[0]) {
		t.Fatal("the oldest, long-idle level must have been reclaimed to admit a level beyond the ceiling")
	}
}
