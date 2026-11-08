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
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
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

// newNamespace creates a fresh namespace. Every scenario that asserts an EXACT
// set (List, Watch) gets its own namespace rather than sharing one across the
// whole test binary: the same "one install per namespace" boundary design D-2
// and the spec's own scope-isolation scenario rest on, used here for test
// isolation instead of install isolation, so one test's fixtures can never
// leak into another's assertions.
func newNamespace(t testing.TB) string {
	t.Helper()
	name := fmt.Sprintf("kube-test-%d-%d", os.Getpid(), nsCounter.Add(1))
	ns := &corev1.Namespace{}
	ns.Name = name
	if err := wwClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create test namespace %q: %v", name, err)
	}
	return name
}

// openBackend opens every store over c in ns, failing the test on a layout the
// backend refuses.
func openBackend(t testing.TB, c client.WithWatch, ns string, opts ...Option) *Backend {
	t.Helper()
	b, err := OpenStores(c, ns, opts...)
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	return b
}

// newTestNamespace creates a fresh namespace and returns the stores scoped to
// it.
func newTestNamespace(t testing.TB) (string, state.Stores) {
	t.Helper()
	ns := newNamespace(t)
	return ns, openBackend(t, wwClient, ns).Stores
}

// recvEvent blocks for one event or fails the test after a generous timeout
// — envtest's own watch latency is normally well under a second, so a real
// gap means a bug, not a slow CI box. Shared by every layout test that reads
// a feed directly (the generic feed scenarios live in
// internal/state/conformance now).
func recvEvent[T any](t *testing.T, events <-chan state.Event[T]) state.Event[T] {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event channel closed unexpectedly")
		}
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a watch event")
	}
	return state.Event[T]{}
}
