package nats

import (
	"os"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// One in-process JetStream server and one connection for the whole test
// binary (mirroring internal/state/kube/suite_test.go's one envtest
// apiserver); each test scopes its own bucket set instead of its own
// server (stores_test.go's newScope).
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

	conn, err := Connect(Config{URL: srv.ClientURL(), Replicas: 1})
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

// sharedTestConn returns the one Conn dialled to the shared test server.
func sharedTestConn(t testing.TB) *Conn {
	t.Helper()
	return testConn
}
