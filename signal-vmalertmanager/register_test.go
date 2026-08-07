package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI stands in for the Kubernetes API server: it stores one
// VMAlertmanagerConfig per name and can be told to refuse writes.
type fakeAPI struct {
	mu      sync.Mutex
	objects map[string]map[string]any
	forbid  bool
	noCRD   bool
	writes  int
}

func (f *fakeAPI) client(t *testing.T) *kubeClient {
	t.Helper()
	if f.objects == nil {
		f.objects = map[string]map[string]any{}
	}
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.noCRD {
			http.Error(w, "the server could not find the requested resource", 404)
			return
		}
		name := r.PathValue("name")
		switch r.Method {
		case "GET":
			obj, ok := f.objects[name]
			if !ok {
				http.Error(w, "not found", 404)
				return
			}
			_ = json.NewEncoder(w).Encode(obj)
		case "POST", "PUT":
			if f.forbid {
				http.Error(w, "forbidden", 403)
				return
			}
			var obj map[string]any
			_ = json.NewDecoder(r.Body).Decode(&obj)
			if r.Method == "POST" {
				name, _ = obj["metadata"].(map[string]any)["name"].(string)
			}
			f.objects[name] = obj
			f.writes++
			_ = json.NewEncoder(w).Encode(obj)
		}
	}
	mux.HandleFunc("/apis/operator.victoriametrics.com/v1beta1/namespaces/{ns}/vmalertmanagerconfigs", handle)
	mux.HandleFunc("/apis/operator.victoriametrics.com/v1beta1/namespaces/{ns}/vmalertmanagerconfigs/{name}", handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &kubeClient{base: srv.URL, token: "t", http: srv.Client()}
}

func testRegisterSpec() *registerSpec {
	reg := &registerSpec{Matchers: []string{`severity=~"critical"`}}
	reg.VMAlertmanager.Name = "vmks"
	reg.VMAlertmanager.Namespace = "monitoring"
	return reg
}

func TestParseRegister(t *testing.T) {
	if parseRegister(nil) != nil {
		t.Fatal("empty config must yield no registration")
	}
	if parseRegister(json.RawMessage(`{"other":1}`)) != nil {
		t.Fatal("config without register must yield none")
	}
	reg := parseRegister(json.RawMessage(`{"register":{"vmalertmanager":{"name":"vmks","namespace":"mon"},"sendResolved":true}}`))
	if reg == nil || reg.VMAlertmanager.Name != "vmks" || reg.VMAlertmanager.Namespace != "mon" || !reg.SendResolved {
		t.Fatalf("register parsed wrong: %+v", reg)
	}
}

func TestEnsureRegistrationCreatesAndRepairs(t *testing.T) {
	api := &fakeAPI{}
	k := api.client(t)
	url := "http://agentops-signal-vm-alertmanager.agent-ops.svc:8080/webhook/vm-alerts"

	ref, err := k.ensureRegistration(context.Background(), "vm-alerts", url, testRegisterSpec())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ref != "monitoring/agentops-vm-alerts" {
		t.Fatalf("object ref: %s", ref)
	}
	obj := api.objects["agentops-vm-alerts"]
	spec := obj["spec"].(map[string]any)
	recv := spec["receivers"].([]any)[0].(map[string]any)
	if got := recv["webhook_configs"].([]any)[0].(map[string]any)["url"]; got != url {
		t.Fatalf("webhook url: %v", got)
	}
	// never divert alerts from receivers that already exist
	if spec["route"].(map[string]any)["continue"] != true {
		t.Fatalf("route must continue: %+v", spec["route"])
	}

	// idempotent: an unchanged object is not rewritten
	writes := api.writes
	if _, err := k.ensureRegistration(context.Background(), "vm-alerts", url, testRegisterSpec()); err != nil {
		t.Fatal(err)
	}
	if api.writes != writes {
		t.Fatal("unchanged registration must not write")
	}

	// drift is repaired
	recv["webhook_configs"].([]any)[0].(map[string]any)["url"] = "http://elsewhere/webhook/x"
	if _, err := k.ensureRegistration(context.Background(), "vm-alerts", url, testRegisterSpec()); err != nil {
		t.Fatal(err)
	}
	got := api.objects["agentops-vm-alerts"]["spec"].(map[string]any)["receivers"].([]any)[0].(map[string]any)["webhook_configs"].([]any)[0].(map[string]any)["url"]
	if got != url {
		t.Fatalf("drift not repaired: %v", got)
	}
}

func TestEnsureRegistrationFailuresAreInstructive(t *testing.T) {
	url := "http://svc/webhook/vm-alerts"

	forbidden := &fakeAPI{forbid: true}
	_, err := forbidden.client(t).ensureRegistration(context.Background(), "vm-alerts", url, testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "vmalertmanagerconfigs") {
		t.Fatalf("403 should name the permission to grant: %v", err)
	}

	missing := &fakeAPI{noCRD: true}
	_, err = missing.client(t).ensureRegistration(context.Background(), "vm-alerts", url, testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "CRD") {
		t.Fatalf("missing CRD should be named: %v", err)
	}
}

func TestReconcileRegistrationReportsInstructions(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.podNS, a.listen = "agent-ops", ":8080"

	// no register block → plain served message naming the webhook
	reason, msg := a.reconcileRegistration(context.Background(), SourceInfo{Name: "plain"})
	if reason != "AdapterReady" || !strings.Contains(msg, "/webhook/plain") {
		t.Fatalf("plain source: %s %s", reason, msg)
	}

	// register block but no cluster access → served + manual instructions
	cfg := json.RawMessage(`{"register":{"vmalertmanager":{"name":"vmks","namespace":"monitoring"}}}`)
	a.kube, a.kubeReason = nil, "ServiceAccount token not mounted"
	reason, msg = a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if reason != "RegistrationManual" {
		t.Fatalf("expected manual degradation, got %s", reason)
	}
	for _, want := range []string{"token not mounted", "monitoring/agentops-vm-alerts", "/webhook/vm-alerts"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("instructions missing %q: %s", want, msg)
		}
	}

	// granting access heals on the next pass
	api := &fakeAPI{}
	a.kube = api.client(t)
	reason, msg = a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if reason != "AdapterReady" || !strings.Contains(msg, "monitoring/agentops-vm-alerts") {
		t.Fatalf("expected registered report: %s %s", reason, msg)
	}
}

func TestWebhookURL(t *testing.T) {
	got := webhookURL("vm-alertmanager", "agent-ops", ":9000", "vm-alerts")
	want := "http://agentops-signal-vm-alertmanager.agent-ops.svc:9000/webhook/vm-alerts"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if got := webhookURL("a", "ns", "", "s"); !strings.Contains(got, ":8080/") {
		t.Fatalf("default port: %s", got)
	}
}
