// signal-ha: a signal adapter that turns Home Assistant's log stream into
// conversations. It serves SignalSources whose spec.adapter names its
// SignalAdapter CR, reading each one's Home Assistant instance over that
// instance's WebSocket API — no Kubernetes client, no operator-granted
// permissions, no cluster access of any kind.
//
// Chain of proof for the contract:
//
//	config:   {"endpoint":"https://ha.example.org","levels":["ERROR"],"rules":[...]}
//	credential: AGENTOPS_CRED_<SOURCE>_token, projected by the reconciler
//	cursor:   state key "last-record" via /signal/state (restart-safe)
//	emitting: POST /signal/inbound, kind=alert, fingerprint "<source>@<logger>@<file:line>"
//
// The adapter normalizes and nothing more: no dedup, no grouping, no cooldown
// beyond its restart cursor. Repetition collapses manager-side, which is why
// the fingerprint keys on the LOGGER and SOURCE LOCATION — Home Assistant's own
// deduplication key — rather than on the occurrence.
//
// Run single-instance (the reconciler's singleton default): two sessions would
// double-post every record.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default
// "home-assistant"), EMIT_PER_MINUTE.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	cursorKey      = "last-record"
	refreshEvery   = 30 * time.Second
	connectBackoff = 10 * time.Second
	maxBatchPerSrc = 50
	// dwellTick is how often closed windows are decided. Fine enough that a
	// `for: 30s` rule is not meaningfully delayed, coarse enough to cost
	// nothing when the queue is empty.
	dwellTick = 5 * time.Second
	// cursorStaleSkew is how far the persisted cursor may sit ahead of the
	// newest record Home Assistant still holds before it is treated as no
	// longer valid upstream. Small clock differences are normal; a cursor
	// minutes ahead of a NON-EMPTY listing means the position it names is gone.
	cursorStaleSkew = 5 * time.Minute
)

// healthSnapshot is what one source's verification ladder reads: the current
// log listing keyed by record identity, and each integration's config-entry
// states. Refreshed on the dwell tick, and only while something is pending.
type healthSnapshot struct {
	records map[string]logRecord
	// entries maps an integration domain to the states of its config entries.
	// A domain absent from the map has no predicate — that is rung 2, not
	// health.
	entries map[string][]string
	at      time.Time
}

// failedEntryStates are config-entry states that mean an integration is broken
// rather than merely not loaded. `not_loaded` is deliberately absent: a
// disabled integration is a choice, not an incident.
var failedEntryStates = map[string]bool{
	"setup_error":     true,
	"setup_retry":     true,
	"migration_error": true,
	"failed_unload":   true,
}

// servedSource is one SignalSource this adapter serves.
type servedSource struct {
	filter *filter
	token  string
	// cursor is the newest record timestamp already considered. Records at or
	// before it are skipped on a backfill pass so a restart does not replay the
	// whole log.
	cursor time.Time
	// cancel stops this source's session runner.
	cancel context.CancelFunc
	// key identifies the connection parameters, so a config edit that changes
	// where or as whom we connect restarts the session and nothing else does.
	key string
	// snapshot is the verification ladder's view of this instance.
	snapshot *healthSnapshot
}

type adapter struct {
	mgr  *Manager
	name string
	// self drops records about agent-ops' own machinery before anything else
	// looks at them. See selfexclude.go for why this is an invariant rather
	// than a filter.
	self *selfExcluder

	// pending holds matched signals through their `for` window.
	pending *pendingQueue
	// inhibit suppresses consequences of an already-reported cause.
	inhibit *inhibitor
	// cap bounds signals per source per minute, reporting what it clips.
	cap *emitCap
	// clipped records the last reported clip count per source, so the
	// condition is only rewritten when it changes.
	clipped sync.Map

	mu       sync.Mutex
	sources  map[string]*servedSource
	reported map[string]string // last reported status per source (avoid spam)
	// postFailing marks sources whose last post to the manager failed, so the
	// failure is reported once and its recovery once.
	postFailing sync.Map
	// sessions holds the live Home Assistant session per source, for the
	// health snapshot refresh.
	sessions map[string]*haSession
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
		name = "home-assistant"
	}
	a := &adapter{
		mgr:      NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		name:     name,
		self:     newSelfExcluder(),
		sources:  map[string]*servedSource{},
		reported: map[string]string{},
		sessions: map[string]*haSession{},
		inhibit:  newInhibitor(),
		cap:      newEmitCap(envInt("EMIT_PER_MINUTE", defaultEmitPerMin)),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	a.pending = newPendingQueue(a.health, func(source string, sigs []Signal) {
		a.post(ctx, source, sigs)
	})
	go a.runDwellFlusher(ctx)
	log.Printf("signal-ha adapter starting (adapter=%s)", name)

	for ctx.Err() == nil {
		a.refreshSources(ctx)
		sleepCtx(ctx, refreshEvery)
	}
	a.stopAll()
}

