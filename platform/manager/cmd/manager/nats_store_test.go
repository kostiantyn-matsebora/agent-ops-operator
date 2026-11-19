package main

import (
	"testing"
	"time"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
)

// testNamespaces is the pair every test in this file resolves to: a
// single-namespace install, two equal strings, exactly as it was before the
// split.
var testNamespaces = namespace.Namespaces{Control: "agent-ops", Conversations: "agent-ops"}

func testManager(t *testing.T) ctrl.Manager {
	t.Helper()
	mgr, err := ctrl.NewManager(&rest.Config{Host: "http://127.0.0.1:1"}, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		t.Skipf("cannot build a manager without a cluster: %v", err)
	}
	return mgr
}

func TestNewNatsStoresWiresTheBackend(t *testing.T) {
	srv := startTestNats(t)
	t.Setenv("NATS_URL", srv.ClientURL())
	t.Setenv("NATS_REPLICAS", "1")
	stores, err := newNatsStores(testManager(t), testNamespaces)
	if err != nil {
		t.Fatalf("newNatsStores: %v", err)
	}
	if stores.Conversations == nil {
		t.Fatal("want a populated inventory")
	}
}

// An unreachable server is not a reason to exit: the manager keeps waiting and
// the probes, not the process, decide how long that may last.
func TestNewNatsStoresKeepsWaitingForAnUnreachableServer(t *testing.T) {
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	done := make(chan error, 1)
	go func() {
		_, err := newNatsStores(testManager(t), testNamespaces)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("returned (%v) while the server was unreachable, want it to keep waiting", err)
	case <-time.After(2 * time.Second):
	}
}

func TestNewKubeStoresWiresTheBackend(t *testing.T) {
	stores, err := newKubeStores(testManager(t), testNamespaces)
	if err != nil {
		t.Fatalf("newKubeStores: %v", err)
	}
	if stores.Conversations == nil {
		t.Fatal("want a populated inventory")
	}
}

func TestNewStateStoresDispatchesBothBackends(t *testing.T) {
	srv := startTestNats(t)
	t.Setenv("NATS_URL", srv.ClientURL())
	t.Setenv("NATS_REPLICAS", "1")
	mgr := testManager(t)
	if _, err := newStateStores(backendKube, mgr, testNamespaces); err != nil {
		t.Fatalf("kube: %v", err)
	}
	if _, err := newStateStores(backendNats, mgr, testNamespaces); err != nil {
		t.Fatalf("nats: %v", err)
	}
}
