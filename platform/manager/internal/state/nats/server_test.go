package nats

import (
	"fmt"
	"os"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// newTestServer starts an in-process, single-node JetStream server: the
// conformance suite and the bench need a real server, never a fake, since
// the backend's own correctness rests on NATS's own compare-and-swap and
// watch semantics (design D-11's first gate). The caller stops it.
func newTestServer() (*natsserver.Server, error) {
	dir, err := os.MkdirTemp("", "agentops-nats-test-*")
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