// refreshSources re-reads served sources, validates config and credentials, and
// reports changes as each source's Ready condition. An invalid source is
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
			a.reportLocked(ctx, info.Name, "config:"+err.Error(), false, "InvalidConfig", err.Error())
			continue
		}
		// A missing credential is reported, never worked around: connecting
		// anonymously would fail on every reconnect and read as an unreachable
		// host rather than as the configuration mistake it is.
		token, err := credentialToken(info)
		if err != nil {
			a.reportLocked(ctx, info.Name, "credential:"+err.Error(), false, "MissingCredential", err.Error())
			continue
		}

		src := &servedSource{filter: f, token: token, key: f.endpoint + "\x00" + token}
		if prev, ok := a.sources[info.Name]; ok {
			// Config edits must not replay history, and must not reconnect
			// unless they changed where or as whom we connect.
			src.cursor = prev.cursor
			src.snapshot = prev.snapshot
			if prev.key == src.key {
				src.cancel = prev.cancel
			} else {
				prev.cancel()
			}
		} else {
			src.cursor = a.loadCursor(ctx, info.Name)
		}
		next[info.Name] = src
		if src.cancel == nil {
			sctx, cancel := context.WithCancel(ctx)
			src.cancel = cancel
			go a.runSession(sctx, info.Name)
		}
	}
	for name, src := range a.sources {
		if _, kept := next[name]; !kept {
			src.cancel()
			delete(a.sessions, name)
			log.Printf("stopped session for %s", name)
		}
	}
	a.sources = next
}

// credentialToken finds the long-lived access token the reconciler projected
// for this source. Secret key `token`; `TOKEN` is accepted because Secrets
// written by hand often use it.
func credentialToken(info SourceInfo) (string, error) {
	if info.CredentialEnvPrefix == "" {
		return "", fmt.Errorf("no credentialsSecretRef on this SignalSource — set one holding a Home Assistant " +
			"long-lived access token under the key `token` (the manager reads no Secrets; the reconciler " +
			"projects it into this pod)")
	}
	for _, key := range []string{"token", "TOKEN"} {
		if v := os.Getenv(info.CredentialEnvPrefix + key); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("the projected credential Secret has no `token` key (expected env %stoken)",
		info.CredentialEnvPrefix)
}

// reportLocked posts a source's Ready condition when its reported state has
// changed. Caller holds a.mu.
func (a *adapter) reportLocked(ctx context.Context, source, state string, ready bool, reason, message string) {
	if a.reported[source] == state {
		return
	}
	a.reported[source] = state
	go func() { _ = a.mgr.ReportStatus(ctx, source, ready, reason, message) }()
}

func (a *adapter) report(ctx context.Context, source, state string, ready bool, reason, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reportLocked(ctx, source, state, ready, reason, message)
}

// loadCursor reads a source's persisted cursor. A missing or unparsable cursor
// starts at "now": a first-time source must not open conversations for every
// error already sitting in the instance's log.
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

// runSession keeps one source connected, reconnecting on failure.
func (a *adapter) runSession(ctx context.Context, source string) {
	for ctx.Err() == nil {
		endpoint, token, ok := a.connectionFor(source)
		if !ok {
			return
		}
		sess, err := haConnect(ctx, endpoint, token, func(eventType string, data json.RawMessage) {
			if eventType == "system_log_event" {
				a.handleEvent(ctx, source, data)
			}
		})
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("%s: %v", source, err)
				a.report(ctx, source, "connect:"+err.Error(), false, "Unreachable", err.Error())
				sleepCtx(ctx, connectBackoff)
			}
			continue
		}

		if err := a.startSession(ctx, source, sess); err != nil {
			if ctx.Err() == nil {
				log.Printf("%s: %v", source, err)
				a.report(ctx, source, "start:"+err.Error(), false, "Unreachable", err.Error())
			}
			sess.Close()
			sleepCtx(ctx, connectBackoff)
			continue
		}
		a.report(ctx, source, "ok", true, "AdapterReady", "connected to "+endpoint)

		select {
		case <-ctx.Done():
			sess.Close()
			return
		case <-sess.Done():
			a.mu.Lock()
			delete(a.sessions, source)
			a.mu.Unlock()
			if ctx.Err() == nil {
				err := sess.Err()
				log.Printf("%s: session ended: %v — reconnecting", source, err)
				a.report(ctx, source, "closed", false, "Disconnected",
					fmt.Sprintf("the Home Assistant connection ended (%v); reconnecting", err))
				sleepCtx(ctx, connectBackoff)
			}
		}
	}
}

