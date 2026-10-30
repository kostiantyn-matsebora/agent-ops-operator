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
// inbound pushes. The three fail* switches let a test flip one endpoint into
// a real HTTP failure without a second, mocked implementation standing in.
type fakeManager struct {
	mu          sync.Mutex
	sources     []SourceInfo
	inbounds    []map[string]any
	statuses    []string
	failList    bool
	failInbound bool
	failStatus  bool
	// statusHit, when non-nil, receives the source name on every status POST
	// — a synchronization point for tests driving the real registryLoop
	// through main(), where polling a.reported would race the goroutine.
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
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": n})
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
		f.mu.Lock()
		f.statuses = append(f.statuses, name)
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
		sourceType: "vm-alertmanager",
		sources:    map[string]string{},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())
	return a
}

func post(t *testing.T, h http.Handler, path, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// twoFiringOneResolved is the CANONICAL webhook body, shared with the system
// pack so a single captured payload cannot drift between the two suites. Read by
// relative path from this test file — a test-only read, no go.mod entry —
// and read inside the test, so a missing file fails that test rather than
// the package's init.
func twoFiringOneResolved(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../test/fixtures/alertmanager-webhook.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return string(b)
}

func TestWebhookRoutingAndFiringFilter(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}}
	a := testAdapter(t, f)
	h := a.handler()

	// unknown source → 404, nothing pushed
	if rec := post(t, h, "/webhook/nope", "", twoFiringOneResolved(t)); rec.Code != 404 {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	// served source: firing-only normalization
	rec := post(t, h, "/webhook/vm-alerts", "", twoFiringOneResolved(t))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Fatalf("webhook: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 1 || f.inbounds[0]["source"] != "vm-alerts" {
		t.Fatalf("inbound push: %+v", f.inbounds)
	}
	signals := f.inbounds[0]["signals"].([]any)
	if len(signals) != 2 {
		t.Fatalf("firing filter: %d signals", len(signals))
	}
	first := signals[0].(map[string]any)
	second := signals[1].(map[string]any)
	wantID := `{}/{alertname="A"}`
	if first["initiationId"] != wantID || first["title"] != "🔍 A — ns1" {
		t.Fatalf("normalization: %+v", first)
	}
	// One Alertmanager group is one conversation: both firing alerts of this
	// webhook body share the group's groupKey as their initiationId, even
	// though they carry different alertnames.
	if second["initiationId"] != wantID {
		t.Fatalf("both firing alerts must share the group's initiationId: %+v vs %+v", first, second)
	}
	if !strings.Contains(first["payload"].(string), `"generatorURL": "http://alertmanager.example.com/a"`) {
		t.Fatalf("payload shape: %s", first["payload"])
	}
	if _, hasKind := first["kind"]; hasKind {
		t.Fatalf("alert-lane signals must carry no kind: %+v", first)
	}

	// all-resolved body → queued 0 with reason, no push
	if rec := post(t, h, "/webhook/vm-alerts", "", `{"alerts":[{"status":"resolved","fingerprint":"x"}]}`); !strings.Contains(rec.Body.String(), "no firing alerts") {
		t.Fatalf("resolved-only: %s", rec.Body.String())
	}
	if len(f.inbounds) != 1 {
		t.Fatal("resolved-only body must not push")
	}
}

func TestGroupLabelsFallbackIsStable(t *testing.T) {
	a1 := groupLabelsFallbackID(map[string]string{"b": "2", "a": "1"})
	a2 := groupLabelsFallbackID(map[string]string{"a": "1", "b": "2"})
	if a1 != a2 || a1 == "" {
		t.Fatalf("fallback not stable across map order: %q vs %q", a1, a2)
	}
	if groupLabelsFallbackID(map[string]string{"a": "1"}) == a1 {
		t.Fatal("different groupLabels must derive different ids")
	}
	// normalize applies the fallback when the sender omits groupKey
	signals := normalize(amPayload{
		GroupLabels: map[string]string{"a": "1", "b": "2"},
		Alerts:      []amAlert{{Status: "firing", Labels: map[string]string{"a": "1", "b": "2"}}},
	}, "d")
	if len(signals) != 1 || signals[0].InitiationID != a1 {
		t.Fatalf("normalize fallback: %+v", signals)
	}
}

// TestGroupKeyWinsOverFallback pins that a present groupKey is used verbatim
// and the groupLabels fallback never runs beside it.
func TestGroupKeyWinsOverFallback(t *testing.T) {
	signals := normalize(amPayload{
		GroupKey:    "{}/{alertname=\"X\"}",
		GroupLabels: map[string]string{"alertname": "X"},
		Alerts:      []amAlert{{Status: "firing", Labels: map[string]string{"alertname": "X"}}},
	}, "d")
	if len(signals) != 1 || signals[0].InitiationID != `{}/{alertname="X"}` {
		t.Fatalf("groupKey must win verbatim: %+v", signals)
	}
}

