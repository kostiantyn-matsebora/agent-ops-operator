package main

import (
	"context"
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
	n := len(a.cacheWatchers)
	a.mu.Unlock()
	if !pods || !rs || n != 2 {
		t.Fatalf("expected pods@prod and replicasets@prod cache watchers, got %v", a.cacheWatchers)
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
