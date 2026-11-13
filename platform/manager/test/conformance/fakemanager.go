//go:build conformance

// Package conformance verifies every adapter BLACK-BOX: it builds the
// adapter's binary, starts it with the contract environment, and speaks the
// adapter contracts to it from a fake manager on the other side. Nothing here
// is imported by an adapter — every adapter module keeps zero dependencies
// outside its own directory — and what is tested is the artifact that ships.
//
// It runs behind the `conformance` build tag so the manager's ordinary
// `go test ./...` does not build seven binaries:
//
//	cd platform/manager && go test -tags conformance ./test/conformance/
//
// No cluster, no network beyond loopback, no credentials. Adapters that reach a
// third party are pointed at a local double through their configured endpoint:
// the fake Bot API (test/fakebotapi) for Telegram, a fake API server for the
// console and k8s-events, a fake Home Assistant websocket for signal-ha, a
// fake Pulse REST API for signal-pulse's webhook registration.
package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	headerContentType = "Content-Type"
	mimeEventStream   = "text/event-stream"
	mimeJSON          = "application/json"
	headerVocabulary  = "X-Agentops-Vocabulary-Revision"
	errRejected      = "rejected by the conformance fake"
)

// ChannelInfo is one served channel, as GET /channel/channels lists it.
type ChannelInfo struct {
	Name                string          `json:"name"`
	Config              json.RawMessage `json:"config,omitempty"`
	CredentialEnvPrefix string          `json:"credentialEnvPrefix,omitempty"`
}

// SourceInfo is one served signal source, as GET /signal/sources lists it.
type SourceInfo = ChannelInfo

// OpsRequest is one GET /channel/ops as the fake saw it.
type OpsRequest struct {
	Adapter  string
	Contract string
	Wait     int
	Refused  bool // no or outdated contract= → 400
	Stream   bool // GET /channel/ops/stream, a receipt adapter's SSE stream
}

// Completion is one POST /channel/ops/{id}/done: the adapter's report, done or
// (with Error) failed. For a receipt adapter (the console) it is the
// acknowledgement, which is all that adapter ever sends: the op is consumed there.
type Completion struct {
	ID       string
	ThreadID string
	Error    string
}

// WorkRequest is one GET /work as the fake saw it.
type WorkRequest struct {
	Convo string
	Pod   string
	Wait  int
}

// WorkCompletion is one POST /work/done. Raw carries the decoded body
// unmodified — field PRESENCE matters to this contract (API-5/API-7/API-9
// all turn on whether a key was sent at all, not just its value), which a
// parsed Go struct with omitempty would already have thrown away.
type WorkCompletion struct {
	RunID string
	Convo string
	Raw   map[string]any
}

// workLatest is the latest-wins pair /work/done tracks per conversation.
type workLatest struct {
	ContextID string
	Brief     string
}

// StatusReport is one POST /channel/channels/{name}/status or
// /signal/sources/{name}/status.
type StatusReport struct {
	Name    string
	Ready   bool
	Reason  string
	Message string
}

// SignalPost is one POST /signal/inbound.
type SignalPost struct {
	Source  string           `json:"source"`
	Signals []map[string]any `json:"signals"`
	// Rejected is true when the fake was configured to refuse it.
	Rejected bool `json:"-"`
	Bearer   bool `json:"-"`
}

