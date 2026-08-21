package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	mu      sync.Mutex
	sources []SourceInfo
	state   map[string]string
	posted  []Signal
	status  []statusReport
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
		{"count rose during the window", &healthSnapshot{
			records: map[string]logRecord{key: {Count: 5}},
			entries: map[string][]string{"hue": {"loaded"}},
		}, verdictUnhealthy},
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
			if got := a.health(ref); got != tc.want {
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

var _ = fmt.Sprintf
