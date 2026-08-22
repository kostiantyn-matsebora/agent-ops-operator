package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sseServer serves a fixed SSE script, recording the `since` cursor of every
// connection so a test can assert the consumer resumes rather than restarts.
type sseServer struct {
	frames []string
	seen   chan string
}

func (s *sseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case s.seen <- r.URL.Query().Get("since"):
	default:
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher, _ := w.(http.Flusher)
	for _, f := range s.frames {
		fmt.Fprint(w, f)
		if flusher != nil {
			flusher.Flush()
		}
	}
	// hold the connection so the consumer does not immediately reconnect
	<-r.Context().Done()
}

func sseEvent(t *testing.T, e ActivityEvent) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return "event: activity\ndata: " + string(b) + "\n\n"
}

// waitForNamed is waitFor with a message naming what was awaited — the stream
// tests have several waits each, and "condition not reached" would not say which.
func waitForNamed(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestActivityConsumerIngestsAndIndexes(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(&sseServer{seen: make(chan string, 4), frames: []string{
		": keep-alive\n\n",
		sseEvent(t, ActivityEvent{Cursor: "0000000000000001", TS: now, Kind: "run.dispatched",
			From: &NodeRef{"pipeline", "k8s-ops"}, To: &NodeRef{"runtime", "default"},
			Conversation: "chat-1", Status: "ok"}),
		sseEvent(t, ActivityEvent{Cursor: "0000000000000002", TS: now, Kind: "run.completed",
			From: &NodeRef{"runtime", "default"}, To: &NodeRef{"pipeline", "k8s-ops"},
			Conversation: "chat-1", Status: "ok", LatencyMs: 4000}),
		sseEvent(t, ActivityEvent{Cursor: "0000000000000003", TS: now, Kind: "run.completed",
			From: &NodeRef{"runtime", "default"}, To: &NodeRef{"pipeline", "k8s-ops"},
			Conversation: "chat-2", Status: "error", LatencyMs: 2000}),
	}})
	defer srv.Close()

	w := NewActivityWindow(NewManager(srv.URL, "t"), 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	waitForNamed(t, "three events", func() bool { return len(w.Since("", 0)) == 3 })

	if h := w.StreamHealth(); !h.Connected || h.Cursor != "0000000000000003" {
		t.Fatalf("stream health: %+v", h)
	}
	if got := w.ForConversation("chat-1"); len(got) != 2 {
		t.Fatalf("conversation index: want 2 hops for chat-1, got %d", len(got))
	}

	edges := w.Edges(time.Minute)
	if len(edges) != 2 {
		t.Fatalf("want 2 distinct edges, got %d: %+v", len(edges), edges)
	}
	var completed *EdgeStat
	for i := range edges {
		if edges[i].From.Kind == "runtime" {
			completed = &edges[i]
		}
	}
	if completed == nil {
		t.Fatalf("runtime -> pipeline edge missing: %+v", edges)
	}
	if completed.Events != 2 || completed.Errors != 1 {
		t.Fatalf("edge counts: %+v", completed)
	}
	if completed.RatePerMin != 2 {
		t.Fatalf("rate over a 1m window with 2 events = %v, want 2", completed.RatePerMin)
	}
	if completed.MaxLatencyMs != 4000 {
		t.Fatalf("max latency: %+v", completed)
	}
	// only the requested window counts
	if edges := w.Edges(time.Minute); len(edges) == 0 {
		t.Fatal("a live window must report the edges it saw")
	}
}

// The window is bounded: it holds a window of recent motion, never an archive.
func TestActivityWindowIsBounded(t *testing.T) {
	w := NewActivityWindow(NewManager("http://unused", "t"), 5)
	for i := 0; i < 50; i++ {
		w.add(ActivityEvent{Cursor: fmt.Sprintf("%016d", i+1), TS: time.Now(), Kind: "input.queued"})
	}
	if got := len(w.Since("", 0)); got != 5 {
		t.Fatalf("window held %d events, want 5", got)
	}
	if w.Cursor() != fmt.Sprintf("%016d", 50) {
		t.Fatalf("cursor did not advance: %q", w.Cursor())
	}
}

// A resync frame is COUNTED and PLACED. The count says history was lost; the
// marker says WHERE — the events either side of it are not consecutive, and a
// timeline showing them adjacent claims something false.
func TestActivityConsumerRecordsResyncAsAGapInTheTimeline(t *testing.T) {
	srv := httptest.NewServer(&sseServer{seen: make(chan string, 4), frames: []string{
		"event: resync\ndata: {\"reason\":\"cursor evicted\"}\n\n",
		sseEvent(t, ActivityEvent{Cursor: "0000000000000009", TS: time.Now(), Kind: "input.queued"}),
	}})
	defer srv.Close()

	w := NewActivityWindow(NewManager(srv.URL, "t"), 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	waitForNamed(t, "resync recorded", func() bool { return w.StreamHealth().Resyncs == 1 })
	waitForNamed(t, "the gap and the event after it", func() bool { return len(w.Since("", 0)) == 2 })

	events := w.Since("", 0)
	gap := events[0]
	if gap.Kind != KindActivityGap {
		t.Fatalf("the timeline must open with the gap, got %q", gap.Kind)
	}
	if gap.Detail != "cursor evicted" {
		t.Fatalf("the gap must carry the reason it was told, got %q", gap.Detail)
	}
	if gap.From != nil || gap.To != nil {
		t.Fatal("a gap is not a hop — it must never render as motion along an edge")
	}
	if events[1].Kind != "input.queued" {
		t.Fatalf("the real event must follow the gap, got %q", events[1].Kind)
	}
}

// The reader who matters is the one who opens the console AFTER something went
// wrong. A gap reported only on the live stream is invisible to exactly that
// person, so it rides on the stream HEALTH, which every view already reads.
func TestGapIsReportedToAViewerThatArrivesLate(t *testing.T) {
	srv := httptest.NewServer(&sseServer{seen: make(chan string, 4), frames: []string{
		"event: resync\ndata: {\"reason\":\"cursor predates this manager process\"}\n\n",
		sseEvent(t, ActivityEvent{Cursor: "0000000000000001", TS: time.Now(), Kind: "input.queued"}),
	}})
	defer srv.Close()

	w := NewActivityWindow(NewManager(srv.URL, "t"), 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	waitForNamed(t, "the gap on the health", func() bool { return w.StreamHealth().LastGap != nil })

	gap := w.StreamHealth().LastGap
	if gap.Detail != "cursor predates this manager process" {
		t.Fatalf("the health must carry WHY history is missing, got %q", gap.Detail)
	}
	if gap.TS.IsZero() {
		t.Fatal("a gap needs its time — the banner says what history is unavailable BEFORE")
	}
	// Health is a snapshot, not a stream: asking again keeps reporting it.
	if again := w.StreamHealth().LastGap; again == nil || !again.TS.Equal(gap.TS) {
		t.Fatal("the gap must persist for the next viewer, not be consumed by the first")
	}
}

// After a MANAGER restart the upstream sequence starts again at 1, so a
// retained cursor is permanently "ahead": every reconnect resyncs, and the same
// backlog is appended each time. Dropping it on resync is what stops that.
func TestResyncDropsTheStaleCursor(t *testing.T) {
	srv := httptest.NewServer(&sseServer{seen: make(chan string, 8), frames: []string{
		sseEvent(t, ActivityEvent{Cursor: "0000000000009999", TS: time.Now(), Kind: "run.completed"}),
		"event: resync\ndata: {\"reason\":\"cursor predates this manager process\"}\n\n",
		sseEvent(t, ActivityEvent{Cursor: "0000000000000001", TS: time.Now(), Kind: "input.queued"}),
	}})
	defer srv.Close()

	w := NewActivityWindow(NewManager(srv.URL, "t"), 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	waitForNamed(t, "the post-restart event", func() bool { return len(w.Since("", 0)) == 3 })
	if got := w.Cursor(); got != "0000000000000001" {
		t.Fatalf("the cursor must track the NEW process's sequence, got %q", got)
	}
}

// Reconnect resumes from the last cursor rather than restarting — that is what
// keeps a reconnect from replaying the whole buffer into every browser.
func TestActivityConsumerResumesFromItsCursor(t *testing.T) {
	seen := make(chan string, 8)
	srv := httptest.NewServer(&sseServer{seen: seen, frames: []string{
		sseEvent(t, ActivityEvent{Cursor: "0000000000000007", TS: time.Now(), Kind: "input.queued"}),
	}})
	defer srv.Close()

	w := NewActivityWindow(NewManager(srv.URL, "t"), 100)
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	first := <-seen
	if first != "" {
		t.Fatalf("first connect must not send a cursor, sent %q", first)
	}
	waitForNamed(t, "the first event", func() bool { return w.Cursor() == "0000000000000007" })

	// force a reconnect by cancelling and restarting the loop with the same window
	cancel()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go w.Run(ctx2)
	second := <-seen
	if second != "0000000000000007" {
		t.Fatalf("reconnect must resume from the last cursor, sent %q", second)
	}
}

// The core group lives under /api, everything else under /apis — the one
// irregularity in the REST layout, and the reason a wrong path here shows up as
// a 404 on pods only.
func TestResourcePathsCoverBothAPIRoots(t *testing.T) {
	k := &Kube{Namespace: "agent-ops"}
	cases := map[string]string{
		"conversations": "/apis/agentops.dev/v1alpha1/namespaces/agent-ops/conversations",
		"deployments":   "/apis/apps/v1/namespaces/agent-ops/deployments",
		"pods":          "/api/v1/namespaces/agent-ops/pods",
	}
	for kind, want := range cases {
		if got := k.resourcePath(kind); got != want {
			t.Fatalf("%s -> %q, want %q", kind, got, want)
		}
	}
	if !AgentOpsKind("conversations") || AgentOpsKind("pods") {
		t.Fatal("install kinds must be distinguishable from agentops.dev kinds")
	}
}
