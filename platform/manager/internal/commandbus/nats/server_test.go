package nats

import (
	"fmt"
	"os"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// newTestServer starts an in-process, single-node JetStream server —
// mirrors internal/state/nats/server_test.go exactly (design D-1 keeps the
// two backends independent, so this is its own copy, not a shared helper):
// the conformance suite and the bench need a real server, since this
// backend's own correctness rests on NATS's compare-and-swap and watch
// semantics, the same reason the transport spike (tasks.md section 1)
// insisted on one.
func newTestServer() (*natsserver.Server, error) {
	dir, err := os.MkdirTemp("", "agentops-commandbus-nats-test-*")
	if err != nil {
		return nil, fmt.Errorf("MkdirTemp: %w", err)
	}
	opts := &natsserver.Options{
		Host:                   "127.0.0.1",
		Port:                   -1, // random free port
		JetStream:              true,
		StoreDir:               dir,
		NoLog:                  true,
		NoSigs:                 true,
		DisableJetStreamBanner: true,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("NewServer: %w", err)
	}
	srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("test nats-server never became ready")
	}
	return srv, nil
}
