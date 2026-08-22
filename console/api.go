package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The browser surface: a login, one JSON snapshot per page, one SSE stream, and
// the embedded SPA.
//
// Two rules shape all of it. First, SNAPSHOTS ARE AUTHORITATIVE and the stream
// carries deltas plus a cursor — a browser that misses every event still
// converges by re-fetching, which is what makes reconnect-after-sleep the same
// code path as first connect and a dropped event cost one stale second rather
// than a wrong screen. Second, NOTHING HERE MUTATES THE CLUSTER: the only two
// writes in the whole console are POST /channel/inbound and POST
// /signal/inbound, both through the manager, both gated.

// API serves the console's browser endpoints.
type API struct {
	cache       *Cache
	transcripts *Transcripts
	adapter     *Adapter
	activity    *ActivityWindow
	mgr         *Manager
	originator  *Originator
	metrics     *MetricsClient
	sessions    *Sessions

	namespace    string
	adapterName  string
	writeEnabled bool
	// staticToken is the fallback browser token from env, used when the console
	// Channel declares no credentials (hand-run deployments).
	staticToken string
	// authEnabled / externalAuthenticator are the process defaults for "does
	// this console authenticate, and if not, what does" — overridden by the
	// served Channel's config the same way writeEnabled is.
	authEnabled           bool
	externalAuthenticator string

	// metricsFromConfig is the client built from the served Channel's config,
	// rebuilt when that URL changes.
	metricsMu         sync.Mutex
	metricsFromConfig *MetricsClient
}

// APIDeps is what the browser surface fans in.
type APIDeps struct {
	Cache       *Cache
	Transcripts *Transcripts
	Adapter     *Adapter
	Activity    *ActivityWindow
	Manager     *Manager
	Originator  *Originator
	Metrics     *MetricsClient
	Config      *Config
}

// NewAPI builds the browser surface.
func NewAPI(d APIDeps) *API {
	return &API{
		cache: d.Cache, transcripts: d.Transcripts, adapter: d.Adapter,
		activity: d.Activity, mgr: d.Manager, originator: d.Originator, metrics: d.Metrics,
		sessions:     NewSessions(),
		namespace:    d.Config.Namespace,
		adapterName:  d.Config.AdapterName,
		writeEnabled: d.Config.WriteEnabled,
		staticToken:  d.Config.UIToken,

		authEnabled:           d.Config.AuthEnabled,
		externalAuthenticator: d.Config.ExternalAuthenticator,
	}
}

// token returns the currently valid browser token: the console Channel's
// projected `uiToken` credential, else the env fallback. Empty means the
// console is UNCONFIGURED, and every authenticated route stays closed — an
// unset token must never read as "no authentication required".
func (a *API) token() string {
	if t := a.adapter.UITokenFromChannel(); t != "" {
		return t
	}
	return a.staticToken
}

// writesAllowed is the effective write gate. The served Channel's config wins
// over the process default, because a ChannelAdapter CR carries no env and the
// chart therefore has no other way to say "this install is read-only". Only an
// EXPLICIT false disables — an absent field is not a decision.
func (a *API) writesAllowed() bool {
	if v := a.adapter.PrimaryConfig().WriteEnabled; v != nil {
		return *v
	}
	return a.writeEnabled
}

// externalAuthenticatorName is the release's declaration of what authenticates
// browsers when the console does not — resolved the same way as every other
// console setting, served Channel config first and process env second.
func (a *API) externalAuthenticatorName() string {
	if v := strings.TrimSpace(a.adapter.PrimaryConfig().ExternalAuthenticator); v != "" {
		return v
	}
	return strings.TrimSpace(a.externalAuthenticator)
}

// authIsExternal reports that this release DECLARED authentication to happen in
// front of the console.
//
// It takes TWO deliberate statements — authentication turned off AND a named
// authenticator — because either one alone is ambiguous. A false with nothing
// named is not "open": it is a half-applied configuration, and the console
// stays closed, which keeps the property that no absent value opens a door.
// The chart refuses that combination at render time; this is what happens if it
// reaches the pod anyway.
func (a *API) authIsExternal() bool {
	enabled := a.authEnabled
	if v := a.adapter.PrimaryConfig().AuthEnabled; v != nil {
		enabled = *v
	}
	return !enabled && a.externalAuthenticatorName() != ""
}

// metricsBackend resolves the historical-metrics client the same way: the
// served Channel's config first, the process env second.
//
// It has to be dynamic rather than constructed at startup, because the channel
// list arrives from the manager AFTER the process is up — a client built in
// main() would always be nil for a chart-configured URL, which is exactly how
// this shipped once: the URL was in spec.config and the console still reported
// "no metrics backend is configured".
func (a *API) metricsBackend() *MetricsClient {
	url := a.adapter.PrimaryConfig().MetricsURL
	if url == "" {
		return a.metrics
	}
	a.metricsMu.Lock()
	defer a.metricsMu.Unlock()
	if a.metricsFromConfig == nil || a.metricsFromConfig.BaseURL != url {
		a.metricsFromConfig = NewMetricsClient(url)
	}
	return a.metricsFromConfig
}

