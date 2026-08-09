package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The browser surface: a login, JSON snapshots, one SSE stream, and the
// embedded SPA.
//
// Two rules shape it. First, snapshots are authoritative and the stream only
// carries hints — a browser that misses every event still converges by
// re-fetching, which is what makes reconnect-after-sleep uneventful. Second,
// nothing here can mutate the cluster: the only write in the whole console is
// POST /channel/inbound, and it goes through the channel contract like any
// other adapter's user message.

const sessionCookie = "agentops_console"
const sessionTTL = 12 * time.Hour

// API serves the console's browser endpoints.
type API struct {
	cache       *Cache
	transcripts *Transcripts
	adapter     *Adapter
	// staticToken is the fallback browser token from env, used when the console
	// Channel declares no credentials (hand-run deployments).
	staticToken string

	mu       sync.Mutex
	sessions map[string]time.Time
}

// NewAPI builds the browser surface.
func NewAPI(cache *Cache, transcripts *Transcripts, adapter *Adapter, staticToken string) *API {
	return &API{cache: cache, transcripts: transcripts, adapter: adapter,
		staticToken: staticToken, sessions: map[string]time.Time{}}
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

// Handler wires the routes.
func (a *API) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /api/session", a.handleSession)
	mux.HandleFunc("GET /api/topology", a.auth(a.handleTopology))
	mux.HandleFunc("GET /api/kinds", a.auth(a.handleKinds))
	mux.HandleFunc("GET /api/kinds/{kind}", a.auth(a.handleInventory))
	mux.HandleFunc("GET /api/kinds/{kind}/{name}", a.auth(a.handleDetail))
	mux.HandleFunc("GET /api/conversations", a.auth(a.handleConversations))
	mux.HandleFunc("GET /api/conversations/{name}", a.auth(a.handleConversation))
	mux.HandleFunc("POST /api/conversations/{name}/messages", a.auth(a.handleSend))
	mux.HandleFunc("GET /api/stream", a.auth(a.handleStream))
	mux.Handle("/", ui)
	return mux
}

// ---- auth -------------------------------------------------------------------

func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		next(w, r)
	}
}

