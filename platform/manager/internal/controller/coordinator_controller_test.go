package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func fakeCoordinatorClient(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

// allCoordinatorRequests lists every Coordinator in the given namespace, and
// only that namespace — the requeue-all mapping fans out per-namespace, never
// cluster-wide.
func TestAllCoordinatorRequestsListsOnlyTheNamespace(t *testing.T) {
	inNS := &agentopsv1alpha1.Coordinator{ObjectMeta: metav1.ObjectMeta{Name: "co-a", Namespace: "ns-a"}}
	otherNS := &agentopsv1alpha1.Coordinator{ObjectMeta: metav1.ObjectMeta{Name: "co-b", Namespace: "ns-b"}}
	c := fakeCoordinatorClient(t, inNS, otherNS).Build()

	reqs := allCoordinatorRequests(context.Background(), c, "ns-a")
	if len(reqs) != 1 || reqs[0].Name != "co-a" || reqs[0].Namespace != "ns-a" {
		t.Fatalf("expected exactly co-a/ns-a, got %+v", reqs)
	}
}

// A namespace with no Coordinators requeues nothing.
func TestAllCoordinatorRequestsEmptyNamespace(t *testing.T) {
	c := fakeCoordinatorClient(t).Build()
	if reqs := allCoordinatorRequests(context.Background(), c, "ns-empty"); len(reqs) != 0 {
		t.Fatalf("expected no requests, got %+v", reqs)
	}
}

// A List that cannot resolve the CoordinatorList kind fails quietly: no
// panic, no requests, so a mapping event that hits it drops the reconcile
// rather than crashing the manager.
func TestAllCoordinatorRequestsListErrorReturnsNil(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	if reqs := allCoordinatorRequests(context.Background(), c, "ns-a"); reqs != nil {
		t.Fatalf("expected nil requests when List fails, got %+v", reqs)
	}
}

// Reconcile on a Coordinator already gone (deleted between the event and the
// reconcile) returns no error — the same client.IgnoreNotFound shape every
// reconciler in this package takes.
func TestCoordinatorReconcileMissingObjectReturnsNilError(t *testing.T) {
	c := fakeCoordinatorClient(t).Build()
	rc := &CoordinatorReconciler{Client: c}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns-a", Name: "missing"}}
	if _, err := rc.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("Reconcile(missing) = %v, want nil", err)
	}
}

// An agents[] entry naming neither ref is refused at admission by CEL
// (integration test), so the default case in coordinatorEntryProblems is
// reachable only through a client that skips that validation — exactly what
// a fake client is.
func TestCoordinatorEntryProblemsNeitherRefIsUnreachableBranch(t *testing.T) {
	c := fakeCoordinatorClient(t).Build()
	entry := agentopsv1alpha1.CoordinatorAgentEntry{Name: "neither", Description: "names nothing"}
	got := coordinatorEntryProblems(context.Background(), c, "ns-a", entry, map[string]bool{})
	want := "agents[neither]: capabilityRef or coordinatorRef"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("coordinatorEntryProblems(neither ref) = %v, want [%q]", got, want)
	}
}
