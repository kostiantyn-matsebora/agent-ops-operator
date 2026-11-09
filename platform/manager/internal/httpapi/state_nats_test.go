package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	natsstate "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/nats"
)

// The console learns a conversation closed, was read or was archived only from
// /state/conversations/stream. Behind the nats backend each of those is a
// write to the conversation record alone, touching no run, and each must still
// arrive as a put carrying the new value.
func natsBackendForTest(t *testing.T) (context.Context, *natsstate.Backend) {
	t.Helper()
	dir, err := os.MkdirTemp("", "agentops-http-nats-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: dir,
		NoLog: true, NoSigs: true, DisableJetStreamBanner: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	t.Cleanup(srv.Shutdown)
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats never became ready")
	}
	conn, err := natsstate.Connect(natsstate.Config{URL: srv.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	backend, err := natsstate.OpenStores(ctx, conn, "http")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)

	return ctx, backend
}

// sseFrames reads an event stream into "<event> <data>" strings until it ends.
func sseFrames(body io.Reader) <-chan string {
	frames := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		var event string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				frames <- event + " " + strings.TrimPrefix(line, "data: ")
			}
		}
		close(frames)
	}()
	return frames
}

// waitForPut consumes frames until a put containing the text arrives.
func waitForPut(t *testing.T, frames <-chan string, what, contains string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("stream ended before %s", what)
			}
			if strings.HasPrefix(f, "put ") && strings.Contains(f, contains) {
				return
			}
		case <-deadline:
			t.Fatalf("no put for %s", what)
		}
	}
}

func TestTheConversationStreamOverNatsCarriesCloseReadAndArchive(t *testing.T) {
	ctx, backend := natsBackendForTest(t)

	s := &Server{Conversations: backend.Stores.Conversations, Runs: backend.Stores.Runs}
	ts := httptest.NewServer(http.HandlerFunc(s.handleStateConversationsStream))
	t.Cleanup(ts.Close)
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	frames := sseFrames(resp.Body)
	wait := func(what, contains string) {
		t.Helper()
		waitForPut(t, frames, what, contains)
	}

	key := state.Key("stream-1")
	if _, err := backend.Stores.Conversations.Create(ctx, key, state.ConversationRecord{Name: "stream-1", Phase: "Active"}); err != nil {
		t.Fatal(err)
	}
	wait("the create", `"stream-1"`)

	closeIt := func(r *state.ConversationRecord) error { r.Phase = state.PhaseClosed; return nil }
	if _, err := state.Mutate(ctx, backend.Stores.Conversations, key, closeIt); err != nil {
		t.Fatal(err)
	}
	wait("the close", `Closed`)

	stamp := "2026-11-09T10:00:00Z"
	markRead := func(r *state.ConversationRecord) error {
		r.Threads = []state.ThreadBinding{{Channel: "ch", ThreadID: "t1", ReadAt: &stamp, ReadTracked: true}}
		return nil
	}
	if _, err := state.Mutate(ctx, backend.Stores.Conversations, key, markRead); err != nil {
		t.Fatal(err)
	}
	wait("the read mark", stamp)

	archive := func(r *state.ConversationRecord) error { r.ThreadsArchived = []string{"thread-9"}; return nil }
	if _, err := state.Mutate(ctx, backend.Stores.Conversations, key, archive); err != nil {
		t.Fatal(err)
	}
	wait("the archive", "thread-9")
}

// A quiet stream must still say something, or a reader cannot tell it from a
// connection that died without closing.
func TestAQuietStreamWritesKeepAlives(t *testing.T) {
	old := streamKeepAlive
	streamKeepAlive = 30 * time.Millisecond
	t.Cleanup(func() { streamKeepAlive = old })

	s := &Server{}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	open := func(ctx context.Context, since state.Revision) (<-chan sseFrame, error) {
		return make(chan sseFrame), nil // never produces a frame
	}
	rec := httptest.NewRecorder()
	s.streamSSE(rec, httptest.NewRequest("GET", "/state/conversations/stream", nil).WithContext(ctx), open)
	if n := strings.Count(rec.Body.String(), ": keep-alive"); n < 3 {
		t.Fatalf("a silent stream wrote %d keep-alives in 400ms, want several: %q", n, rec.Body.String())
	}
}

// The console's whole recovery is: list, keep the revision, open the stream
// from it. If the stream answers resync to that, the console relists and
// retries at once, for ever.
func TestAStreamOpenedFromTheListedRevisionIsNotToldToResync(t *testing.T) {
	ctx, backend := natsBackendForTest(t)
	key := state.Key("listed-1")
	if _, err := backend.Stores.Conversations.Create(ctx, key, state.ConversationRecord{Name: "listed-1", Phase: "Active"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Stores.Runs.Append(ctx, key, state.Run{RunID: "r1"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Conversations: backend.Stores.Conversations, Runs: backend.Stores.Runs}

	var rev string
	eventually := time.Now().Add(10 * time.Second)
	for rev == "" && time.Now().Before(eventually) {
		rec := httptest.NewRecorder()
		s.handleStateConversationsList(rec, httptest.NewRequest("GET", "/state/conversations", nil))
		var out struct {
			Revision string `json:"revision"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		rev = out.Revision
		time.Sleep(10 * time.Millisecond)
	}
	if rev == "" {
		t.Fatal("the listing never carried a revision")
	}

	sctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	s.handleStateConversationsStream(rec, httptest.NewRequest("GET", "/state/conversations/stream?since="+rev, nil).WithContext(sctx))
	if strings.Contains(rec.Body.String(), "event: resync") {
		t.Fatalf("the stream refused the revision the listing returned (%q): %q", rev, rec.Body.String())
	}
}
