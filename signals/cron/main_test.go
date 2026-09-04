package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// statusCall records one ReportStatus POST as observed by a test server.
type statusCall struct {
	name    string
	ready   bool
	reason  string
	message string
}

// newSourcesServer builds a signal-cron test double: GET /signal/sources
// returns whatever `list` currently points at (so a test can mutate it
// between refreshSources calls), and every status POST is appended to
// `calls` under a mutex.
func newSourcesServer(t *testing.T, list *[]SourceInfo, calls *[]statusCall, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/signal/sources":
			mu.Lock()
			cur := *list
			mu.Unlock()
			// Built by hand, never json.Marshal(cur): some test cases plant a
			// deliberately-invalid Config to exercise refreshSources' bad-JSON
			// branch, and json.Marshal validates json.RawMessage fields and
			// errors on it — which would silently empty the response body.
			// Real config bytes arrive verbatim from the manager's API, so
			// splicing them in unvalidated is the more faithful double too.
			var sb strings.Builder
			sb.WriteByte('[')
			for i, s := range cur {
				if i > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, `{"name":%q`, s.Name)
				if len(s.Config) > 0 {
					fmt.Fprintf(&sb, `,"config":%s`, s.Config)
				}
				if s.CredentialEnvPrefix != "" {
					fmt.Fprintf(&sb, `,"credentialEnvPrefix":%q`, s.CredentialEnvPrefix)
				}
				sb.WriteByte('}')
			}
			sb.WriteByte(']')
			_, _ = w.Write([]byte(sb.String()))
		case strings.HasSuffix(r.URL.Path, "/status"):
			var body struct {
				Ready   bool   `json:"ready"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/signal/sources/"), "/status")
			mu.Lock()
			*calls = append(*calls, statusCall{name: name, ready: body.Ready, reason: body.Reason, message: body.Message})
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func rawConfig(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}

// TestRefreshSourcesClassifiesEveryProblem closes refreshSources' 0% gap: one
// call against a real HTTP double covering every validation branch (missing
// config, invalid JSON, missing input, bad schedule, valid) and asserting
// which sources actually get served plus what each reports.
func TestRefreshSourcesClassifiesEveryProblem(t *testing.T) {
	var mu sync.Mutex
	var calls []statusCall
	list := []SourceInfo{
		{Name: "missing-config"},
		// A syntactically valid JSON scalar (not an object) still fails
		// json.Unmarshal into sourceConfig with a real type-mismatch error —
		// genuinely malformed JSON syntax can't survive re-encoding as part
		// of this response's own array, so this is the faithful way to
		// reach the "not valid JSON for the cron adapter" branch.
		{Name: "bad-json", Config: json.RawMessage(`42`)},
		{Name: "no-input", Config: rawConfig(t, sourceConfig{Schedule: "* * * * *"})},
		{Name: "bad-schedule", Config: rawConfig(t, sourceConfig{Schedule: "nope", Input: "x"})},
		{Name: "valid", Config: rawConfig(t, sourceConfig{Schedule: "* * * * *", Input: "hi", Title: "T"})},
	}
	srv := newSourcesServer(t, &list, &calls, &mu)
	defer srv.Close()

	a := &adapter{
		mgr:        NewManager(srv.URL, "t"),
		sourceType: "cron",
		sources:    map[string]servedSource{},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())

	a.mu.Lock()
	served := a.sources
	a.mu.Unlock()
	if len(served) != 1 {
		t.Fatalf("expected exactly one served source, got %+v", served)
	}
	if _, ok := served["valid"]; !ok {
		t.Fatalf("expected 'valid' to be served, got %+v", served)
	}

	mu.Lock()
	defer mu.Unlock()
	byName := map[string]statusCall{}
	for _, c := range calls {
		byName[c.name] = c
	}
	if len(byName) != 5 {
		t.Fatalf("expected a status report per source, got %+v", calls)
	}
	if byName["missing-config"].ready || byName["missing-config"].reason != "InvalidConfig" ||
		!strings.Contains(byName["missing-config"].message, "spec.config is missing") {
		t.Errorf("missing-config report: %+v", byName["missing-config"])
	}
	if byName["bad-json"].ready || !strings.Contains(byName["bad-json"].message, "not valid JSON") {
		t.Errorf("bad-json report: %+v", byName["bad-json"])
	}
	if byName["no-input"].ready || !strings.Contains(byName["no-input"].message, "spec.config.input is required") {
		t.Errorf("no-input report: %+v", byName["no-input"])
	}
	if byName["bad-schedule"].ready || !strings.Contains(byName["bad-schedule"].message, "spec.config.schedule") {
		t.Errorf("bad-schedule report: %+v", byName["bad-schedule"])
	}
	if !byName["valid"].ready || byName["valid"].reason != "AdapterReady" {
		t.Errorf("valid report: %+v", byName["valid"])
	}
}

// TestRefreshSourcesIsIdempotentAndTracksTransitions closes the "avoid spam"
// branches: an unchanged problem or an unchanged ok status must not re-report,
// while flipping ok->invalid or invalid->ok must.
func TestRefreshSourcesIsIdempotentAndTracksTransitions(t *testing.T) {
	var mu sync.Mutex
	var calls []statusCall
	list := []SourceInfo{
		{Name: "flaky", Config: rawConfig(t, sourceConfig{Schedule: "* * * * *", Input: "hi"})},
	}
	srv := newSourcesServer(t, &list, &calls, &mu)
	defer srv.Close()

	a := &adapter{
		mgr:        NewManager(srv.URL, "t"),
		sourceType: "cron",
		sources:    map[string]servedSource{},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background()) // -> ok, 1 report
	a.refreshSources(context.Background()) // still ok, no new report

	mu.Lock()
	if len(calls) != 1 {
		mu.Unlock()
		t.Fatalf("expected exactly one report while staying ok, got %+v", calls)
	}
	// flip to invalid: same problem repeated should not double-report either.
	list[0].Config = json.RawMessage(`42`) // valid JSON scalar, wrong shape
	mu.Unlock()

	a.refreshSources(context.Background())
	a.refreshSources(context.Background())

	mu.Lock()
	if len(calls) != 2 {
		mu.Unlock()
		t.Fatalf("expected ok->invalid to add exactly one report, got %+v", calls)
	}
	// flip back to ok: must report AdapterReady again.
	list[0].Config = rawConfig(t, sourceConfig{Schedule: "* * * * *", Input: "hi"})
	mu.Unlock()

	a.refreshSources(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 || calls[2].reason != "AdapterReady" {
		t.Fatalf("expected invalid->ok to report AdapterReady, got %+v", calls)
	}
}

// TestRefreshSourcesKeepsPreviousOnListError closes the mgr.Sources()
// error branch: a broken manager must not wipe out sources already being
// served.
func TestRefreshSourcesKeepsPreviousOnListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	existing := map[string]servedSource{"kept": {}}
	a := &adapter{
		mgr:        NewManager(srv.URL, "t"),
		sourceType: "cron",
		sources:    existing,
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.sources["kept"]; !ok {
		t.Fatalf("expected previous sources to survive a list error, got %+v", a.sources)
	}
}

// evalServer is a real HTTP double for the state/inbound calls evaluate
// makes, letting each test control the GetState response and observe
// PutState/Inbound bodies.
type evalServer struct {
	mu          sync.Mutex
	getStateVal string
	getStateErr bool
	inboundErr  bool
	puts        []string
	inbound     []Signal
}

func newEvalServer(t *testing.T, es *evalServer) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/signal/state/"):
			es.mu.Lock()
			defer es.mu.Unlock()
			if es.getStateErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"value": es.getStateVal})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/signal/state/"):
			var body struct {
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.mu.Lock()
			es.puts = append(es.puts, body.Value)
			es.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/signal/inbound":
			es.mu.Lock()
			defer es.mu.Unlock()
			if es.inboundErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var body struct {
				Source  string   `json:"source"`
				Signals []Signal `json:"signals"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.inbound = append(es.inbound, body.Signals...)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func dailySource(t *testing.T) servedSource {
	t.Helper()
	sched, err := ParseCron("0 6 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	return servedSource{sched: sched, cfg: sourceConfig{Input: "payload", Title: "title"}}
}

// TestEvaluateUnknownSourceIsNoop closes the "not served" early return —
// no HTTP call is made at all (a server that fails any request proves it).
func TestEvaluateUnknownSourceIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request for an unknown source: %s", r.URL.Path)
	}))
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{}}
	if err := a.evaluate(context.Background(), "ghost", time.Now()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
}

