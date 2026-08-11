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
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	cursorKey      = "last-event"
	refreshEvery   = 30 * time.Second
	relistBackoff  = 5 * time.Second
	maxBatchPerSrc = 50
	// dwellTick is how often closed windows are decided. Fine enough that a
	// `for: 30s` rule is not meaningfully delayed, coarse enough to cost
	// nothing when the queue is empty.
	dwellTick = 5 * time.Second
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
	// self drops events about agent-ops' own machinery before anything else
	// looks at them. See selfexclude.go for why this is an invariant rather
	// than a filter.
	self *selfExcluder

	// cache is the trimmed pod/replicaset view backing workload enrichment,
	// the dwell liveness re-check, and self-exclusion mechanism 2.
	cache *objectCache
	// pending holds matched signals through their `for` window.
	pending *pendingQueue
	// inhibit suppresses consequences of an already-reported cause.
	inhibit *inhibitor
	// cap bounds signals per source per minute, reporting what it clips.
	cap *emitCap
	// clipped records the last reported clip count per source, so the
	// condition is only rewritten when it changes.
	clipped sync.Map
	// muted records the active mute window and its running count per source, so
	// the condition changes when the situation does rather than per batch.
	muted sync.Map

	mu       sync.Mutex
	sources  map[string]*servedSource
	reported map[string]string // last reported status per source (avoid spam)

	// watchers maps a namespace scope to its cancel func.
	watchers map[string]context.CancelFunc
	// cacheWatchers maps "<resource>@<scope>" to its cancel func. Kept
	// separate from watchers because the two share scopes but not lifetimes.
	cacheWatchers map[string]context.CancelFunc

	// accessErr holds a permission failure reading pods/replicasets. The grant
	// is always external (the operator grants adapters nothing), so a 403 is
	// not something waiting fixes — every served source must say so.
	accessErr atomic.Pointer[string]
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
	cache := newObjectCache()
	a := &adapter{
		mgr:           NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		kube:          kube,
		name:          name,
		cache:         cache,
		self:          newSelfExcluder().withCache(cache),
		sources:       map[string]*servedSource{},
		reported:      map[string]string{},
		watchers:      map[string]context.CancelFunc{},
		cacheWatchers: map[string]context.CancelFunc{},
		inhibit:       newInhibitor(),
		cap:           newEmitCap(envInt("EMIT_PER_MINUTE", defaultEmitPerMin)),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	a.pending = newPendingQueue(a.health, func(source string, sigs []Signal) {
		a.post(ctx, source, sigs)
	})
	go a.runDwellFlusher(ctx)
	log.Printf("signal-k8s-events adapter starting (adapter=%s)", name)

	for ctx.Err() == nil {
		a.refreshSources(ctx)
		a.reconcileWatchers(ctx)
		a.reconcileCacheWatchers(ctx)
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
		// A missing pods/replicasets grant is reported on every served source
		// rather than degrading silently: without the cache there is no
		// workload label and no liveness re-check, so the source would look
		// healthy while doing markedly less than it claims.
		state, ready, reason, message := "ok", true, "AdapterReady", "served by signal-k8s-events"
		if msg := a.accessErr.Load(); msg != nil {
			state, ready, reason, message = "access:"+*msg, false, "MissingPermissions", *msg
		}
		if a.reported[info.Name] != state {
			go func(n string) { _ = a.mgr.ReportStatus(ctx, n, ready, reason, message) }(info.Name)
			a.reported[info.Name] = state
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

// reconcileCacheWatchers keeps the pod/replicaset cache covering exactly the
// scopes the events watch covers.
//
// Sharing desiredScopes() is not an optimization, it is a requirement: the
// chart renders a NAMESPACED Role when rbac.clusterWide is false, and the
// pods/replicasets grant is namespaced identically. A cluster-wide pod watch
// would 403 outright in that supported configuration.
func (a *adapter) reconcileCacheWatchers(ctx context.Context) {
	want := map[string]bool{}
	for _, s := range a.desiredScopes() {
		want[s] = true
	}
	specs := []struct {
		resource string
		watcher  func() *cacheWatcher
	}{
		{"pods", func() *cacheWatcher {
			return &cacheWatcher{kube: a.kube, cache: a.cache, kind: "Pod",
				pathFor: podsPath, decode: decodePod, onError: a.reportAccessError("pods")}
		}},
		{"replicasets", func() *cacheWatcher {
			return &cacheWatcher{kube: a.kube, cache: a.cache, kind: "ReplicaSet",
				pathFor: replicaSetsPath, decode: decodeReplicaSet, onError: a.reportAccessError("replicasets")}
		}},
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	live := map[string]bool{}
	for _, spec := range specs {
		for scope := range want {
			live[spec.resource+"@"+scope] = true
		}
	}
	for key, cancel := range a.cacheWatchers {
		if !live[key] {
			cancel()
			delete(a.cacheWatchers, key)
		}
	}
	for _, spec := range specs {
		for scope := range want {
			key := spec.resource + "@" + scope
			if _, running := a.cacheWatchers[key]; running {
				continue
			}
			wctx, cancel := context.WithCancel(ctx)
			a.cacheWatchers[key] = cancel
			w := spec.watcher()
			go w.run(wctx, scope)
			log.Printf("caching %s in scope %q", spec.resource, scopeName(scope))
		}
	}
}

// reportAccessError records a permission failure for one resource so every
// served source reports it. The operator grants adapters nothing, so a 403 is
// an external grant that is missing — naming it is the only useful response.
func (a *adapter) reportAccessError(resource string) func(error) {
	return func(err error) {
		msg := "cannot read " + resource + ": " + err.Error() +
			" — this adapter needs list/watch on " + resource +
			" bound to its ServiceAccount (the operator grants adapters nothing; the chart binds it)"
		a.accessErr.Store(&msg)
	}
}

func (a *adapter) stopAllWatchers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for scope, cancel := range a.watchers {
		cancel()
		delete(a.watchers, scope)
	}
	for key, cancel := range a.cacheWatchers {
		cancel()
		delete(a.cacheWatchers, key)
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
			// Self-exclusion runs FIRST — before matching, before the cursor.
			// An event about agent-ops' own machinery must not reach the
			// filters at all: no rule may re-admit it, and it must not
			// advance a cursor as though it had been considered.
			if excluded, _ := a.self.Excludes(ev, src.filter.includeOwnNamespace); excluded {
				continue
			}
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
			sig := normalize(name, ev, a.enrich(ev))
			rule, hasRule := src.filter.Rule(&sig)
			if hasRule && rule.drop {
				continue
			}

			// Inhibition before the dwell queue: a consequence of an already
			// reported cause must not even occupy a window.
			a.inhibit.Observe(name, src.filter.rules, &sig)
			if a.inhibit.Inhibited(name, src.filter.rules, &sig) {
				continue
			}

			// The cursor advances for a deferred signal too. A restart inside a
			// window loses at most that window, and a problem real enough to
			// have survived verification is still emitting events — so it is
			// re-detected within the minute. Holding the cursor back instead
			// would replay the whole window on every restart.
			if when.After(p.newest) {
				p.newest = when
			}

			if hasRule && rule.dwell > 0 {
				a.pending.Add(name, sig, rule)
				continue
			}
			p.signals = append(p.signals, sig)
			if len(p.signals) >= maxBatchPerSrc {
				break
			}
		}
		// A batch with no immediate signals but a fresh timestamp still matters:
		// its events went into the dwell queue, and the cursor must move.
		if len(p.signals) > 0 || !p.newest.IsZero() {
			batches[name] = p
		}
	}
	a.mu.Unlock()

	for name, p := range batches {
		if len(p.signals) > 0 && !a.post(ctx, name, p.signals) {
			continue // cursor not advanced — the next pass retries
		}
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

// post sends a batch through the emit cap and reports clipping. It is the
// single exit from this adapter: both the immediate path and the dwell queue
// go through it, so the cap cannot be bypassed by one of them.
func (a *adapter) post(ctx context.Context, source string, sigs []Signal) bool {
	// MUTE FIRST, and note WHERE this sits: after the dwell queue (both callers
	// have already been through it) and before the emit cap.
	//
	// After the dwell, because Alertmanager's mute intervals mute NOTIFICATIONS
	// rather than evaluation — which buys the property that makes the feature
	// safe: a problem that starts inside the window and PERSISTS past it still
	// surfaces, because the cluster keeps producing events for anything genuinely
	// broken and the next one dwells and emits normally once the window closes.
	// Only transient noise is lost, which is what the window was asked to lose.
	//
	// Before the cap, because muted signals were never emitted; charging them
	// would make a maintenance window read as a runaway.
	sigs, muted, window := a.applyMute(source, sigs, time.Now())
	a.reportMute(ctx, source, muted, window)
	if len(sigs) == 0 {
		return false
	}
	allowed, clipped := a.cap.Allow(source, sigs)
	a.reportClipping(ctx, source, clipped)
	if len(allowed) == 0 {
		return false
	}
	if err := a.mgr.Inbound(ctx, source, allowed); err != nil {
		if ctx.Err() == nil {
			log.Printf("posting %d signals for %s: %v", len(allowed), source, err)
		}
		return false
	}
	log.Printf("posted %d event signal(s) for %s", len(allowed), source)
	return true
}


// applyMute drops the signals a time window silences, returning what survives,
// how many were muted, and the window that did it.
func (a *adapter) applyMute(source string, sigs []Signal, now time.Time) ([]Signal, int, string) {
	a.mu.Lock()
	src, ok := a.sources[source]
	a.mu.Unlock()
	if !ok || src.filter == nil || src.filter.rules == nil || len(src.filter.rules.mutes) == 0 {
		return sigs, 0, ""
	}
	kept := sigs[:0:0]
	muted, window := 0, ""
	for i := range sigs {
		if w := src.filter.rules.MutedBy(&sigs[i], now); w != "" {
			muted++
			window = w
			continue
		}
		kept = append(kept, sigs[i])
	}
	return kept, muted, window
}

// muteState is what a source has reported about muting, so the condition
// changes when the situation does rather than on every batch.
type muteState struct {
	window string
	count  int
}

// reportMute surfaces an ACTIVE window and, when it closes, what it cost.
//
// Muting is NEVER silent. A muted lane and an idle lane are indistinguishable
// from outside and only one of them means the cluster is healthy, so "why has
// nothing arrived since four?" is answerable from the source object rather than
// from adapter logs. Same reasoning as emit-cap clipping, one axis over.
func (a *adapter) reportMute(ctx context.Context, source string, muted int, window string) {
	prev, _ := a.muted.Load(source)
	cur, _ := prev.(muteState)

	if muted > 0 {
		next := muteState{window: window, count: cur.count + muted}
		a.muted.Store(source, next)
		if cur.window == window {
			return // already reported this window; the count is summarised on close
		}
		msg := fmt.Sprintf("muted by time interval %q — events matching this window are being "+
			"suppressed on purpose; this lane is configured, not idle", window)
		log.Printf("%s: %s", source, msg)
		// Ready stays TRUE: the source is doing exactly what it was told. Marking
		// it unhealthy for obeying its own configuration would train an operator
		// to ignore the condition.
		go func() { _ = a.mgr.ReportStatus(ctx, source, true, "Muted", msg) }()
		return
	}

	// Nothing muted in this batch: if a window was active, it has closed.
	if cur.window == "" {
		return
	}
	a.muted.Store(source, muteState{})
	msg := fmt.Sprintf("time interval %q ended: %d event(s) were muted while it was active",
		cur.window, cur.count)
	log.Printf("%s: %s", source, msg)
	go func() { _ = a.mgr.ReportStatus(ctx, source, true, "MuteEnded", msg) }()
}

// reportClipping surfaces a clipped window on the source's condition. Silent
// clipping is how an incident goes missing, so it is reported the first time
// the count changes and never merely logged.
func (a *adapter) reportClipping(ctx context.Context, source string, clipped int) {
	if clipped == 0 {
		return
	}
	if prev, ok := a.clipped.Load(source); ok && prev.(int) == clipped {
		return
	}
	a.clipped.Store(source, clipped)
	msg := fmt.Sprintf("emit cap reached: %d signal(s) clipped in the last minute — "+
		"something is producing events far faster than an agent can act on them", clipped)
	log.Printf("%s: %s", source, msg)
	go func() { _ = a.mgr.ReportStatus(ctx, source, false, "EmitCapReached", msg) }()
}

// runDwellFlusher decides pending entries whose window has closed.
func (a *adapter) runDwellFlusher(ctx context.Context) {
	for ctx.Err() == nil {
		a.pending.Flush(time.Now())
		sleepCtx(ctx, dwellTick)
	}
}

// envInt reads an integer env var, falling back when unset or unparsable.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
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
