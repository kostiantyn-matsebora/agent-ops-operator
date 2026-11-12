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

// streamingManager is the manager's /channel/ops/stream as the console sees it:
// a retained log of ops identified by command id, a replay of every entry after
// the id Last-Event-ID names (the whole log when it names none or one the log
// does not hold), then each new entry. There is no acknowledgement to receive:
// a POST to /done is recorded so a test can assert there is none.
type streamingManager struct {
	t      *testing.T
	server *httptest.Server

	mu       sync.Mutex
	ops      []Op
	resumes  []string // Last-Event-ID of each connection, in order
	dones    int
	wake     chan struct{}
	dropConn []context.CancelFunc
	// replayFrom, when set, replays after this id whatever the client names: a
	// cursor older than what the console already rendered.
	replayFrom *string
}

func newStreamingManager(t *testing.T) *streamingManager {
	t.Helper()
	m := &streamingManager{t: t, wake: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /channel/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []ChannelInfo{{Name: "console"}})
	})
	mux.HandleFunc("POST /channel/channels/{name}/status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("GET /channel/ops/stream", m.serveStream)
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

func (m *streamingManager) serveStream(w http.ResponseWriter, r *http.Request) {
	after := r.Header.Get("Last-Event-ID")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	m.mu.Lock()
	m.resumes = append(m.resumes, after)
	m.dropConn = append(m.dropConn, cancel)
	if m.replayFrom != nil {
		after = *m.replayFrom
	}
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher := w.(http.Flusher)
	flusher.Flush()
	next := -1 // resolved on the first pass: an id this log does not hold starts from the oldest op
	for {
		m.mu.Lock()
		if next < 0 {
			next = 0
			for i, o := range m.ops {
				if o.ID == after && after != "" {
					next = i + 1
				}
			}
		}
		due := append([]Op(nil), m.ops[next:]...)
		next = len(m.ops)
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
		}
	}
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

func TestConsoleConsumesTheStreamInOrderAndAcknowledgesNothing(t *testing.T) {
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
	if m.resume(0) != "" {
		t.Fatalf("a first connection has nothing to resume, sent %q", m.resume(0))
	}
	time.Sleep(50 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dones != 0 {
		t.Fatalf("the console called /done %d times, want never", m.dones)
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

// A reconnect names the last event RECEIVED, with no acknowledgement ever sent,
// and an entry produced while away arrives after it.
func TestConsoleReconnectsFromTheLastReceivedIDWithoutLosingOrDuplicating(t *testing.T) {
	fastReconnect(t)
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)

	m.push("a")
	m.push("b")
	waitUntil(t, "a and b rendered", func() bool { return len(tr.Thread("thread-a")) == 1 && len(tr.Thread("thread-b")) == 1 })

	m.dropAll()
	m.push("c") // produced while the console reconnects
	waitUntil(t, "c rendered after the reconnect", func() bool { return len(tr.Thread("thread-c")) == 1 })
	if got := m.resume(-1); got != "b" {
		t.Fatalf("the reconnect sent Last-Event-ID %q, want b, the last event received", got)
	}
	time.Sleep(50 * time.Millisecond)
	for _, id := range []string{"a", "b", "c"} {
		if n := len(tr.Thread("thread-" + id)); n != 1 {
			t.Errorf("thread-%s rendered %d times, want exactly once", id, n)
		}
	}
}

// A cursor older than what the console already rendered replays entries it has:
// each is rendered once, by its command id.
func TestConsoleRendersAReplayedEntryOnce(t *testing.T) {
	fastReconnect(t)
	m := newStreamingManager(t)
	_, tr := startConsole(t, m)
	m.push("a")
	m.push("b")
	waitUntil(t, "a and b rendered", func() bool { return len(tr.Thread("thread-b")) == 1 })

	start := ""
	m.mu.Lock()
	m.replayFrom = &start // the next connection replays the whole log
	m.mu.Unlock()
	m.dropAll()
	m.push("c")
	waitUntil(t, "c rendered", func() bool { return len(tr.Thread("thread-c")) == 1 })
	for _, id := range []string{"a", "b", "c"} {
		if n := len(tr.Thread("thread-" + id)); n != 1 {
			t.Errorf("thread-%s rendered %d times after a replay, want once", id, n)
		}
	}
}

// A connection that goes silent is dropped by the idle watchdog and the console
// reconnects from its cursor.
func TestConsoleReconnectsWhenTheStreamGoesSilent(t *testing.T) {
	fastReconnect(t)
	old := streamIdleTimeout
	streamIdleTimeout = 80 * time.Millisecond
	t.Cleanup(func() { streamIdleTimeout = old })
	m := newStreamingManager(t)
	startConsole(t, m)
	waitUntil(t, "a second connection after a silent first", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.resumes) >= 2
	})
}

func TestStreamOpsRefusalsAreErrorsToBackOffFrom(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		t.Cleanup(srv.Close)
		err := NewManager(srv.URL, "t").StreamOps(context.Background(), "console", "", nil, func(*Op, string) error { return nil })
		if err == nil {
			t.Fatalf("status %d must be an error", status)
		}
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