// Handler wires the routes.
func (a *API) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /api/session", a.handleSession)

	mux.HandleFunc("GET /api/overview", a.auth(a.handleOverview))
	mux.HandleFunc("GET /api/queues", a.auth(a.handleQueues))
	mux.HandleFunc("GET /api/config", a.auth(a.handleKinds))
	mux.HandleFunc("GET /api/config/{kind}", a.auth(a.handleInventory))
	mux.HandleFunc("GET /api/config/{kind}/{name}", a.auth(a.handleDetail))
	mux.HandleFunc("GET /api/findings", a.auth(a.handleFindings))
	mux.HandleFunc("GET /api/topology", a.auth(a.handleTopology))
	mux.HandleFunc("GET /api/activity", a.auth(a.handleActivity))
	mux.HandleFunc("GET /api/charts", a.auth(a.handleCharts))
	mux.HandleFunc("GET /api/charts/{chart}", a.auth(a.handleHistory))
	mux.HandleFunc("GET /api/sources", a.auth(a.handleOriginationSources))
	mux.HandleFunc("GET /api/agents", a.auth(a.handleAgents))

	mux.HandleFunc("GET /api/conversations", a.auth(a.handleConversations))
	mux.HandleFunc("GET /api/conversations/{name}", a.auth(a.handleConversation))
	mux.HandleFunc("GET /api/conversations/{name}/graph", a.auth(a.handleConversationGraph))
	mux.HandleFunc("POST /api/conversations", a.write("start-conversation", a.handleStart))
	// registered BEFORE the {name} routes it cannot collide with: closing is a
	// batch over named conversations, not an action on one
	mux.HandleFunc("POST /api/conversations/close", a.write("bulk-close", a.handleBulkClose))
	// Delete is a batch for the same reason close is; reopen deliberately is
	// NOT. Reopening re-materialises threads on every bound channel, so a batch
	// of them would announce itself on surfaces nobody is watching — it is a
	// decision about one conversation.
	mux.HandleFunc("POST /api/conversations/delete", a.write("bulk-delete", a.handleBulkDelete))
	// Marking read is authenticated and attributed, but NOT gated by
	// console.write.enabled: that gate makes the console a strict viewer by
	// removing its ability to instruct an agent or start work, and a read
	// watermark does neither. A read-only console is exactly the install where
	// an unread badge earns its keep, and one that could show a backlog but
	// never clear it would be broken in the way the badge exists to fix.
	mux.HandleFunc("POST /api/conversations/read", a.auth(a.handleMarkRead))
	mux.HandleFunc("POST /api/conversations/{name}/messages", a.write("send-message", a.handleSend))
	mux.HandleFunc("POST /api/conversations/{name}/reopen", a.write("reopen", a.handleReopen))

	mux.HandleFunc("GET /api/stream", a.auth(a.handleStream))
	mux.Handle("/", ui)
	return mux
}

// ---- topology + activity ------------------------------------------------------

// defaultTrafficWindow is the traffic window when the caller names none.
const defaultTrafficWindow = 5 * time.Minute

func windowFrom(r *http.Request) time.Duration {
	if v := r.URL.Query().Get("windowSeconds"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultTrafficWindow
}

func (a *API) handleTopology(w http.ResponseWriter, r *http.Request) {
	window := windowFrom(r)
	topo := BuildTopology(a.cache)
	a.applyTraffic(&topo, a.activity.Edges(window), window)

	// bufferCovers tells the UI whether the requested window is inside what the
	// activity buffer can answer. Outside it, the view must say "unavailable"
	// (or switch to aggregates) rather than render a short window as a quiet one.
	oldest := ""
	if events := a.activity.Since("", 1); len(events) > 0 {
		oldest = events[0].TS.Format(time.RFC3339Nano)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"topology":          topo,
		"consoleChannel":    a.adapter.PrimaryChannel(),
		"unjoinedPipelines": UnjoinedPipelines(a.cache, a.adapter.PrimaryChannel()),
		"synced":            a.syncState(),
		"stream":            a.activity.StreamHealth(),
		"oldestEvent":       oldest,
		"metricsAvailable":  a.metricsBackend() != nil,
	})
}

func (a *API) syncState() map[string]bool {
	out := map[string]bool{}
	for _, k := range append(append([]string{}, Kinds...), InstallKinds...) {
		out[k] = a.cache.Synced(k)
	}
	return out
}

func (a *API) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 5000 {
		limit = n
	}
	events := a.activity.Since(r.URL.Query().Get("since"), limit)
	if events == nil {
		events = []ActivityEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events, "cursor": a.activity.Cursor(), "stream": a.activity.StreamHealth(),
	})
}

