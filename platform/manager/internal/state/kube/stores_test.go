// This is the kube backend's own test, task 4.3 of
// openspec/changes/state-store-generic-contract/: one call into the generic
// suite, driving every field of state.Stores through
// internal/state/conformance, plus the layout-specific tests elsewhere in
// this package (the ones asserting an actual object or annotation shape —
// fixtures_test.go, lifecycle_test.go, store_test.go, runs_test.go,
// signals_test.go, expiry_test.go, items_test.go, getwithruns_test.go,
// statusconflict_test.go, bookmark_test.go, cachedlists_test.go,
// adapterstate_test.go, vocabulary_test.go).
package kube

import (
	"context"
	"flag"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/conformance"
)

// provisionFixtureParents creates the one SignalSource and one Channel every
// Cooldowns, Receipts and AdapterState fixture in internal/state/conformance
// reads or writes through (see fixtures.go's own comment on
// FixtureSourceName/FixtureChannelName: a Part-based collection's Create
// needs its owning object to already exist). ns is the CONTROL namespace
// (split-conversation-namespace task 9.4): Cooldowns, Receipts and
// AdapterState all open there, so that is where their owning objects must
// live, whether or not Conversations/Runs/Blobs share that namespace.
func provisionFixtureParents(t testing.TB, ns string) {
	t.Helper()
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = conformance.FixtureSourceName, ns
	src.Spec.Adapter = "cron"
	if err := wwClient.Create(context.Background(), src); err != nil {
		t.Fatalf("provision fixture SignalSource: %v", err)
	}
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = conformance.FixtureChannelName, ns
	ch.Spec.Adapter = "telegram"
	if err := wwClient.Create(context.Background(), ch); err != nil {
		t.Fatalf("provision fixture Channel: %v", err)
	}
}

// openStoresPair returns a func() state.Stores that opens a fresh, empty,
// isolated backend on every call, scoped to a pair mkPair builds fresh each
// time: its fixture parents (provisioned in pair.Control, where Cooldowns/
// Receipts/AdapterState actually open), OpenStores over it with a fast sweep
// (the expiry scenarios wait on it), and the sweep goroutine itself running
// for the life of the test.
func openStoresPair(t testing.TB, mkPair func(testing.TB) namespace.Namespaces) func() state.Stores {
	return func() state.Stores {
		pair := mkPair(t)
		provisionFixtureParents(t, pair.Control)
		b := openBackendPair(t, wwClient, pair, WithSweepInterval(50*time.Millisecond))
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go b.Run(ctx)
		return b.Stores
	}
}

// equalNamespacePair is today's single-namespace layout: one fresh
// namespace, both fields of the pair naming it.
func equalNamespacePair(t testing.TB) namespace.Namespaces {
	n := newNamespace(t)
	return namespace.Namespaces{Control: n, Conversations: n}
}

// splitNamespacePair is two fresh, GENUINELY distinct namespaces — the
// layout split-conversation-namespace adds. Conversations, Runs and Blobs
// open in the second; Signals, Cooldowns, Receipts and AdapterState open in
// the first (kube.OpenStores' own split by kind, decision 3).
func splitNamespacePair(t testing.TB) namespace.Namespaces {
	return namespace.Namespaces{Control: newNamespace(t), Conversations: newNamespace(t)}
}

// openStores is openStoresPair fixed to the equal-namespace layout — the
// one every pre-split caller of this file already got.
func openStores(t testing.TB) func() state.Stores {
	return openStoresPair(t, equalNamespacePair)
}

// TestStores is the kube backend's pre-split shape: one namespace, both
// fields of the pair equal.
func TestStores(t *testing.T) {
	conformance.RunStores(t, openStores(t))
}

// TestStoresSplitNamespaces pins split-conversation-namespace decision 3:
// the generic conformance suite — every field of state.Stores, Signals/
// Cooldowns/Receipts/AdapterState included — passes exactly the same way
// when Control and Conversations are two genuinely different namespaces,
// with no scenario skipped and no assertion relaxed.
func TestStoresSplitNamespaces(t *testing.T) {
	conformance.RunStores(t, openStoresPair(t, splitNamespacePair))
}

func BenchmarkStores(b *testing.B) {
	conformance.BenchStores(b, openStores(b))
}

// TestBenchStoresRuns executes the benchmark harness once per store so a
// broken bench helper fails `go test`, not only a manual `-bench` run. One
// iteration each (-test.benchtime=1x): the numbers are not asserted, only
// that every helper runs to completion against the backend.
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
