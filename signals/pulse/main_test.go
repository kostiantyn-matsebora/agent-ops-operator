package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
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
		sources:    map[string]sourceState{},
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

// resolvedBody shares firingBody's startsAt on purpose — "startsAt" renders
// Pulse's `.StartTime` (payload.go), documented to persist across an
// occurrence's lifecycle including into its resolved event, so a firing
// event and its later resolved pair for the same occurrence carry the same
// startsAt. That equality is exactly what lets the "id+startTime" strategy
// still collapse a firing/resolved pair into one initiationId.
func resolvedBody(id string) string {
	return `{"event":"resolved","id":"` + id + `","resource":"vm-101","resourceType":"vm","node":"pve1","severity":"critical","message":"CPU high","value":"12","startsAt":"2026-01-01T00:00:00Z"}`
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
	// the default correlationStrategy ("id+startTime") folds startsAt in —
	// see firingBody's fixture value.
	if first["initiationId"] != "pulse:vm-101:abc:2026-01-01T00:00:00Z" || !strings.Contains(first["title"].(string), "CPU high") {
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

func TestNormalizeTitleIsTheAlertMessage(t *testing.T) {
	e := pulseEvent{Event: "alert", Resource: "master", Message: "IoT: Tion 4S Bedroom is unreachable from every current observation location by ICMP probe: context deadline exceeded"}
	signals := normalize(e, correlationStrategyIDStartTime)
	if len(signals) != 1 {
		t.Fatalf("expected one signal, got %d", len(signals))
	}
	if got := signals[0].Title; got != "🔍 "+e.Message {
		t.Fatalf("title should be the alert's message, not the bare resource: %q", got)
	}
}

func TestNormalizeTitleFallsBackToResourceWhenMessageIsBlank(t *testing.T) {
	e := pulseEvent{Event: "alert", Resource: "master", Message: ""}
	signals := normalize(e, correlationStrategyIDStartTime)
	if len(signals) != 1 {
		t.Fatalf("expected one signal, got %d", len(signals))
	}
	if got := signals[0].Title; got != "🔍 master" {
		t.Fatalf("a message-less alert should fall back to the resource name: %q", got)
	}
}

// TestInitiationIDStableAcrossFiringAndResolvedPair pins the behavior under
// the new default strategy ("id+startTime") — resolvedBody shares
// firingBody's startsAt on purpose, so this still passes without the
// fixtures themselves changing meaning.
func TestInitiationIDStableAcrossFiringAndResolvedPair(t *testing.T) {
	firing, err := parsePulseEvent([]byte(firingBody("occ-1")))
	if err != nil {
		t.Fatalf("parse firing: %v", err)
	}
	resolved, err := parsePulseEvent([]byte(resolvedBody("occ-1")))
	if err != nil {
		t.Fatalf("parse resolved: %v", err)
	}
	if firing.initiationID(correlationStrategyIDStartTime) != resolved.initiationID(correlationStrategyIDStartTime) {
		t.Fatalf("same occurrence must carry the same initiationId across firing and resolved: firing=%q resolved=%q",
			firing.initiationID(correlationStrategyIDStartTime), resolved.initiationID(correlationStrategyIDStartTime))
	}
	other, _ := parsePulseEvent([]byte(firingBody("occ-2")))
	if other.initiationID(correlationStrategyIDStartTime) == firing.initiationID(correlationStrategyIDStartTime) {
		t.Fatal("different alert ids must derive different initiationIds")
	}
}

// TestInitiationIDFallbackWithoutID pins the no-ID hash fallback, which
// design.md's own non-goal keeps identical regardless of strategy — both
// canonical strategy values are exercised here to prove that.
func TestInitiationIDFallbackWithoutID(t *testing.T) {
	for _, strategy := range []string{correlationStrategyID, correlationStrategyIDStartTime} {
		t.Run(strategy, func(t *testing.T) {
			e1 := pulseEvent{Resource: "ct-1", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
			e2 := pulseEvent{Resource: "ct-1", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
			if e1.initiationID(strategy) != e2.initiationID(strategy) || e1.initiationID(strategy) == "" {
				t.Fatalf("identical fields with no id must derive the same non-empty initiationId: %q vs %q", e1.initiationID(strategy), e2.initiationID(strategy))
			}
			e3 := pulseEvent{Resource: "ct-2", ResourceType: "container", Severity: "warning", Message: "disk 90%"}
			if e3.initiationID(strategy) == e1.initiationID(strategy) {
				t.Fatal("different resources must derive different initiationIds")
			}
		})
	}
	if e1, e2 := (pulseEvent{Resource: "ct-1", Message: "disk 90%"}), (pulseEvent{Resource: "ct-1", Message: "disk 90%"}); e1.initiationID(correlationStrategyID) != e2.initiationID(correlationStrategyIDStartTime) {
		t.Fatalf("the no-ID fallback must be identical across strategies: %q vs %q",
			e1.initiationID(correlationStrategyID), e2.initiationID(correlationStrategyIDStartTime))
	}
}

// TestWebhookInitiationIDRespectsConfiguredCorrelationStrategy is task 1.3's
// verification: the same Pulse alert id, posted with two different start
// times (a re-fire), collapses to one initiationId under the served
// source's "id" strategy but yields two distinct ones under
// "id+startTime" — proving the resolved strategy actually threads from the
// source's own spec.config through refreshSources/served into the pushed
// signal, not just through initiationID in isolation.
func TestWebhookInitiationIDRespectsConfiguredCorrelationStrategy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		strategy string
		wantSame bool
	}{
		{"id strategy collapses a re-fire with a new start time", `"id"`, true},
		{"id+startTime strategy distinguishes a re-fire", `"id+startTime"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeManager{sources: []SourceInfo{{
				Name:   "pulse-default",
				Config: json.RawMessage(`{"correlationStrategy":` + tc.strategy + `}`),
			}}}
			a := testAdapter(t, f)
			h := a.handler()

			first := `{"event":"alert","id":"refire-1","resource":"vm-101","resourceType":"vm","node":"pve1","severity":"critical","message":"CPU high","value":"97","startsAt":"2026-01-01T00:00:00Z"}`
			second := `{"event":"alert","id":"refire-1","resource":"vm-101","resourceType":"vm","node":"pve1","severity":"critical","message":"CPU high","value":"97","startsAt":"2026-02-01T00:00:00Z"}`

			post(t, h, "/webhook/pulse-default", first)
			post(t, h, "/webhook/pulse-default", second)

			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.inbounds) != 2 {
				t.Fatalf("expected two pushes: %+v", f.inbounds)
			}
			idOf := func(i int) string {
				signals := f.inbounds[i]["signals"].([]any)
				return signals[0].(map[string]any)["initiationId"].(string)
			}
			id1, id2 := idOf(0), idOf(1)
			if same := id1 == id2; same != tc.wantSame {
				t.Fatalf("strategy %s: id1=%q id2=%q same=%v, want same=%v", tc.strategy, id1, id2, same, tc.wantSame)
			}
		})
	}
}

// TestUnrelatedPulseAlertsNeverMerge pins the motivating case from the
// signal-correlation-id proposal: a node-memory threshold alert and an
// unreachable-device alert are unrelated problems and must open distinct
// conversations rather than hashing to the same empty-signature bucket the
// manager used to collapse them into.
func TestUnrelatedPulseAlertsNeverMerge(t *testing.T) {
	memoryThreshold := `{"event":"alert","id":"metric-threshold:memory","resource":"k8s-node-3","resourceType":"node","node":"k8s-node-3","severity":"warning","message":"memory above threshold","value":"92","startsAt":"2026-01-01T00:00:00Z"}`
	unreachableDevice := `{"event":"alert","id":"provider-incident:8f21a","resource":"switch-12","resourceType":"device","node":"","severity":"critical","message":"device unreachable","value":"","startsAt":"2026-01-01T00:00:00Z"}`

	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	h := a.handler()

	post(t, h, "/webhook/pulse-default", memoryThreshold)
	post(t, h, "/webhook/pulse-default", unreachableDevice)

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 2 {
		t.Fatalf("expected two separate pushes: %+v", f.inbounds)
	}
	idOf := func(inbound map[string]any) string {
		signals := inbound["signals"].([]any)
		return signals[0].(map[string]any)["initiationId"].(string)
	}
	id1, id2 := idOf(f.inbounds[0]), idOf(f.inbounds[1])
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("two unrelated Pulse alerts must carry distinct, non-empty initiationIds: %q vs %q", id1, id2)
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

// TestRecreatedSourceIsReportedAgain pins pruneReported: a source deleted and
// recreated under the same name, reaching the same reason and message, must
// get its Ready condition posted again — the new object has none — and a
// source no longer served leaves nothing behind in the reported map.
func TestRecreatedSourceIsReportedAgain(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	posts := func() int {
		f.mu.Lock()
		defer f.mu.Unlock()
		n := 0
		for _, s := range f.statuses {
			if s["name"] == "pulse-default" {
				n++
			}
		}
		return n
	}
	if got := posts(); got != 1 {
		t.Fatalf("first refresh must report once: %d", got)
	}

	f.mu.Lock()
	f.sources = nil
	f.mu.Unlock()
	a.refreshSources(context.Background())
	a.mu.Lock()
	_, kept := a.reported["pulse-default"]
	a.mu.Unlock()
	if kept {
		t.Fatal("an unserved source's last report must be pruned")
	}

	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "pulse-default"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if got := posts(); got != 2 {
		t.Fatalf("a recreated source must be reported again: %d posts", got)
	}
}

// TestHealthzOK closes the /healthz route, never exercised by any other test
// here (every other test posts to /webhook/*).
func TestHealthzOK(t *testing.T) {
	a := &adapter{sources: map[string]sourceState{}, reported: map[string]string{}}
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

	if _, _, ok := a.served("pulse-default"); !ok {
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

// TestEnvOrDefaults is the default-fill-in branch's own unit test — the
// thing TestMainRunsUntilSignaled used to exercise by unsetting LISTEN_ADDR
// and letting main() bind the real default :8080, which collided with
// anything else already listening there on the host or CI runner and took
// the whole test binary down with it (main.go's log.Fatalf on a bind
// failure calls os.Exit inside the test process). Covering the branch here
// instead means the integration test below never needs to bind a fixed
// port at all.
func TestEnvOrDefaults(t *testing.T) {
	os.Unsetenv("ENVOR_TEST_KEY")
	if got := envOr("ENVOR_TEST_KEY", "fallback"); got != "fallback" {
		t.Fatalf("expected the fallback when unset, got %q", got)
	}
	t.Setenv("ENVOR_TEST_KEY", "set")
	if got := envOr("ENVOR_TEST_KEY", "fallback"); got != "set" {
		t.Fatalf("expected the set value to win, got %q", got)
	}
}

// TestMainRunsUntilSignaled closes main() itself: it starts the real process
// loop (adapter construction, the real HTTP server, registryLoop) against a
// real HTTP double for the manager serving one source with no spec.config —
// which exercises the RegistrationNotConfigured report as the sync point —
// then cancels baseContext directly — main()'s own signal.NotifyContext
// derives from it, so this stops main() exactly as a real SIGTERM would.
func TestMainRunsUntilSignaled(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}, statusHit: make(chan string, 4)}
	mgrSrv := f.server(t)

	env := map[string]string{
		"MANAGER_URL":   mgrSrv.URL,
		"ADAPTER_TOKEN": "t",
		"LISTEN_ADDR":   "127.0.0.1:0", // OS-assigned port — never a fixed one a real process could already hold
	}
	prev := map[string]string{}
	had := map[string]bool{}
	for k, v := range env {
		prev[k], had[k] = os.LookupEnv(k)
		os.Setenv(k, v)
	}
	os.Unsetenv("ADAPTER_NAME") // exercise the default-name fill-in branch
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

// TestConcurrentRefreshAndWebhookDelivery actually races registryLoop's
// kind of source-map refresh against inbound webhook delivery on the same
// adapter, the scenario every other test in this package drives
// sequentially from one goroutine — `go test -race` has nothing to catch
// without a test that does this. One goroutine repeatedly calls
// refreshSources (what registryLoop's ticker does on every tick), another
// concurrently posts to the webhook endpoint (what a real Pulse delivery
// does), both touching a.sources/a.reported through the same mutex-guarded
// paths main.go reads and writes in production.
func TestConcurrentRefreshAndWebhookDelivery(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	mgrSrv := f.server(t)
	a := &adapter{
		mgr:        NewManager(mgrSrv.URL, "tok"),
		sourceType: "pulse",
		sources:    map[string]sourceState{},
		reported:   map[string]string{},
	}
	h := a.handler()

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			a.refreshSources(context.Background())
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			post(t, h, "/webhook/pulse-default", firingBody("race-"+strconv.Itoa(i)))
		}
	}()

	wg.Wait()

	if _, _, ok := a.served("pulse-default"); !ok {
		t.Fatal("the source must still be served once both goroutines finish")
	}
}

// TestUnknownSourceRefusalNamesIt is task 1.3's verification: the 404 for an
// unknown or unserved webhook path names the source it failed to match, so a
// misrouted Pulse delivery is diagnosable from the response alone.
func TestUnknownSourceRefusalNamesIt(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	h := testAdapter(t, f).handler()

	rec := post(t, h, "/webhook/pulse-other", firingBody("abc"))
	if rec.Code != 404 {
		t.Fatalf("unserved source: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("404 body must be JSON: %v", err)
	}
	if !strings.Contains(body["error"], `"pulse-other"`) {
		t.Fatalf("404 body must name the source, got %q", body["error"])
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 0 {
		t.Fatalf("a refused source must push nothing: %d pushes", len(f.inbounds))
	}
}