// TestEvaluateFirstSightSeedsCursorWithoutFiring closes the raw=="" branch:
// a source seen for the first time must seed last-fire to "now" and never
// fire a signal for ticks that predate it.
func TestEvaluateFirstSightSeedsCursorWithoutFiring(t *testing.T) {
	es := &evalServer{getStateVal: ""}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	now := at("2026-08-06T06:00:00Z")
	if err := a.evaluate(context.Background(), "s", now); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.inbound) != 0 {
		t.Fatalf("expected no signal fired on first sight, got %+v", es.inbound)
	}
	if len(es.puts) != 1 || es.puts[0] != "2026-08-06T06:00:00Z" {
		t.Fatalf("expected cursor seeded to now, got %+v", es.puts)
	}
}

// TestEvaluateResetsUnparsableCursor closes the corrupt-cursor recovery
// branch: garbage state resets to "now" rather than erroring forever.
func TestEvaluateResetsUnparsableCursor(t *testing.T) {
	es := &evalServer{getStateVal: "not-a-timestamp"}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	now := at("2026-08-06T06:00:00Z")
	if err := a.evaluate(context.Background(), "s", now); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.inbound) != 0 {
		t.Fatalf("expected no signal fired when resetting, got %+v", es.inbound)
	}
	if len(es.puts) != 1 || es.puts[0] != "2026-08-06T06:00:00Z" {
		t.Fatalf("expected cursor reset to now, got %+v", es.puts)
	}
}