// FakeManager is the manager's adapter-facing surface, recording everything.
type FakeManager struct {
	Token string
	// ContractVersion is what /channel/ops requires; "2" today.
	ContractVersion string

	srv *httptest.Server

	mu   sync.Mutex
	cond *sync.Cond

	// The state read surface's Conversations (docs/contracts.md, "The state
	// read surface"): what the console reads instead of the Kubernetes API.
	convs      map[string]map[string]any
	convRev    int
	stateFail  map[string]int // conversation name -> status its single read answers
	stateWatch []chan []byte
	channels   []ChannelInfo
	sources    []SourceInfo
	queue      []json.RawMessage // ops offered and not yet acknowledged, in order
	acks       []string          // the id of every op acknowledged, in order
	workQueue  []json.RawMessage // work units to serve, in order — see QueueWork

	opsRequests   []OpsRequest
	completions   []Completion
	inbound       []map[string]any
	statusReports []StatusReport
	signalPosts   []SignalPost
	state         map[string]string // "<kind>/<name>/<key>" → value
	unauthorized  int
	listed        map[string]int // "channel:<adapter>" / "signal:<adapter>" → count

	workRequests    []WorkRequest
	workCompletions []WorkCompletion
	// workState is the ONE stateful fact /work/done keeps about its own body
	// (API-5): runtimeContextId/brief are latest-wins, so a report that omits
	// either must leave the previously recorded value untouched rather than
	// clearing it. Everything else about a completion is just RECORDED, never
	// interpreted — see workCompletions.
	workState map[string]workLatest

	rejectWorkDone int // HTTP status to answer POST /work/done with; 0 = accept

	rejectSignals int // HTTP status to answer /signal/inbound with; 0 = accept
	rejectInbound int
	// deliveryTTLSeconds is what every acknowledgement answers, as the real
	// manager's per-kind delivery TTL does (design D-17). Defaults to the stable
	// kind's 600 and is overridable so the retry-budget conformance assertion
	// can run against a TTL small enough to finish in test time.
	deliveryTTLSeconds int
	// receiptAdapters are the adapters whose acknowledgement is only their
	// receipt: the fake consumes the op there, as the real manager does for
	// them. Every other adapter reports done or failed.
	receiptAdapters map[string]bool
	// rejectOpsAsNonLeader counts down the next /channel/ops polls answered
	// with 503 before falling back to the ordinary 204/200 behavior —
	// durable-chat-ops-broker's non-leader rejection, simulated without a
	// second manager process.
	rejectOpsAsNonLeader int
}

// SetDeliveryTTLSeconds changes the delivery TTL every acknowledgement answers
// from here on. See the field's own doc comment.
func (f *FakeManager) SetDeliveryTTLSeconds(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveryTTLSeconds = n
}

// Acks returns the id of every op acknowledged so far, in order.
func (f *FakeManager) Acks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.acks...)
}

// RejectOpsAsNonLeader makes the fake answer the next n GET /channel/ops
// polls with 503 (Retry-After: 0), exactly as a manager replica that is not
// the current leader does. A conforming adapter must retry at once rather
// than treating it as a failure.
func (f *FakeManager) RejectOpsAsNonLeader(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejectOpsAsNonLeader = n
}

// NewFakeManager starts the fake. Every adapter-facing endpoint requires the
// bearer token, as the real manager does. Its cleanup is registered FIRST so
// it runs LAST: cleanups are LIFO, and closing the server while an adapter
// process still holds a long-poll blocks forever.
func NewFakeManager(t *testing.T, token string) *FakeManager {
	t.Helper()
	f := &FakeManager{Token: token, ContractVersion: "2", state: map[string]string{}, listed: map[string]int{}, deliveryTTLSeconds: 600, receiptAdapters: map[string]bool{"console": true},
		workState: map[string]workLatest{}}
	t.Cleanup(func() { f.Close() })
	f.cond = sync.NewCond(&f.mu)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	// GET /work and POST /work/done carry no bearer requirement in the real
	// contract (docs/contracts/runtime.md) — a runtime's Control.Poll/Report
	// send no Authorization header at all — so these two, alone among every
	// route below, are registered unauthenticated.
	mux.HandleFunc("GET /work", f.handleWork)
	mux.HandleFunc("POST /work/done", f.handleWorkDone)
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != token {
				f.mu.Lock()
				f.unauthorized++
				f.mu.Unlock()
				writeJSON(w, 401, map[string]string{"error": "unauthorized"})
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /state/conversations", auth(f.stateList))
	mux.HandleFunc("GET /state/conversations/stream", auth(f.stateStream))
	mux.HandleFunc("GET /state/conversations/{name}", auth(f.stateOne))
	// Signals are served empty: no conformance case seeds one.
	mux.HandleFunc("GET /state/signals", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"revision": "r1", "items": []any{}})
	}))
	mux.HandleFunc("GET /state/signals/stream", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerContentType, mimeEventStream)
		w.WriteHeader(200)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
	}))
	mux.HandleFunc("GET /channel/ops", auth(f.handleOps))
	mux.HandleFunc("GET /channel/ops/stream", auth(f.handleOpsStream))
	mux.HandleFunc("POST /channel/ops/{id}/ack", auth(f.handleAck))
	mux.HandleFunc("POST /channel/ops/{id}/done", auth(f.handleDone))
	mux.HandleFunc("POST /channel/inbound", auth(f.handleInbound))
	mux.HandleFunc("GET /channel/channels", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.listed["channel:"+r.URL.Query().Get("adapter")]++
		out := append([]ChannelInfo{}, f.channels...)
		f.cond.Broadcast()
		f.mu.Unlock()
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("GET /channel/vocabulary", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"revision": "r1", "entries": []any{}})
	}))
	mux.HandleFunc("GET /channel/state/{channel}/{key}", auth(f.stateGet("channel")))
	mux.HandleFunc("PUT /channel/state/{channel}/{key}", auth(f.statePut("channel")))
	mux.HandleFunc("POST /channel/channels/{name}/status", auth(f.handleStatus))
	mux.HandleFunc("POST /channel/read", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"results": []any{}})
	}))
	mux.HandleFunc("POST /channel/conversations/{name}/reopen", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))
	mux.HandleFunc("POST /channel/conversations/{name}/delete", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))
	mux.HandleFunc("POST /channel/conversations/{name}/reset-context", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))

	mux.HandleFunc("POST /signal/inbound", auth(f.handleSignalInbound))
	mux.HandleFunc("GET /signal/sources", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.listed["signal:"+r.URL.Query().Get("adapter")]++
		out := append([]SourceInfo{}, f.sources...)
		f.cond.Broadcast()
		f.mu.Unlock()
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("GET /signal/state/{source}/{key}", auth(f.stateGet("signal")))
	mux.HandleFunc("PUT /signal/state/{source}/{key}", auth(f.statePut("signal")))
	mux.HandleFunc("POST /signal/sources/{name}/status", auth(f.handleStatus))

	// The console reads these too; nothing conformance asserts on them.
	mux.HandleFunc("GET /activity", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"events": []any{}, "next": 0})
	}))
	mux.HandleFunc("GET /activity/stream", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerContentType, mimeEventStream)
		w.WriteHeader(200)
		<-r.Context().Done()
	}))
	mux.HandleFunc("POST /activity", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	mux.HandleFunc("GET /status", auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) }))
	f.srv = httptest.NewServer(mux)
	return f
}

