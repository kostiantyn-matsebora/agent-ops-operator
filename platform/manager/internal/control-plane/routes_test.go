package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRegisterRoutesUnknownKindIs404AndTouchesNothing(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(probeTestScheme(t)).Build()
	mux := http.NewServeMux()
	RegisterRoutes(mux, "the-token", c, c, "agent-ops")

	req := httptest.NewRequest("PUT", "/control-plane/widgets/x", nil)
	req.Header.Set("Authorization", "Bearer the-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// No probe was created by the request for the unhandled kind either.
	h := &ProbeHandler{Writer: c, Reader: c, Namespace: "agent-ops"}
	if getProbe(h, "x").Code != 404 {
		t.Fatal("an unknown kind's request must not create a probe")
	}
}

func TestRegisterRoutesProbeRoutesAreReachable(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(probeTestScheme(t)).Build()
	mux := http.NewServeMux()
	RegisterRoutes(mux, "the-token", c, c, "agent-ops")

	putReq := httptest.NewRequest("PUT", "/control-plane/probes/p1", strings.NewReader(`{"key":"k1"}`))
	putReq.Header.Set("Authorization", "Bearer the-token")
	putRec := httptest.NewRecorder()
	mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest("GET", "/control-plane/probes/p1", nil)
	getReq.Header.Set("Authorization", "Bearer the-token")
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
}
