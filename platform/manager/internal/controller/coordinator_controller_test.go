package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
