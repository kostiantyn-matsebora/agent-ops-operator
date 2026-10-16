package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeManager serves the contract endpoints the adapter consumes and records
// inbound pushes and status reports. The fail* switches let a test flip one
// endpoint into a real HTTP failure without a second, mocked implementation
// standing in.
type fakeManager struct {
	mu          sync.Mutex
	sources     []SourceInfo
	inbounds    []map[string]any
	statuses    []map[string]any
	failList    bool
	failInbound bool
	failStatus  bool
	// statusHit, when non-nil, receives the source name on every status POST
	// — a synchronization point for tests driving background loops through
	// main(), where polling a.reported would race the goroutine.
	statusHit chan string
}

func (f *fakeManager) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /signal/sources", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail, srcs := f.failList, f.sources
		f.mu.Unlock()
		if fail {
			http.Error(w, "sources temporarily unavailable", 503)
			return
		}
		_ = json.NewEncoder(w).Encode(srcs)
	})
	mux.HandleFunc("POST /signal/inbound", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail := f.failInbound
		f.mu.Unlock()
		if fail {
			http.Error(w, "manager overloaded", 503)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.inbounds = append(f.inbounds, body)
		n := len(body["signals"].([]any))
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": n, "conversations": 1})
	})
	mux.HandleFunc("POST /signal/sources/{name}/status", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail := f.failStatus
		f.mu.Unlock()
		if fail {
			http.Error(w, "status sink unavailable", 503)
			return
		}
		name := r.PathValue("name")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["name"] = name
		f.mu.Lock()
		f.statuses = append(f.statuses, body)
		hit := f.statusHit
		f.mu.Unlock()
		if hit != nil {
			select {
			case hit <- name:
			default:
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// errReader always fails, standing in for a client that hangs up mid-body —
// a real io.Reader failure, not a flag flipped inside the handler under test.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

func testAdapter(t *testing.T, f *fakeManager) *adapter {
	t.Helper()
	mgrSrv := f.server(t)
	a := &adapter{
		mgr:        NewManager(mgrSrv.URL, "tok"),
		sourceType: "pulse",
		sources:    map[string]bool{},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())
	return a
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func firingBody(id string) string {
	return `{"event":"alert","id":"` + id + `","resource":"vm-101","resourceType":"vm","node":"pve1","severity":"critical","message":"CPU high","value":"97","startsAt":"2026-01-01T00:00:00Z"}`
}

func resolvedBody(id string) string {
	return `{"event":"resolved","id":"` + id + `","resource":"vm-101","resourceType":"vm","node":"pve1","severity":"critical","message":"CPU high","value":"12","startsAt":"2026-01-01T00:05:00Z"}`
}

func TestWebhookRoutingAndEventFilter(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	h := a.handler()

	// unknown source → 404, nothing pushed
	if rec := post(t, h, "/webhook/nope", firingBody("abc")); rec.Code != 404 {
		t.Fatalf("unknown source: %d", rec.Code)
	}

	// served source, firing event → one signal pushed
	rec := post(t, h, "/webhook/pulse-default", firingBody("abc"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("firing webhook: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	if len(f.inbounds) != 1 || f.inbounds[0]["source"] != "pulse-default" {
		t.Fatalf("inbound push: %+v", f.inbounds)
	}
	signals := f.inbounds[0]["signals"].([]any)
	f.mu.Unlock()
	if len(signals) != 1 {
		t.Fatalf("firing event should produce exactly one signal: %d", len(signals))
	}
	first := signals[0].(map[string]any)
	if first["fingerprint"] != "pulse:vm-101:abc" || !strings.Contains(first["title"].(string), "vm-101") {
		t.Fatalf("normalization: %+v", first)
	}
	if _, hasKind := first["kind"]; hasKind {
		t.Fatalf("alert-lane signals must carry no kind: %+v", first)
	}

	// resolved event → 200, zero signals, nothing pushed
	rec = post(t, h, "/webhook/pulse-default", resolvedBody("abc"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "not a firing alert") {
		t.Fatalf("resolved webhook: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 1 {
		t.Fatal("a resolved event must not push a signal")
	}
}

func TestFingerprintStableAcrossFiringAndResolvedPair(t *testing.T) {
	firing, err := parsePulseEvent([]byte(firingBody("occ-1")))
	if err != nil {
		t.Fatalf("parse firing: %v", err)
	}
	resolved, err := parsePulseEvent([]byte(resolvedBody("occ-1")))
	if err != nil {
		t.Fatalf("parse resolved: %v", err)
	}
	if firing.fingerprint() != resolved.fingerprint() {
		t.Fatalf("same occurrence must fingerprint identically: firing=%q resolved=%q",
			firing.fingerprint(), resolved.fingerprint())
	}
	other, _ := parsePulseEvent([]byte(firingBody("occ-2")))
	if other.fingerprint() == firing.fingerprint() {
		t.Fatal("different alert ids must fingerprint differently")
	}
}

func TestFingerprintFallbackWithoutID(t *testing.T) {
	e1 := pulseEvent{Resource: "ct-1", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
	e2 := pulseEvent{Resource: "ct-1", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
	if e1.fingerprint() != e2.fingerprint() || e1.fingerprint() == "" {
		t.Fatalf("identical fields with no id must fingerprint identically and non-empty: %q vs %q", e1.fingerprint(), e2.fingerprint())
	}
	e3 := pulseEvent{Resource: "ct-2", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
	if e3.fingerprint() == e1.fingerprint() {
		t.Fatal("different resources must fingerprint differently")
	}
}

func TestSourceListRefresh(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	h := a.handler()

	if rec := post(t, h, "/webhook/late", firingBody("x")); rec.Code != 404 {
		t.Fatalf("before refresh: %d", rec.Code)
	}
	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "late"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if rec := post(t, h, "/webhook/late", firingBody("x")); rec.Code != 200 {
		t.Fatalf("after refresh: %d", rec.Code)
	}
}

// TestHealthzOK closes the /healthz route, never exercised by any other test
// here (every other test posts to /webhook/*).
func TestHealthzOK(t *testing.T) {
	a := &adapter{sources: map[string]bool{}, reported: map[string]string{}}
	rec := httptest.NewRecorder()
	a.handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookReadBodyError closes handleWebhook's io.ReadAll error
// branch with a body that genuinely fails to read.
func TestHandleWebhookReadBodyError(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	req := httptest.NewRequest("POST", "/webhook/pulse-default", errReader{})
	rec := httptest.NewRecorder()
	a.handler().ServeHTTP(rec, req)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "read body") {
		t.Fatalf("body-read failure should be 400: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookInvalidJSON closes handleWebhook's parsePulseEvent error
// branch with a real malformed payload.
func TestHandleWebhookInvalidJSON(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	rec := post(t, a.handler(), "/webhook/pulse-default", "{not-json")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid Pulse webhook JSON") {
		t.Fatalf("malformed JSON should be 400: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookMissingFieldsIs400 closes parsePulseEvent's missing-field
// branches via the handler, for a body that is valid JSON but not a Pulse
// webhook this adapter understands.
func TestHandleWebhookMissingFieldsIs400(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	if rec := post(t, a.handler(), "/webhook/pulse-default", `{"resource":"vm-1"}`); rec.Code != 400 {
		t.Fatalf("missing event field should be 400: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(t, a.handler(), "/webhook/pulse-default", `{"event":"alert"}`); rec.Code != 400 {
		t.Fatalf("missing resource field should be 400: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookInboundFailureIs502 closes handleWebhook's a.mgr.Inbound
// error branch against a manager double that genuinely refuses the push.
func TestHandleWebhookInboundFailureIs502(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}, failInbound: true}
	a := testAdapter(t, f)
	rec := post(t, a.handler(), "/webhook/pulse-default", firingBody("x"))
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "manager rejected the signals") {
		t.Fatalf("inbound failure should be 502: %d %s", rec.Code, rec.Body.String())
	}
}

// TestReportSkipsRecordingOnFailure closes report()'s error branch: a failed
// status POST must not be recorded as reported, so the next attempt retries
// it rather than believing the status reached the manager.
func TestReportSkipsRecordingOnFailure(t *testing.T) {
	f := &fakeManager{failStatus: true}
	a := testAdapter(t, f)
	a.report(context.Background(), "pulse-default", "R", "M")
	if got := a.reported["pulse-default"]; got != "" {
		t.Fatalf("a failed status post must not be recorded as reported: %q", got)
	}

	f.mu.Lock()
	f.failStatus = false
	f.mu.Unlock()
	a.report(context.Background(), "pulse-default", "R", "M")
	if got := a.reported["pulse-default"]; got == "" {
		t.Fatal("a successful status post must now be recorded")
	}
}

// TestRefreshSourcesKeepsPreviousOnListError closes refreshSources' error
// branch: a transient listing failure must not drop sources already known
// to be served, or every hiccup would 404 every in-flight webhook.
func TestRefreshSourcesKeepsPreviousOnListError(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)

	f.mu.Lock()
	f.failList = true
	f.mu.Unlock()
	a.refreshSources(context.Background())

	if !a.served("pulse-default") {
		t.Fatal("a failed source listing must not drop previously known sources")
	}
}

func TestMustEnvReturnsValue(t *testing.T) {
	t.Setenv("PULSE_TEST_MUSTENV_VAL", "present")
	if got := mustEnv("PULSE_TEST_MUSTENV_VAL"); got != "present" {
		t.Fatalf("got %q", got)
	}
}

// TestMustEnvMissingExits closes mustEnv's log.Fatalf branch, which calls
// os.Exit(1) — untestable in-process, so this re-execs the test binary as a
// real subprocess and asserts its exit code and stderr.
func TestMustEnvMissingExits(t *testing.T) {
	if os.Getenv("PULSE_TEST_MUSTENV_SUBPROCESS") == "1" {
		mustEnv("PULSE_TEST_DOES_NOT_EXIST")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvMissingExits")
	cmd.Env = append(os.Environ(), "PULSE_TEST_MUSTENV_SUBPROCESS=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected the subprocess to exit non-zero; output: %s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %v; output: %s", err, out)
	}
	if !strings.Contains(string(out), "missing required env") {
		t.Errorf("expected the missing-env message in output, got: %s", out)
	}
}

// TestMainRunsUntilSignaled closes main() itself: it starts the real process
// loop (env defaulting, adapter construction, the real HTTP server,
// registryLoop, registerLoop) against a real HTTP double for the manager,
// then cancels baseContext directly — main()'s own signal.NotifyContext
// derives from it, so this stops main() exactly as a real SIGTERM would.
func TestMainRunsUntilSignaled(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}, statusHit: make(chan string, 4)}
	mgrSrv := f.server(t)

	env := map[string]string{
		"MANAGER_URL":       mgrSrv.URL,
		"ADAPTER_TOKEN":     "t",
		"PULSE_SOURCE_NAME": "pulse-default", // unset PULSE_URL exercises the RegistrationNotConfigured report, our sync point
	}
	prev := map[string]string{}
	had := map[string]bool{}
	for k, v := range env {
		prev[k], had[k] = os.LookupEnv(k)
		os.Setenv(k, v)
	}
	os.Unsetenv("ADAPTER_NAME") // exercise the default-name fill-in branch
	os.Unsetenv("LISTEN_ADDR")  // exercise the default-listen fill-in branch
	os.Unsetenv("PULSE_URL")
	defer func() {
		for k := range env {
			if had[k] {
				os.Setenv(k, prev[k])
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	origBaseContext := baseContext
	baseContext = func() context.Context { return ctx }
	defer func() { baseContext = origBaseContext }()

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	select {
	case <-f.statusHit:
	case <-time.After(5 * time.Second):
		t.Fatal("main() never completed a registration attempt")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after its context was cancelled")
	}
}
