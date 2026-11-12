package nats

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func startServerOn(t *testing.T, port int, dir string) *natsserver.Server {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: port, JetStream: true, StoreDir: dir,
		NoLog: true, NoSigs: true, DisableJetStreamBanner: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("server never became ready")
	}
	return srv
}

// The state server's pod restarts (a node drain, an upgrade). The manager must
// come back by itself: reads and lists serve what was written, and a feed that
// was open keeps delivering what is written after the server is back.
func TestTheBackendRecoversWhenTheServerRestarts(t *testing.T) {
	dir, err := os.MkdirTemp("", "agentops-nats-restart-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	port := freePort(t)
	srv := startServerOn(t, port, dir)
	t.Cleanup(func() { srv.Shutdown() })

	conn, err := Connect(Config{URL: fmt.Sprintf("nats://127.0.0.1:%d", port), Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	b := openBackend(t, conn, newScope(t))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	feed, err := b.Stores.Runs.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	key := state.Key("survivor")
	if _, err := b.Stores.Conversations.Create(ctx, key, state.ConversationRecord{Name: "survivor", Phase: "Active"}); err != nil {
		t.Fatal(err)
	}
	waitPut(t, ctx, feed, key, "Active")

	srv.Shutdown()
	srv.WaitForShutdown()
	srv = startServerOn(t, port, dir)

	// Writes succeed again once the client has reconnected.
	eventually(t, "a write goes through after the restart", func() bool {
		_, err := state.Mutate(ctx, b.Stores.Conversations, key, func(r *state.ConversationRecord) error {
			r.Phase = state.PhaseClosed
			return nil
		})
		return err == nil
	})
	eventually(t, "the list serves the post-restart write", func() bool {
		got, err := b.Stores.Conversations.List(ctx, state.ByPhase, state.PhaseClosed)
		return err == nil && len(got) == 1
	})
	waitPut(t, ctx, feed, key, state.PhaseClosed)
}

func waitPut(t *testing.T, ctx context.Context, feed <-chan state.Event[state.WithItems[state.ConversationRecord, state.Run]], key state.Key, phase string) {
	t.Helper()
	for {
		select {
		case ev, ok := <-feed:
			if !ok {
				t.Fatalf("the feed closed before delivering %q", phase)
			}
			if ev.Type == state.EventPut && ev.Key == key && ev.Record.Record.Phase == phase {
				return
			}
		case <-ctx.Done():
			t.Fatalf("the feed never delivered phase %q", phase)
		}
	}
}
