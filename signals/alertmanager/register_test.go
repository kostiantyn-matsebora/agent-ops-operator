package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI stands in for the Kubernetes API server: it stores one
// VMAlertmanagerConfig per name and can be told to refuse writes, hide the
// CRD, force an arbitrary GET status, or hand back an unparsable body.
type fakeAPI struct {
	mu          sync.Mutex
	objects     map[string]map[string]any
	forbid      bool
	noCRD       bool
	forceStatus int  // nonzero: every GET answers with this status regardless of object state
	corruptBody bool // GET 200 answers with unparsable JSON
	writes      int
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
		if f.forceStatus != 0 {
			http.Error(w, "forced status", f.forceStatus)
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
			if f.corruptBody {
				_, _ = w.Write([]byte("{not-valid-json"))
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
	reg := &registerSpec{
		Matchers:       []string{`severity=~"critical"`},
		GroupWait:      "1m",
		RepeatInterval: "12h",
		MaxAlerts:      10,
	}
	return reg
}

// A source's register block must be able to express everything a hand-written
// receiver + route did, or it cannot replace one.
func TestDesiredVMACCarriesRouteTuning(t *testing.T) {
	obj := desiredVMAC("vm-alerts", "monitoring", "http://svc/webhook/vm-alerts", testRegisterSpec())
	spec := obj["spec"].(map[string]any)
	route := spec["route"].(map[string]any)
	if route["group_wait"] != "1m" || route["repeat_interval"] != "12h" {
		t.Fatalf("route timing not carried: %+v", route)
	}
	if route["continue"] != true {
		t.Fatalf("route must not divert alerts from existing receivers: %+v", route)
	}
	webhook := spec["receivers"].([]any)[0].(map[string]any)["webhook_configs"].([]any)[0].(map[string]any)
	if webhook["max_alerts"] != 10 {
		t.Fatalf("max_alerts not carried: %+v", webhook)
	}

	// omitted knobs stay absent rather than rendering zero values
	bare := &registerSpec{}
	spec = desiredVMAC("s", "monitoring", "http://svc/webhook/s", bare)["spec"].(map[string]any)
	route = spec["route"].(map[string]any)
	for _, k := range []string{"group_wait", "group_interval", "repeat_interval", "matchers"} {
		if _, present := route[k]; present {
			t.Fatalf("unset %s must be omitted: %+v", k, route)
		}
	}
	webhook = spec["receivers"].([]any)[0].(map[string]any)["webhook_configs"].([]any)[0].(map[string]any)
	if _, present := webhook["max_alerts"]; present {
		t.Fatalf("unset max_alerts must be omitted: %+v", webhook)
	}
}

func TestParseRegister(t *testing.T) {
	if parseRegister(nil) != nil {
		t.Fatal("empty config must yield no registration")
	}
	if parseRegister(json.RawMessage(`{"other":1}`)) != nil {
		t.Fatal("config without register must yield none")
	}
	reg := parseRegister(json.RawMessage(`{"register":{"namespace":"mon","sendResolved":true}}`))
	if reg == nil || reg.Namespace != "mon" || !reg.SendResolved {
		t.Fatalf("register parsed wrong: %+v", reg)
	}
	// namespace is optional — the adapter falls back to its own
	if reg := parseRegister(json.RawMessage(`{"register":{}}`)); reg == nil || reg.Namespace != "" {
		t.Fatalf("bare register block must parse with an empty namespace: %+v", reg)
	}
}

func TestEnsureRegistrationCreatesAndRepairs(t *testing.T) {
	api := &fakeAPI{}
	k := api.client(t)
	url := "http://agentops-signal-vm-alertmanager.agent-ops.svc:8080/webhook/vm-alerts"

	ref, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", url, testRegisterSpec())
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
	if _, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", url, testRegisterSpec()); err != nil {
		t.Fatal(err)
	}
	if api.writes != writes {
		t.Fatal("unchanged registration must not write")
	}

	// drift is repaired
	recv["webhook_configs"].([]any)[0].(map[string]any)["url"] = "http://elsewhere/webhook/x"
	if _, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", url, testRegisterSpec()); err != nil {
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
	_, err := forbidden.client(t).ensureRegistration(context.Background(), "vm-alerts", "monitoring", url, testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "vmalertmanagerconfigs") {
		t.Fatalf("403 should name the permission to grant: %v", err)
	}

	missing := &fakeAPI{noCRD: true}
	_, err = missing.client(t).ensureRegistration(context.Background(), "vm-alerts", "monitoring", url, testRegisterSpec())
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
	cfg := json.RawMessage(`{"register":{"namespace":"monitoring"}}`)
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

// The whole point of defaulting: no namespace in config means the adapter
// writes into its own namespace, which needs no cross-namespace grant.
func TestRegistrationDefaultsToAdapterNamespace(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.podNS, a.listen = "agent-ops", ":8080"
	api := &fakeAPI{}
	a.kube = api.client(t)

	reason, msg := a.reconcileRegistration(context.Background(),
		SourceInfo{Name: "vm-alerts", Config: json.RawMessage(`{"register":{}}`)})
	if reason != "AdapterReady" || !strings.Contains(msg, "agent-ops/agentops-vm-alerts") {
		t.Fatalf("expected registration in the adapter's own namespace: %s %s", reason, msg)
	}
	if ns := api.objects["agentops-vm-alerts"]["metadata"].(map[string]any)["namespace"]; ns != "agent-ops" {
		t.Fatalf("object namespace: %v", ns)
	}
}

// TestNewInClusterClientNotMounted closes newInClusterClient's real
// not-mounted branch: this test sandbox genuinely has no ServiceAccount
// token at the fixed mount path, so this exercises the actual file-read
// failure rather than faking an absent file.
func TestNewInClusterClientNotMounted(t *testing.T) {
	client, reason := newInClusterClient()
	if client != nil || !strings.Contains(reason, "token not mounted") {
		t.Fatalf("expected the not-mounted degradation, got client=%v reason=%q", client, reason)
	}
}

// TestKubeClientDoMarshalError closes do()'s json.Marshal error branch: an
// unmarshalable body must fail before any request is attempted.
func TestKubeClientDoMarshalError(t *testing.T) {
	k := &kubeClient{base: "http://unused", token: "t", http: http.DefaultClient}
	_, _, err := k.do(context.Background(), "POST", "/x", make(chan int))
	if err == nil {
		t.Fatal("unmarshalable body should error before any request is sent")
	}
}

// TestKubeClientDoInvalidMethodError closes do()'s
// http.NewRequestWithContext error branch with a real invalid HTTP method.
func TestKubeClientDoInvalidMethodError(t *testing.T) {
	k := &kubeClient{base: "http://unused", token: "t", http: http.DefaultClient}
	_, _, err := k.do(context.Background(), "BAD METHOD", "/x", nil)
	if err == nil {
		t.Fatal("invalid HTTP method should be rejected before any request is sent")
	}
}

// TestEnsureRegistrationAPIUnreachable closes both do()'s real connection
// error branch and ensureRegistration's "API unreachable" wrapping, against
// a real refused TCP connection rather than a mock transport.
func TestEnsureRegistrationAPIUnreachable(t *testing.T) {
	k := &kubeClient{base: "http://127.0.0.1:1", token: "t", http: &http.Client{Timeout: 2 * time.Second}}
	_, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", "http://svc/webhook/vm-alerts", testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "API unreachable") {
		t.Fatalf("a refused connection should be reported as unreachable: %v", err)
	}
}

// TestEnsureRegistrationUnreadableCurrentObject closes the branch where the
// existing object's body cannot be parsed as JSON.
func TestEnsureRegistrationUnreadableCurrentObject(t *testing.T) {
	api := &fakeAPI{objects: map[string]map[string]any{"agentops-vm-alerts": {}}, corruptBody: true}
	k := api.client(t)
	_, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", "http://svc/webhook/vm-alerts", testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "unreadable current object") {
		t.Fatalf("a corrupt existing object should be reported: %v", err)
	}
}

