package nats

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// One in-process JetStream server and one connection for the whole test
// binary, mirroring internal/state/nats/suite_test.go's own setup; each
// test scopes its own bucket set (newScope) instead of its own server.
var (
	testServer *natsserver.Server
	testConn   *Conn
)

func TestMain(m *testing.M) {
	srv, err := newTestServer()
	if err != nil {
		panic(err)
	}
	testServer = srv

	conn, err := Connect(Config{URL: srv.ClientURL(), Replicas: 1}) // a single-node test server has nothing to replicate to
	if err != nil {
		srv.Shutdown()
		panic(err)
	}
	testConn = conn

	code := m.Run()
	testConn.Close()
	testServer.Shutdown()
	testServer.WaitForShutdown()
	os.Exit(code)
}

var scopeCounter atomic.Int64

// newScope returns a fresh bucket-set scope, isolating one test's fixtures
// from another's assertions over the one shared server.
func newScope(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("test%d_%d", os.Getpid(), scopeCounter.Add(1))
}
