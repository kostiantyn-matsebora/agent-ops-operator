package main

import (
	"context"
	"testing"
	"time"
)

// heldAdapter is a test adapter with a SYNCED cache and the owner hold wired,
// as main() wires it.
func heldAdapter(t *testing.T) (*adapter, *fakeManager) {
	t.Helper()
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	a.cache = syncedCache()
	a.self = newSelfExcluder().withCache(a.cache)
	a.held = newOwnerHold(ownerResolveWait)
	return a, mgr
}

// The bound is one tick of the cache's own refresh cadence — reused, not a new
// constant.
func TestOwnerResolveWaitIsOneRefreshTick(t *testing.T) {
	if ownerResolveWait != refreshEvery {
		t.Fatalf("ownerResolveWait = %s, want one refresh tick (%s)", ownerResolveWait, refreshEvery)
	}
}

// A pod's event arriving before the cache holds its owner chain waits, and is
// emitted under the WORKLOAD's id once the owner appears inside the wait —
// never under a per-pod one.
func TestACacheCatchingUpDoesNotSplitAWorkload(t *testing.T) {
	a, mgr := heldAdapter(t)
	ctx := context.Background()

	a.deliver(ctx, []Event{evt("Warning", "prod", "Pod", "api-7f8d9-xk2p9", "BackOff")}, pass{})
	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("an unresolved pod must be held, not emitted per-pod: %+v", got)
	}
	if a.held.Len() != 1 {
		t.Fatalf("expected one held event, got %d", a.held.Len())
	}

	// The pod lands, owned by a ReplicaSet the cache does not hold yet: still
	// unsettled, since emitting now would key on the ReplicaSet.
	putPod(t, a.cache, podJSON("prod", "api-7f8d9-xk2p9", "n1", nil,
		&ownerRef{Kind: "ReplicaSet", Name: "api-7f8d9", Controller: boolPtr(true)}, false, "Running"))
	a.releaseHeld(ctx, time.Now())
	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("a half-walked owner chain must keep holding: %+v", got)
	}

	putRS(t, a.cache, rsJSON("prod", "api-7f8d9", &ownerRef{Kind: "Deployment", Name: "api", Controller: boolPtr(true)}))
	a.releaseHeld(ctx, time.Now())

	got := mgr.signalsFor("src")
	if len(got) != 1 || got[0].InitiationID != "src@prod/Deployment/api" {
		t.Fatalf("the settled event must carry the workload-keyed id: %+v", got)
	}
	if a.held.Len() != 0 {
		t.Fatalf("a released event must leave the hold: %d", a.held.Len())
	}
}

// The wait is bounded: an owner that never appears (the object is gone) emits
// under the object's own id once one refresh tick has passed — delayed, never
// lost.
func TestAnUnresolvedOwnerIsReleasedAfterOneTick(t *testing.T) {
	a, mgr := heldAdapter(t)
	ctx := context.Background()

	a.deliver(ctx, []Event{evt("Warning", "prod", "Pod", "gone-pod", "BackOff")}, pass{})
	a.releaseHeld(ctx, time.Now().Add(ownerResolveWait-time.Second))
	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("released before the bound: %+v", got)
	}

	a.releaseHeld(ctx, time.Now().Add(ownerResolveWait+time.Second))
	got := mgr.signalsFor("src")
	if len(got) != 1 || got[0].InitiationID != "src@prod/Pod/gone-pod" {
		t.Fatalf("past the bound the event must emit under the object's own id: %+v", got)
	}
}

// A confirmed-unowned pod is settled at once, and keys on itself.
func TestAnUnownedPodIsNotHeld(t *testing.T) {
	a, mgr := heldAdapter(t)
	putPod(t, a.cache, podJSON("prod", "bare", "n1", nil, nil, false, "Running"))

	a.deliver(context.Background(), []Event{evt("Warning", "prod", "Pod", "bare", "BackOff")}, pass{})

	got := mgr.signalsFor("src")
	if len(got) != 1 || got[0].InitiationID != "src@prod/Pod/bare" || a.held.Len() != 0 {
		t.Fatalf("an unowned pod is its own workload, emitted without a wait: %+v", got)
	}
}

// A Node carries no owner chain: urgent node reasons ship with `for: 0` so that
// they are never delayed, and the hold must not delay them either.
func TestANodeEventIsNeverHeld(t *testing.T) {
	a, mgr := heldAdapter(t)

	a.deliver(context.Background(), []Event{evt("Warning", "", "Node", "node-1", "NodeNotReady")}, pass{})

	if got := mgr.signalsFor("src"); len(got) != 1 || a.held.Len() != 0 {
		t.Fatalf("a Node event must emit immediately: %+v", got)
	}
}

// A held event whose object turns out to be agent-ops' own machinery is
// excluded at release, once the cache can answer the owner rule.
func TestAHeldEventIsSelfExcludedAtRelease(t *testing.T) {
	a, mgr := heldAdapter(t)
	ctx := context.Background()

	a.deliver(ctx, []Event{evt("Warning", "prod", "Pod", "worker-x", "BackOff")}, pass{})
	putPod(t, a.cache, podJSON("prod", "worker-x", "n1", map[string]string{"agentops.dev/conversation": "c"},
		nil, false, "Running"))
	a.releaseHeld(ctx, time.Now())

	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("an agent-ops object must not emit after its hold: %+v", got)
	}
}