// startSession subscribes, learns who this token is, and backfills.
func (a *adapter) startSession(ctx context.Context, source string, sess *haSession) error {
	cctx, cancel := context.WithTimeout(ctx, haCommandTimeout)
	defer cancel()
	if err := sess.SubscribeEvents(cctx, "system_log_event"); err != nil {
		return fmt.Errorf("subscribing to system_log_event: %w", err)
	}
	// Self-exclusion mechanism 3. A failure here is NOT fatal: mechanisms 1 and
	// 2 need no read, which is the entire reason there are three.
	if user, err := sess.CurrentUser(cctx); err != nil {
		log.Printf("%s: cannot read the token's own user (%v) — self-exclusion falls back to its two "+
			"read-free mechanisms", source, err)
	} else {
		a.self.learnUser(user)
	}
	a.mu.Lock()
	a.sessions[source] = sess
	a.mu.Unlock()
	a.refreshSnapshot(ctx, source, sess)
	a.backfill(ctx, source, sess)
	return nil
}

// backfill reports what was logged while this adapter was down.
//
// It is also where a stale cursor is recovered: a position minutes AHEAD of
// everything Home Assistant still holds names a record that is gone (the log
// was cleared, or this cursor was written against another instance), and
// keeping it would silence the source forever. Re-read instead of stalling.
func (a *adapter) backfill(ctx context.Context, source string, sess *haSession) {
	src := a.source(source)
	if src == nil || !src.filter.backfill {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, haCommandTimeout)
	defer cancel()
	records, err := sess.SystemLog(cctx)
	if err != nil {
		log.Printf("%s: backfill: %v", source, err)
		return
	}
	newest := time.Time{}
	for i := range records {
		if at := records[i].At(); at.After(newest) {
			newest = at
		}
	}
	cursor := src.cursor
	if len(records) > 0 && !cursor.IsZero() && newest.Add(cursorStaleSkew).Before(cursor) {
		log.Printf("%s: persisted cursor %s is ahead of everything Home Assistant still holds (newest %s) — "+
			"re-reading the whole log", source, cursor.Format(time.RFC3339), newest.Format(time.RFC3339))
		cursor = time.Time{}
		a.setCursor(source, time.Time{})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].At().Before(records[j].At()) })
	for i := range records {
		if at := records[i].At(); !at.IsZero() && !cursor.IsZero() && !at.After(cursor) {
			continue
		}
		a.consider(ctx, source, &records[i])
	}
}

// handleEvent turns one live system_log_event into a signal.
func (a *adapter) handleEvent(ctx context.Context, source string, data json.RawMessage) {
	var rec logRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		log.Printf("%s: undecodable system_log_event: %v", source, err)
		return
	}
	a.consider(ctx, source, &rec)
}

// consider runs one record through exclusion, scope, rules, inhibition and the
// dwell queue, posting whatever survives immediately.
func (a *adapter) consider(ctx context.Context, source string, rec *logRecord) {
	// Self-exclusion runs FIRST — before matching, before the cursor. A record
	// about agent-ops' own machinery must not reach the filters at all: no rule
	// may re-admit it, and it must not advance a cursor as though it had been
	// considered.
	if excluded, _ := a.self.Excludes(rec); excluded {
		return
	}

	a.mu.Lock()
	src, ok := a.sources[source]
	if !ok {
		a.mu.Unlock()
		return
	}
	f := src.filter
	a.mu.Unlock()

	if !f.Matches(rec) {
		return
	}
	sig := normalize(source, rec)
	rule, hasRule := f.Rule(&sig)
	if hasRule && rule.drop {
		a.advance(ctx, source, rec.At())
		return
	}

	// Inhibition before the dwell queue: a consequence of an already reported
	// cause must not even occupy a window.
	a.inhibit.Observe(source, f.rules, &sig)
	if a.inhibit.Inhibited(source, f.rules, &sig) {
		a.advance(ctx, source, rec.At())
		return
	}

	// The cursor advances for a deferred signal too. A restart inside a window
	// loses at most that window, and a problem real enough to survive
	// verification is still logging — so it is re-detected. Holding the cursor
	// back instead would replay the whole window on every restart.
	if hasRule && rule.dwell > 0 {
		a.pending.Add(source, sig, rule, rec.Count)
		a.advance(ctx, source, rec.At())
		return
	}
	if a.post(ctx, source, []Signal{sig}) {
		a.advance(ctx, source, rec.At())
	}
}