// TestEnsureRegistrationUpdateFailureIsInstructive closes the repair-write
// (PUT) failure branch, distinct from the create (POST) failure already
// covered elsewhere: drift is detected against an existing object, and the
// repair write is then refused.
func TestEnsureRegistrationUpdateFailureIsInstructive(t *testing.T) {
	api := &fakeAPI{objects: map[string]map[string]any{
		"agentops-vm-alerts": desiredVMAC("vm-alerts", "monitoring", "http://old/webhook/vm-alerts", testRegisterSpec()),
	}}
	k := api.client(t)
	api.mu.Lock()
	api.forbid = true
	api.mu.Unlock()
	_, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", "http://new/webhook/vm-alerts", testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "update") {
		t.Fatalf("a refused repair write should be reported as an update failure: %v", err)
	}
}

// TestEnsureRegistrationUnexpectedStatusIsInstructive closes the default
// branch of the GET status switch (anything but 200/404) and, with it,
// apiFailure's default formatting case.
func TestEnsureRegistrationUnexpectedStatusIsInstructive(t *testing.T) {
	api := &fakeAPI{forceStatus: 500}
	k := api.client(t)
	_, err := k.ensureRegistration(context.Background(), "vm-alerts", "monitoring", "http://svc/webhook/vm-alerts", testRegisterSpec())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("an unexpected GET status should be reported verbatim: %v", err)
	}
}

// TestApiFailureFormatsEveryCase closes apiFailure's remaining branches
// directly: the plain-error path (called when the underlying request never
// got an HTTP response at all) and the default status-code case. Both are
// exercised indirectly above too, but a direct call pins the exact wording
// this pure formatter owes an operator.
func TestApiFailureFormatsEveryCase(t *testing.T) {
	if err := apiFailure("update", "ns/name", 0, nil, context.DeadlineExceeded); !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("plain-error case should carry the error verbatim: %v", err)
	}
	if err := apiFailure("read", "ns/name", 500, []byte("boom"), nil); !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("default case should carry the status and body: %v", err)
	}
}
