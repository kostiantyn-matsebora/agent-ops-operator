// signal-k8s-events: a signal adapter that turns Kubernetes Events into
// conversations. It serves SignalSources whose spec.adapter names its
// SignalAdapter CR, watching core v1 Events through the in-cluster API with its
// own ServiceAccount token — no client-go, no operator-granted permissions.
//
// Chain of proof for the contract:
//
//	config:   {"severities":["Warning"],"namespaces":[],"includeReasons":[]}   (opaque spec.config)
//	cursor:   state key "last-event" via /signal/state (restart-safe)
//	emitting: POST /signal/inbound, kind=alert, fingerprint "<source>@<ns>/<kind>/<name>/<reason>"
//
// The adapter normalizes and nothing more: no dedup, no grouping, no cooldown
// beyond its restart cursor. Repetition collapses manager-side, which is why
// the fingerprint keys on the involved OBJECT and REASON rather than on the
// Event object (Kubernetes recreates those for a recurring problem).
//
// The SA token it mounts confers only what the chart bound to
// agentops-signal-<name> out of band — the operator grants adapters nothing.
// Run single-instance (the reconciler's singleton default): two watchers would
// double-post every event.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default "k8s-events").
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	cursorKey      = "last-event"
	refreshEvery   = 30 * time.Second
	relistBackoff  = 5 * time.Second
	maxBatchPerSrc = 50
)

// servedSource is one SignalSource this adapter serves.
type servedSource struct {
	filter *filter
	// cursor is the newest event timestamp already emitted. Events at or before
	// it are skipped on a startup/relist pass so a restart does not replay the
	// cluster's entire event backlog.
	cursor time.Time
}