// advance moves and persists a source's cursor.
//
// Post-then-persist: a crash between the two re-posts the same fingerprints,
// which the manager's cooldown absorbs. Losing a record would be the worse
// failure, so the ordering is deliberate.
func (a *adapter) advance(ctx context.Context, source string, at time.Time) {
	if at.IsZero() {
		return
	}
	a.mu.Lock()
	src, ok := a.sources[source]
	if !ok || !at.After(src.cursor) {
		a.mu.Unlock()
		return
	}
	src.cursor = at
	a.mu.Unlock()
	if err := a.mgr.PutState(ctx, source, cursorKey, at.UTC().Format(time.RFC3339)); err != nil && ctx.Err() == nil {
		log.Printf("persisting cursor for %s: %v", source, err)
	}
}

func (a *adapter) setCursor(source string, at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if src, ok := a.sources[source]; ok {
		src.cursor = at
	}
}

func (a *adapter) source(name string) *servedSource {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sources[name]
}

func (a *adapter) connectionFor(source string) (endpoint, token string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	src, found := a.sources[source]
	if !found {
		return "", "", false
	}
	return src.filter.endpoint, src.token, true
}

// post sends a batch through the emit cap and reports clipping. It is the
// single exit from this adapter: both the immediate path and the dwell queue go
// through it, so the cap cannot be bypassed by one of them.
func (a *adapter) post(ctx context.Context, source string, sigs []Signal) bool {
	if len(sigs) == 0 {
		return false
	}
	if len(sigs) > maxBatchPerSrc {
		sigs = sigs[:maxBatchPerSrc]
	}
	allowed, clipped := a.cap.Allow(source, sigs)
	if len(allowed) == 0 {
		a.reportClipping(ctx, source, clipped)
		return false
	}
	if err := a.mgr.Inbound(ctx, source, allowed); err != nil {
		if ctx.Err() == nil {
			log.Printf("posting %d signals for %s: %v", len(allowed), source, err)
			a.reportPostFailure(ctx, source, err)
		}
		return false
	}
	// Recovery FIRST, the clip AFTER: a window still clipping re-asserts
	// EmitCapReached on top of the recovered condition rather than under it.
	a.reportPostRecovered(ctx, source)
	a.reportClipping(ctx, source, clipped)
	log.Printf("posted %d log signal(s) for %s", len(allowed), source)
	return true
}

// reportPostFailure surfaces a rejected or unreachable manager on the source's
// Ready condition. A signal this adapter could not hand over is otherwise a
// silent drop behind a process that looks healthy — the one outcome the
// contract forbids — and the emission is not replayable: the event stream
// has moved on. So the loss is REPORTED, once per outage, where an operator
// reads conditions; the next successful post clears it.
func (a *adapter) reportPostFailure(ctx context.Context, source string, err error) {
	if _, failing := a.postFailing.LoadOrStore(source, true); failing {
		return
	}
	msg := "signals could not be handed to the manager and were lost: " + err.Error()
	// Synchronous, so the failure and its recovery are ordered as they happened.
	_ = a.mgr.ReportStatus(ctx, source, false, "PostFailed", msg)
}

// reportPostRecovered clears a reported post failure on the first success.
func (a *adapter) reportPostRecovered(ctx context.Context, source string) {
	if _, failing := a.postFailing.LoadAndDelete(source); !failing {
		return
	}
	_ = a.mgr.ReportStatus(ctx, source, true, "AdapterReady", "posting to the manager again")
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
		"something is logging far faster than an agent can act on it", clipped)
	log.Printf("%s: %s", source, msg)
	go func() { _ = a.mgr.ReportStatus(ctx, source, false, "EmitCapReached", msg) }()
}