// TestEvaluateNoTickDueIsNoop closes the "latest.IsZero()" branch: a cursor
// already at/after the next tick fires nothing and writes nothing.
func TestEvaluateNoTickDueIsNoop(t *testing.T) {
	es := &evalServer{getStateVal: "2026-08-06T06:00:00Z"}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	now := at("2026-08-06T06:30:00Z") // next tick (08-07T06:00) is still ahead
	if err := a.evaluate(context.Background(), "s", now); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.inbound) != 0 || len(es.puts) != 0 {
		t.Fatalf("expected no calls at all, got inbound=%+v puts=%+v", es.inbound, es.puts)
	}
}

// TestEvaluateFiresElapsedTickThenPersistsCursor closes the main firing path:
// a due tick posts exactly one Inbound signal with the documented
// fingerprint/labels/kind shape, then persists the fired tick (not "now").
func TestEvaluateFiresElapsedTickThenPersistsCursor(t *testing.T) {
	es := &evalServer{getStateVal: "2026-08-05T06:00:00Z"}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	now := at("2026-08-06T12:00:00Z") // one tick (08-06T06:00) has elapsed
	if err := a.evaluate(context.Background(), "s", now); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.inbound) != 1 {
		t.Fatalf("expected exactly one fired signal, got %+v", es.inbound)
	}
	sig := es.inbound[0]
	if sig.Fingerprint != "s@2026-08-06T06:00:00Z" || sig.Kind != "job" ||
		sig.Payload != "payload" || sig.Title != "title" {
		t.Fatalf("unexpected signal: %+v", sig)
	}
	if sig.Labels["alertgroup"] != "cron" || sig.Labels["alertname"] != "s" || sig.Labels["source"] != "s" {
		t.Fatalf("unexpected labels: %+v", sig.Labels)
	}
	if len(es.puts) != 1 || es.puts[0] != "2026-08-06T06:00:00Z" {
		t.Fatalf("expected cursor persisted to the fired tick, got %+v", es.puts)
	}
}

// TestEvaluateGetStateErrorPropagates closes the GetState error branch.
func TestEvaluateGetStateErrorPropagates(t *testing.T) {
	es := &evalServer{getStateErr: true}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	if err := a.evaluate(context.Background(), "s", time.Now()); err == nil {
		t.Fatal("expected an error from a failing GetState")
	}
}

// TestEvaluateInboundErrorSkipsPersist closes the Inbound error branch: a
// failed fire must not advance the cursor (the tick is retried next round).
func TestEvaluateInboundErrorSkipsPersist(t *testing.T) {
	es := &evalServer{getStateVal: "2026-08-05T06:00:00Z", inboundErr: true}
	srv := newEvalServer(t, es)
	defer srv.Close()

	a := &adapter{mgr: NewManager(srv.URL, "t"), sources: map[string]servedSource{"s": dailySource(t)}}
	now := at("2026-08-06T12:00:00Z")
	if err := a.evaluate(context.Background(), "s", now); err == nil {
		t.Fatal("expected an error from a failing Inbound")
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.puts) != 0 {
		t.Fatalf("expected the cursor untouched after a failed fire, got %+v", es.puts)
	}
}