func TestOptInBearerAuth(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{
		{Name: "locked", CredentialEnvPrefix: "AGENTOPS_CRED_LOCKED_"},
		{Name: "open"},
	}}
	t.Setenv("AGENTOPS_CRED_LOCKED_TOKEN", "s3cret")
	a := testAdapter(t, f)
	h := a.handler()
	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{"alertname":"X"}}]}`

	if rec := post(t, h, "/webhook/locked", "", body); rec.Code != 401 {
		t.Fatalf("missing bearer: %d", rec.Code)
	}
	if rec := post(t, h, "/webhook/locked", "wrong", body); rec.Code != 401 {
		t.Fatalf("wrong bearer: %d", rec.Code)
	}
	if rec := post(t, h, "/webhook/locked", "s3cret", body); rec.Code != 200 {
		t.Fatalf("valid bearer: %d %s", rec.Code, rec.Body.String())
	}
	// uncredentialed source stays open (built-in-endpoint parity)
	if rec := post(t, h, "/webhook/open", "", body); rec.Code != 200 {
		t.Fatalf("anonymous on open source: %d", rec.Code)
	}
	// credentialed but projection missing → fail closed
	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "locked", CredentialEnvPrefix: "AGENTOPS_CRED_MISSING_"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if rec := post(t, h, "/webhook/locked", "anything", body); rec.Code != 401 {
		t.Fatalf("missing projection must fail closed: %d", rec.Code)
	}
}

func TestSourceListRefresh(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	h := a.handler()
	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{}}]}`

	if rec := post(t, h, "/webhook/late", "", body); rec.Code != 404 {
		t.Fatalf("before refresh: %d", rec.Code)
	}
	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "late"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if rec := post(t, h, "/webhook/late", "", body); rec.Code != 200 {
		t.Fatalf("after refresh: %d", rec.Code)
	}
	// Ready reported once for the newly served source
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.statuses {
		if s == "late" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("Ready reports for 'late': %d", n)
	}
}

