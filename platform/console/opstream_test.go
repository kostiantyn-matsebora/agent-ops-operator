package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// streamingManager is the manager's /channel/ops/stream and /done as the
// console sees them: ops in log order identified by command id, a resume that
// replays what is unacknowledged after the op Last-Event-ID names (every
// unacknowledged op when it names none, or one the log does not hold), and
// acknowledgements that settle.
type streamingManager struct {
	t      *testing.T
	server *httptest.Server

	mu       sync.Mutex
	ops      []streamedOp
	acked    map[string]int // id -> acknowledgements received
	settled  map[string]bool
	failAck  map[string]int // id -> acknowledgements to refuse with a 500 before accepting
	resumes  []string       // Last-Event-ID of each connection, in order
	wake     chan struct{}
	dropConn []context.CancelFunc
}

type streamedOp struct{ op Op }

func newStreamingManager(t *testing.T) *streamingManager {
	t.Helper()
	m := &streamingManager{t: t, acked: map[string]int{}, settled: map[string]bool{}, failAck: map[string]int{}, wake: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /channel/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []ChannelInfo{{Name: "console"}})
	})
	mux.HandleFunc("POST /channel/channels/{name}/status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("GET /channel/ops/stream", m.serveStream)
	mux.HandleFunc("POST /channel/ops/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.failAck[id] > 0 {
			m.failAck[id]--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		m.acked[id]++
		m.settled[id] = true
		w.WriteHeader(http.StatusOK)
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func (m *streamingManager) push(id string) {
	m.mu.Lock()
	m.ops = append(m.ops, streamedOp{op: Op{
		ID: id, Channel: "console", Kind: "send", ThreadID: strPtr("thread-" + id),
		Message: &OpMessage{Kind: "answer", Body: "body " + id},
	}})
	old := m.wake
	m.wake = make(chan struct{})
	m.mu.Unlock()
	close(old)
}

func strPtr(s string) *string { return &s }

func (m *streamingManager) serveStream(w http.ResponseWriter, r *http.Request) {
	after := r.Header.Get("Last-Event-ID")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	m.mu.Lock()
	m.resumes = append(m.resumes, r.Header.Get("Last-Event-ID"))
	m.dropConn = append(m.dropConn, cancel)
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher := w.(http.Flusher)
	flusher.Flush()
	sent := map[string]bool{}
	for {
		m.mu.Lock()
		first := 0 // an id this log does not hold starts from the oldest op
		for i, o := range m.ops {
			if o.op.ID == after {
				first = i + 1
			}
		}
		var due []streamedOp
		for _, o := range m.ops[first:] {
			if !m.settled[o.op.ID] && !sent[o.op.ID] {
				due = append(due, o)
				sent[o.op.ID] = true
			}
		}
		wake := m.wake
		m.mu.Unlock()
		for _, o := range due {
			data, _ := json.Marshal(claimedOpWire{Op: o.op, ReclaimAfterSeconds: 300})
			fmt.Fprintf(w, "event: op\nid: %s\ndata: %s\n\n", o.op.ID, data)
			flusher.Flush()
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
	}
}

// claimedOpWire is the manager's claimedOp: the op plus the lease hint.
type claimedOpWire struct {
	Op
	ReclaimAfterSeconds int `json:"reclaimAfterSeconds"`
}

// resume is the Last-Event-ID of connection i (negative counts from the last).
func (m *streamingManager) resume(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 {
		i += len(m.resumes)
	}
	return m.resumes[i]
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

func (m *streamingManager) waitFor(what string, cond func() bool) {
	m.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		ok := cond()
		m.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.t.Fatalf("timed out waiting for %s", what)
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

func TestConsoleConsumesTheStreamInOrderAndAcknowledgesEachOp(t *testing.T) {
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)
	m.push("a")
	m.push("b")
	m.push("c")

	m.waitFor("three acknowledgements", func() bool { return len(m.settled) == 3 })
	for _, id := range []string{"a", "b", "c"} {
		if msgs := tr.Thread("thread-" + id); len(msgs) != 1 || msgs[0].Text != "body "+id {
			t.Fatalf("thread-%s = %+v, want the one op rendered", id, msgs)
		}
	}
	if m.resume(0) != "" {
		t.Fatalf("a first connection has nothing to resume, sent %q", m.resume(0))
	}
}

func TestConsoleResumesAfterADropWithoutLosingOrDuplicating(t *testing.T) {
	fastReconnect(t)
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)

	m.push("a")
	m.push("b")
	m.waitFor("a and b acknowledged", func() bool { return m.settled["a"] && m.settled["b"] })

	// c is delivered and rendered, but its acknowledgement is refused: the
	// manager still holds it unacknowledged when the console reconnects.
	m.mu.Lock()
	m.failAck["c"] = 1
	m.mu.Unlock()
	m.push("c")
	m.waitFor("a reconnect after the refused ack", func() bool { return len(m.resumes) >= 2 })
	m.waitFor("c acknowledged on the replay", func() bool { return m.settled["c"] })

	if got := m.resume(1); got != "" {
		t.Fatalf("the reconnect sent Last-Event-ID %q, want none: c is outstanding, so the manager starts from the oldest unacknowledged op", got)
	}
	m.push("d")
	m.waitFor("d acknowledged", func() bool { return m.settled["d"] })

	// A forced drop with everything acknowledged resumes from the last
	// acknowledged command id and replays nothing.
	m.mu.Lock()
	before := len(m.resumes)
	m.mu.Unlock()
	m.dropAll()
	m.waitFor("a reconnect after the forced drop", func() bool { return len(m.resumes) > before })
	if got := m.resume(-1); got != "d" {
		t.Fatalf("after acknowledging everything the reconnect sent %q, want d, the last acknowledged id", got)
	}

	time.Sleep(50 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range []string{"a", "b", "c", "d"} {
		if msgs := tr.Thread("thread-" + id); len(msgs) != 1 {
			t.Errorf("thread-%s rendered %d times, want exactly once", id, len(msgs))
		}
		if m.acked[id] != 1 {
			t.Errorf("%s acknowledged %d times, want once", id, m.acked[id])
		}
	}
}

func TestConsoleFallsBackToTheLongPollWhenTheManagerHasNoStream(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := newFakeManager(t, ChannelInfo{Name: "console"})
			streamHits := 0
			var mu sync.Mutex
			inner := f.server.Config.Handler
			f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/channel/ops/stream" {
					mu.Lock()
					streamHits++
					mu.Unlock()
					w.WriteHeader(status)
					return
				}
				inner.ServeHTTP(w, r)
			})
			a, tr, _ := consoleUnderTest(t, f)
			thread := "console-uid-1"
			f.queue(Op{ID: "p1", Channel: "console", Kind: "send", ThreadID: &thread, Message: &OpMessage{Kind: "answer", Body: "polled"}})

			runUntil(t, a, func() bool { return len(f.completions()) == 1 })
			if msgs := tr.Thread(thread); len(msgs) != 1 || msgs[0].Text != "polled" {
				t.Fatalf("the long-poll did not serve the op: %+v", msgs)
			}
			mu.Lock()
			defer mu.Unlock()
			if streamHits != 1 {
				t.Fatalf("the stream was asked %d times, want once and then the long-poll", streamHits)
			}
		})
	}
}

