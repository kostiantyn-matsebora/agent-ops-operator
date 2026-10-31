// Envtest-backed tests for the kube state.Store backend: a real API
// server, no kubelet, no controller — exactly what internal/integration
// bootstraps for the rest of the manager, with the chartDir depth adjusted
// for this package living one level deeper (internal/state/kube vs
// internal/integration).
package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

var (
	wwClient client.WithWatch
	testEnv  *envtest.Environment
	scheme   = runtime.NewScheme()
)

// chartDir locates the chart from this package's own working directory —
// internal/state/kube, one level deeper than internal/integration, whose
// own chartDir (suite_test.go) counts four ".."; this one needs five.
func chartDir(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "..", "..", "..", "chart"}, parts...)...)
}

func TestMain(m *testing.M) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentopsv1alpha1.AddToScheme(scheme))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{chartDir("crds")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}

	wwClient, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

var nsCounter atomic.Int64

// newTestNamespace creates a fresh namespace and returns a Store scoped to
// it. Every scenario that asserts an EXACT set (List, Watch) gets its own
// namespace rather than sharing one across the whole test binary — the
// same "one install per namespace" boundary design D-2 and the spec's own
// scope-isolation scenario rest on, used here for test isolation instead
// of install isolation, so one test's fixtures can never leak into
// another's assertions.
func newTestNamespace(t *testing.T) (string, *Store) {
	t.Helper()
	return newTestNamespaceWithSweepInterval(t, DefaultSweepInterval)
}

// newTestNamespaceWithSweepInterval is newTestNamespace with the expiry
// sweep's poll period overridden, for the one scenario (2.6) that needs to
// observe a sweep inside a test's own timeout rather than wait out
// DefaultSweepInterval.
func newTestNamespaceWithSweepInterval(t *testing.T, sweepInterval time.Duration) (string, *Store) {
	t.Helper()
	name := fmt.Sprintf("kube-test-%d-%d", os.Getpid(), nsCounter.Add(1))
	ns := &corev1.Namespace{}
	ns.Name = name
	if err := wwClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create test namespace %q: %v", name, err)
	}
	return name, NewStoreWithSweepInterval(wwClient, name, sweepInterval)
}
