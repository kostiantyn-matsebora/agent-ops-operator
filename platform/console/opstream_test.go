package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// streamingManager is the manager's /channel/ops/stream and ack as the console
// sees them (design D-17): every unacknowledged op is pushed once per
// connection, an acknowledgement consumes the op, and a reconnect is offered
// whatever was never acknowledged. A POST to /done is recorded so a test can
// assert there is none.
type streamingManager struct {
	t      *testing.T
	server *httptest.Server

	mu          sync.Mutex
	ops         []Op
	acked       map[string]bool
	ackOrder    []string
	ackStatus   map[string][]int // per id, the statuses to answer, first to last; then 200
	connections int
	dones       int
	wake        chan struct{}
	dropConn    []context.CancelFunc
}

func newStreamingManager(t *testing.T) *streamingManager {
	t.Helper()
	m := &streamingManager{t: t, wake: make(chan struct{}), acked: map[string]bool{}, ackStatus: map[string][]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /channel/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []ChannelInfo{{Name: "console"}})
	})
	mux.HandleFunc("POST /channel/channels/{name}/status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("GET /channel/ops/stream", m.serveStream)
	mux.HandleFunc("POST /channel/ops/{id}/ack", m.serveAck)
	mux.HandleFunc("POST /channel/ops/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.dones++
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func (m *streamingManager) push(id string) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{
		ID: id, Channel: "console", Kind: "send", ThreadID: strPtr("thread-" + id),
		Message: &OpMessage{Kind: "answer", Body: "body " + id},
	})
	old := m.wake
	m.wake = make(chan struct{})
	m.mu.Unlock()
	close(old)
}

func strPtr(s string) *string { return &s }

// serveAck consumes the op, answering a scripted status first when a test set
// one. A message already consumed answers 404, as the manager does.
func (m *streamingManager) serveAck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m.mu.Lock()
	defer m.mu.Unlock()
	if script := m.ackStatus[id]; len(script) > 0 {
		m.ackStatus[id] = script[1:]
		w.WriteHeader(script[0])
		return
	}
	if m.acked[id] {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	m.acked[id] = true
	m.ackOrder = append(m.ackOrder, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "consumed": true})
}

func (m *streamingManager) serveStream(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	m.mu.Lock()
	m.connections++
	m.dropConn = append(m.dropConn, cancel)
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher := w.(http.Flusher)
	flusher.Flush()
	pushed := map[int]bool{}
	for {
		m.mu.Lock()
		var due []Op
		for i, o := range m.ops {
			if !m.acked[o.ID] && !pushed[i] {
				pushed[i] = true
				due = append(due, o)
			}
		}
		wake := m.wake
		m.mu.Unlock()
		for _, op := range due {
			data, _ := json.Marshal(op)
			fmt.Fprintf(w, "event: op\nid: %s\ndata: %s\n\n", op.ID, data)
			flusher.Flush()
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (m *streamingManager) acknowledged() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.ackOrder...)
}

func (m *streamingManager) connectionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connections
}

func (m *streamingManager) dropAll() {
	m.mu.Lock()
	cancels := m.dropConn
	m.dropConn = nil
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func fastReconnect(t *testing.T) {
	t.Helper()
	oi, om := streamBackoffInitial, streamBackoffMax
	streamBackoffInitial, streamBackoffMax = time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { streamBackoffInitial, streamBackoffMax = oi, om })
}

func startConsole(t *testing.T, m *streamingManager) (*Adapter, *Transcripts) {
	t.Helper()
	cache := staticCache()
	for _, k := range ConfigurableKinds {
		cache.replace(k, cache.List(k))
	}
	tr := NewTranscripts()
	a := NewAdapter(NewManager(m.server.URL, "token"), cache, tr, "console")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()
	// Stopped before the test returns: Run reads package settings a later test
	// changes.
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return a, tr
}

