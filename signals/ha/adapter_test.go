package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- the manager double -----------------------------------------------------

type statusReport struct {
	Source  string
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type fakeManager struct {
	srv *httptest.Server

	mu          sync.Mutex
	sources     []SourceInfo
	state       map[string]string
	posted      []Signal
	status      []statusReport
	failInbound bool
}

// SetFailInbound makes /signal/inbound answer as an unreachable manager would,
// so a test can exercise the post-failure and recovery reporting without a
// real outage.
func (m *fakeManager) SetFailInbound(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failInbound = v
}

func newFakeManager(t *testing.T, sources ...SourceInfo) *fakeManager {
	t.Helper()
	m := &fakeManager{sources: sources, state: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/signal/sources", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(m.sources)
	})
	mux.HandleFunc("/signal/inbound", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		fail := m.failInbound
		m.mu.Unlock()
		if fail {
			http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Source  string   `json:"source"`
			Signals []Signal `json:"signals"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m.mu.Lock()
		m.posted = append(m.posted, in.Signals...)
		m.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("/signal/state/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/signal/state/")
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"value": m.state[key]})
		case http.MethodPut:
			var in struct {
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			m.state[key] = in.Value
			w.WriteHeader(204)
		}
	})
	mux.HandleFunc("/signal/sources/", func(w http.ResponseWriter, r *http.Request) {
		// .../<name>/status
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/signal/sources/"), "/")
		var rep statusReport
		_ = json.NewDecoder(r.Body).Decode(&rep)
		rep.Source = parts[0]
		m.mu.Lock()
		m.status = append(m.status, rep)
		m.mu.Unlock()
		w.WriteHeader(204)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func (m *fakeManager) Posted() []Signal {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Signal(nil), m.posted...)
}

func (m *fakeManager) Status() []statusReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]statusReport(nil), m.status...)
}

func (m *fakeManager) State(source, key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state[source+"/"+key]
}

func (m *fakeManager) SetState(source, key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state[source+"/"+key] = value
}

// ---- the adapter under test -------------------------------------------------

func newTestAdapter(m *fakeManager) *adapter {
	a := &adapter{
		mgr:      NewManager(m.srv.URL, "adapter-token"),
		name:     "home-assistant",
		self:     newSelfExcluder(),
		sources:  map[string]*servedSource{},
		reported: map[string]string{},
		sessions: map[string]*haSession{},
		inhibit:  newInhibitor(),
		cap:      newEmitCap(defaultEmitPerMin),
	}
	return a
}

func sourceInfo(name, endpoint string, extra map[string]any) SourceInfo {
	cfg := map[string]any{"endpoint": endpoint}
	for k, v := range extra {
		cfg[k] = v
	}
	raw, _ := json.Marshal(cfg)
	return SourceInfo{
		Name:                name,
		Config:              raw,
		CredentialEnvPrefix: "AGENTOPS_CRED_HA_LOGS_",
	}
}

func record(logger, message string, at time.Time, count int) logRecord {
	return logRecord{
		Name:      logger,
		Message:   []string{message},
		Level:     "ERROR",
		Source:    []json.RawMessage{json.RawMessage(`"` + logger + `.py"`), json.RawMessage(`10`)},
		Timestamp: float64(at.UnixNano()) / 1e9,
		Count:     count,
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func runAdapter(t *testing.T, a *adapter) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		a.stopAll()
	})
	a.pending = newPendingQueue(a.health, func(source string, sigs []Signal) { a.post(ctx, source, sigs) })
	a.refreshSources(ctx)
	return ctx
}

// ---- tests ------------------------------------------------------------------

func TestLiveRecordBecomesASignal(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	ha := newFakeHA(t, "secret")
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, nil))
	a := newTestAdapter(fm)
	runAdapter(t, a)

	<-ha.subscribed
	ha.PushLogEvent(record("homeassistant.components.zwave_js", "Failed to set temperature", time.Now(), 1))

	waitFor(t, "the signal to be posted", func() bool { return len(fm.Posted()) == 1 })
	sig := fm.Posted()[0]
	if sig.Labels["integration"] != "zwave_js" || sig.Kind != "alert" {
		t.Fatalf("unexpected signal %+v", sig)
	}
	waitFor(t, "the cursor to be persisted", func() bool { return fm.State("ha-logs", "last-record") != "" })
}

// A restart resumes where it stopped: what predates the cursor is not replayed,
// and what arrived while the adapter was down still gets reported.
func TestCursorResume(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	now := time.Now().UTC().Truncate(time.Second)
	ha := newFakeHA(t, "secret")
	ha.SetRecords(
		record("homeassistant.components.hue", "old and already reported", now.Add(-time.Hour), 1),
		record("homeassistant.components.mqtt", "logged while we were down", now.Add(-time.Minute), 1),
	)
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, nil))
	fm.SetState("ha-logs", "last-record", now.Add(-30*time.Minute).Format(time.RFC3339))

	a := newTestAdapter(fm)
	runAdapter(t, a)

	waitFor(t, "the backfilled signal", func() bool { return len(fm.Posted()) >= 1 })
	time.Sleep(200 * time.Millisecond) // let a wrong second post show up
	posted := fm.Posted()
	if len(posted) != 1 {
		t.Fatalf("expected exactly the record newer than the cursor, got %d: %+v", len(posted), posted)
	}
	if posted[0].Labels["integration"] != "mqtt" {
		t.Fatalf("replayed the wrong record: %+v", posted[0])
	}
}

// A position the upstream no longer holds must cause a full re-read, never a
// stall: the alternative is a lane that goes silent forever and looks healthy.
func TestStaleCursorRecovers(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	now := time.Now().UTC().Truncate(time.Second)
	ha := newFakeHA(t, "secret")
	ha.SetRecords(
		record("homeassistant.components.hue", "one", now.Add(-2*time.Minute), 1),
		record("homeassistant.components.mqtt", "two", now.Add(-time.Minute), 1),
	)
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, nil))
	// Far ahead of anything Home Assistant still holds — the log was cleared,
	// or this cursor belongs to another instance.
	fm.SetState("ha-logs", "last-record", now.Add(time.Hour).Format(time.RFC3339))

	a := newTestAdapter(fm)
	runAdapter(t, a)

	waitFor(t, "the whole log to be re-read", func() bool { return len(fm.Posted()) == 2 })
}

// The credential is projected by the reconciler, and its absence is a
// configuration fact to report — not something to work around by connecting
// anonymously, which would read as an unreachable host.
func TestMissingCredentialIsReported(t *testing.T) {
	ha := newFakeHA(t, "secret")
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, nil))
	a := newTestAdapter(fm)
	runAdapter(t, a)

	waitFor(t, "the MissingCredential condition", func() bool {
		for _, s := range fm.Status() {
			if s.Reason == "MissingCredential" && !s.Ready {
				return true
			}
		}
		return false
	})
	if len(a.sources) != 0 {
		t.Fatal("a source with no credential must not be served")
	}
}

func TestInvalidConfigIsReported(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	raw, _ := json.Marshal(map[string]any{"levels": []string{"ERROR"}}) // no endpoint
	fm := newFakeManager(t, SourceInfo{
		Name: "ha-logs", Config: raw, CredentialEnvPrefix: "AGENTOPS_CRED_HA_LOGS_",
	})
	a := newTestAdapter(fm)
	runAdapter(t, a)

	waitFor(t, "the InvalidConfig condition", func() bool {
		for _, s := range fm.Status() {
			if s.Reason == "InvalidConfig" && strings.Contains(s.Message, "endpoint") {
				return true
			}
		}
		return false
	})
}

// The loop breaker, end to end: a record from a surface agent-ops calls Home
// Assistant through must not reach the manager, whatever the rules say.
func TestAgentSurfaceRecordNeverPosts(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	ha := newFakeHA(t, "secret")
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, nil))
	a := newTestAdapter(fm)
	runAdapter(t, a)

	<-ha.subscribed
	ha.PushLogEvent(record("homeassistant.components.mcp_server", "Error handling request", time.Now(), 1))
	ha.PushLogEvent(record("homeassistant.components.hue", "real problem", time.Now(), 1))

	waitFor(t, "the ordinary record", func() bool { return len(fm.Posted()) >= 1 })
	time.Sleep(200 * time.Millisecond)
	for _, sig := range fm.Posted() {
		if strings.Contains(sig.Labels["logger"], "mcp_server") {
			t.Fatalf("a record from an agent surface reached the manager: %+v", sig)
		}
	}
}

// A source whose endpoint is unreachable reports it and keeps trying, rather
// than dropping out of the served set.
func TestUnreachableEndpointIsReported(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	fm := newFakeManager(t, sourceInfo("ha-logs", "http://127.0.0.1:1", nil))
	a := newTestAdapter(fm)
	runAdapter(t, a)

	waitFor(t, "the Unreachable condition", func() bool {
		for _, s := range fm.Status() {
			if s.Reason == "Unreachable" {
				return true
			}
		}
		return false
	})
}

// ---- the verification ladder ------------------------------------------------

func adapterWithSnapshot(snap *healthSnapshot) *adapter {
	a := &adapter{sources: map[string]*servedSource{
		"ha-logs": {snapshot: snap},
	}}
	return a
}

func TestHealthLadder(t *testing.T) {
	ref := recordRef{source: "ha-logs", integration: "hue", logger: "homeassistant.components.hue",
		location: "light.py:88", countAtOpen: 3}
	key := ref.logger + "@" + ref.location
	// The closing part of the window began at `since`; a record's Timestamp is
	// its LATEST occurrence, in epoch seconds.
	since := time.Date(2026, 8, 21, 12, 2, 0, 0, time.UTC)
	before, after := float64(since.Add(-time.Minute).Unix()), float64(since.Add(30*time.Second).Unix())

	cases := []struct {
		name string
		snap *healthSnapshot
		want verdict
	}{
		{"no snapshot at all", nil, verdictUnknown},
		{"config entry still failing", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 3}},
			entries: map[string][]string{"hue": {"setup_retry"}},
		}, verdictUnhealthy},
		{"count rose, still rising at the close", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 5, Timestamp: after}},
			entries: map[string][]string{"hue": {"loaded"}},
		}, verdictUnhealthy},
		// It recurred, and then it stopped: a blip that healed. The loaded
		// entry says recovered.
		{"count rose early, quiet at the close, loaded", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 5, Timestamp: before}},
			entries: map[string][]string{"hue": {"loaded"}},
		}, verdictHealthy},
		// Same blip, no predicate: the log alone cannot say, so rung 2 decides
		// from the arrival timeline.
		{"count rose early, quiet at the close, no predicate", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 5, Timestamp: before}},
			entries: map[string][]string{},
		}, verdictUnknown},
		{"loaded and quiet", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 3}},
			entries: map[string][]string{"hue": {"loaded"}},
		}, verdictHealthy},
		{"no predicate, quiet", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 3}},
			entries: map[string][]string{},
		}, verdictUnknown},
		{"record gone, integration loaded", &healthSnapshot{
			records: map[string]logRecord{},
			entries: map[string][]string{"hue": {"loaded"}},
		}, verdictGone},
		{"record gone, no predicate", &healthSnapshot{
			records: map[string]logRecord{},
			entries: map[string][]string{},
		}, verdictUnknown},
		// A disabled integration is a choice, not an incident.
		{"not_loaded is not a failure", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 3}},
			entries: map[string][]string{"hue": {"not_loaded"}},
		}, verdictHealthy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := adapterWithSnapshot(tc.snap)
			if got := a.health(ref, since); got != tc.want {
				t.Fatalf("health = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCredentialTokenErrors(t *testing.T) {
	if _, err := credentialToken(SourceInfo{Name: "ha-logs"}); err == nil ||
		!strings.Contains(err.Error(), "credentialsSecretRef") {
		t.Fatalf("expected an error naming credentialsSecretRef, got %v", err)
	}
	if _, err := credentialToken(SourceInfo{Name: "ha-logs", CredentialEnvPrefix: "NOPE_"}); err == nil ||
		!strings.Contains(err.Error(), "NOPE_token") {
		t.Fatalf("expected an error naming the expected env var, got %v", err)
	}
	t.Setenv("NOPE_TOKEN", "x")
	if got, err := credentialToken(SourceInfo{CredentialEnvPrefix: "NOPE_"}); err != nil || got != "x" {
		t.Fatalf("uppercase TOKEN should be accepted, got %q %v", got, err)
	}
}

// ---- the dwell verification ladder, wired end to end ------------------------

// Everywhere else, health() is tested against a hand-built healthSnapshot
// (TestHealthLadder). This test instead runs the REAL wiring the reconciler
// exercises in production: a dwell rule defers a matched record into the
// pending queue, runDwellFlusher's own ticker refreshes the snapshot from a
// real Home Assistant session (system_log/list + config_entries/get), and the
// flush decides it — none of runDwellFlusher, refreshSnapshots or
// refreshSnapshot (all 0%/81.2% in the coverage profile before this test) were
// reached by any other test.
func TestDwellVerificationLadderEndToEnd(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	ha := newFakeHA(t, "secret")
	// A config entry stuck in setup_retry is rung 1 of the ladder: still
	// broken, whatever the log itself says.
	ha.SetEntries(configEntry{Domain: "hue", State: "setup_retry"})

	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, map[string]any{
		"rules": []map[string]any{{
			"matchers": []string{`integration="hue"`},
			"for":      "150ms",
		}},
	}))
	a := newTestAdapter(fm)
	ctx := runAdapter(t, a)
	go a.runDwellFlusher(ctx)

	<-ha.subscribed
	ha.PushLogEvent(record("homeassistant.components.hue", "temporary failure", time.Now(), 1))

	// dwellTick is 5s in production code, so the flusher's second tick is what
	// actually decides this entry; give it generous room.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && len(fm.Posted()) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	posted := fm.Posted()
	if len(posted) != 1 {
		t.Fatalf("expected exactly one dwell-verified signal, got %d: %+v", len(posted), posted)
	}
	if !strings.Contains(posted[0].Payload, "failing for") {
		t.Fatalf("expected the dwell evidence in the payload, got %q", posted[0].Payload)
	}
}

// A config-entry setup failure that never recurs is exactly the incident this
// change fixes: before it, the record's mislabeled identity (the logger name,
// never the domain) meant rung 1 could never apply, and a single occurrence
// with no second log line was dropped as quiet by rung 2's recurrence check
// (pending.go's stillRecurring) — even though the entry was genuinely still
// broken. With the domain resolved correctly, rung 1's config-entry predicate
// confirms it and the signal is emitted despite never recurring.
func TestDwellVerificationLadderConfigEntryNeverRecurring(t *testing.T) {
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	ha := newFakeHA(t, "secret")
	ha.SetEntries(configEntry{Domain: "tuya", State: "setup_retry"})

	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, map[string]any{
		"rules": []map[string]any{{
			"matchers": []string{`integration="tuya"`},
			"for":      "150ms",
		}},
	}))
	a := newTestAdapter(fm)
	ctx := runAdapter(t, a)
	go a.runDwellFlusher(ctx)

	<-ha.subscribed
	ha.PushLogEvent(record(configEntriesLogger, "Error setting up entry someone@example.com for tuya", time.Now(), 1))

	// A single record, never repeated — the exact shape rung 2 alone cannot
	// confirm. dwellTick is 5s in production code, so the flusher's second
	// tick is what decides this entry; give it generous room.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && len(fm.Posted()) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	posted := fm.Posted()
	if len(posted) != 1 {
		t.Fatalf("expected exactly one dwell-verified signal despite no recurrence, got %d: %+v", len(posted), posted)
	}
	if posted[0].Labels["integration"] != "tuya" {
		t.Fatalf("integration label = %q, want %q", posted[0].Labels["integration"], "tuya")
	}
}

// ---- the emit cap and post failure/recovery reporting ------------------------

// post() is the single exit from the adapter, and reportClipping/
// reportPostFailure/reportPostRecovered were 25%/0%/66.7% — none of their
// "already reported, do nothing again" branches, nor the len(allowed)==0 exit
// from post() itself, were exercised.
func TestPostReportsClippingOnceAndTakesTheZeroAllowedExit(t *testing.T) {
	fm := newFakeManager(t)
	a := newTestAdapter(fm)
	a.cap = newEmitCap(1)
	ctx := context.Background()

	sigs := []Signal{{Fingerprint: "a"}, {Fingerprint: "b"}}
	if !a.post(ctx, "src", sigs) {
		t.Fatal("expected the signal within the cap to post")
	}
	if len(fm.Posted()) != 1 {
		t.Fatalf("expected exactly the one signal under the cap, got %d", len(fm.Posted()))
	}
	clipReports := func() int {
		n := 0
		for _, s := range fm.Status() {
			if s.Reason == "EmitCapReached" {
				n++
			}
		}
		return n
	}
	if clipReports() != 1 {
		t.Fatalf("expected exactly one EmitCapReached report, got %d", clipReports())
	}

	// The window is still exhausted: this call finds zero room, so post()
	// takes its len(allowed)==0 exit and never reaches mgr.Inbound.
	if a.post(ctx, "src", sigs) {
		t.Fatal("expected a fully-clipped batch to report false")
	}
	if len(fm.Posted()) != 1 {
		t.Fatal("the fully-clipped batch must not have reached the manager")
	}
	if clipReports() != 2 {
		t.Fatalf("expected the clip count CHANGE to be reported again, got %d reports", clipReports())
	}
}

// A manager that rejects a post is reported once, not once per failed
// attempt, and its recovery is reported once the next post succeeds.
func TestPostReportsFailureOnceThenRecovers(t *testing.T) {
	fm := newFakeManager(t)
	fm.SetFailInbound(true)
	a := newTestAdapter(fm)
	ctx := context.Background()

	if a.post(ctx, "src", []Signal{{Fingerprint: "x"}}) {
		t.Fatal("expected the post to fail while the manager is down")
	}
	failReports := func() int {
		n := 0
		for _, s := range fm.Status() {
			if s.Reason == "PostFailed" {
				n++
			}
		}
		return n
	}
	if failReports() != 1 {
		t.Fatalf("expected exactly one PostFailed report, got %d", failReports())
	}

	// Still down: reportPostFailure's LoadOrStore guard must keep this quiet.
	if a.post(ctx, "src", []Signal{{Fingerprint: "x"}}) {
		t.Fatal("expected the post to keep failing")
	}
	if failReports() != 1 {
		t.Fatalf("a repeated failure while already failing must not re-report, got %d", failReports())
	}

	fm.SetFailInbound(false)
	if !a.post(ctx, "src", []Signal{{Fingerprint: "x"}}) {
		t.Fatal("expected the post to succeed once the manager recovers")
	}
	recovered := false
	for _, s := range fm.Status() {
		if s.Reason == "AdapterReady" && s.Message == "posting to the manager again" {
			recovered = true
		}
	}
	if !recovered {
		t.Fatal("expected the recovery to be reported")
	}
}

// ---- small env-reading helpers ----------------------------------------------

func TestMustEnvReturnsTheSetValue(t *testing.T) {
	t.Setenv("HA_TEST_MUST_ENV", "configured")
	if got := mustEnv("HA_TEST_MUST_ENV"); got != "configured" {
		t.Fatalf("mustEnv = %q, want %q", got, "configured")
	}
}

func TestEnvIntFallsBackOnUnsetOrUnparsable(t *testing.T) {
	os.Unsetenv("HA_TEST_ENV_INT")
	if got := envInt("HA_TEST_ENV_INT", 42); got != 42 {
		t.Fatalf("unset: envInt = %d, want the fallback 42", got)
	}
	t.Setenv("HA_TEST_ENV_INT", "not-a-number")
	if got := envInt("HA_TEST_ENV_INT", 42); got != 42 {
		t.Fatalf("unparsable: envInt = %d, want the fallback 42", got)
	}
	t.Setenv("HA_TEST_ENV_INT", "7")
	if got := envInt("HA_TEST_ENV_INT", 42); got != 7 {
		t.Fatalf("envInt = %d, want the parsed value 7", got)
	}
}

var _ = fmt.Sprintf