// URL is the manager base URL for MANAGER_URL.
func (f *FakeManager) URL() string { return f.srv.URL }

// Close stops the fake, dropping in-flight long-polls rather than waiting
// for them.
func (f *FakeManager) Close() {
	f.srv.CloseClientConnections()
	f.srv.Close()
}

// ServeChannels sets the channel listing.
func (f *FakeManager) ServeChannels(infos ...ChannelInfo) {
	f.mu.Lock()
	f.channels = infos
	f.mu.Unlock()
}

// ServeSources sets the source listing.
func (f *FakeManager) ServeSources(infos ...SourceInfo) {
	f.mu.Lock()
	f.sources = infos
	f.mu.Unlock()
}

// Seed writes a state value the adapter will read back.
func (f *FakeManager) Seed(kind, name, key, value string) {
	f.mu.Lock()
	f.state[kind+"/"+name+"/"+key] = value
	f.mu.Unlock()
}

// State reads a value the adapter persisted.
func (f *FakeManager) State(kind, name, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[kind+"/"+name+"/"+key]
}

// QueueOp queues one op, as raw JSON, to be served on the next long-poll.
// Queue the same op twice to exercise at-least-once delivery.
func (f *FakeManager) QueueOp(op map[string]any) {
	raw, _ := json.Marshal(op)
	f.mu.Lock()
	f.queue = append(f.queue, raw)
	f.cond.Broadcast()
	f.mu.Unlock()
}

// QueueWork queues one work unit, as raw JSON, to be served on the next
// GET /work long-poll. Mirrors QueueOp.
func (f *FakeManager) QueueWork(unit map[string]any) {
	raw, _ := json.Marshal(unit)
	f.mu.Lock()
	f.workQueue = append(f.workQueue, raw)
	f.cond.Broadcast()
	f.mu.Unlock()
}

// RejectWorkDone makes /work/done answer with the status code (0 accepts).
// Mirrors RejectInbound.
func (f *FakeManager) RejectWorkDone(code int) {
	f.mu.Lock()
	f.rejectWorkDone = code
	f.mu.Unlock()
}

// WorkRequests returns every GET /work the fake has seen so far.
func (f *FakeManager) WorkRequests() []WorkRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]WorkRequest{}, f.workRequests...)
}

// WorkCompletions returns every accepted POST /work/done so far.
func (f *FakeManager) WorkCompletions() []WorkCompletion {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]WorkCompletion{}, f.workCompletions...)
}

