package main

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	natsstate "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/nats"
)

func startTestNats(t *testing.T) *natsserver.Server {
	t.Helper()
	dir, err := os.MkdirTemp("", "agentops-nats-ready-*")
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
		t.Fatal("test nats-server never became ready")
	}
	return srv
}

// The readiness check reports not-ready while the one-time import has not
// completed, and while the server is unreachable.
func TestNatsReadinessCheckGatesOnTheImportAndTheServer(t *testing.T) {
	srv := startTestNats(t)
	conn, err := natsstate.Connect(natsstate.Config{URL: srv.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	backend, err := natsstate.OpenStores(context.Background(), conn, "agent-ops")
	if err != nil {
		t.Fatal(err)
	}

	check := natsReadinessCheck(conn, backend)
	req := httptest.NewRequest("GET", "/readyz", nil)
	if err := check(req); err == nil {
		t.Fatal("want not-ready before the import has completed")
	}

	conn.Close()
	if err := check(req); err == nil {
		t.Fatal("want not-ready with the server unreachable")
	}
}
