package main

import (
	"testing"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

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
	stores, err := newNatsStores(testManager(t), "agent-ops")
	if err != nil {
		t.Fatalf("newNatsStores: %v", err)
	}
	if stores.Conversations == nil {
		t.Fatal("want a populated inventory")
	}
}

func TestNewNatsStoresRefusesAnUnreachableServer(t *testing.T) {
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	if _, err := newNatsStores(testManager(t), "agent-ops"); err == nil {
		t.Fatal("want an error connecting to a closed port")
	}
}

func TestNewKubeStoresWiresTheBackend(t *testing.T) {
	stores, err := newKubeStores(testManager(t), "agent-ops")
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
	if _, err := newStateStores(backendKube, mgr, "agent-ops"); err != nil {
		t.Fatalf("kube: %v", err)
	}
	if _, err := newStateStores(backendNats, mgr, "agent-ops"); err != nil {
		t.Fatalf("nats: %v", err)
	}
}
