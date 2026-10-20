package controlplane

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func probeTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newProbeHandler(t *testing.T) *ProbeHandler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(probeTestScheme(t)).Build()
	return &ProbeHandler{Writer: c, Reader: c, Namespace: "agent-ops"}
}

func putProbe(h *ProbeHandler, name, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", "/control-plane/probes/"+name, strings.NewReader(body))
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	h.Put(rec, req)
	return rec
}

func getProbe(h *ProbeHandler, name string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/control-plane/probes/"+name, nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	return rec
}

func TestProbeWriteThenRead(t *testing.T) {
	h := newProbeHandler(t)

	if rec := putProbe(h, "p1", `{"key":"k1","value":"v1"}`); rec.Code != 200 {
		t.Fatalf("put status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec := getProbe(h, "p1")
	if rec.Code != 200 {
		t.Fatalf("get status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out probeBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Key != "k1" || out.Value != "v1" {
		t.Fatalf("got %+v, want key=k1 value=v1", out)
	}
}

func TestProbeUpdateOverwrites(t *testing.T) {
	h := newProbeHandler(t)
	putProbe(h, "p1", `{"key":"k1","value":"v1"}`)

	if rec := putProbe(h, "p1", `{"key":"k2","value":"v2"}`); rec.Code != 200 {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec := getProbe(h, "p1")
	var out probeBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Key != "k2" || out.Value != "v2" {
		t.Fatalf("got %+v, want the update to overwrite in place, not add a second object", out)
	}
}

func TestProbeReadUnknownIs404(t *testing.T) {
	h := newProbeHandler(t)

	if rec := getProbe(h, "nope"); rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestProbePutNameMismatchIsRefused covers 2.2's explicit case: a PUT whose
// body "name" differs from the route's {name} must be refused with nothing
// stored under EITHER name.
func TestProbePutNameMismatchIsRefused(t *testing.T) {
	h := newProbeHandler(t)

	rec := putProbe(h, "route-name", `{"name":"body-name","key":"k1"}`)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if getProbe(h, "route-name").Code != 404 {
		t.Fatal("the route's name must not have been stored")
	}
	if getProbe(h, "body-name").Code != 404 {
		t.Fatal("the body's name must not have been stored either")
	}
}

func TestProbePutMissingKeyIsRefusedAndStoresNothing(t *testing.T) {
	h := newProbeHandler(t)

	rec := putProbe(h, "p1", `{"value":"v1"}`)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if getProbe(h, "p1").Code != 404 {
		t.Fatal("nothing should have been stored without a key")
	}
}

func TestProbePutInvalidJSONIsRefused(t *testing.T) {
	h := newProbeHandler(t)

	rec := putProbe(h, "p1", `{not json`)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProbePutSameNameInBodyAndRouteIsAccepted(t *testing.T) {
	h := newProbeHandler(t)

	rec := putProbe(h, "p1", `{"name":"p1","key":"k1"}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if getProbe(h, "p1").Code != 200 {
		t.Fatal("the probe should have been stored")
	}
}
