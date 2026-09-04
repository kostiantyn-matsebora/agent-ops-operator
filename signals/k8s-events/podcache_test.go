package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// ---- fixtures ---------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

// podJSON builds a core/v1 Pod as the API server would return it.
func podJSON(ns, name, node string, labels map[string]string, owner *ownerRef, ready bool, phase string, waiting ...string) map[string]any {
	meta := map[string]any{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = labels
	}
	if owner != nil {
		meta["ownerReferences"] = []map[string]any{
			{"kind": owner.Kind, "name": owner.Name, "controller": true},
		}
	}
	readyStr := "False"
	if ready {
		readyStr = "True"
	}
	var cs []map[string]any
	for _, w := range waiting {
		cs = append(cs, map[string]any{"state": map[string]any{"waiting": map[string]any{"reason": w}}})
	}
	return map[string]any{
		"metadata": meta,
		"spec":     map[string]any{"nodeName": node},
		"status": map[string]any{
			"phase":             phase,
			"conditions":        []map[string]any{{"type": "Ready", "status": readyStr}},
			"containerStatuses": cs,
		},
	}
}

func rsJSON(ns, name string, owner *ownerRef) map[string]any {
	meta := map[string]any{"name": name, "namespace": ns}
	if owner != nil {
		meta["ownerReferences"] = []map[string]any{
			{"kind": owner.Kind, "name": owner.Name, "controller": true},
		}
	}
	return map[string]any{"metadata": meta}
}

func collectionJSON(rv string, items ...map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"resourceVersion": rv},
		"items":    items,
	})
	return string(b)
}

// putPod decodes a pod fixture straight into a cache, bypassing the transport
// for tests that are about resolution rather than watching.
func putPod(t *testing.T, c *objectCache, obj map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(obj)
	oi, err := decodePod(raw)
	if err != nil {
		t.Fatal(err)
	}
	c.put(oi)
}

func putRS(t *testing.T, c *objectCache, obj map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(obj)
	oi, err := decodeReplicaSet(raw)
	if err != nil {
		t.Fatal(err)
	}
	c.put(oi)
}

// ---- cache mechanics --------------------------------------------------------

func TestCachePopulatesFromListAndUpdatesOnWatch(t *testing.T) {
	watchStarted := make(chan struct{})
	kube := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("10",
				podJSON("prod", "api-1", "node-a", nil, nil, true, "Running"))))
			return
		}
		close(watchStarted)
		// one MODIFIED frame flipping the pod to not-ready, then hold the
		// stream open until the context cancels
		_, _ = w.Write([]byte(`{"type":"MODIFIED","object":` + mustJSON(
			podJSON("prod", "api-1", "node-a", nil, nil, false, "Running", "CrashLoopBackOff")) + "}\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))

	cache := newObjectCache()
	cw := &cacheWatcher{kube: kube, cache: cache, kind: "Pod", pathFor: podsPath, decode: decodePod}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cw.run(ctx, "")

	<-watchStarted
	waitForMsg(t, func() bool {
		oi, known := cache.Get("prod", "Pod", "api-1")
		return known && oi != nil && !oi.Ready
	}, "watch frame must update the cached pod")

	oi, _ := cache.Get("prod", "Pod", "api-1")
	if len(oi.WaitingReasons) != 1 || oi.WaitingReasons[0] != "CrashLoopBackOff" {
		t.Fatalf("waiting reasons must be cached: %+v", oi.WaitingReasons)
	}
	if oi.Node != "node-a" {
		t.Fatalf("node must be cached: %q", oi.Node)
	}
}

func TestCacheRelistsAfterWatchExpiry(t *testing.T) {
	var lists, watches int
	relisted := make(chan struct{}, 1)
	kube := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			lists++
			if lists > 1 {
				select {
				case relisted <- struct{}{}:
				default:
				}
			}
			_, _ = w.Write([]byte(collectionJSON("10",
				podJSON("prod", "api-1", "node-a", nil, nil, true, "Running"))))
			return
		}
		watches++
		w.WriteHeader(http.StatusGone)
	}))

	cache := newObjectCache()
	cw := &cacheWatcher{kube: kube, cache: cache, kind: "Pod", pathFor: podsPath, decode: decodePod}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cw.run(ctx, "")

	select {
	case <-relisted:
	case <-time.After(3 * time.Second):
		t.Fatalf("410 Gone must trigger a relist (lists=%d watches=%d)", lists, watches)
	}
}

func TestCacheDeletedFrameDropsObject(t *testing.T) {
	deleted := make(chan struct{})
	kube := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("10",
				podJSON("prod", "api-1", "node-a", nil, nil, true, "Running"))))
			return
		}
		_, _ = w.Write([]byte(`{"type":"DELETED","object":` + mustJSON(
			podJSON("prod", "api-1", "node-a", nil, nil, true, "Running")) + "}\n"))
		w.(http.Flusher).Flush()
		close(deleted)
		<-r.Context().Done()
	}))

	cache := newObjectCache()
	cw := &cacheWatcher{kube: kube, cache: cache, kind: "Pod", pathFor: podsPath, decode: decodePod}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cw.run(ctx, "")

	<-deleted
	waitForMsg(t, func() bool {
		oi, known := cache.Get("prod", "Pod", "api-1")
		return known && oi == nil
	}, "a DELETED frame must remove the object, and absence must be a definite answer")
}

