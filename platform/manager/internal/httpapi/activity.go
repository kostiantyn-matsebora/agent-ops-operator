// Activity endpoints: the manager's per-hop telemetry, replayed by cursor and
// streamed over SSE.
//
//	GET  /activity?since=&limit=   bounded replay from the ring buffer
//	GET  /activity/stream          SSE, one event per hop, each with its cursor
//	POST /activity                 adapter-reported hops (delivery confirmation)
//
// All three sit under the adapter bearer scheme. THIS IS NOT A SIGNAL SURFACE:
// nothing posted here creates a Conversation, appends an input, or writes any
// Kubernetes object. agent-ops' own machinery reports STATUS, never SIGNAL —
// routing telemetry back through ingest is the loop the no-signal-loops
// invariant exists to prevent, and keeping the two surfaces apart is what makes
// that structural rather than a rule to remember.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// streamKeepAlive bounds how long an idle SSE connection goes without bytes.
// Proxies and load balancers reap silent connections; a comment frame is the
// standard way to stay under that.
const streamKeepAlive = 20 * time.Second

// activityResponse is the replay envelope. `resync` is the load-bearing field:
// a client whose cursor has been evicted is TOLD so, rather than handed a
// shorter list it would mistake for continuity.
type activityResponse struct {
	Events []activity.Event `json:"events"`
	Cursor string           `json:"cursor"`
	Resync bool             `json:"resync"`
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if s.Activity == nil {
		writeJSON(w, 503, map[string]string{"error": "activity telemetry is disabled"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, resync := s.Activity.Since(r.URL.Query().Get("since"), limit)
	if events == nil {
		events = []activity.Event{}
	}
	cursor := s.Activity.Latest()
	if len(events) > 0 {
		cursor = events[len(events)-1].Cursor
	}
	writeJSON(w, 200, activityResponse{Events: events, Cursor: cursor, Resync: resync})
}

func (s *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	if s.Activity == nil {
		writeJSON(w, 503, map[string]string{"error": "activity telemetry is disabled"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
		return
	}
	// Subscribe BEFORE replaying, so an event landing between the two is
	// duplicated rather than lost. Cursors make a duplicate harmless; a gap is
	// not.
	sub := s.Activity.Subscribe()
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx buffers text/event-stream by default, which turns a live stream into
	// a batch delivered when the connection closes.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	flusher.Flush()

	backlog, resync := s.Activity.Since(r.URL.Query().Get("since"), 0)
	if resync {
		writeSSE(w, "resync", map[string]string{
			"reason": "requested cursor is older than the buffer holds; re-read snapshots",
		})
	}
	for _, e := range backlog {
		writeSSE(w, "activity", e)
	}
	flusher.Flush()

	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-sub.Events():
			writeSSE(w, "activity", e)
			flusher.Flush()
		case <-keepAlive.C:
			if sub.Lagged() {
				// The subscriber fell behind and events were dropped for it.
				// Telling it to resync is the whole difference between a console
				// that is stale and one that is wrong.
				writeSSE(w, "resync", map[string]string{
					"reason": "stream fell behind and dropped events; re-read snapshots",
				})
			}
			_, _ = io.WriteString(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = io.WriteString(w, "event: "+event+"\ndata: ")
	_, _ = w.Write(body)
	_, _ = io.WriteString(w, "\n\n")
}

// reportedEvent is the subset of an event an adapter may report. Cursor, TS and
// the reporting adapter are assigned here — an adapter supplies observations,
// never identity or ordering.
type reportedEvent struct {
	Kind         string            `json:"kind"`
	From         *activity.NodeRef `json:"from,omitempty"`
	To           *activity.NodeRef `json:"to,omitempty"`
	Status       string            `json:"status,omitempty"`
	Conversation string            `json:"conversation,omitempty"`
	OpID         string            `json:"opId,omitempty"`
	LatencyMs    int64             `json:"latencyMs,omitempty"`
	Detail       string            `json:"detail,omitempty"`
	// Adapter, when present, must equal the caller's own scope. It exists so a
	// mismatch is REFUSED rather than silently rewritten — an adapter that
	// believes it is reporting for another needs to hear that it cannot.
	Adapter string `json:"adapter,omitempty"`
}

// handleActivityReport accepts hops only the adapter can see — notably
// channel.op.completed with real delivery latency, which upgrades an edge from
// "sent, unconfirmed" to confirmed.
//
// Reporting is OPTIONAL. An adapter that reports nothing still appears on the
// graph through manager-side events; it simply never confirms delivery.
func (s *Server) handleActivityReport(w http.ResponseWriter, r *http.Request) {
	if s.Activity == nil {
		writeJSON(w, 503, map[string]string{"error": "activity telemetry is disabled"})
		return
	}
	scope, _ := r.Context().Value(adapterScopeKey{}).(string)
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in reportedEvent
	if err := json.Unmarshal(body, &in); err != nil || in.Kind == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"kind",...}`})
		return
	}
	// An adapter may only speak for itself. The master token (empty scope) has
	// no adapter identity to attribute to, so it must name one.
	switch {
	case scope == "" && in.Adapter == "":
		writeJSON(w, 400, map[string]string{"error": "the master token must name the reporting adapter"})
		return
	case scope != "" && in.Adapter != "" && in.Adapter != scope:
		writeJSON(w, 403, map[string]string{"error": "an adapter may only report events attributed to itself"})
		return
	}
	reporter := scope
	if reporter == "" {
		reporter = in.Adapter
	}
	// from/to default to the reporting adapter's own hop into the manager, so a
	// minimal report is still renderable as motion along a real edge.
	from, to := in.From, in.To
	if from == nil {
		from = activity.Node(activity.NodeChannelAdapter, reporter)
	}
	if to == nil {
		to = activity.Node(activity.NodeManager, activity.NodeManager)
	}
	s.Activity.Emit(activity.Event{
		Kind: in.Kind, From: from, To: to, Status: in.Status,
		Conversation: in.Conversation, OpID: in.OpID,
		LatencyMs: in.LatencyMs, Detail: in.Detail, Adapter: reporter,
	})
	writeJSON(w, 202, map[string]bool{"ok": true})
}
