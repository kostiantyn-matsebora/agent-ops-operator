package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// The console's consumer of the manager's per-hop telemetry.
//
// ONE upstream connection, multiplexed to N browsers. The manager's ring buffer
// is the source of truth for recent motion; this keeps a windowed index of it so
// a graph can ask "what moved along this edge, and how fast" and a conversation
// view can ask "what happened to THIS conversation" without either walking the
// whole buffer.
//
// It is a strictly READ path. Nothing here posts a signal, creates a
// Conversation, or touches the Kubernetes API — telemetry is not signal, and
// the console is where that rule would be easiest to break by accident.

// ActivityEvent mirrors the manager's event shape. Kept as a local struct
// rather than importing the operator module: the console is a separate,
// dependency-free module, and the contract is the wire format, not a Go type.
type ActivityEvent struct {
	Cursor       string    `json:"cursor"`
	TS           time.Time `json:"ts"`
	Kind         string    `json:"kind"`
	From         *NodeRef  `json:"from,omitempty"`
	To           *NodeRef  `json:"to,omitempty"`
	Status       string    `json:"status"`
	Conversation string    `json:"conversation,omitempty"`
	Pipeline     string    `json:"pipeline,omitempty"`
	RunID        string    `json:"runId,omitempty"`
	OpID         string    `json:"opId,omitempty"`
	InputID      string    `json:"inputId,omitempty"`
	LatencyMs    int64     `json:"latencyMs,omitempty"`
	Code         string    `json:"code,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	Adapter      string    `json:"adapter,omitempty"`
}

// NodeRef names one graph node, in the manager's vocabulary — which is the
// topology graph's vocabulary, so an event is renderable as motion along an
// edge without translation.
type NodeRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// EdgeKey identifies one directed edge for rate and latency aggregation.
type EdgeKey struct {
	FromKind, FromName string
	ToKind, ToName     string
}

func (e *ActivityEvent) edge() (EdgeKey, bool) {
	if e.From == nil || e.To == nil {
		return EdgeKey{}, false
	}
	return EdgeKey{e.From.Kind, e.From.Name, e.To.Kind, e.To.Name}, true
}

// EdgeStat summarizes one edge over a window.
type EdgeStat struct {
	From   NodeRef `json:"from"`
	To     NodeRef `json:"to"`
	Events int     `json:"events"`
	Errors int     `json:"errors"`
	// RatePerMin is events per minute over the requested window — what drives
	// edge animation speed.
	RatePerMin float64 `json:"ratePerMin"`
	// P50LatencyMs / MaxLatencyMs are over the events that MEASURED a latency;
	// an edge whose hops carry none reports zero rather than inventing one.
	P50LatencyMs int64  `json:"p50LatencyMs,omitempty"`
	MaxLatencyMs int64  `json:"maxLatencyMs,omitempty"`
	LastTS       string `json:"lastTs,omitempty"`
	// Unconfirmed marks an edge whose traffic is manager-side INTENT with no
	// adapter delivery confirmation. Rendered as "sent, unconfirmed" — never as
	// success, because reporting is optional and an adapter that reports
	// nothing must not look like one that delivered.
	Unconfirmed bool `json:"unconfirmed"`
}

// ActivityWindow is the bounded in-memory index of recent hops.
type ActivityWindow struct {
	mgr *Manager
	// Token is the SIGNAL or CHANNEL identity used upstream; either opens the
	// activity surface.
	max int

	mu     sync.RWMutex
	events []ActivityEvent // oldest first, bounded by max
	cursor string
	// connected/lastErr describe the upstream link, so the UI can say "the
	// graph is not moving because the stream is down" rather than "nothing is
	// happening" — the two look identical otherwise.
	connected bool
	lastErr   string
	// resyncs counts explicit gap notifications. Surfaced rather than hidden:
	// a console that silently dropped hops would be rendering a partial truth.
	resyncs int
	// lastGap is the most recent one, kept so a browser opening LATER still
	// learns that the window it is reading is not continuous.
	lastGap *ActivityGap

	subMu  sync.Mutex
	subs   map[int]chan ActivityEvent
	nextID int
}

// NewActivityWindow builds the consumer. max bounds memory here independently
// of the manager's own ring — the console holds a window, never an archive.
func NewActivityWindow(mgr *Manager, max int) *ActivityWindow {
	if max <= 0 {
		max = 5000
	}
	return &ActivityWindow{mgr: mgr, max: max, subs: map[int]chan ActivityEvent{}}
}

// Run keeps one SSE connection to the manager open, reconnecting with the last
// cursor it saw. A reconnect is the same code path as a first connect: the
// manager answers an evicted cursor with an explicit resync, so a gap is
// reported rather than silently skipped.
func (w *ActivityWindow) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := w.stream(ctx)
		w.setConnected(false, err)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("activity stream: %v (reconnecting in %s)", err, backoff)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
	}
}

func (w *ActivityWindow) stream(ctx context.Context) error {
	since := w.Cursor()
	path := "/activity/stream"
	if since != "" {
		path += "?since=" + since
	}
	resp, err := w.mgr.stream(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.setConnected(true, nil)

	// SSE frames are "event: <name>\ndata: <json>\n\n"; comments (": ...") are
	// keep-alives and carry nothing.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "" || strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event == "resync" {
				var r struct {
					Reason string `json:"reason"`
				}
				_ = json.Unmarshal([]byte(payload), &r)
				w.noteResync(r.Reason)
				continue
			}
			var e ActivityEvent
			if err := json.Unmarshal([]byte(payload), &e); err != nil {
				continue
			}
			w.add(e)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (w *ActivityWindow) setConnected(up bool, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.connected = up
	if err != nil {
		w.lastErr = err.Error()
	} else if up {
		w.lastErr = ""
	}
}

// KindActivityGap marks lost history IN the timeline. It is a console-local
// kind, not a manager hop: it has no from/to, so nothing renders it as motion
// along an edge.
//
// The counter alone was not enough. A gap has a POSITION — the events either
// side of it are not consecutive — and a timeline that shows them adjacent is
// telling the reader something false, however honest the number in the health
// panel is.
const KindActivityGap = "activity.gap"

// GapReasonDefault is what a resync frame means when it carries no reason.
const GapReasonDefault = "history before this point is unavailable"

func (w *ActivityWindow) noteResync(reason string) {
	if reason == "" {
		reason = GapReasonDefault
	}
	w.mu.Lock()
	w.resyncs++
	now := time.Now().UTC()
	w.lastGap = &ActivityGap{TS: now, Detail: reason}
	// Drop the cursor. It belonged to a history that is gone — and after a
	// MANAGER RESTART it is worse than stale: the manager's sequence restarts at
	// 1, so a retained cursor is permanently "ahead", every reconnect resyncs
	// again, and the same backlog is appended each time. Clearing it lets the
	// next event set the cursor in the new process's own sequence space.
	w.cursor = ""
	gap := ActivityEvent{TS: now, Kind: KindActivityGap, Status: "warn", Detail: reason}
	w.events = append(w.events, gap)
	if len(w.events) > w.max {
		w.events = w.events[len(w.events)-w.max:]
	}
	w.mu.Unlock()
	w.publish(gap)
}

func (w *ActivityWindow) add(e ActivityEvent) {
	w.mu.Lock()
	w.events = append(w.events, e)
	if len(w.events) > w.max {
		w.events = w.events[len(w.events)-w.max:]
	}
	if e.Cursor > w.cursor {
		w.cursor = e.Cursor
	}
	w.mu.Unlock()
	w.publish(e)
}

// Cursor is the newest cursor seen ("" before the first event).
func (w *ActivityWindow) Cursor() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cursor
}

// StreamHealth describes the upstream link. It is shown rather than hidden: a
// stream that is down and a system that is idle look identical on a graph, and
// only one of them is a problem.
type StreamHealth struct {
	Connected bool   `json:"connected"`
	Cursor    string `json:"cursor,omitempty"`
	Events    int    `json:"events"`
	Resyncs   int    `json:"resyncs"`
	Error     string `json:"error,omitempty"`
	// LastGap is when history was most recently lost, and why.
	//
	// It rides on the HEALTH rather than on the stream because a gap outlives
	// the connection that reported it: the browser that matters is usually the
	// one opened AFTER something went wrong, and a live-only signal is invisible
	// to exactly that reader.
	LastGap *ActivityGap `json:"lastGap,omitempty"`
}

// ActivityGap describes lost history for a viewer.
type ActivityGap struct {
	TS     time.Time `json:"ts"`
	Detail string    `json:"detail,omitempty"`
}

// StreamHealth reports the state of the upstream connection.
func (w *ActivityWindow) StreamHealth() StreamHealth {
	w.mu.RLock()
	defer w.mu.RUnlock()
	h := StreamHealth{Connected: w.connected, Cursor: w.cursor, Events: len(w.events),
		Resyncs: w.resyncs, Error: w.lastErr}
	if w.lastGap != nil {
		gap := *w.lastGap
		h.LastGap = &gap
	}
	return h
}

// Since returns events newer than a cursor, oldest first.
func (w *ActivityWindow) Since(cursor string, limit int) []ActivityEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []ActivityEvent
	for _, e := range w.events {
		if cursor == "" || e.Cursor > cursor {
			out = append(out, e)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// ForConversation returns one conversation's hops in time order — the
// sequence/waterfall view's whole input.
func (w *ActivityWindow) ForConversation(name string) []ActivityEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []ActivityEvent
	for _, e := range w.events {
		if e.Conversation == name {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cursor < out[j].Cursor })
	return out
}

// Edges aggregates per-edge rates and latencies over a window ending now.
//
// A window LONGER than what the buffer holds is not an error and not a lie: the
// rate is computed over the requested duration, so a partially-covered window
// under-reports rather than over-reports. The UI is what says whether a longer
// window needs the metrics backend.
func (w *ActivityWindow) Edges(window time.Duration) []EdgeStat {
	if window <= 0 {
		window = 5 * time.Minute
	}
	cutoff := time.Now().Add(-window)

	w.mu.RLock()
	defer w.mu.RUnlock()

	type agg struct {
		stat      EdgeStat
		latencies []int64
		confirmed bool
		intent    bool
	}
	byEdge := map[EdgeKey]*agg{}
	for _, e := range w.events {
		if e.TS.Before(cutoff) {
			continue
		}
		key, ok := e.edge()
		if !ok {
			continue
		}
		a := byEdge[key]
		if a == nil {
			a = &agg{stat: EdgeStat{From: *e.From, To: *e.To}}
			byEdge[key] = a
		}
		a.stat.Events++
		if e.Status == "error" {
			a.stat.Errors++
		}
		if e.LatencyMs > 0 {
			a.latencies = append(a.latencies, e.LatencyMs)
		}
		if e.TS.Format(time.RFC3339Nano) > a.stat.LastTS {
			a.stat.LastTS = e.TS.Format(time.RFC3339Nano)
		}
		switch e.Kind {
		case "channel.op.enqueued":
			a.intent = true
		case "channel.op.completed":
			a.confirmed = true
		}
	}

	out := make([]EdgeStat, 0, len(byEdge))
	minutes := window.Minutes()
	for _, a := range byEdge {
		a.stat.RatePerMin = float64(a.stat.Events) / minutes
		if len(a.latencies) > 0 {
			sort.Slice(a.latencies, func(i, j int) bool { return a.latencies[i] < a.latencies[j] })
			a.stat.P50LatencyMs = a.latencies[len(a.latencies)/2]
			a.stat.MaxLatencyMs = a.latencies[len(a.latencies)-1]
		}
		a.stat.Unconfirmed = a.intent && !a.confirmed
		out = append(out, a.stat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From.Name != out[j].From.Name {
			return out[i].From.Name < out[j].From.Name
		}
		return out[i].To.Name < out[j].To.Name
	})
	return out
}

// Subscribe returns a live feed of events for the browser stream, plus its
// cancel. Buffered and lossy for the same reason the cache's is: a slow browser
// must never stall the upstream reader, and a missed event costs one stale
// second because snapshots are authoritative.
func (w *ActivityWindow) Subscribe() (<-chan ActivityEvent, func()) {
	w.subMu.Lock()
	defer w.subMu.Unlock()
	id := w.nextID
	w.nextID++
	ch := make(chan ActivityEvent, 256)
	w.subs[id] = ch
	return ch, func() {
		w.subMu.Lock()
		defer w.subMu.Unlock()
		if cur, ok := w.subs[id]; ok {
			delete(w.subs, id)
			close(cur)
		}
	}
}

func (w *ActivityWindow) publish(e ActivityEvent) {
	w.subMu.Lock()
	defer w.subMu.Unlock()
	for _, ch := range w.subs {
		select {
		case ch <- e:
		default: // dropped: the browser re-fetches its snapshot
		}
	}
}

// stream opens a streaming GET against the manager with no client timeout — an
// SSE connection is long-lived by design, bounded by ctx.
func (m *Manager) stream(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Transport: m.HTTP.Transport} // no Timeout
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %d", path, resp.StatusCode)
	}
	return resp, nil
}