func TestConsoleAppliesThenAcknowledgesEachOpInOrderAndReportsNoDone(t *testing.T) {
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)
	m.push("a")
	m.push("b")
	m.push("c")

	for _, id := range []string{"a", "b", "c"} {
		id := id
		waitUntil(t, "thread-"+id+" rendered", func() bool { return len(tr.Thread("thread-"+id)) == 1 })
		if msgs := tr.Thread("thread-" + id); msgs[0].Text != "body "+id {
			t.Fatalf("thread-%s = %+v, want the op rendered", id, msgs)
		}
	}
	waitUntil(t, "every op acknowledged", func() bool { return len(m.acknowledged()) == 3 })
	if got := fmt.Sprint(m.acknowledged()); got != "[a b c]" {
		t.Fatalf("acknowledged %s, want [a b c] in order", got)
	}
	time.Sleep(50 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dones != 0 {
		t.Fatalf("the console called /done %d times, want never: its acknowledgement is its receipt", m.dones)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A dropped connection reconnects with no cursor and is offered only what was
// never acknowledged: what it acknowledged is consumed and never returns.
func TestConsoleReconnectIsOfferedOnlyWhatWasNeverAcknowledged(t *testing.T) {
	fastReconnect(t)
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)

	m.push("a")
	m.push("b")
	waitUntil(t, "a and b acknowledged", func() bool { return len(m.acknowledged()) == 2 })

	m.dropAll()
	m.push("c") // produced while the console reconnects
	waitUntil(t, "c rendered after the reconnect", func() bool { return len(tr.Thread("thread-c")) == 1 })
	waitUntil(t, "c acknowledged", func() bool { return len(m.acknowledged()) == 3 })
	time.Sleep(50 * time.Millisecond)
	if got := fmt.Sprint(m.acknowledged()); got != "[a b c]" {
		t.Fatalf("acknowledged %s: a consumed op must never be offered again", got)
	}
	for _, id := range []string{"a", "b", "c"} {
		if n := len(tr.Thread("thread-" + id)); n != 1 {
			t.Errorf("thread-%s rendered %d times, want exactly once", id, n)
		}
	}
}

// A failed acknowledgement drops the connection; the manager still holds the op,
// so the reconnect is offered it again, and it renders once.
func TestConsoleFailedAcknowledgementIsOfferedAgainAndRendersOnce(t *testing.T) {
	fastReconnect(t)
	m := newStreamingManager(t)
	m.ackStatus["a"] = []int{http.StatusServiceUnavailable}
	_, tr := startConsole(t, m)
	m.push("a")
	waitUntil(t, "a acknowledged on the second offer", func() bool { return len(m.acknowledged()) == 1 })
	if m.connectionCount() < 2 {
		t.Fatalf("a failed acknowledgement must drop the connection, saw %d", m.connectionCount())
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(tr.Thread("thread-a")); n != 1 {
		t.Fatalf("thread-a rendered %d times, want exactly once", n)
	}
}

// An acknowledgement the manager answers 404 (already consumed) is a harmless
// duplicate, not a reason to drop the connection.
func TestConsoleAcknowledgementOfAConsumedOpIsHarmless(t *testing.T) {
	m := newStreamingManager(t)
	m.ackStatus["a"] = []int{http.StatusNotFound}
	_, tr := startConsole(t, m)
	m.push("a")
	m.push("b")
	waitUntil(t, "b rendered", func() bool { return len(tr.Thread("thread-b")) == 1 })
	waitUntil(t, "b acknowledged", func() bool { return len(m.acknowledged()) == 1 })
	if n := m.connectionCount(); n != 1 {
		t.Fatalf("a 404 acknowledgement dropped the connection: %d connections", n)
	}
}

// An op the manager sends again is rendered once, by its id.
func TestConsoleRendersARepeatedOpOnce(t *testing.T) {
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)
	m.push("a")
	m.push("a")
	waitUntil(t, "both pushed and acknowledged", func() bool { return len(m.acknowledged()) >= 1 })
	time.Sleep(50 * time.Millisecond)
	if n := len(tr.Thread("thread-a")); n != 1 {
		t.Errorf("thread-a rendered %d times after a repeat, want once", n)
	}
}

// A connection that goes silent is dropped by the idle watchdog and the console
// reconnects.
func TestConsoleReconnectsWhenTheStreamGoesSilent(t *testing.T) {
	fastReconnect(t)
	old := streamIdleTimeout
	streamIdleTimeout = 80 * time.Millisecond
	t.Cleanup(func() { streamIdleTimeout = old })
	m := newStreamingManager(t)
	startConsole(t, m)
	waitUntil(t, "a second connection after a silent first", func() bool { return m.connectionCount() >= 2 })
}

func TestStreamOpsRefusalsAreErrorsToBackOffFrom(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		t.Cleanup(srv.Close)
		err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console", nil, func(*Op) error { return nil })
		if err == nil {
			t.Fatalf("status %d must be an error", status)
		}
	}
}

func TestStreamOpsCarriesVocabularyRevisions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(VocabularyRevisionHeader, "r1")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keep-alive\n\nevent: vocabulary\ndata: {\"revision\":\"r2\"}\n\n")
	}))
	t.Cleanup(srv.Close)
	var revs []string
	err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console",
		func(r string) { revs = append(revs, r) }, func(*Op) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 || revs[0] != "r1" || revs[1] != "r2" {
		t.Fatalf("revisions %v", revs)
	}
}

func TestStreamOpsTreatsASilentConnectionAsDead(t *testing.T) {
	old := streamIdleTimeout
	streamIdleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { streamIdleTimeout = old })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console", nil, func(*Op) error { return nil })
	if !errors.Is(err, errStreamIdle) {
		t.Fatalf("a stream that went silent returned %v, want errStreamIdle", err)
	}
}

func TestAckOpEscapesTheIDAndToleratesAGoneMessage(t *testing.T) {
	var gotPath, gotQuery string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.EscapedPath(), r.URL.RawQuery
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	m := NewManager(srv.URL, "t")
	if err := m.AckOp(context.Background(), "console", "topic:conv/1:console"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/channel/ops/topic:conv%2F1:console/ack" || gotQuery != "adapter=console" {
		t.Fatalf("request %s?%s", gotPath, gotQuery)
	}
	status = http.StatusNotFound
	if err := m.AckOp(context.Background(), "console", "x"); err != nil {
		t.Fatalf("a gone message is a harmless duplicate, got %v", err)
	}
	status = http.StatusConflict
	if err := m.AckOp(context.Background(), "console", "x"); err == nil {
		t.Fatal("a refusal other than 404 must be an error")
	}
}
