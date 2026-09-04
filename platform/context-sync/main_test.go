package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		envUpstream: "http://manager:8080",
		envStore:    "/data/context/conv-1",
		envPaths:    ".claude/projects/-data-workspace/**",
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	setEnv(t, baseEnv())
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.interval != 2*time.Minute {
		t.Fatalf("interval = %v, want the 2m default", c.interval)
	}
	if c.retain != 3 {
		t.Fatalf("retain = %d, want 3", c.retain)
	}
	if c.live != "/data/context" {
		t.Fatalf("live = %q, want /data/context", c.live)
	}
}

// An empty include list would persist NOTHING while looking configured — the
// exact silent failure this whole change exists to end.
func TestLoadConfigRefusesAnEmptyIncludeList(t *testing.T) {
	env := baseEnv()
	env[envPaths] = "   \n  \n"
	setEnv(t, env)
	if _, err := loadConfig(); err == nil {
		t.Fatal("an empty include list must be refused, not silently accepted")
	}
}

func TestLoadConfigRequiresItsEndpoints(t *testing.T) {
	for _, missing := range []string{envUpstream, envStore} {
		env := baseEnv()
		delete(env, missing)
		// t.Setenv cannot unset, so clear explicitly.
		t.Run(missing, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			t.Setenv(missing, "")
			if _, err := loadConfig(); err == nil {
				t.Fatalf("%s missing must be an error", missing)
			}
		})
	}
}

// Zero is a legitimate setting, not a disabled config: it means work-boundary
// checkpoints only, which is right for a low-churn backend.
func TestZeroIntervalMeansWorkBoundariesOnly(t *testing.T) {
	env := baseEnv()
	env[envInterval] = "0s"
	setEnv(t, env)
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.interval != 0 {
		t.Fatalf("interval = %v, want 0", c.interval)
	}
}

func TestLoadConfigRejectsNonsense(t *testing.T) {
	env := baseEnv()
	env[envInterval] = "soon"
	setEnv(t, env)
	if _, err := loadConfig(); err == nil {
		t.Fatal("an unparseable interval must fail loudly rather than fall back silently")
	}

	env2 := baseEnv()
	env2[envRetain] = "0"
	setEnv(t, env2)
	t.Setenv(envInterval, "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("retain must be at least 1: keeping zero generations keeps no context")
	}
}

func TestLinesIgnoresBlanks(t *testing.T) {
	got := lines("a\n\n  b  \n\n")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("lines = %q", got)
	}
}

// A skip must not update the "last wrote" clock, or the debounce would suppress
// real checkpoints after a run of skips.
func TestSkipDoesNotAdvanceTheWriteClock(t *testing.T) {
	live, store := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &syncer{
		cfg:      config{live: live, paths: []string{"**"}},
		store:    &Store{Root: store, Retain: 2},
		reporter: &reporter{},
	}
	s.checkpoint("first")
	first := s.lastAt
	if first.IsZero() {
		t.Fatal("a real checkpoint must stamp the clock")
	}
	s.checkpoint("second") // nothing changed: a skip
	if !s.lastAt.Equal(first) {
		t.Fatal("a skip must not advance the write clock")
	}
	if _, err := os.Stat(store); err != nil {
		t.Fatal(err)
	}
}

// envOr's "value present" branch was never exercised — every existing test
// left LISTEN_ADDR/CONTEXT_LIVE_DIR unset and only ever observed the default.
func TestEnvOrReturnsTheSetValue(t *testing.T) {
	env := baseEnv()
	env[envListen] = ":9999"
	setEnv(t, env)
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.listen != ":9999" {
		t.Fatalf("listen = %q, want the configured :9999", c.listen)
	}
}

// sinceLast is what tick's debounce reads. A zero lastAt (nothing has ever
// written) must report a duration too large for any debounce window to
// mistake for "recent", and a real lastAt must report roughly its own age.
func TestSinceLastReportsElapsedTime(t *testing.T) {
	s := &syncer{}
	if s.sinceLast() < 24*time.Hour {
		t.Fatal("a syncer with no checkpoint yet must report an effectively-infinite duration")
	}
	s.lastAt = time.Now().Add(-90 * time.Second)
	d := s.sinceLast()
	if d < 60*time.Second || d > 150*time.Second {
		t.Fatalf("sinceLast() = %v, want roughly 90s", d)
	}
}

// report() is the telemetry path: best-effort, and it must never be able to
// fail a checkpoint that already succeeded. Exercise the real HTTP POST,
// including the bearer header and the conversation stamp, which no existing
// test sent anywhere (every prior reporter{} in this file carries an empty
// url, which report() short-circuits on).
func TestReportPostsJSONWithAuthorizationAndConversation(t *testing.T) {
	var gotAuth string
	var got event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	r := &reporter{url: ts.URL, token: "secret-token", convo: "conv-9", c: http.DefaultClient}
	r.report(event{Kind: "context.checkpoint", Bytes: 42})

	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q, want a bearer token", gotAuth)
	}
	if got.Kind != "context.checkpoint" || got.Conversation != "conv-9" || got.Bytes != 42 {
		t.Fatalf("posted event = %+v", got)
	}
}