// WorkState reads the latest-wins runtimeContextId/brief this fake has
// recorded for convo (API-5) — the one derived fact /work/done keeps,
// updated only when a report actually carries the field.
func (f *FakeManager) WorkState(convo string) (contextID, brief string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.workState[convo]
	return st.ContextID, st.Brief
}

// RejectSignals makes /signal/inbound answer with the status code (0 accepts).
func (f *FakeManager) RejectSignals(code int) {
	f.mu.Lock()
	f.rejectSignals = code
	f.mu.Unlock()
}

// RejectInbound makes /channel/inbound answer with the status code (0 accepts).
func (f *FakeManager) RejectInbound(code int) {
	f.mu.Lock()
	f.rejectInbound = code
	f.mu.Unlock()
}

// Listed reports how many times an adapter listed its channels or sources.
func (f *FakeManager) Listed(kind, adapter string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listed[kind+":"+adapter]
}

func (f *FakeManager) OpsRequests() []OpsRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]OpsRequest{}, f.opsRequests...)
}

func (f *FakeManager) Completions() []Completion {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Completion{}, f.completions...)
}

func (f *FakeManager) Inbound() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any{}, f.inbound...)
}

func (f *FakeManager) StatusReports() []StatusReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]StatusReport{}, f.statusReports...)
}

func (f *FakeManager) SignalPosts() []SignalPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SignalPost{}, f.signalPosts...)
}

func (f *FakeManager) Unauthorized() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unauthorized
}

func (f *FakeManager) handleOps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := OpsRequest{Adapter: q.Get("adapter"), Contract: q.Get("contract")}
	fmt.Sscanf(q.Get("wait"), "%d", &req.Wait)
	if req.Contract != f.ContractVersion {
		req.Refused = true
		f.mu.Lock()
		f.opsRequests = append(f.opsRequests, req)
		f.mu.Unlock()
		writeJSON(w, 400, map[string]string{"error": "contract=" + f.ContractVersion + " is required"})
		return
	}
	f.mu.Lock()
	f.opsRequests = append(f.opsRequests, req)
	if f.rejectOpsAsNonLeader > 0 {
		f.rejectOpsAsNonLeader--
		f.mu.Unlock()
		w.Header().Set(headerVocabulary, "r1")
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(503)
		return
	}
	f.mu.Unlock()
	w.Header().Set(headerVocabulary, "r1")
	wait := req.Wait
	if wait > 30 {
		wait = 30
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Until(deadline)):
		}
		f.mu.Lock()
		f.cond.Broadcast()
		f.mu.Unlock()
		close(done)
	}()
	f.mu.Lock()
	for len(f.queue) == 0 && time.Now().Before(deadline) && r.Context().Err() == nil {
		f.cond.Wait()
	}
	if len(f.queue) == 0 {
		f.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	// A read changes nothing: the op stays offered until acknowledged.
	raw := f.queue[0]
	f.mu.Unlock()
	w.Header().Set(headerContentType, mimeJSON)
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

func opID(raw json.RawMessage) string {
	var op struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &op)
	return op.ID
}

// handleAck is POST /channel/ops/{id}/ack: the hand-over. The op is never offered
// again; a receipt adapter's op is consumed here, anyone else's awaits a report.
// An op that is not offered answers 404, as the real manager does.
func (f *FakeManager) handleAck(w http.ResponseWriter, r *http.Request) {
	id, adapter := r.PathValue("id"), r.URL.Query().Get("adapter")
	f.mu.Lock()
	idx := -1
	for i, raw := range f.queue {
		if opID(raw) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		f.mu.Unlock()
		writeJSON(w, 404, map[string]string{"error": "no such message on the adapter's queues"})
		return
	}
	f.queue = append(f.queue[:idx:idx], f.queue[idx+1:]...)
	f.acks = append(f.acks, id)
	receipt, ttl := f.receiptAdapters[adapter], f.deliveryTTLSeconds
	if receipt {
		f.completions = append(f.completions, Completion{ID: id})
	}
	f.mu.Unlock()
	reply := map[string]any{"ok": true, "consumed": receipt}
	if !receipt {
		reply["deliveryTtlSeconds"] = ttl
	}
	writeJSON(w, 200, reply)
}

