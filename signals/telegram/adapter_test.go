package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// ---- refreshSources -------------------------------------------------------
//
// refreshSources was 0% covered: nothing in this package called it, so
// neither the four InvalidConfig branches, the Ready branch, the dedup
// against a.reported, nor the served-map rebuild had ever run. fakeSourceServer
// answers the two calls it makes to the manager (list, then per-source
// status) and records what was reported.

type fakeSourceServer struct {
	mu       sync.Mutex
	sources  []SourceInfo
	statuses []map[string]any
}

func (f *fakeSourceServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /signal/sources", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.sources)
	})
	mux.HandleFunc("POST /signal/sources/{name}/status", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["name"] = r.PathValue("name")
		f.mu.Lock()
		f.statuses = append(f.statuses, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (f *fakeSourceServer) set(sources []SourceInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sources = sources
}

func (f *fakeSourceServer) statusesFor(name string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, s := range f.statuses {
		if s["name"] == name {
			out = append(out, s)
		}
	}
	return out
}

func rawConfig(t *testing.T, v sourceConfig) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRefreshSourcesReportsEachInvalidConfigProblem closes the whole
// zero-coverage function: every InvalidConfig branch, the Ready branch, and
// the served-map rebuild that only keeps the valid sources.
func TestRefreshSourcesReportsEachInvalidConfigProblem(t *testing.T) {
	f := &fakeSourceServer{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	f.set([]SourceInfo{
		{Name: "no-config"},
		// A CRD's opaque config field is always syntactically valid JSON on
		// the wire (apiextensions guarantees it); "not valid JSON for the
		// telegram signal adapter" is this adapter's own unmarshal-error
		// branch, reached here by a shape json.Unmarshal rejects for
		// sourceConfig (an array where an object is expected), not by
		// malformed JSON syntax -- which would also break this fake
		// server's own encoding of the response, since json.RawMessage must
		// itself be valid JSON to marshal.
		{Name: "bad-json", Config: json.RawMessage(`[]`)},
		{Name: "no-chat-id", Config: rawConfig(t, sourceConfig{Channel: "c"})},
		{Name: "no-channel", Config: rawConfig(t, sourceConfig{ChatID: "1"})},
		{Name: "valid", Config: rawConfig(t, sourceConfig{ChatID: "1", Channel: "c"})},
	})

	a := &adapter{mgr: NewManager(srv.URL, "tok"), sourceType: "telegram",
		sources: map[string]servedSource{}, reported: map[string]string{}}
	a.refreshSources(context.Background())

	if len(a.sources) != 1 {
		t.Fatalf("sources = %v, want only the valid one served", a.sources)
	}
	if _, ok := a.sources["valid"]; !ok {
		t.Fatalf("valid source not served: %v", a.sources)
	}

	wantSub := map[string]string{
		"no-config":  "spec.config is missing",
		"bad-json":   "not valid JSON",
		"no-chat-id": "spec.config.chatId is required",
		"no-channel": "spec.config.channel is required",
	}
	for name, sub := range wantSub {
		statuses := f.statusesFor(name)
		if len(statuses) != 1 {
			t.Fatalf("%s: got %d status reports, want 1: %v", name, len(statuses), statuses)
		}
		if statuses[0]["ready"] != false {
			t.Fatalf("%s: ready = %v, want false", name, statuses[0]["ready"])
		}
		if !strings.Contains(fmt.Sprint(statuses[0]["message"]), sub) {
			t.Fatalf("%s: message = %v, want it to contain %q", name, statuses[0]["message"], sub)
		}
	}
	valid := f.statusesFor("valid")
	if len(valid) != 1 || valid[0]["ready"] != true || valid[0]["reason"] != "AdapterReady" {
		t.Fatalf("valid: statuses = %v", valid)
	}

	// A second, unchanged pass must not re-report anything -- the dedup
	// against a.reported is what keeps a steady-state install from hammering
	// the manager's status endpoint every 15s.
	a.refreshSources(context.Background())
	for name := range wantSub {
		if got := len(f.statusesFor(name)); got != 1 {
			t.Fatalf("%s: got %d reports after an unchanged pass, want still 1", name, got)
		}
	}
	if got := len(f.statusesFor("valid")); got != 1 {
		t.Fatalf("valid: got %d reports after an unchanged pass, want still 1", got)
	}
}

// TestRefreshSourcesRecoversAfterAFix: a source that was invalid and is then
// fixed must flip to reported-ready, not stay stuck on its last report.
func TestRefreshSourcesRecoversAfterAFix(t *testing.T) {
	f := &fakeSourceServer{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	f.set([]SourceInfo{{Name: "s", Config: json.RawMessage(`{}`)}})

	a := &adapter{mgr: NewManager(srv.URL, "tok"), sourceType: "telegram",
		sources: map[string]servedSource{}, reported: map[string]string{}}
	a.refreshSources(context.Background())
	if len(f.statusesFor("s")) != 1 {
		t.Fatal("want one problem report before the fix")
	}

	f.set([]SourceInfo{{Name: "s", Config: rawConfig(t, sourceConfig{ChatID: "1", Channel: "c"})}})
	a.refreshSources(context.Background())
	statuses := f.statusesFor("s")
	if len(statuses) != 2 || statuses[1]["ready"] != true {
		t.Fatalf("statuses = %v, want a second report flipping to ready", statuses)
	}
	if _, ok := a.sources["s"]; !ok {
		t.Fatal("the fixed source should now be served")
	}
}

// TestRefreshSourcesLeavesServedSourcesUntouchedOnAFailedList: the manager
// being briefly unreachable must not clear what is currently served -- a
// transient outage must not drop live chat sources.
func TestRefreshSourcesLeavesServedSourcesUntouchedOnAFailedList(t *testing.T) {
	a := &adapter{
		mgr:        NewManager("http://127.0.0.1:1", "tok"),
		sourceType: "telegram",
		sources:    map[string]servedSource{"kept": {cfg: sourceConfig{ChatID: "1", Channel: "c"}}},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())
	if _, ok := a.sources["kept"]; !ok {
		t.Fatal("a failed list call cleared the served set")
	}
}

// ---- registryLoop ----------------------------------------------------------

// TestRegistryLoopStopsOnContextCancellation: the loop must return promptly
// once its context ends rather than blocking out the 15s tick -- otherwise
// shutdown (main's SIGTERM path) would hang on this goroutine.
func TestRegistryLoopStopsOnContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SourceInfo{})
	}))
	defer srv.Close()
	a := &adapter{mgr: NewManager(srv.URL, "tok"), sourceType: "telegram",
		sources: map[string]servedSource{}, reported: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { a.registryLoop(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("registryLoop did not return after its context ended")
	}
}

// ---- handler ---------------------------------------------------------------

// TestHandlerServesHealthzAndRoutesUpdates exercises handler() itself (the
// mux wiring) over real HTTP -- every other test in this package calls
// handleUpdate directly and never went through the mux or /healthz.
func TestHandlerServesHealthzAndRoutesUpdates(t *testing.T) {
	mgrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mgrSrv.Close()

	a := &adapter{
		mgr: NewManager(mgrSrv.URL, "tok"), sourceType: "telegram",
		sources:  map[string]servedSource{"tg-chat": {cfg: sourceConfig{ChatID: "-1001", Channel: "c"}}},
		reported: map[string]string{},
	}
	srv := httptest.NewServer(a.handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Post(srv.URL+"/updates", "application/json",
		strings.NewReader(`{"update_id":1,"message":{"text":"go","chat":{"id":-1001}}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("updates status = %d, want 204", resp.StatusCode)
	}
}

// ---- handleUpdate's remaining branches -------------------------------------
//
// TestNormalizeChatSignal and friends in main_test.go cover normalize()'s
// ok=true and ok=false paths directly. handleUpdate itself was only ever
// driven with a valid, readable, originating body -- these close its own
// four branches: the body-read error, the malformed-JSON error, the !ok path
// through the real HTTP entry point (proving the manager is never called),
// and the manager's error response.

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestHandleUpdateRejectsAnUnreadableBody(t *testing.T) {
	a := &adapter{sources: map[string]servedSource{}, reported: map[string]string{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/updates", io.NopCloser(erroringReader{}))
	a.handleUpdate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleUpdateRejectsInvalidJSON(t *testing.T) {
	a := &adapter{sources: map[string]servedSource{}, reported: map[string]string{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/updates", strings.NewReader("not json"))
	a.handleUpdate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleUpdateDropsAnUnoriginatingUpdateWithoutCallingTheManager(t *testing.T) {
	called := false
	mgrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer mgrSrv.Close()
	a := &adapter{mgr: NewManager(mgrSrv.URL, "tok"), sources: map[string]servedSource{}, reported: map[string]string{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/updates", strings.NewReader(`{"update_id":1,"message":{"text":"go","chat":{"id":-9999}}}`))
	a.handleUpdate(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if called {
		t.Fatal("an update that does not originate must never reach the manager")
	}
}

func TestHandleUpdateReturnsBadGatewayWhenTheManagerRejectsInbound(t *testing.T) {
	mgrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mgrSrv.Close()
	a := &adapter{mgr: NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]servedSource{"tg-chat": {cfg: sourceConfig{ChatID: "-1001", Channel: "c"}}},
		reported: map[string]string{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/updates", strings.NewReader(`{"update_id":1,"message":{"text":"go","chat":{"id":-1001}}}`))
	a.handleUpdate(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// ---- mustEnv and main, as real subprocesses --------------------------------
//
// log.Fatalf calls os.Exit(1), and main() blocks in ListenAndServe -- neither
// can be driven in-process without killing or hanging the test binary. Both
// are exercised as a real subprocess of the compiled test binary, the
// standard os/exec "crasher" pattern, rather than mocked away.

func TestMustEnvFatalsOnAMissingVariable(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		mustEnv("SIGNAL_TELEGRAM_TEST_VAR_DOES_NOT_EXIST")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvFatalsOnAMissingVariable")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.Success() {
		t.Fatalf("process exited %v, want a non-zero exit; output: %s", err, out)
	}
	if !strings.Contains(string(out), "missing required env") {
		t.Fatalf("output = %q, want the missing-env message", out)
	}
}

// TestMainStartsServesAndShutsDownOnSIGTERM runs the real main() in a
// subprocess -- a real (deliberately unreachable) MANAGER_URL, a real
// ephemeral listen port -- then sends it an actual SIGTERM, the same signal
// Kubernetes sends on pod termination, and checks it exits cleanly. Closes
// main, handler (via ListenAndServe actually binding), and registryLoop's
// first iteration plus its context-cancellation exit -- none of which any
// other test in this package reaches, since every other test calls the
// unexported pieces directly.
func TestMainStartsServesAndShutsDownOnSIGTERM(t *testing.T) {
	if os.Getenv("BE_MAIN") == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainStartsServesAndShutsDownOnSIGTERM")
	cmd.Env = append(os.Environ(), "BE_MAIN=1",
		"MANAGER_URL=http://127.0.0.1:1", "ADAPTER_TOKEN=tok", "LISTEN_ADDR=127.0.0.1:0")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Give the server a moment to bind before asking it to stop.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("main did not exit cleanly on SIGTERM: %v; output: %s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("main did not exit within 5s of SIGTERM; output: %s", out.String())
	}
	if !strings.Contains(out.String(), "signal-telegram adapter starting") {
		t.Fatalf("output = %q, want the startup log line", out.String())
	}
}