type adapter struct {
	mgr  *Manager
	kube *Kube
	name string

	mu       sync.Mutex
	sources  map[string]*servedSource
	reported map[string]string // last reported status per source (avoid spam)

	// watchers maps a namespace scope to its cancel func.
	watchers map[string]context.CancelFunc
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func main() {
	name := os.Getenv("ADAPTER_NAME")
	if name == "" {
		name = "k8s-events"
	}
	kube, err := NewInClusterKube()
	if err != nil {
		log.Fatalf("kubernetes access: %v (this adapter needs kubernetesAccess: true on its SignalAdapter "+
			"and an events get/list/watch grant bound to its ServiceAccount)", err)
	}
	a := &adapter{
		mgr:      NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		kube:     kube,
		name:     name,
		sources:  map[string]*servedSource{},
		reported: map[string]string{},
		watchers: map[string]context.CancelFunc{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-k8s-events adapter starting (adapter=%s)", name)

	for ctx.Err() == nil {
		a.refreshSources(ctx)
		a.reconcileWatchers(ctx)
		sleepCtx(ctx, refreshEvery)
	}
	a.stopAllWatchers()
}

// refreshSources re-reads served sources, validates config, and reports
// validity changes as each source's Ready condition. An invalid source is
// dropped from the served set; the others keep flowing.
func (a *adapter) refreshSources(ctx context.Context) {
	infos, err := a.mgr.Sources(ctx, a.name)
	if err != nil {
		log.Printf("list sources: %v", err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	next := map[string]*servedSource{}
	for _, info := range infos {
		f, err := parseConfig(info.Config)
		if err != nil {
			if a.reported[info.Name] != err.Error() {
				go func(n, msg string) { _ = a.mgr.ReportStatus(ctx, n, false, "InvalidConfig", msg) }(info.Name, err.Error())
				a.reported[info.Name] = err.Error()
			}
			continue
		}
		if a.reported[info.Name] != "ok" {
			go func(n string) {
				_ = a.mgr.ReportStatus(ctx, n, true, "AdapterReady", "served by signal-k8s-events")
			}(info.Name)
			a.reported[info.Name] = "ok"
		}
		src := &servedSource{filter: f}
		if prev, ok := a.sources[info.Name]; ok {
			src.cursor = prev.cursor // config edits must not replay history
		} else {
			src.cursor = a.loadCursor(ctx, info.Name)
		}
		next[info.Name] = src
	}
	a.sources = next
}

// loadCursor reads a source's persisted cursor. A missing or unparsable cursor
// starts at "now": a first-time source must not open conversations for every
// warning already sitting in the cluster's event backlog.
func (a *adapter) loadCursor(ctx context.Context, source string) time.Time {
	raw, err := a.mgr.GetState(ctx, source, cursorKey)
	if err != nil {
		log.Printf("reading cursor for %s: %v", source, err)
		return time.Now().UTC()
	}
	if raw == "" {
		now := time.Now().UTC()
		if err := a.mgr.PutState(ctx, source, cursorKey, now.Format(time.RFC3339)); err != nil {
			log.Printf("seeding cursor for %s: %v", source, err)
		}
		return now
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		log.Printf("resetting unparsable cursor for %s: %q", source, raw)
		return time.Now().UTC()
	}
	return t.UTC()
}

// desiredScopes is the minimal set of namespace watches covering every source.
// One cluster-wide source makes per-namespace watches redundant.
func (a *adapter) desiredScopes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	scopes := map[string]bool{}
	for _, src := range a.sources {
		for _, s := range src.filter.Scopes() {
			if s == "" {
				return []string{""}
			}
			scopes[s] = true
		}
	}
	out := make([]string, 0, len(scopes))
	for s := range scopes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// reconcileWatchers starts a watcher per desired scope and stops the rest.
func (a *adapter) reconcileWatchers(ctx context.Context) {
	want := map[string]bool{}
	for _, s := range a.desiredScopes() {
		want[s] = true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for scope, cancel := range a.watchers {
		if !want[scope] {
			cancel()
			delete(a.watchers, scope)
			log.Printf("stopped watcher for scope %q", scopeName(scope))
		}
	}
	for scope := range want {
		if _, running := a.watchers[scope]; running {
			continue
		}
		wctx, cancel := context.WithCancel(ctx)
		a.watchers[scope] = cancel
		go a.watchScope(wctx, scope)
		log.Printf("watching events in scope %q", scopeName(scope))
	}
}

func (a *adapter) stopAllWatchers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for scope, cancel := range a.watchers {
		cancel()
		delete(a.watchers, scope)
	}
}

// watchScope runs the list-then-watch loop for one namespace scope, relisting
// whenever the watch expires or the stream breaks.
func (a *adapter) watchScope(ctx context.Context, scope string) {
	for ctx.Err() == nil {
		events, rv, err := a.kube.ListEvents(ctx, scope)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("listing events in %q: %v", scopeName(scope), err)
				sleepCtx(ctx, relistBackoff)
			}
			continue
		}
		// The initial list is the replay-prone one: filter it against each
		// source's cursor so a restart re-emits nothing.
		a.deliver(ctx, events, true)

		err = a.kube.WatchEvents(ctx, scope, rv, func(ev Event) {
			a.deliver(ctx, []Event{ev}, false)
		})
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrWatchExpired):
			log.Printf("watch expired for %q — relisting", scopeName(scope))
		case err != nil:
			log.Printf("watch %q: %v — relisting", scopeName(scope), err)
			sleepCtx(ctx, relistBackoff)
		}
	}
}

// deliver fans a batch of events out to every source whose filter matches,
// posts one contract call per source, and advances that source's cursor.
//
// Post-then-persist: a crash between the two re-posts the same fingerprints,
// which the manager's cooldown absorbs. Losing an event would be the worse
// failure, so the ordering is deliberate.
func (a *adapter) deliver(ctx context.Context, events []Event, fromList bool) {
	a.mu.Lock()
	names := make([]string, 0, len(a.sources))
	for name := range a.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	type pending struct {
		signals []Signal
		newest  time.Time
	}
	batches := map[string]*pending{}
	for _, name := range names {
		src := a.sources[name]
		p := &pending{}
		for i := range events {
			ev := &events[i]
			if !src.filter.Matches(ev) {
				continue
			}
			when := ev.When()
			// Only the list pass consults the cursor: during a live watch every
			// frame is news, and an event whose timestamps are all unparsable
			// (zero) must not be silently dropped.
			if fromList && !when.IsZero() && !when.After(src.cursor) {
				continue
			}
			p.signals = append(p.signals, normalize(name, ev))
			if when.After(p.newest) {
				p.newest = when
			}
			if len(p.signals) >= maxBatchPerSrc {
				break
			}
		}
		if len(p.signals) > 0 {
			batches[name] = p
		}
	}
	a.mu.Unlock()

	for name, p := range batches {
		if err := a.mgr.Inbound(ctx, name, p.signals); err != nil {
			if ctx.Err() == nil {
				log.Printf("posting %d signals for %s: %v", len(p.signals), name, err)
			}
			continue // cursor not advanced — the next pass retries
		}
		log.Printf("posted %d event signal(s) for %s", len(p.signals), name)
		if p.newest.IsZero() {
			continue
		}
		a.mu.Lock()
		src, ok := a.sources[name]
		if ok && p.newest.After(src.cursor) {
			src.cursor = p.newest
		}
		a.mu.Unlock()
		if ok {
			if err := a.mgr.PutState(ctx, name, cursorKey, p.newest.Format(time.RFC3339)); err != nil && ctx.Err() == nil {
				log.Printf("persisting cursor for %s: %v", name, err)
			}
		}
	}
}

func scopeName(scope string) string {
	if scope == "" {
		return "all namespaces"
	}
	return scope
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