// A nil reporter and a reporter with no url configured are both the same
// "telemetry is off" state, and neither may panic or attempt a request.
func TestReportIsANoopWithNoReporterOrURL(t *testing.T) {
	var r *reporter
	r.report(event{Kind: "x"})
	(&reporter{}).report(event{Kind: "x"})
}

// A request that cannot even be constructed (a malformed target URL) must be
// swallowed exactly like a network failure — telemetry never propagates.
func TestReportSwallowsAnUnconstructableRequest(t *testing.T) {
	r := &reporter{url: "http://x/%zz", c: http.DefaultClient}
	r.report(event{Kind: "x"})
}

// An unreachable upstream must be logged and swallowed, never panic and never
// escape as an error the caller has to handle.
func TestReportSwallowsAnUnreachableUpstream(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	r := &reporter{url: "http://127.0.0.1:1", c: &http.Client{Timeout: 2 * time.Second}}
	r.report(event{Kind: "context.failed"})

	if !strings.Contains(buf.String(), "reporting context.failed failed") {
		t.Fatalf("want the failure logged, got %q", buf.String())
	}
}

// A Scan failure (not a missing path — a real error) must be reported as
// context.failed, never silently dropped.
func TestCheckpointReportsFailureWhenScanErrors(t *testing.T) {
	live, store := t.TempDir(), t.TempDir()
	blocked := filepath.Join(live, "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, blocked, "a.jsonl", "x")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	var got event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer ts.Close()

	s := &syncer{
		cfg:      config{live: live, paths: []string{"blocked/**"}},
		store:    &Store{Root: store, Retain: 2},
		reporter: &reporter{url: ts.URL, c: http.DefaultClient},
	}
	s.checkpoint("test")

	if got.Kind != "context.failed" || got.Reason != "test" || got.Error == "" {
		t.Fatalf("want a reported scan failure, got %+v", got)
	}
}

// A Store.Checkpoint failure (distinct call site from the Scan failure above)
// must also be reported as context.failed.
func TestCheckpointReportsFailureWhenStoreCheckpointErrors(t *testing.T) {
	live := t.TempDir()
	write(t, live, "a.jsonl", "x")

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	root := filepath.Join(parent, "store") // never created: parent forbids it

	var got event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer ts.Close()

	s := &syncer{
		cfg:      config{live: live, paths: []string{"**"}},
		store:    &Store{Root: root, Retain: 2},
		reporter: &reporter{url: ts.URL, c: http.DefaultClient},
	}
	s.checkpoint("test")

	if got.Kind != "context.failed" || got.Reason != "test" {
		t.Fatalf("want a reported store failure, got %+v", got)
	}
}

// restore() is what runs before the runtime's first work unit. It must seed
// s.last from what it restored (so the following checkpoint does not re-copy
// an unchanged tree) and report what it found.
func TestSyncerRestoreSeedsManifestAndReports(t *testing.T) {
	srcLive := t.TempDir()
	write(t, srcLive, "a.jsonl", "hi")
	store := t.TempDir()
	st := &Store{Root: store, Retain: 2}
	m, err := Scan(srcLive, []string{"**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Checkpoint(srcLive, m, true, time.Now()); err != nil {
		t.Fatal(err)
	}

	var got event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer ts.Close()

	live := t.TempDir()
	s := &syncer{
		cfg:      config{live: live, paths: []string{"**"}},
		store:    st,
		reporter: &reporter{url: ts.URL, c: http.DefaultClient},
	}
	if err := s.restore(); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "context.restore" || !got.Found || got.Files != 1 {
		t.Fatalf("reported restore = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(live, "a.jsonl")); err != nil {
		t.Fatalf("restore did not populate the live tree: %v", err)
	}
	// The manifest must be seeded: an immediate checkpoint against the
	// untouched restored tree must see it as unchanged and skip.
	s.checkpoint("first")
	if !s.lastAt.IsZero() {
		t.Fatal("a checkpoint against an unchanged, freshly-restored tree must skip, not write")
	}
}

// A failed restore (the durable store cannot be read) must be reported as
// context.failed and returned to the caller — the proxy refuses to hand out
// work on this error.
func TestSyncerRestoreReportsFailure(t *testing.T) {
	store := t.TempDir()
	link := filepath.Join(store, currentLink)
	if err := os.Symlink(link, link); err != nil {
		t.Fatal(err)
	}

	var got event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer ts.Close()

	s := &syncer{
		cfg:      config{live: t.TempDir(), paths: []string{"**"}},
		store:    &Store{Root: store, Retain: 2},
		reporter: &reporter{url: ts.URL, c: http.DefaultClient},
	}
	if err := s.restore(); err == nil {
		t.Fatal("a broken store must fail restore, not report success against nothing")
	}
	if got.Kind != "context.failed" || got.Reason != "restore" {
		t.Fatalf("reported event = %+v", got)
	}
}

// tick is the periodic checkpoint loop. This exercises the ticker firing and
// actually invoking a checkpoint — previously 0% covered — leaving only the
// timing-dependent debounce skip (untestable without a flaky race) uncovered.
func TestTickRunsCheckpointsOnSchedule(t *testing.T) {
	live, store := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "x")
	s := &syncer{
		cfg:      config{live: live, paths: []string{"**"}},
		store:    &Store{Root: store, Retain: 2},
		reporter: &reporter{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go s.tick(ctx, 20*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for s.sinceLast() > 24*time.Hour {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("tick never triggered a checkpoint")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
}