// handleOpsStream is the manager's GET /channel/ops/stream for a receipt
// adapter: every op offered and not yet acknowledged, once per connection, then
// each new one, as an SSE event. Nothing is claimed: an op the adapter never
// acknowledges is pushed again on the next connection.
func (f *FakeManager) handleOpsStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := OpsRequest{Adapter: q.Get("adapter"), Contract: q.Get("contract"), Stream: true}
	f.mu.Lock()
	f.opsRequests = append(f.opsRequests, req)
	reject := f.rejectOpsAsNonLeader > 0
	if reject {
		f.rejectOpsAsNonLeader--
	}
	f.mu.Unlock()
	if req.Contract != f.ContractVersion {
		writeJSON(w, 400, map[string]string{"error": "contract=" + f.ContractVersion + " is required"})
		return
	}
	if reject {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(503)
		return
	}
	w.Header().Set(headerVocabulary, "r1")
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher := w.(http.Flusher)
	flusher.Flush()
	pushed := map[string]bool{}
	for r.Context().Err() == nil {
		f.mu.Lock()
		if f.rejectOpsAsNonLeader > 0 {
			// A leader change drops the connection a non-leader was proxying.
			f.mu.Unlock()
			return
		}
		var due []json.RawMessage
		seen := map[string]int{}
		for _, raw := range f.queue {
			// An op queued twice (at-least-once) is two messages with one id.
			id := opID(raw)
			key := fmt.Sprintf("%s#%d", id, seen[id])
			seen[id]++
			if !pushed[key] {
				pushed[key] = true
				due = append(due, raw)
			}
		}
		f.mu.Unlock()
		for _, raw := range due {
			fmt.Fprintf(w, "event: op\nid: %s\ndata: %s\n\n", opID(raw), raw)
			flusher.Flush()
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (f *FakeManager) handleDone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ThreadID string `json:"threadId"`
		Error    string `json:"error"`
	}
	raw, _ := io.ReadAll(r.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	f.mu.Lock()
	f.completions = append(f.completions, Completion{ID: r.PathValue("id"), ThreadID: body.ThreadID, Error: body.Error})
	f.mu.Unlock()
	w.WriteHeader(204)
}

// handleWork mirrors handleOps: a long-poll that dequeues one work unit, or
// answers 204 once wait (clamped to 30s, same as the real contract) elapses
// with nothing queued.
func (f *FakeManager) handleWork(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := WorkRequest{Convo: q.Get("convo"), Pod: q.Get("pod")}
	fmt.Sscanf(q.Get("wait"), "%d", &req.Wait)
	f.mu.Lock()
	f.workRequests = append(f.workRequests, req)
	f.mu.Unlock()
	if req.Convo == "" {
		writeJSON(w, 400, map[string]string{"error": "missing convo"})
		return
	}
	wait := req.Wait
	if wait > 30 {
		wait = 30
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Until(deadline)):
		}
		f.mu.Lock()
		f.cond.Broadcast()
		f.mu.Unlock()
		close(done)
	}()
	f.mu.Lock()
	for len(f.workQueue) == 0 && time.Now().Before(deadline) && r.Context().Err() == nil {
		f.cond.Wait()
	}
	if len(f.workQueue) == 0 {
		f.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	raw := f.workQueue[0]
	f.workQueue = f.workQueue[1:]
	f.mu.Unlock()
	w.Header().Set(headerContentType, mimeJSON)
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

// handleWorkDone mirrors handleDone, recording the whole decoded body (so a
// test can assert on exact field PRESENCE — API-5/API-7/API-9) and updating
// the one derived, stateful fact this endpoint keeps about its own body: the
// latest-wins runtimeContextId/brief (API-5).
func (f *FakeManager) handleWorkDone(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	runID, _ := body["runId"].(string)
	convo, _ := body["convo"].(string)
	f.mu.Lock()
	reject := f.rejectWorkDone
	if reject == 0 {
		f.workCompletions = append(f.workCompletions, WorkCompletion{RunID: runID, Convo: convo, Raw: body})
		st := f.workState[convo]
		if v, ok := body["runtimeContextId"].(string); ok && v != "" {
			st.ContextID = v
		}
		if v, ok := body["brief"].(string); ok && v != "" {
			st.Brief = v
		}
		f.workState[convo] = st
	}
	f.mu.Unlock()
	if reject != 0 {
		writeJSON(w, reject, map[string]string{"error": errRejected})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (f *FakeManager) handleInbound(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	reject := f.rejectInbound
	if reject == 0 {
		f.inbound = append(f.inbound, body)
	}
	f.mu.Unlock()
	if reject != 0 {
		writeJSON(w, reject, map[string]string{"error": errRejected})
		return
	}
	writeJSON(w, 200, map[string]any{"accepted": true})
}

func (f *FakeManager) handleSignalInbound(w http.ResponseWriter, r *http.Request) {
	var post SignalPost
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &post)
	post.Bearer = strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
	f.mu.Lock()
	post.Rejected = f.rejectSignals != 0
	f.signalPosts = append(f.signalPosts, post)
	reject := f.rejectSignals
	f.mu.Unlock()
	if reject != 0 {
		writeJSON(w, reject, map[string]string{"error": errRejected})
		return
	}
	writeJSON(w, 200, map[string]any{"queued": len(post.Signals)})
}

func (f *FakeManager) handleStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ready   bool   `json:"ready"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	f.statusReports = append(f.statusReports, StatusReport{Name: r.PathValue("name"), Ready: in.Ready, Reason: in.Reason, Message: in.Message})
	f.mu.Unlock()
	w.WriteHeader(204)
}

func (f *FakeManager) stateGet(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("channel")
		if name == "" {
			name = r.PathValue("source")
		}
		f.mu.Lock()
		v := f.state[kind+"/"+name+"/"+r.PathValue("key")]
		f.mu.Unlock()
		writeJSON(w, 200, map[string]string{"value": v})
	}
}

func (f *FakeManager) statePut(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("channel")
		if name == "" {
			name = r.PathValue("source")
		}
		var in struct {
			Value string `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.state[kind+"/"+name+"/"+r.PathValue("key")] = in.Value
		f.mu.Unlock()
		w.WriteHeader(204)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set(headerContentType, mimeJSON)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// SetConversation stores (or replaces) a Conversation record, in the wire
// shape /state/conversations serves ({name,id,revision,createdAt,
// deletionRequestedAt,spec,status}), and broadcasts it as a `put` frame.
func (f *FakeManager) SetConversation(name, id string, spec, status map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.convs == nil {
		f.convs = map[string]map[string]any{}
	}
	f.convRev++
	rec := map[string]any{"name": name, "id": id, "revision": fmt.Sprint(f.convRev),
		"createdAt": "2026-01-01T00:00:00Z", "deletionRequestedAt": nil, "spec": spec, "status": status}
	f.convs[name] = rec
	body, _ := json.Marshal(rec)
	frame := []byte(fmt.Sprintf("event: put\nid: %d\ndata: %s\n\n", f.convRev, body))
	for _, ch := range f.stateWatch {
		select {
		case ch <- frame:
		default:
		}
	}
}

// FailConversationRead makes GET /state/conversations/{name} answer status
// until ClearConversationRead; the listing and stream are unaffected.
func (f *FakeManager) FailConversationRead(name string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stateFail == nil {
		f.stateFail = map[string]int{}
	}
	f.stateFail[name] = status
}

func (f *FakeManager) ClearConversationRead(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.stateFail, name)
}

func (f *FakeManager) stateList(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	items := make([]any, 0, len(f.convs))
	for _, rec := range f.convs {
		items = append(items, rec)
	}
	rev := fmt.Sprint(f.convRev)
	f.mu.Unlock()
	writeJSON(w, 200, map[string]any{"revision": rev, "items": items})
}

func (f *FakeManager) stateOne(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f.mu.Lock()
	status, failing := f.stateFail[name]
	rec, ok := f.convs[name]
	f.mu.Unlock()
	switch {
	case failing:
		writeJSON(w, status, map[string]string{"error": "injected"})
	case !ok:
		writeJSON(w, 404, map[string]string{"error": "not found"})
	default:
		writeJSON(w, 200, rec)
	}
}

func (f *FakeManager) stateStream(w http.ResponseWriter, r *http.Request) {
	ch := make(chan []byte, 16)
	f.mu.Lock()
	f.stateWatch = append(f.stateWatch, ch)
	f.mu.Unlock()
	w.Header().Set(headerContentType, mimeEventStream)
	w.WriteHeader(200)
	fl, _ := w.(http.Flusher)
	if fl != nil {
		fl.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case frame := <-ch:
			w.Write(frame)
			if fl != nil {
				fl.Flush()
			}
		}
	}
}