// authorized accepts a live session cookie or a bearer token equal to the
// configured one (constant-time). An unconfigured console authorizes nobody.
func (a *API) authorized(r *http.Request) bool {
	want := a.token()
	if want == "" {
		return false
	}
	if c, err := r.Cookie(sessionCookie); err == nil && a.validSession(c.Value) {
		return true
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (a *API) validSession(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[id]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.sessions, id)
		return false
	}
	return true
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"token":"…"}`})
		return
	}
	want := a.token()
	if want == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(want)) != 1 {
		// same answer either way: an unconfigured console must not be
		// distinguishable from a wrong password
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session"})
		return
	}
	id := hex.EncodeToString(buf)
	a.mu.Lock()
	a.sessions[id] = time.Now().Add(sessionTTL)
	for sid, exp := range a.sessions { // opportunistic expiry sweep
		if time.Now().After(exp) {
			delete(a.sessions, sid)
		}
	}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL / time.Second),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// handleSession lets the SPA decide between the login form and the app without
// provoking a 401 on every load.
func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": a.authorized(r),
		"configured":    a.token() != "",
	})
}

// ---- snapshots --------------------------------------------------------------

func (a *API) handleTopology(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"topology":          BuildTopology(a.cache),
		"consoleChannel":    a.adapter.PrimaryChannel(),
		"unjoinedPipelines": UnjoinedPipelines(a.cache, a.adapter.PrimaryChannel()),
		"synced":            a.syncState(),
	})
}

func (a *API) syncState() map[string]bool {
	out := map[string]bool{}
	for _, k := range Kinds {
		out[k] = a.cache.Synced(k)
	}
	return out
}

func (a *API) handleKinds(w http.ResponseWriter, r *http.Request) {
	type kindInfo struct {
		Kind   string `json:"kind"`
		Title  string `json:"title"`
		Count  int    `json:"count"`
		Synced bool   `json:"synced"`
	}
	out := make([]kindInfo, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, kindInfo{Kind: k, Title: Singular[k], Count: len(a.cache.List(k)), Synced: a.cache.Synced(k)})
	}
	writeJSON(w, http.StatusOK, out)
}

// inventoryRow is one CR in a per-kind listing: identity, health, and the
// conditions verbatim. Opaque config is not summarized here — the detail view
// shows it untouched.
type inventoryRow struct {
	Name       string      `json:"name"`
	Created    string      `json:"created,omitempty"`
	Health     Health      `json:"health"`
	Conditions []Condition `json:"conditions,omitempty"`
	Summary    string      `json:"summary,omitempty"`
}

func (a *API) handleInventory(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownKind(kind) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind " + kind})
		return
	}
	objs := a.cache.List(kind)
	rows := make([]inventoryRow, 0, len(objs))
	for _, o := range objs {
		h, _, _ := health(o)
		rows = append(rows, inventoryRow{
			Name: o.Metadata.Name, Created: o.Metadata.CreationTimestamp,
			Health: h, Conditions: o.Conditions(), Summary: summaryLine(o),
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

// summaryLine is the one key fact per kind worth showing in a list.
func summaryLine(o *Object) string {
	switch o.Kind {
	case "pipelines":
		spec := decodeSpec[pipelineSpec](o.Spec)
		parts := []string{"profile " + spec.ProfileRef.Name}
		if n := len(spec.SignalSourceRefs); n > 0 {
			parts = append(parts, plural(n, "source"))
		}
		if n := len(spec.ChannelRefs); n > 0 {
			parts = append(parts, plural(n, "channel"))
		}
		return strings.Join(parts, ", ")
	case "channels", "signalsources":
		if adapter := decodeSpec[servedSpec](o.Spec).Adapter; adapter != "" {
			return "adapter " + adapter
		}
	case "agentprofiles":
		spec := decodeSpec[profileSpec](o.Spec)
		if spec.RuntimeRef.Name != "" {
			return "runtime " + spec.RuntimeRef.Name
		}
	case "conversations":
		v := conversationView(o)
		if v.Status.Phase != "" {
			return strings.ToLower(v.Status.Phase)
		}
	}
	return ""
}

func plural(n int, word string) string {
	s := word
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

func (a *API) handleDetail(w http.ResponseWriter, r *http.Request) {
	kind, name := r.PathValue("kind"), r.PathValue("name")
	if !knownKind(kind) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind " + kind})
		return
	}
	obj := a.cache.Get(kind, name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Singular[kind] + " " + name + " not found"})
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

// conversationPageSize bounds the listing. A namespace can hold thousands of
// conversations (an event storm makes one per fingerprint), and the browser
// re-fetches this on every stream hint — so the list is the most recent N,
// newest first, and says how many it left out.
const conversationPageSize = 200

func (a *API) handleConversations(w http.ResponseWriter, r *http.Request) {
	limit := conversationPageSize
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 2000 {
		limit = n
	}
	items, total := a.conversationList(limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "shown": len(items),
	})
}

// conversationList returns the most recently active conversations plus the
// total held in cache. Run history is DROPPED from list rows — it is the
// bulkiest field by far (results are whole agent messages) and the detail view
// is where it belongs.
func (a *API) conversationList(limit int) ([]ConversationSummary, int) {
	pipelines := a.cache.List("pipelines")
	consoleChannel := a.adapter.PrimaryChannel()
	objs := a.cache.List("conversations")
	out := make([]ConversationSummary, 0, len(objs))
	for _, o := range objs {
		s := summarize(o, pipelines, consoleChannel)
		s.RunCount = len(s.Runs)
		s.Runs = nil
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].sortKey() > out[j].sortKey() })
	total := len(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total
}

func (a *API) handleConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation " + name + " not found"})
		return
	}
	summary := summarize(obj, a.cache.List("pipelines"), a.adapter.PrimaryChannel())
	var messages []Message
	archived := false
	if summary.ConsoleThread != "" {
		messages = a.transcripts.Thread(summary.ConsoleThread)
		archived = a.transcripts.Archived(summary.ConsoleThread)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": summary,
		"object":       obj,
		"transcript":   messages,
		// archived: a close-topic op ended this thread. The transcript stays
		// readable; there is just nothing left to reply to.
		"archived": archived,
	})
}

func (a *API) handleSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string `json:"text"`
		Sender string `json:"sender,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"text":"…"}`})
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	sender := in.Sender
	if sender == "" {
		sender = "console"
	}
	msg, err := a.adapter.Send(r.Context(), r.PathValue("name"), sender, in.Text)
	if err != nil {
		if errors.Is(err, errNotJoined) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "this conversation has no console thread — add the console channel to its pipeline's channels[] to join it",
			})
			return
		}
		log.Printf("inbound send: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sending failed"})
		return
	}
	writeJSON(w, http.StatusAccepted, msg)
}

// ---- stream -----------------------------------------------------------------

// handleStream multiplexes watch deltas and transcript appends onto one SSE
// connection.
//
// Events are HINTS. A `delta` says "some CR of this kind changed"; the browser
// re-fetches the snapshots it is showing. That keeps the wire format stable no
// matter how the CRDs evolve, and makes a missed event cost one stale second
// rather than a wrong screen.
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
	// tell the client to load a fresh snapshot before streaming — the
	// reconnect path and the first-connect path are then identical
	if !send("resync", map[string]string{"reason": "connected"}) {
		return
	}

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
			// only the kind and name travel: the browser re-reads what it needs
			if !send("delta", map[string]string{"type": d.Type, "kind": d.Kind, "name": d.Name}) {
				return
			}
		case m, ok := <-messages:
			if !ok {
				return
			}
			if !send("message", m) {
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

// ---- helpers ----------------------------------------------------------------

func knownKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
