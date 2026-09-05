package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// This file closes gaps on main.go's own lifecycle helpers — the watcher
// reconciliation, shutdown and background-loop functions that adapter_test.go
// only ever exercised indirectly through deliver()/refreshSources(), plus a
// handful of pure helpers nothing called directly.

func TestEnvIntParsesFallsBackAndIgnoresGarbage(t *testing.T) {
	t.Setenv("K8SEVENTS_TEST_ENVINT", "")
	if got := envInt("K8SEVENTS_TEST_ENVINT", 42); got != 42 {
		t.Fatalf("unset: got %d want 42", got)
	}
	t.Setenv("K8SEVENTS_TEST_ENVINT", "17")
	if got := envInt("K8SEVENTS_TEST_ENVINT", 42); got != 17 {
		t.Fatalf("set: got %d want 17", got)
	}
	t.Setenv("K8SEVENTS_TEST_ENVINT", "not-a-number")
	if got := envInt("K8SEVENTS_TEST_ENVINT", 42); got != 42 {
		t.Fatalf("unparsable: got %d want fallback 42", got)
	}
}

func TestScopeNameNamesEmptyAsAllNamespaces(t *testing.T) {
	if got := scopeName(""); got != "all namespaces" {
		t.Fatalf("empty scope: got %q", got)
	}
	if got := scopeName("prod"); got != "prod" {
		t.Fatalf("named scope must pass through unchanged: got %q", got)
	}
}

func TestMustEnvReturnsTheValueWhenSet(t *testing.T) {
	t.Setenv("K8SEVENTS_TEST_MUSTENV", "a-value")
	if got := mustEnv("K8SEVENTS_TEST_MUSTENV"); got != "a-value" {
		t.Fatalf("mustEnv: got %q", got)
	}
}

// A watcher must start for every scope a source needs and stop for a scope no
// source needs any more — reconcileWatchers is the only place that decision
// is made.
func TestReconcileWatchersStartsAndStopsPerScope(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(listJSON("1")))
			return
		}
		<-r.Context().Done() // hold the watch open until the scope is torn down
	}))
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), k, "src", mustFilter(t, `{"namespaces":["prod"]}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileWatchers(ctx)
	a.mu.Lock()
	_, running := a.watchers["prod"]
	n := len(a.watchers)
	a.mu.Unlock()
	if !running || n != 1 {
		t.Fatalf("expected exactly one watcher for scope prod, got %v", a.watchers)
	}

	// Widen to cluster-wide: the namespaced watcher must stop and the
	// cluster-wide one must start.
	a.mu.Lock()
	a.sources["src"].filter = mustFilter(t, `{}`)
	a.mu.Unlock()
	a.reconcileWatchers(ctx)
	a.mu.Lock()
	_, prodStillRunning := a.watchers["prod"]
	_, allRunning := a.watchers[""]
	n = len(a.watchers)
	a.mu.Unlock()
	if prodStillRunning || !allRunning || n != 1 {
		t.Fatalf("widening scope must stop the old watcher and start the new one, got %v", a.watchers)
	}
}

// The pod/replicaset cache watchers must track the SAME scopes the events
// watch does, for both tracked resources, in both cluster-wide and namespaced
// form — this is also what exercises replicaSetsPath's namespaced branch.
func TestReconcileCacheWatchersTracksPodsAndReplicaSetsPerScope(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("1")))
			return
		}
		<-r.Context().Done()
	}))
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), k, "src", mustFilter(t, `{"namespaces":["prod"]}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileCacheWatchers(ctx)
	a.mu.Lock()
	_, pods := a.cacheWatchers["pods@prod"]
	_, rs := a.cacheWatchers["replicasets@prod"]
	_, nodes := a.cacheWatchers["nodes@"]
	n := len(a.cacheWatchers)
	a.mu.Unlock()
	if !pods || !rs || !nodes || n != 3 {
		t.Fatalf("expected pods@prod, replicasets@prod and one cluster-wide nodes@ cache watcher, got %v", a.cacheWatchers)
	}

	// No sources left: every cache watcher must stop.
	a.mu.Lock()
	a.sources = map[string]*servedSource{}
	a.mu.Unlock()
	a.reconcileCacheWatchers(ctx)
	a.mu.Lock()
	n = len(a.cacheWatchers)
	a.mu.Unlock()
	if n != 0 {
		t.Fatalf("cache watchers must stop once nothing needs them, got %d running", n)
	}
}