// TestHealthzOK closes the /healthz route, never exercised by any other
// test here (every other test posts to /webhook/*).
func TestHealthzOK(t *testing.T) {
	a := &adapter{sources: map[string]string{}, reported: map[string]string{}}
	rec := httptest.NewRecorder()
	a.handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookReadBodyError closes handleWebhook's io.ReadAll error
// branch with a body that genuinely fails to read, rather than a size limit
// or a closed connection standing in for it.
func TestHandleWebhookReadBodyError(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}}
	a := testAdapter(t, f)
	req := httptest.NewRequest("POST", "/webhook/vm-alerts", errReader{})
	rec := httptest.NewRecorder()
	a.handler().ServeHTTP(rec, req)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "read body") {
		t.Fatalf("body-read failure should be 400: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookInvalidJSON closes handleWebhook's json.Unmarshal error
// branch with a real malformed payload.
func TestHandleWebhookInvalidJSON(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}}
	a := testAdapter(t, f)
	rec := post(t, a.handler(), "/webhook/vm-alerts", "", "{not-json")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid Alertmanager webhook JSON") {
		t.Fatalf("malformed JSON should be 400: %d %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookInboundFailureIs502 closes handleWebhook's a.mgr.Inbound
// error branch against a manager double that genuinely refuses the push.
func TestHandleWebhookInboundFailureIs502(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}, failInbound: true}
	a := testAdapter(t, f)
	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{"alertname":"X"}}]}`
	rec := post(t, a.handler(), "/webhook/vm-alerts", "", body)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "manager rejected the signals") {
		t.Fatalf("inbound failure should be 502: %d %s", rec.Code, rec.Body.String())
	}
}

// TestReportSkipsRecordingOnFailure closes report()'s error branch: a failed
// status POST must not be recorded as reported, so the next refresh retries
// it rather than believing the source's Ready state reached the manager.
func TestReportSkipsRecordingOnFailure(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}, failStatus: true}
	a := testAdapter(t, f)
	if got := a.reported["vm-alerts"]; got != "" {
		t.Fatalf("a failed status post must not be recorded as reported: %q", got)
	}

	f.mu.Lock()
	f.failStatus = false
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if got := a.reported["vm-alerts"]; got == "" {
		t.Fatal("a successful status post must now be recorded")
	}
}

// TestRefreshSourcesKeepsPreviousOnListError closes refreshSources' error
// branch: a transient listing failure must not drop sources already known
// to be served, or every hiccup would 404 every in-flight webhook.
func TestRefreshSourcesKeepsPreviousOnListError(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}}
	a := testAdapter(t, f)

	f.mu.Lock()
	f.failList = true
	f.mu.Unlock()
	a.refreshSources(context.Background())

	if _, served := a.source("vm-alerts"); !served {
		t.Fatal("a failed source listing must not drop previously known sources")
	}
}

// TestReconcileRegistrationURLUnknownRegistersNothing pins the branch where
// a token IS mounted but POD_NAMESPACE is unset: the adapter cannot compute
// its own webhook URL, so it writes no object (a placeholder URL would look
// registered and deliver nothing) and hands the operator the URL shape.
func TestReconcileRegistrationURLUnknownRegistersNothing(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.listen = ":8080"
	api := &fakeAPI{}
	a.kube = api.client(t) // token mounted
	cfg := json.RawMessage(`{"register":{"namespace":"monitoring"}}`)

	reason, msg := a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if reason != "RegistrationManual" {
		t.Fatalf("expected manual degradation, got %s: %s", reason, msg)
	}
	for _, want := range []string{
		"POD_NAMESPACE is unset",
		"http://agentops-signal-vm-alertmanager.<adapter-namespace>.svc:8080/webhook/vm-alerts",
		"monitoring/agentops-vm-alerts",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("instructions missing %q: %s", want, msg)
		}
	}
	if api.writes != 0 || len(api.objects) != 0 {
		t.Fatalf("no object may be written without a computable URL: %d writes, %v", api.writes, api.objects)
	}
}

// TestReconcileRegistrationURLUnknownWithoutTokenNamesBoth pins that a
// hand-deployed adapter missing BOTH the namespace fact and the token reports
// both causes at once, and still names the object to create in full, with
// the unknown namespace as a slot.
func TestReconcileRegistrationURLUnknownWithoutTokenNamesBoth(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.listen = ":8080"
	a.kube, a.kubeReason = nil, "not running in a cluster (KUBERNETES_SERVICE_HOST unset)"

	reason, msg := a.reconcileRegistration(context.Background(),
		SourceInfo{Name: "vm-alerts", Config: json.RawMessage(`{"register":{}}`)})
	if reason != "RegistrationManual" {
		t.Fatalf("expected manual degradation, got %s: %s", reason, msg)
	}
	for _, want := range []string{
		"POD_NAMESPACE is unset",
		"not running in a cluster",
		"VMAlertmanagerConfig <adapter-namespace>/agentops-vm-alerts",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("instructions missing %q: %s", want, msg)
		}
	}
}

// TestReconcileRegistrationMalformedBlockIsNamed pins that a register block
// which does not parse surfaces as RegistrationManual naming the parse
// problem, attempts no API call, and is distinct from an absent block.
func TestReconcileRegistrationMalformedBlockIsNamed(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.podNS, a.listen = "agent-ops", ":8080"
	api := &fakeAPI{}
	a.kube = api.client(t)

	cfg := json.RawMessage(`{"register":{"namespace":"monitoring","maxAlerts":"ten"}}`)
	reason, msg := a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if reason != "RegistrationManual" {
		t.Fatalf("a malformed block must not read as absent: %s %s", reason, msg)
	}
	for _, want := range []string{"malformed register block", "maxAlerts", "/webhook/vm-alerts"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("instructions missing %q: %s", want, msg)
		}
	}
	if api.writes != 0 || len(api.objects) != 0 {
		t.Fatalf("a malformed block must attempt no registration: %d writes", api.writes)
	}

	// correcting the block heals on the next pass
	reason, msg = a.reconcileRegistration(context.Background(),
		SourceInfo{Name: "vm-alerts", Config: json.RawMessage(`{"register":{"namespace":"monitoring","maxAlerts":10}}`)})
	if reason != "AdapterReady" || !strings.Contains(msg, "monitoring/agentops-vm-alerts") {
		t.Fatalf("corrected block should register: %s %s", reason, msg)
	}
}

// TestReconcileRegistrationEnsureRegistrationErrorDegradesToManual closes
// the branch where kube access exists but the write itself fails, and pins
// that a refusal names the account to bind the grant to.
func TestReconcileRegistrationEnsureRegistrationErrorDegradesToManual(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	a.podNS = "agent-ops"
	api := &fakeAPI{forbid: true}
	a.kube = api.client(t)
	cfg := json.RawMessage(`{"register":{}}`)
	reason, msg := a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if reason != "RegistrationManual" || !strings.Contains(msg, "forbidden") {
		t.Fatalf("ensureRegistration failure should degrade to manual instructions: %s %s", reason, msg)
	}
	assertNamesLiveAccountFields(t, msg)

	// a non-permission failure names no account: binding one would not help
	missing := &fakeAPI{noCRD: true}
	a.kube = missing.client(t)
	_, msg = a.reconcileRegistration(context.Background(), SourceInfo{Name: "vm-alerts", Config: cfg})
	if !strings.Contains(msg, "CRD") || strings.Contains(msg, "serviceAccountName") {
		t.Fatalf("a missing CRD should be named without an account remediation: %s", msg)
	}
}

func TestMustEnvReturnsValue(t *testing.T) {
	t.Setenv("ALERTMANAGER_TEST_MUSTENV_VAL", "present")
	if got := mustEnv("ALERTMANAGER_TEST_MUSTENV_VAL"); got != "present" {
		t.Fatalf("got %q", got)
	}
}

// TestMustEnvMissingExits closes mustEnv's log.Fatalf branch, which calls
// os.Exit(1) — untestable in-process, so this re-execs the test binary as a
// real subprocess (the standard Go idiom for testing os.Exit paths, already
// used by signal-cron's own mustEnv test) and asserts its exit code and
// stderr, rather than mocking the exit away.
func TestMustEnvMissingExits(t *testing.T) {
	if os.Getenv("ALERTMANAGER_TEST_MUSTENV_SUBPROCESS") == "1" {
		mustEnv("ALERTMANAGER_TEST_DOES_NOT_EXIST")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvMissingExits")
	cmd.Env = append(os.Environ(), "ALERTMANAGER_TEST_MUSTENV_SUBPROCESS=1")
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
// registryLoop) against a real HTTP double for the manager, then cancels
// baseContext directly — main()'s own signal.NotifyContext derives from it,
// so this stops main() exactly as a real SIGTERM would, without sending any
// OS signal that could also reach other tests sharing this process.
func TestMainRunsUntilSignaled(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}, statusHit: make(chan string, 4)}
	mgrSrv := f.server(t)

	prevURL, hadURL := os.LookupEnv("MANAGER_URL")
	prevTok, hadTok := os.LookupEnv("ADAPTER_TOKEN")
	prevName, hadName := os.LookupEnv("ADAPTER_NAME")
	prevListen, hadListen := os.LookupEnv("LISTEN_ADDR")
	os.Setenv("MANAGER_URL", mgrSrv.URL)
	os.Setenv("ADAPTER_TOKEN", "t")
	os.Unsetenv("ADAPTER_NAME") // exercise the default-name fill-in branch
	os.Unsetenv("LISTEN_ADDR")  // exercise the default-listen fill-in branch
	defer func() {
		restore := func(had bool, key, prev string) {
			if had {
				os.Setenv(key, prev)
			} else {
				os.Unsetenv(key)
			}
		}
		restore(hadURL, "MANAGER_URL", prevURL)
		restore(hadTok, "ADAPTER_TOKEN", prevTok)
		restore(hadName, "ADAPTER_NAME", prevName)
		restore(hadListen, "LISTEN_ADDR", prevListen)
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
		t.Fatal("main() never completed a registry refresh")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after its context was cancelled")
	}
}

// TestSignalIDStableAcrossRetryAndChangesWithBody pins that a re-delivered
// (byte-identical) body repeats each alert's signalId, a changed body changes
// it, and an alert without a fingerprint falls back to its index.
func TestSignalIDStableAcrossRetryAndChangesWithBody(t *testing.T) {
	payload := amPayload{GroupKey: "g", Alerts: []amAlert{
		{Status: "firing", Fingerprint: "f1", Labels: map[string]string{"alertname": "X"}},
		{Status: "firing", Labels: map[string]string{"alertname": "Y"}},
	}}
	d1 := bodyDigest([]byte(`{"a":1}`))
	first := normalize(payload, d1)
	retry := normalize(payload, bodyDigest([]byte(`{"a":1}`)))
	changed := normalize(payload, bodyDigest([]byte(`{"a":2}`)))
	if len(first) != 2 {
		t.Fatalf("signals: %+v", first)
	}
	if first[0].SignalID != "am:"+d1+":f1" || first[1].SignalID != "am:"+d1+":1" {
		t.Fatalf("formats: %q %q", first[0].SignalID, first[1].SignalID)
	}
	for i := range first {
		if first[i].SignalID != retry[i].SignalID {
			t.Fatalf("retry changed signalId: %q vs %q", first[i].SignalID, retry[i].SignalID)
		}
		if first[i].SignalID == changed[i].SignalID {
			t.Fatalf("changed body kept signalId %q", first[i].SignalID)
		}
	}
	if first[0].SignalID == first[1].SignalID {
		t.Fatal("two alerts of one body share a signalId")
	}
}