// A 403 is not fixed by waiting: the grant is external. It must be surfaced.
func TestCacheReportsForbidden(t *testing.T) {
	kube := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"pods is forbidden"}`))
	}))

	got := make(chan error, 1)
	cache := newObjectCache()
	cw := &cacheWatcher{kube: kube, cache: cache, kind: "Pod", pathFor: podsPath, decode: decodePod,
		onError: func(err error) {
			select {
			case got <- err:
			default:
			}
		}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cw.run(ctx, "")

	select {
	case err := <-got:
		if !IsForbidden(err) {
			t.Fatalf("expected a forbidden error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("a 403 must be reported, not retried silently")
	}
}

// Before the first list lands, "unknown" must not be readable as "absent" —
// otherwise every event during startup looks like it concerns a deleted object.
func TestUnsyncedCacheAnswersNothing(t *testing.T) {
	c := newObjectCache()
	if _, known := c.Get("prod", "Pod", "api-1"); known {
		t.Fatalf("an unsynced cache must not claim to know anything")
	}
	if _, _, ok := c.Workload("prod", "Pod", "api-1"); ok {
		t.Fatalf("an unsynced cache must not resolve a workload")
	}
}

func TestSyncedReportsWhetherAListHasLanded(t *testing.T) {
	c := newObjectCache()
	if c.Synced() {
		t.Fatal("a brand-new cache must not report itself synced")
	}
	c.markSynced()
	if !c.Synced() {
		t.Fatal("Synced() must report true once a list has landed")
	}
}

// OwnedByAgentOps implements self-exclusion mechanism 2 directly on the real
// cache — the "unknown", "absent", "labelled", "no owner" and multi-hop
// branches, none of which the selfexclude tests reach because they go through
// a stub objectLookup instead.
func TestOwnedByAgentOpsUnknownWhenTheCacheHasNotSynced(t *testing.T) {
	c := newObjectCache()
	if owned, known := c.OwnedByAgentOps("prod", "Pod", "api-1"); owned || known {
		t.Fatalf("an unsynced cache must not claim to know anything: owned=%v known=%v", owned, known)
	}
}

func TestOwnedByAgentOpsSyncedAndAbsentIsKnownAndUnowned(t *testing.T) {
	c := syncedCache()
	owned, known := c.OwnedByAgentOps("prod", "Pod", "ghost")
	if owned || !known {
		t.Fatalf("a synced, absent object must be a definite 'not owned': owned=%v known=%v", owned, known)
	}
}

func TestOwnedByAgentOpsLabelledObjectIsOwnedDirectly(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("agentops", "console-abc", "node-a",
		map[string]string{"app.kubernetes.io/name": "agentops-runtime"}, nil, true, "Running"))
	owned, known := c.OwnedByAgentOps("agentops", "Pod", "console-abc")
	if !owned || !known {
		t.Fatalf("a labelled object must be owned: owned=%v known=%v", owned, known)
	}
}

// No owner reference and no label: the loop never runs, and the object is a
// definite "not ours".
func TestOwnedByAgentOpsUnownedObjectWithNoOwnerIsNotOwned(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("prod", "debug-shell", "node-a", nil, nil, true, "Running"))
	owned, known := c.OwnedByAgentOps("prod", "Pod", "debug-shell")
	if owned || !known {
		t.Fatalf("a bare, unlabelled pod must not be owned: owned=%v known=%v", owned, known)
	}
}

// The owner chain is walked past one hop: a pod owned by a replicaset that
// carries no agent-ops label of its own, but whose OWNER is unknown to the
// cache, must stop rather than loop or panic.
func TestOwnedByAgentOpsStopsWhenTheChainLeavesTheCache(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("prod", "api-1", "node-a", nil,
		&ownerRef{Kind: "ReplicaSet", Name: "api-rs"}, true, "Running"))
	// api-rs itself is never put into the cache.
	owned, known := c.OwnedByAgentOps("prod", "Pod", "api-1")
	if owned || !known {
		t.Fatalf("a chain leaving the cache must resolve to 'not owned', known: owned=%v known=%v", owned, known)
	}
}

// The label may be found a hop up the chain rather than on the object itself.
func TestOwnedByAgentOpsFindsALabelOneHopUp(t *testing.T) {
	c := syncedCache()
	c.put(&objectInfo{Kind: "ReplicaSet", Namespace: "agentops", Name: "adapter-rs",
		Labels: map[string]string{"agentops.dev/adapter": "console"}})
	putPod(t, c, podJSON("agentops", "adapter-pod", "node-a", nil,
		&ownerRef{Kind: "ReplicaSet", Name: "adapter-rs"}, true, "Running"))
	owned, known := c.OwnedByAgentOps("agentops", "Pod", "adapter-pod")
	if !owned || !known {
		t.Fatalf("a label one hop up the owner chain must still mark the pod owned: owned=%v known=%v", owned, known)
	}
}

// waitForMsg is waitFor with a failure message naming what was expected.
func waitForMsg(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}