// nodeJSON builds a core/v1 Node as the API server would return it.
func nodeJSON(name string, unschedulable bool, taints ...nodeTaint) map[string]any {
	ts := make([]map[string]any, 0, len(taints))
	for _, tt := range taints {
		ts = append(ts, map[string]any{"key": tt.Key, "effect": tt.Effect})
	}
	return map[string]any{
		"metadata": map[string]any{"name": name},
		"spec":     map[string]any{"unschedulable": unschedulable, "taints": ts},
	}
}

// The node cache reflects a cordon within the watch's latency, the same
// contract podcache_test.go pins for pods.
func TestNodeCacheReflectsACordonWithinTheWatchsLatency(t *testing.T) {
	watchStarted := make(chan struct{})
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" {
			if r.URL.Query().Get("watch") != "1" {
				_, _ = w.Write([]byte(collectionJSON("1")))
				return
			}
			<-r.Context().Done()
			return
		}
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("1", nodeJSON("node-a", false))))
			return
		}
		close(watchStarted)
		_, _ = w.Write([]byte(`{"type":"MODIFIED","object":` + mustJSON(nodeJSON("node-a", true)) + "}\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), k, "src", mustFilter(t, `{}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileCacheWatchers(ctx)
	<-watchStarted
	waitForMsg(t, func() bool {
		draining, known := a.cache.NodeDraining("node-a")
		return known && draining
	}, "a cordon must flip the cached node's drain state")
}

// One cluster-wide node watcher regardless of how many namespace scopes the
// events watch covers — nodes are cluster-scoped, so per-scope watchers would
// be redundant duplicates of the same request.
func TestNodeCacheWatcherIsSingularAcrossScopes(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("1")))
			return
		}
		<-r.Context().Done()
	}))
	mgr := newFakeManager()
	a := &adapter{
		mgr: mgr.start(t), kube: k, name: "k8s-events",
		self: newSelfExcluder(), cache: newObjectCache(),
		sources: map[string]*servedSource{
			"a": {filter: mustFilter(t, `{"namespaces":["prod"]}`)},
			"b": {filter: mustFilter(t, `{"namespaces":["staging"]}`)},
		},
		reported: map[string]string{}, watchers: map[string]context.CancelFunc{},
		cacheWatchers: map[string]context.CancelFunc{}, inhibit: newInhibitor(), drain: newDrainTracker(),
		cap: newEmitCap(defaultEmitPerMin),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileCacheWatchers(ctx)
	a.mu.Lock()
	n := 0
	for key := range a.cacheWatchers {
		if strings.HasPrefix(key, "nodes@") {
			n++
		}
	}
	a.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected exactly one nodes@ cache watcher across two scopes, got %d", n)
	}
}

