package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

const (
	cpTestAdapterToken      = "adapter-master-key"
	cpTestControlPlaneToken = "control-plane-key"
)

// cpTestServer builds a Server wired with BOTH the channel-adapter token and
// the control-plane token, each a distinct value, so a cross-surface request
// is testing real isolation rather than two empty strings happening to
// compare equal.
func cpTestServer(t *testing.T) *Server {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build()
	return &Server{
		Client: c, Reader: c, Namespace: "agent-ops",
		AdapterToken:      cpTestAdapterToken,
		ControlPlaneToken: cpTestControlPlaneToken,
	}
}

func cpRequest(s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// TestControlPlaneRefusesTheAdapterToken covers 2.3: ADAPTER_TOKEN must not
// reach /control-plane/*.
func TestControlPlaneRefusesTheAdapterToken(t *testing.T) {
	s := cpTestServer(t)
	rec := cpRequest(s, "PUT", "/control-plane/probes/p1", cpTestAdapterToken, `{"key":"k1"}`)
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestControlPlaneRefusesTheCoordinatorToken covers 2.3: a derived
// coordinator token must not reach /control-plane/* either, even though it
// is itself derived from the same master key ADAPTER_TOKEN holds.
func TestControlPlaneRefusesTheCoordinatorToken(t *testing.T) {
	s := cpTestServer(t)
	coordinatorToken := chat.DeriveCoordinatorToken(cpTestAdapterToken, "co-a", "root-1")
	rec := cpRequest(s, "GET", "/control-plane/probes/p1", coordinatorToken, "")
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestAdapterSurfaceRefusesTheControlPlaneToken covers 2.3's other
// direction: a valid CONTROL_PLANE_TOKEN must not reach /channel/*.
func TestAdapterSurfaceRefusesTheControlPlaneToken(t *testing.T) {
	s := cpTestServer(t)
	rec := cpRequest(s, "GET", "/channel/channels?adapter=slack", cpTestControlPlaneToken, "")
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestSignalSurfaceRefusesTheControlPlaneToken: same direction, the other
// adapter-contract surface.
func TestSignalSurfaceRefusesTheControlPlaneToken(t *testing.T) {
	s := cpTestServer(t)
	rec := cpRequest(s, "GET", "/signal/sources", cpTestControlPlaneToken, "")
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestCoordinateSurfaceRefusesTheControlPlaneToken: and /coordinate/*, which
// authenticates by re-deriving against ADAPTER_TOKEN — a flat
// CONTROL_PLANE_TOKEN never re-derives to anything it accepts. Seeds a real
// Coordinator-rooted conversation (coordServer, from coordinate_test.go) so
// the 401 comes from the token comparison itself, not a missing object.
func TestCoordinateSurfaceRefusesTheControlPlaneToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	s.ControlPlaneToken = cpTestControlPlaneToken
	rec := cpRequest(s, "POST", "/coordinate/invoke", cpTestControlPlaneToken,
		`{"conversation":"root-1","agent":"worker","task":"do it"}`)
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestControlPlaneValidTokenUnknownKindIs404AndTouchesNothing covers 2.3: a
// valid CONTROL_PLANE_TOKEN requesting a kind with no handler gets 404, and
// no probe (the one kind this surface does handle) is created, read or
// modified by that request.
func TestControlPlaneValidTokenUnknownKindIs404AndTouchesNothing(t *testing.T) {
	s := cpTestServer(t)

	rec := cpRequest(s, "PUT", "/control-plane/widgets/p1", cpTestControlPlaneToken, `{"key":"k1"}`)
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}

	getRec := cpRequest(s, "GET", "/control-plane/probes/p1", cpTestControlPlaneToken, "")
	if getRec.Code != 404 {
		t.Fatalf("no probe named %q should have been created by the unhandled-kind request, got status %d", "p1", getRec.Code)
	}
}

// TestControlPlaneValidTokenWritesAndReadsAProbe is the happy-path round
// trip, exercised through the FULL Handler() stack (auth + mux + leader
// middleware), not just the package's own unit tests.
func TestControlPlaneValidTokenWritesAndReadsAProbe(t *testing.T) {
	s := cpTestServer(t)

	putRec := cpRequest(s, "PUT", "/control-plane/probes/p1", cpTestControlPlaneToken, `{"key":"k1","value":"v1"}`)
	if putRec.Code != 200 {
		t.Fatalf("put status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}

	getRec := cpRequest(s, "GET", "/control-plane/probes/p1", cpTestControlPlaneToken, "")
	if getRec.Code != 200 {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "k1") {
		t.Fatalf("body = %q, want it to carry the stored key", getRec.Body.String())
	}
}

// TestControlPlaneUnconfiguredTokenRefusesWith503 pins the
// unconfigured-authorizes-nobody rule at the Server level, mirroring
// adapterAuth's own zero-value behaviour.
func TestControlPlaneUnconfiguredTokenRefusesWith503(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build()
	s := &Server{Client: c, Reader: c, Namespace: "agent-ops", AdapterToken: cpTestAdapterToken}

	rec := cpRequest(s, "GET", "/control-plane/probes/p1", "anything", "")
	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
}