func TestStreamOpsReportsAnUnsupportedManagerAndRefusals(t *testing.T) {
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	t.Cleanup(srv.Close)
	m := NewManager(srv.URL, "t")
	call := func() error {
		return m.StreamOps(context.Background(), "console", "", nil, func(*Op, string) error { return nil })
	}
	if err := call(); !errors.Is(err, errStreamUnsupported) {
		t.Fatalf("404 = %v, want errStreamUnsupported", err)
	}
	status = http.StatusForbidden
	if err := call(); err == nil || errors.Is(err, errStreamUnsupported) {
		t.Fatalf("403 must be an error to back off from, not a fallback: %v", err)
	}
}

func TestStreamOpsCarriesVocabularyRevisionsAndTheResumeHeader(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("Last-Event-ID")
		w.Header().Set(VocabularyRevisionHeader, "r1")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keep-alive\n\nevent: vocabulary\ndata: {\"revision\":\"r2\"}\n\n")
	}))
	t.Cleanup(srv.Close)
	var revs []string
	err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console", "op-7",
		func(r string) { revs = append(revs, r) }, func(*Op, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if gotID != "op-7" || len(revs) != 2 || revs[0] != "r1" || revs[1] != "r2" {
		t.Fatalf("Last-Event-ID %q, revisions %v", gotID, revs)
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
	err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console", "", nil, func(*Op, string) error { return nil })
	if !errors.Is(err, errStreamIdle) {
		t.Fatalf("a stream that went silent returned %v, want errStreamIdle", err)
	}
}

func TestFeedCursorResumeToken(t *testing.T) {
	var c feedCursor
	if got := c.resumeToken(); got != "" {
		t.Fatalf("nothing received: %q", got)
	}
	c.received("op-a")
	if got := c.resumeToken(); got != "" {
		t.Fatalf("an op received and not acknowledged leaves nothing to resume after: %q", got)
	}
	c.acknowledged("op-a")
	if got := c.resumeToken(); got != "op-a" {
		t.Fatalf("after acking op-a: %q", got)
	}
	c.received("op-c")
	c.received("op-b") // another channel's op, older in the log, delivered later
	c.acknowledged("op-c")
	if got := c.resumeToken(); got != "" {
		t.Fatalf("op-b is still outstanding after op-c was acknowledged, so no id may be named: %q", got)
	}
	c.acknowledged("op-b")
	if got := c.resumeToken(); got != "op-b" {
		t.Fatalf("all acknowledged: the last acknowledged id is the token, got %q", got)
	}
}
