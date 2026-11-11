package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	natsstate "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/nats"
)

var errBoom = errors.New("boom")

// failingReader is a client.Reader whose every read fails.
type failingReader struct{ client.Reader }

func (failingReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errBoom
}

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errBoom
}

func TestEveryReadTheImportMakesOfKubernetesSurfacesItsFailure(t *testing.T) {
	k := kubeSourceNames{c: failingReader{}, namespace: "agent-ops"}
	if _, err := k.SignalSourceNames(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("SignalSourceNames = %v", err)
	}
	if _, err := k.AdapterStateEntries(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("AdapterStateEntries = %v", err)
	}
	if _, err := oldManagerReplicaPresent(failingReader{}, "agent-ops", "v2")(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("oldManagerReplicaPresent = %v", err)
	}
}

type recordedRun struct {
	ran int
	err error
}

func (r *recordedRun) Run(context.Context) error { r.ran++; return r.err }

func TestTheImportRunsOnlyOnTheReplicaThatWinsTheLease(t *testing.T) {
	elected := make(chan struct{})
	close(elected)
	r := &recordedRun{err: errBoom}
	if err := runWhenElected(context.Background(), elected, r); !errors.Is(err, errBoom) || r.ran != 1 {
		t.Fatalf("elected: err=%v ran=%d, want the import's own error after one run", err, r.ran)
	}

	standby := &recordedRun{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runWhenElected(ctx, make(chan struct{}), standby); !errors.Is(err, context.Canceled) || standby.ran != 0 {
		t.Fatalf("standby: err=%v ran=%d, want it to leave without importing", err, standby.ran)
	}

	ok := &recordedRun{}
	if err := runWhenElected(context.Background(), elected, ok); err != nil || ok.ran != 1 {
		t.Fatalf("a clean import: err=%v ran=%d", err, ok.ran)
	}
}

func TestTheReadinessCheckPassesOnceTheImportHasCompleted(t *testing.T) {
	srv := startTestNats(t)
	conn, err := natsstate.Connect(natsstate.Config{URL: srv.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	dest, err := natsstate.OpenStores(ctx, conn, "dest")
	if err != nil {
		t.Fatal(err)
	}
	source, err := natsstate.OpenStores(ctx, conn, "source")
	if err != nil {
		t.Fatal(err)
	}
	check := natsReadinessCheck(conn, dest, elected())
	req := httptest.NewRequest("GET", "/readyz", nil)
	if err := check(req); err == nil || !strings.Contains(err.Error(), "import") {
		t.Fatalf("before the import: %v, want not ready naming the import", err)
	}
	im := &natsstate.Importer{Source: source.Stores, SourceNames: noNames{}, Dest: dest}
	if err := im.Run(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}
	if err := check(req); err != nil {
		t.Fatalf("after the import: %v, want ready", err)
	}
}

type noNames struct{}

func (noNames) SignalSourceNames(context.Context) ([]string, error) { return nil, nil }
func (noNames) AdapterStateEntries(context.Context) ([]natsstate.AdapterStateEntry, error) {
	return nil, nil
}

// brokenConfigManager is a manager whose rest config is then made unusable, so
// building a client from it fails.
func brokenConfigManager(t *testing.T) ctrl.Manager {
	t.Helper()
	cfg := &rest.Config{Host: "http://127.0.0.1:1"}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	mgr.GetConfig().Host = "://not a url"
	return mgr
}

func TestTheBackendsRefuseToWireWithoutAUsableKubernetesClient(t *testing.T) {
	if _, err := newKubeStores(brokenConfigManager(t), "agent-ops"); err == nil {
		t.Error("newKubeStores built a store on an unusable config")
	}
	srv := startTestNats(t)
	t.Setenv("NATS_URL", srv.ClientURL())
	t.Setenv("NATS_REPLICAS", "1")
	_, err := newNatsStores(brokenConfigManager(t), "agent-ops")
	if err == nil || !strings.Contains(err.Error(), "import") {
		t.Errorf("newNatsStores = %v, want the import's kube client failure", err)
	}
}

func TestTheNatsStoreRefusesABadReplicaCount(t *testing.T) {
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	t.Setenv("NATS_REPLICAS", "many")
	if _, err := newNatsStores(testManager(t), "agent-ops"); err == nil || !strings.Contains(err.Error(), "NATS_REPLICAS") {
		t.Fatalf("newNatsStores = %v, want the replica count refused by name", err)
	}
}

func TestTheNatsStoreFailsWhenTheBucketsCannotBeCreated(t *testing.T) {
	srv := startTestNats(t)
	t.Setenv("NATS_URL", srv.ClientURL())
	t.Setenv("NATS_REPLICAS", "3") // one server cannot hold three replicas
	old := natsOpenTimeout
	natsOpenTimeout = 2 * time.Second
	t.Cleanup(func() { natsOpenTimeout = old })
	done := make(chan error, 1)
	go func() {
		_, err := newNatsStores(testManager(t), "agent-ops")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "nats state store") {
			t.Fatalf("newNatsStores = %v, want the bucket failure", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("newNatsStores never returned")
	}
}