// TestSleepCtxHonorsCancellation closes sleepCtx's context.Done() branch: it
// must return well before the requested duration when the context ends
// early.
func TestSleepCtxHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	sleepCtx(ctx, time.Minute)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sleepCtx did not honor cancellation, took %s", elapsed)
	}
}

// TestSleepCtxWaitsOutDuration closes sleepCtx's time.After() branch: with a
// live context it waits at least the requested duration.
func TestSleepCtxWaitsOutDuration(t *testing.T) {
	start := time.Now()
	sleepCtx(context.Background(), 20*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("sleepCtx returned early, took %s", elapsed)
	}
}

// TestMustEnvReturnsValue closes mustEnv's success path.
func TestMustEnvReturnsValue(t *testing.T) {
	if err := os.Setenv("CRON_TEST_MUSTENV_VAL", "present"); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv("CRON_TEST_MUSTENV_VAL")
	if got := mustEnv("CRON_TEST_MUSTENV_VAL"); got != "present" {
		t.Fatalf("got %q", got)
	}
}

// TestMustEnvMissingExits closes mustEnv's log.Fatalf branch, which calls
// os.Exit(1) — untestable in-process, so this re-execs the test binary as a
// real subprocess (the standard Go idiom for testing os.Exit paths) and
// asserts its exit code and stderr, rather than mocking the exit away.
func TestMustEnvMissingExits(t *testing.T) {
	if os.Getenv("CRON_TEST_MUSTENV_SUBPROCESS") == "1" {
		mustEnv("CRON_TEST_DOES_NOT_EXIST")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvMissingExits")
	cmd.Env = append(os.Environ(), "CRON_TEST_MUSTENV_SUBPROCESS=1")
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

// TestMainRunsUntilSignaled closes main() itself: it starts the real
// process loop (env parsing, adapter construction, the refresh/evaluate
// loop, signal.NotifyContext) against a real HTTP double, then sends this
// process a real SIGTERM — which main()'s own signal.NotifyContext must
// catch and use to unwind sleepCtx immediately, letting main() return.
func TestMainRunsUntilSignaled(t *testing.T) {
	var mu sync.Mutex
	var calls []statusCall
	iterated := make(chan struct{}, 1)
	list := []SourceInfo{
		{Name: "s", Config: rawConfig(t, sourceConfig{Schedule: "* * * * *", Input: "hi"})},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/signal/sources":
			mu.Lock()
			_ = json.NewEncoder(w).Encode(list)
			mu.Unlock()
		case strings.HasSuffix(r.URL.Path, "/status"):
			mu.Lock()
			calls = append(calls, statusCall{})
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/signal/state/"):
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]string{"value": ""})
			case http.MethodPut:
				select {
				case iterated <- struct{}{}:
				default:
				}
				w.WriteHeader(http.StatusOK)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	prevURL, hadURL := os.LookupEnv("MANAGER_URL")
	prevTok, hadTok := os.LookupEnv("ADAPTER_TOKEN")
	prevName, hadName := os.LookupEnv("ADAPTER_NAME")
	os.Setenv("MANAGER_URL", srv.URL)
	os.Setenv("ADAPTER_TOKEN", "t")
	os.Setenv("ADAPTER_NAME", "cron")
	defer func() {
		if hadURL {
			os.Setenv("MANAGER_URL", prevURL)
		} else {
			os.Unsetenv("MANAGER_URL")
		}
		if hadTok {
			os.Setenv("ADAPTER_TOKEN", prevTok)
		} else {
			os.Unsetenv("ADAPTER_TOKEN")
		}
		if hadName {
			os.Setenv("ADAPTER_NAME", prevName)
		} else {
			os.Unsetenv("ADAPTER_NAME")
		}
	}()

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	select {
	case <-iterated:
	case <-time.After(5 * time.Second):
		t.Fatal("main() never completed a full refresh/evaluate iteration")
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signaling self: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after SIGTERM")
	}
}