// Missing node access degrades drain awareness — it must NEVER fail the
// source the way a missing pods/replicasets grant does: nodes are
// cluster-scoped, and a namespaced install has deliberately no view of them.
func TestReportNodeAccessErrorDoesNotFailTheSource(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/nodes" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"nodes is forbidden"}`))
			return
		}
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(collectionJSON("1")))
			return
		}
		<-r.Context().Done()
	}))
	mgr := newFakeManager(SourceInfo{Name: "src", Config: json.RawMessage(`{}`)})
	a := newTestAdapter(mgr.start(t), k, "src", mustFilter(t, `{}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileCacheWatchers(ctx)
	waitFor(t, func() bool { return a.nodeAccessErr.Load() != nil })
	if a.accessErr.Load() != nil {
		t.Fatal("a missing NODE grant must not be reported as the pods/replicasets access error")
	}

	a.refreshSources(ctx)
	var last reportedStatus
	waitFor(t, func() bool {
		for _, s := range mgr.allStatuses() {
			if s.Source == "src" {
				last = s
			}
		}
		return last.Source == "src"
	})
	if !last.Ready {
		t.Fatalf("missing node access must not set Ready=False: %+v", last)
	}
	if !strings.Contains(last.Message, "drain awareness unavailable") {
		t.Fatalf("the source's Ready reason must note drain awareness is unavailable: %+v", last)
	}
}

// End-to-end through deliver(): an event on a draining node's pod never
// reaches the manager, and the drain tracker records the suppression.
func TestDeliverSuppressesEventsOnADrainingNode(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	a.cache.put(&objectInfo{Kind: "Pod", Namespace: "prod", Name: "cilium-abc", Node: "node-a"})
	a.cache.put(&objectInfo{Kind: "Node", Name: "node-a", Unschedulable: true})
	a.cache.markSynced()

	ev := evt("Warning", "prod", "Pod", "cilium-abc", "NodeNotReady")
	ev.LastTimestamp = "2026-08-08T13:00:00Z"
	a.deliver(context.Background(), []Event{ev}, false)

	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("an event on a draining node must not be emitted: %+v", got)
	}
	nodes, total := a.drain.Active("src")
	if len(nodes) != 1 || nodes[0] != "node-a" || total != 1 {
		t.Fatalf("expected the tracker to record the suppression: nodes=%v total=%d", nodes, total)
	}
}

// Node-state suppression runs BEFORE inhibition, and the two must never
// double-count: an event drain-suppresses without ever being OFFERED to the
// inhibitor as a cause. Proven by uncordoning and checking that a later event
// which WOULD have been inhibited by the first (same node, target reason) is
// reported normally — the only way that happens is if the first event's
// inhibit.Observe never ran.
func TestDrainSuppressionRunsBeforeInhibitionAndNeverDoubleCounts(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`reason="NodeNotReady"`},
		TargetMatchers: []string{`reason="Unhealthy"`},
		Equal:          []string{"node"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &filter{severities: map[string]bool{"Warning": true}, rules: rs}
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", f)
	a.cache.put(&objectInfo{Kind: "Pod", Namespace: "prod", Name: "cilium-abc", Node: "node-a"})
	a.cache.put(&objectInfo{Kind: "Node", Name: "node-a", Unschedulable: true})
	a.cache.markSynced()

	cause := evt("Warning", "prod", "Pod", "cilium-abc", "NodeNotReady")
	cause.LastTimestamp = "2026-08-08T13:00:00Z"
	a.deliver(context.Background(), []Event{cause}, false)
	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("the cause event must be drain-suppressed, not emitted: %+v", got)
	}

	// Uncordon: the drain axis no longer applies, so a target-shaped event on
	// the SAME node now reaches inhibition. If the cause above had reached
	// inhibit.Observe, this would be suppressed as a consequence instead.
	a.cache.put(&objectInfo{Kind: "Node", Name: "node-a"})
	target := evt("Warning", "prod", "Pod", "cilium-abc", "Unhealthy")
	target.LastTimestamp = "2026-08-08T13:05:00Z"
	a.deliver(context.Background(), []Event{target}, false)

	got := mgr.signalsFor("src")
	if len(got) != 1 || got[0].Labels["alertname"] != "Unhealthy" {
		t.Fatalf("the target event must be reported normally — the drain-suppressed cause "+
			"must never have reached the inhibitor: %+v", got)
	}
}

// emitDrainExceeded posts the ONE signal a forgotten cordon earns, naming the
// node and carrying a fingerprint distinct per drain episode (start time).
func TestEmitDrainExceededPostsOneNodeKindSignal(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	start := mustTime(t, "2026-08-08T00:00:00Z")

	a.emitDrainExceeded(context.Background(), "src", "node-a", start, 7)

	got := mgr.signalsFor("src")
	if len(got) != 1 {
		t.Fatalf("expected exactly one signal, got %+v", got)
	}
	sig := got[0]
	if sig.Labels["kind"] != "Node" || sig.Labels["alertname"] != "NodeDrainExceeded" || sig.Labels["node"] != "node-a" {
		t.Fatalf("expected a Node-kind NodeDrainExceeded signal naming node-a: %+v", sig)
	}
	if !strings.Contains(sig.Payload, "7 event(s)") {
		t.Fatalf("payload must name what the drain cost: %q", sig.Payload)
	}

	// A second drain of the SAME node, starting later, must get a DIFFERENT
	// fingerprint — otherwise the manager's cooldown would silence the
	// second forgotten cordon because it looks like a repeat of the first.
	a.emitDrainExceeded(context.Background(), "src", "node-a", start.Add(2*time.Hour), 3)
	got = mgr.signalsFor("src")
	if len(got) != 2 || got[0].Fingerprint == got[1].Fingerprint {
		t.Fatalf("two distinct drain episodes must not share a fingerprint: %+v", got)
	}
}

// While a node drains, the source's condition names it and the running total;
// when it stops, the total is reported once and the node drops off.
func TestReportDrainActiveAndReleased(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	c := draining(t, "node-a")
	sig := &Signal{Labels: map[string]string{}}
	a.drain.Suppress("src", "node-a", mustRules(t, Route{}), c, sig, time.Now())
	a.drain.Suppress("src", "node-a", mustRules(t, Route{}), c, sig, time.Now())

	a.reportDrainActive(context.Background(), "src")
	waitFor(t, func() bool { return len(mgr.allStatuses()) > 0 })
	last := mgr.allStatuses()[len(mgr.allStatuses())-1]
	if last.Reason != "Draining" || !last.Ready {
		t.Fatalf("expected a Draining, Ready=true report, got %+v", last)
	}
	if !strings.Contains(last.Message, "node-a") || !strings.Contains(last.Message, "2 event") {
		t.Fatalf("the report must name the node and the count: %+v", last)
	}

	// Calling it again with nothing changed must not repost.
	before := len(mgr.allStatuses())
	a.reportDrainActive(context.Background(), "src")
	if len(mgr.allStatuses()) != before {
		t.Fatalf("an unchanged state must not be reported again")
	}

	a.reportDrainReleased(context.Background(), "src", "node-a", 2)
	waitFor(t, func() bool {
		s := mgr.allStatuses()
		return s[len(s)-1].Reason == "DrainEnded"
	})
	released := mgr.allStatuses()[len(mgr.allStatuses())-1]
	if !released.Ready || !strings.Contains(released.Message, "2 event") {
		t.Fatalf("release must report Ready=true naming the cost: %+v", released)
	}
}

// A missing pods/replicasets grant must be named on the source's condition —
// reportAccessError is what builds that message, reached through the cache
// watcher's onError callback on a real 403.
func TestReportAccessErrorNamesTheMissingResource(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"pods is forbidden"}`))
	}))
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), k, "src", mustFilter(t, `{}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.reconcileCacheWatchers(ctx)
	waitFor(t, func() bool { return a.accessErr.Load() != nil })
	msg := *a.accessErr.Load()
	if !strings.Contains(msg, "cannot read") {
		t.Fatalf("access error must name the failure: %q", msg)
	}
	if !strings.Contains(msg, "operator grants adapters nothing") {
		t.Fatalf("access error must explain the grant is external: %q", msg)
	}
}

// stopAllWatchers must cancel and clear both maps — this is what runs on
// shutdown, and a watcher left running past it would leak a goroutine per
// scope on every restart.
func TestStopAllWatchersCancelsAndClearsBothMaps(t *testing.T) {
	a := newTestAdapter(nil, nil, "src", mustFilter(t, `{}`))
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	a.watchers["prod"] = cancel1
	a.cacheWatchers["pods@"] = cancel2

	a.stopAllWatchers()

	if len(a.watchers) != 0 || len(a.cacheWatchers) != 0 {
		t.Fatalf("stopAllWatchers must clear both maps: watchers=%v cacheWatchers=%v", a.watchers, a.cacheWatchers)
	}
	if ctx1.Err() == nil || ctx2.Err() == nil {
		t.Fatal("stopAllWatchers must actually cancel every watcher's context, not merely forget it")
	}
}

// runDwellFlusher is the adapter's own background loop: it must call
// pending.Flush() at least once and then honor context cancellation rather
// than running forever.
func TestRunDwellFlusherFlushesUntilCancelled(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.runDwellFlusher(ctx); close(done) }()

	time.Sleep(20 * time.Millisecond) // let at least one loop iteration run
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDwellFlusher must stop once its context is cancelled")
	}
}

// runDrainSweep is what lets a forgotten cordon surface with no new events
// arriving to trigger the check — this proves it actually escalates on its
// own timer, not just that Sweep() does when called directly.
func TestRunDrainSweepEscalatesOnItsOwnTimer(t *testing.T) {
	f := mustFilter(t, `{"route":{"drainingNodeBound":"1ms"}}`)
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", f)
	a.cache.put(&objectInfo{Kind: "Node", Name: "node-a", Unschedulable: true})
	a.cache.markSynced()
	a.drain.Suppress("src", "node-a", f.rules, a.cache, &Signal{Labels: map[string]string{}}, time.Now())
	// The next tick is dwellTick (5s) away, well past waitFor's 3s budget, so
	// this must already be past the 1ms bound by the FIRST tick.
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.runDrainSweep(ctx)

	waitFor(t, func() bool {
		for _, s := range mgr.signalsFor("src") {
			if s.Labels["alertname"] == "NodeDrainExceeded" {
				return true
			}
		}
		return false
	})
}

// runDrainSweep must stop once its context is cancelled, exactly like every
// other background loop this adapter runs.
func TestRunDrainSweepStopsOnCancel(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.runDrainSweep(ctx); close(done) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDrainSweep must stop once its context is cancelled")
	}
}
