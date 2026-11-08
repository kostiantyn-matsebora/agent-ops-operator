// This is the nats backend's own test, task 2.1-2.6 of
// openspec/changes/nats-state-store-default/: one call into the generic
// suite, driving every field of state.Stores through
// internal/state/conformance, against an in-process JetStream server
// (server_test.go), plus this package's own layout-specific tests.
package nats

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/conformance"
)

var scopeCounter atomic.Int64

// newScope returns a fresh scope name — a nats analogue of kube's own
// per-test namespace (suite_test.go there): every scenario asserting an
// exact set (List, Watch, scope isolation) gets its own bucket set so one
// test's fixtures never leak into another's assertions.
func newScope(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("test%d_%d", os.Getpid(), scopeCounter.Add(1))
}

// openBackend opens every store over conn scoped to scope, failing the test
// on anything this backend refuses.
func openBackend(t testing.TB, conn *Conn, scope string, opts ...Option) *Backend {
	t.Helper()
	b, err := OpenStores(context.Background(), conn, scope, opts...)
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	t.Cleanup(b.Close)
	return b
}

// openStores returns a func() state.Stores that opens a fresh, empty,
// isolated backend on every call (conformance.go's own contract on open):
// a brand-new scope, a fast sweep (the expiry scenarios wait on it), and the
// sweep goroutine running for the life of the test.
func openStores(t testing.TB) func() state.Stores {
	conn := sharedTestConn(t)
	return func() state.Stores {
		scope := newScope(t)
		b := openBackend(t, conn, scope, WithSweepInterval(50*time.Millisecond))
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go b.Run(ctx)
		return b.Stores
	}
}

func TestStores(t *testing.T) {
	conformance.RunStores(t, openStores(t))
}

func BenchmarkStores(b *testing.B) {
	conformance.BenchStores(b, openStores(b))
}

// TestBenchStoresRuns executes the benchmark harness once per store so a
// broken bench helper fails `go test`, not only a manual `-bench` run.
func TestBenchStoresRuns(t *testing.T) {
	prev := flag.Lookup("test.benchtime").Value.String()
	if err := flag.Set("test.benchtime", "1x"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.Set("test.benchtime", prev) })
	res := testing.Benchmark(func(b *testing.B) {
		conformance.BenchStores(b, openStores(t))
	})
	if res.N == 0 {
		t.Fatal("the benchmark harness did not complete a single iteration")
	}
}