// runDwellFlusher refreshes each source's health view and decides pending
// entries whose window has closed.
func (a *adapter) runDwellFlusher(ctx context.Context) {
	for ctx.Err() == nil {
		a.refreshSnapshots(ctx)
		a.pending.Flush(time.Now())
		sleepCtx(ctx, dwellTick)
	}
}

// refreshSnapshots re-reads the health view for every connected source that has
// something pending. Nothing pending means nothing to verify, so a quiet
// install issues no commands at all.
func (a *adapter) refreshSnapshots(ctx context.Context) {
	a.mu.Lock()
	sessions := map[string]*haSession{}
	for name, sess := range a.sessions {
		sessions[name] = sess
	}
	a.mu.Unlock()
	if !a.pending.HasEntries() {
		return
	}
	for name, sess := range sessions {
		a.refreshSnapshot(ctx, name, sess)
	}
}

// refreshSnapshot reads the current log listing and config-entry states.
//
// A failed read leaves the previous snapshot in place rather than clearing it:
// the ladder's answer for "cannot say" already exists, and clearing would turn
// a momentary hiccup into a verdict.
func (a *adapter) refreshSnapshot(ctx context.Context, source string, sess *haSession) {
	cctx, cancel := context.WithTimeout(ctx, haCommandTimeout)
	defer cancel()
	records, err := sess.SystemLog(cctx)
	if err != nil {
		log.Printf("%s: reading system_log for verification: %v", source, err)
		return
	}
	snap := &healthSnapshot{
		records: make(map[string]logRecord, len(records)),
		entries: map[string][]string{},
		at:      time.Now(),
	}
	for _, r := range records {
		snap.records[r.Key()] = r
	}
	// config_entries/get is an ADMIN command. A read-only token simply gets no
	// predicate, which is rung 2 — not an error worth reporting on the source.
	if entries, err := sess.ConfigEntries(cctx); err == nil {
		for _, e := range entries {
			snap.entries[e.Domain] = append(snap.entries[e.Domain], e.State)
		}
	}
	a.mu.Lock()
	if src, ok := a.sources[source]; ok {
		src.snapshot = snap
	}
	a.mu.Unlock()
}

// health is the adapter's verification ladder over one source's snapshot.
//
//	rung 1  the integration has config entries -> is any of them still failing?
//	rung 2  no predicate -> was the record STILL occurring as the window closed?
//	rung 3  no snapshot at all -> cannot say
//
// Only integrations with config entries carry a predicate. Core loggers (and
// YAML-configured integrations, which have no entries) fall to rung 2
// deliberately: recurrence is what the log itself can prove, and inventing a
// health check for the rest would mean guessing. Recurrence early in the window
// that then stopped is a blip that healed, not a problem still live — which is
// what `since` is for.
//
// `since` is the start of the window's closing part. A count that rose during
// the window is the log saying it kept happening — but a count that rose in the
// first thirty seconds and never again is a blip that healed, and the record's
// own timestamp (its latest occurrence) is what tells the two apart.
func (a *adapter) health(ref recordRef, since time.Time) verdict {
	a.mu.Lock()
	src, ok := a.sources[ref.source]
	var snap *healthSnapshot
	if ok {
		snap = src.snapshot
	}
	a.mu.Unlock()
	if snap == nil {
		return verdictUnknown
	}

	states, hasPredicate := snap.entries[ref.integration]
	for _, st := range states {
		if failedEntryStates[st] {
			return verdictUnhealthy
		}
	}

	rec, present := snap.records[ref.logger+"@"+ref.location]
	switch {
	case present && rec.Count > ref.countAtOpen && !rec.At().Before(since):
		// It was still happening as the window closed. That is the strongest
		// evidence the log can give, and it outranks a loaded config entry: an
		// integration can be up and failing at the same time. A count that rose
		// only BEFORE `since` falls through: it recurred, and then it stopped.
		return verdictUnhealthy
	case present && hasPredicate:
		// Every entry loaded and no new occurrences: it recovered.
		return verdictHealthy
	case present:
		return verdictUnknown
	case hasPredicate:
		// The record is gone from the listing — the log was cleared or Home
		// Assistant restarted — and the integration is loaded.
		return verdictGone
	default:
		return verdictUnknown
	}
}

func (a *adapter) stopAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for name, src := range a.sources {
		src.cancel()
		delete(a.sources, name)
	}
	for name, sess := range a.sessions {
		sess.Close()
		delete(a.sessions, name)
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

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
