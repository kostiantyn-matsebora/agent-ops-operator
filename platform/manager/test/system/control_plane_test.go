//go:build system

package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// /control-plane/* as the cluster decides it: whether the CRD the surface
// writes through is genuinely established (the API server, not envtest's
// fixture) and whether the authorizer actually grants the manager's Role
// what rbac.yaml claims and nothing to any other component. ControlPlaneProbe
// is disposable scaffolding (see its own type doc comment in api/v1alpha1) —
// kept here only to prove the surface end to end, deleted or renamed once a
// real first resident of /control-plane/* is proposed.
func TestControlPlaneRoundTrip(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	name := fmt.Sprintf("e2e-probe-%d", time.Now().UnixNano())

	code, out := e.do(t, "PUT", e.Manager.URL()+"/control-plane/probes/"+name,
		[]byte(`{"key":"k","value":"v"}`), bearerPrefix+e.Values.ControlPlaneToken)
	if code != 200 {
		t.Fatalf("PUT /control-plane/probes/%s: %d %s", name, code, out)
	}
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.ControlPlaneProbe{
			ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: name},
		})
	})

	// The object a correct token wrote is readable through the manager's own
	// client, not merely through the HTTP surface that wrote it.
	var probe agentopsv1alpha1.ControlPlaneProbe
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: name}, &probe); err != nil {
		t.Fatalf("the written ControlPlaneProbe is not readable via the API server: %v", err)
	}
	if probe.Spec.Key != "k" || probe.Spec.Value != "v" {
		t.Fatalf("stored spec: %+v", probe.Spec)
	}

	code, out = e.do(t, "GET", e.Manager.URL()+"/control-plane/probes/"+name, nil, bearerPrefix+e.Values.ControlPlaneToken)
	if code != 200 {
		t.Fatalf("GET /control-plane/probes/%s: %d %s", name, code, out)
	}

	// Token isolation, live: a caller's own master token must not reach the
	// other surface, in both directions — this is the same claim
	// internal/httpapi's unit tests make, against a real deployed manager
	// rather than an in-process Server.
	if code, _ := e.do(t, "GET", e.Manager.URL()+"/control-plane/probes/"+name, nil, bearerPrefix+"not-the-control-plane-token"); code != 401 {
		t.Fatalf("a wrong token must be refused with 401, got %d", code)
	}
	if code, _ := e.do(t, "GET", e.Manager.URL()+"/control-plane/probes/"+name, nil, bearerPrefix+e.Values.AdapterToken); code != 401 {
		t.Fatalf("ADAPTER_TOKEN must not reach /control-plane/*, got %d", code)
	}
	if code, _ := e.do(t, "GET", e.Manager.URL()+"/channel/channels", nil, bearerPrefix+e.Values.ControlPlaneToken); code != 401 {
		t.Fatalf("CONTROL_PLANE_TOKEN must not reach /channel/*, got %d", code)
	}

	// A kind this package does not yet handle is 404, and the request
	// creates nothing — no other ControlPlaneProbe appears under the name an
	// unhandled route was asked to use.
	code, out = e.do(t, "PUT", e.Manager.URL()+"/control-plane/widgets/"+name,
		[]byte(`{"key":"k"}`), bearerPrefix+e.Values.ControlPlaneToken)
	if code != 404 {
		t.Fatalf("an unhandled kind must be 404, got %d %s", code, out)
	}
	var stray agentopsv1alpha1.ControlPlaneProbeList
	if err := e.K.List(ctx, &stray, client.InNamespace(Namespace)); err != nil {
		t.Fatal(err)
	}
	for _, p := range stray.Items {
		if p.Name != name {
			t.Fatalf("an unhandled-kind request must touch nothing, but found stray object %q", p.Name)
		}
	}
}

// RBAC as enforced for the new kind — the live authorizer, not the rendered
// Role envtest already checks. Extends substrate_test.go's
// TestRBACAsEnforced pattern to controlplaneprobes: the manager's Role holds
// full CRUD, and no other component's Role holds anything on it at all,
// since this change adds no reader.
func TestControlPlaneRBACAsEnforced(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	for _, verb := range []string{"get", "list", "watch", "create", "update", "patch", "delete"} {
		ok, err := e.K.Can(ctx, "agentops-manager", verb, "agentops.dev", "controlplaneprobes", Namespace)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("agentops-manager must be allowed to %s controlplaneprobes.agentops.dev", verb)
		}
		ok, err = e.K.Can(ctx, "agentops-adapter-console", verb, "agentops.dev", "controlplaneprobes", Namespace)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("agentops-adapter-console must NOT be allowed to %s controlplaneprobes.agentops.dev", verb)
		}
	}
}
