package httpapi

import (
	"bufio"
	"context"
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
func TestTheConversationStreamOverNatsCarriesCloseReadAndArchive(t *testing.T) {
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
	defer cancel()
	backend, err := natsstate.OpenStores(ctx, conn, "http")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)

	s := &Server{Conversations: backend.Stores.Conversations, Runs: backend.Stores.Runs}
	ts := httptest.NewServer(http.HandlerFunc(s.handleStateConversationsStream))
	t.Cleanup(ts.Close)
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	frames := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
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
	wait := func(what, contains string) {
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