func (a *API) handleFindings(w http.ResponseWriter, r *http.Request) {
	f := a.findings()
	if f == nil {
		f = []Finding{}
	}
	writeJSON(w, http.StatusOK, f)
}

// OriginationSource is one place this console can start a conversation from.
//
// The picker is a RENDERING OF THE TOPOLOGY, not a free-text pipeline field: a
// source is claimed by exactly one Pipeline, so what you can start is what is
// wired. A source that is not `Wired=True` is listed WITH its reason rather than
// hidden — "nothing is wired yet" is the state an operator must be able to see
// and fix.
type OriginationSource struct {
	Name     string `json:"name"`
	Wired    bool   `json:"wired"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	Pipeline string `json:"pipeline,omitempty"`
	Profile  string `json:"profile,omitempty"`
	// Patch is the exact edit that claims this source, shown when nothing does.
	Patch string `json:"patch,omitempty"`
}

func (a *API) handleOriginationSources(w http.ResponseWriter, r *http.Request) {
	out := []OriginationSource{}
	if a.originator != nil {
		name := a.originator.Source()
		src := OriginationSource{Name: name}
		if obj := a.cache.Get("signalsources", name); obj != nil {
			if c := obj.Condition("Wired"); c != nil {
				src.Wired = c.Status == "True"
				src.Reason, src.Message = c.Reason, c.Message
			}
		} else {
			src.Reason = "NotFound"
			src.Message = "SignalSource " + name + " does not exist"
		}
		// A source is SHAREABLE, so this collects every server rather than
		// keeping whichever the loop saw last — which was an arbitrary pick
		// dressed as a fact. The profile is filled only when ONE pipeline
		// serves the source: with several, a bare message is refused as
		// ambiguous and no single profile answers here.
		var servers, profiles []string
		for _, p := range a.cache.List("pipelines") {
			for _, ref := range decodeSpec[pipelineSpec](p.Spec).SignalSourceRefs {
				if ref.Name == name {
					servers = append(servers, p.Metadata.Name)
					profiles = append(profiles, decodeSpec[pipelineSpec](p.Spec).ProfileRef.Name)
				}
			}
		}
		sort.Strings(servers)
		src.Pipeline = strings.Join(servers, ", ")
		if len(profiles) == 1 {
			src.Profile = profiles[0]
		}
		if !src.Wired {
			src.Patch = `kubectl patch pipeline <name> --type=json -p ` +
				`'[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"` + name + `"}}]'`
		}
		out = append(out, src)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":      out,
		"canOriginate": a.originator != nil,
		"writeEnabled": a.writeEnabled,
	})
}

// ---- stream -------------------------------------------------------------------

// handleStream multiplexes THREE sources onto one SSE connection: CR watch
// deltas, activity events, and transcript appends. One connection per browser,
// not three — and the BFF holds one upstream activity connection for all of them.
//
// CR deltas are HINTS carrying kind and name; the browser re-fetches the
// snapshots it is showing. That keeps the wire format stable no matter how the
// CRDs evolve. Activity events travel WHOLE, because they are the animation
// itself and re-fetching would defeat the point.
func (a *API) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // proxies must not buffer SSE
	w.WriteHeader(http.StatusOK)

	deltas, cancelDeltas := a.cache.Subscribe()
	defer cancelDeltas()
	messages, cancelMessages := a.transcripts.Subscribe()
	defer cancelMessages()
	events, cancelEvents := a.activity.Subscribe()
	defer cancelEvents()

	send := func(event string, payload any) bool {
		b, err := json.Marshal(payload)
		if err != nil {
			return true
		}
		if _, err := io.WriteString(w, "event: "+event+"\ndata: "+string(b)+"\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	// Tell the client to load a fresh snapshot before streaming — reconnect and
	// first connect are then identical, which is the property that makes a
	// missed event a non-incident.
	if !send("resync", map[string]any{
		"reason": "connected", "cursor": a.activity.Cursor(),
	}) {
		return
	}

	// Queue state is polled, not watched: the OpQueue lives in the manager's
	// memory with no watch to subscribe to. Polling HERE rather than in every
	// browser is the whole reason the BFF holds this connection.
	queueTick := time.NewTicker(5 * time.Second)
	defer queueTick.Stop()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deltas:
			if !ok {
				return
			}
			if !send("delta", map[string]string{"type": d.Type, "kind": d.Kind, "name": d.Name}) {
				return
			}
		case e, ok := <-events:
			if !ok {
				return
			}
			if !send("activity", e) {
				return
			}
		case m, ok := <-messages:
			if !ok {
				return
			}
			if !send("message", m) {
				return
			}
		case <-queueTick.C:
			qctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			q := a.queues(qctx)
			cancel()
			if !send("queues", q) {
				return
			}
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func sortStrings(s []string) { sort.Strings(s) }
