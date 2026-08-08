package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- fakes -------------------------------------------------------------------

// fakeManager stands in for the operator's /signal/* contract surface.
type fakeManager struct {
	mu       sync.Mutex
	sources  []SourceInfo
	state    map[string]string
	posted   map[string][]Signal
	statuses []reportedStatus
}

type reportedStatus struct {
	Source  string
	Ready   bool
	Reason  string
	Message string
}

func newFakeManager(sources ...SourceInfo) *fakeManager {
	return &fakeManager{sources: sources, state: map[string]string{}, posted: map[string][]Signal{}}
}

func (f *fakeManager) start(t *testing.T) *Manager {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /signal/sources", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.sources)
	})
	mux.HandleFunc("POST /signal/inbound", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Source  string   `json:"source"`
			Signals []Signal `json:"signals"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.posted[in.Source] = append(f.posted[in.Source], in.Signals...)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": len(in.Signals)})
	})
	mux.HandleFunc("GET /signal/state/{source}/{key}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"value": f.state[r.PathValue("source")+"/"+r.PathValue("key")]})
	})
	mux.HandleFunc("PUT /signal/state/{source}/{key}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Value string `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.state[r.PathValue("source")+"/"+r.PathValue("key")] = in.Value
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /signal/sources/{name}/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Ready   bool   `json:"ready"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.statuses = append(f.statuses, reportedStatus{r.PathValue("name"), in.Ready, in.Reason, in.Message})
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewManager(srv.URL, "test-token")
}

func (f *fakeManager) signalsFor(source string) []Signal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Signal(nil), f.posted[source]...)
}

func (f *fakeManager) allStatuses() []reportedStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]reportedStatus(nil), f.statuses...)
}

// testKube points a Kube at a fake API server with a real token file, so the
// token re-read path is exercised rather than stubbed.
func testKube(t *testing.T, handler http.Handler) *Kube {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("sa-token-v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Kube{BaseURL: srv.URL, HTTP: srv.Client(), TokenPath: tokenPath}
}

func listJSON(rv string, events ...Event) string {
	b, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"resourceVersion": rv},
		"items":    events,
	})
	return string(b)
}

// ---- kube client -------------------------------------------------------------

func TestListEventsSendsBearerAndParses(t *testing.T) {
	var gotAuth, gotPath string
	e := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(listJSON("42", e)))
	}))
	events, rv, err := k.ListEvents(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sa-token-v1" {
		t.Fatalf("SA token must be sent: %q", gotAuth)
	}
	if gotPath != "/api/v1/events" {
		t.Fatalf("cluster-wide scope path: %q", gotPath)
	}
	if rv != "42" || len(events) != 1 || events[0].Reason != "BackOff" {
		t.Fatalf("list: rv=%q events=%+v", rv, events)
	}

	// namespaced scope hits the namespaced collection
	k2 := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(listJSON("1")))
	}))
	if _, _, err := k2.ListEvents(context.Background(), "prod"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/namespaces/prod/events" {
		t.Fatalf("namespaced scope path: %q", gotPath)
	}
}

// A rotated projected token must be picked up without a restart.
func TestBearerRefreshesRotatedToken(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	first, err := k.bearer()
	if err != nil || first != "sa-token-v1" {
		t.Fatalf("initial token: %q %v", first, err)
	}
	if err := os.WriteFile(k.TokenPath, []byte("sa-token-v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, _ := k.bearer(); again != "sa-token-v1" {
		t.Fatalf("token must be cached within its TTL, got %q", again)
	}
	k.mu.Lock()
	k.tokenLoaded = time.Now().Add(-2 * tokenMaxAge) // age the cache
	k.mu.Unlock()
	rotated, err := k.bearer()
	if err != nil || rotated != "sa-token-v2" {
		t.Fatalf("rotated token must be re-read: %q %v", rotated, err)
	}
}

func TestWatchStreamsAddedAndModified(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			t.Errorf("expected a watch request, got %q", r.URL.RawQuery)
		}
		if rv := r.URL.Query().Get("resourceVersion"); rv != "42" {
			t.Errorf("watch must resume from the list rv, got %q", rv)
		}
		enc := json.NewEncoder(w)
		for _, frame := range []struct {
			Type   string `json:"type"`
			Object Event  `json:"object"`
		}{
			{"ADDED", evt("Warning", "prod", "Pod", "a", "BackOff")},
			{"DELETED", evt("Warning", "prod", "Pod", "b", "Killing")},
			{"MODIFIED", evt("Warning", "prod", "Pod", "c", "Unhealthy")},
		} {
			_ = enc.Encode(frame)
		}
	}))
	var seen []string
	err := k.WatchEvents(context.Background(), "", "42", func(e Event) {
		seen = append(seen, e.InvolvedObject.Name)
	})
	if err != nil {
		t.Fatal(err)
	}
	// DELETED is an event object aging out, not a new problem
	if strings.Join(seen, ",") != "a,c" {
		t.Fatalf("want ADDED+MODIFIED only, got %v", seen)
	}
}

func TestWatchExpiryIsDistinguishable(t *testing.T) {
	// as an ERROR frame mid-stream
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"type":"ERROR","object":{"code":410,"reason":"Expired"}}` + "\n"))
	}))
	if err := k.WatchEvents(context.Background(), "", "1", func(Event) {}); !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("410 ERROR frame must report ErrWatchExpired, got %v", err)
	}

	// as an HTTP status on the watch request itself
	k2 := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"code":410,"reason":"Expired"}`))
	}))
	if err := k2.WatchEvents(context.Background(), "", "1", func(Event) {}); !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("410 response must report ErrWatchExpired, got %v", err)
	}

	// a real failure must NOT masquerade as expiry — that would hide a lost grant
	k3 := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"reason":"Forbidden"}`))
	}))
	err := k3.WatchEvents(context.Background(), "", "1", func(Event) {})
	if err == nil || errors.Is(err, ErrWatchExpired) {
		t.Fatalf("403 must surface as a real error, got %v", err)
	}
}

// After an expiry the loop relists and keeps emitting — the recovery path that
// otherwise silently stops event flow forever.
func TestWatchScopeRelistsAfterExpiry(t *testing.T) {
	// atomic: the fake's handler runs on the httptest server's goroutines while
	// the assertions below read from the test goroutine
	var calls atomic.Int32
	mgr := newFakeManager()
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") != "1" {
			_, _ = w.Write([]byte(listJSON("7")))
			return
		}
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"type":"ERROR","object":{"code":410,"reason":"Expired"}}` + "\n"))
			return
		}
		_, _ = w.Write([]byte(`{"type":"ADDED","object":` + mustJSON(evt("Warning", "prod", "Pod", "late", "BackOff")) + "}\n"))
	}))
	a := newTestAdapter(mgr.start(t), k, "recover", &filter{severities: map[string]bool{"Warning": true}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.watchScope(ctx, ""); close(done) }()
	waitFor(t, func() bool { return len(mgr.signalsFor("recover")) > 0 })
	cancel()
	<-done

	if calls.Load() < 2 {
		t.Fatalf("expiry must trigger a relist and re-watch, watch calls=%d", calls.Load())
	}
	if got := mgr.signalsFor("recover"); got[0].Labels["name"] != "late" {
		t.Fatalf("post-expiry event must still be emitted: %+v", got)
	}
}

// ---- adapter behavior --------------------------------------------------------

func newTestAdapter(m *Manager, k *Kube, source string, f *filter) *adapter {
	return &adapter{
		mgr: m, kube: k, name: "k8s-events",
		sources:  map[string]*servedSource{source: {filter: f}},
		reported: map[string]string{},
		watchers: map[string]context.CancelFunc{},
	}
}

// A restart must not replay the cluster's event backlog into new conversations.
func TestCursorSkipsAlreadySeenEventsOnList(t *testing.T) {
	mgr := newFakeManager()
	m := mgr.start(t)
	f := mustFilter(t, `{}`)
	a := newTestAdapter(m, nil, "src", f)
	a.sources["src"].cursor = mustTime(t, "2026-08-08T12:00:00Z")

	old := evt("Warning", "prod", "Pod", "old", "BackOff")
	old.LastTimestamp = "2026-08-08T11:00:00Z"
	same := evt("Warning", "prod", "Pod", "same", "BackOff")
	same.LastTimestamp = "2026-08-08T12:00:00Z"
	fresh := evt("Warning", "prod", "Pod", "fresh", "BackOff")
	fresh.LastTimestamp = "2026-08-08T13:00:00Z"

	a.deliver(context.Background(), []Event{old, same, fresh}, true)

	got := mgr.signalsFor("src")
	if len(got) != 1 || got[0].Labels["name"] != "fresh" {
		t.Fatalf("only events after the cursor may be emitted on a list pass: %+v", got)
	}
	if a.sources["src"].cursor.Format(time.RFC3339) != "2026-08-08T13:00:00Z" {
		t.Fatalf("cursor must advance to the newest emitted event: %v", a.sources["src"].cursor)
	}
	if mgr.state["src/"+cursorKey] != "2026-08-08T13:00:00Z" {
		t.Fatalf("cursor must be persisted through the contract: %q", mgr.state["src/"+cursorKey])
	}
}

// During a live watch every frame is news; applying the cursor there would drop
// events that share a timestamp with the last one seen.
func TestWatchPassIgnoresCursor(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	a.sources["src"].cursor = mustTime(t, "2026-08-08T12:00:00Z")

	old := evt("Warning", "prod", "Pod", "old", "BackOff")
	old.LastTimestamp = "2026-08-08T11:00:00Z"
	a.deliver(context.Background(), []Event{old}, false)

	if len(mgr.signalsFor("src")) != 1 {
		t.Fatal("watch-delivered events must not be filtered by the cursor")
	}
}

// An event with no parsable timestamp must still reach the agent rather than
// being silently swallowed by a cursor comparison against the zero time.
func TestUndatedEventIsStillEmitted(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	a.sources["src"].cursor = mustTime(t, "2026-08-08T12:00:00Z")

	a.deliver(context.Background(), []Event{evt("Warning", "prod", "Pod", "undated", "BackOff")}, true)
	if len(mgr.signalsFor("src")) != 1 {
		t.Fatal("an undated event must not be dropped by the cursor check")
	}
}

// A failed post must not advance the cursor, or the events would be lost.
func TestFailedPostLeavesCursorAlone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signal/inbound", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	a := newTestAdapter(NewManager(srv.URL, "t"), nil, "src", mustFilter(t, `{}`))
	before := mustTime(t, "2026-08-08T12:00:00Z")
	a.sources["src"].cursor = before

	fresh := evt("Warning", "prod", "Pod", "fresh", "BackOff")
	fresh.LastTimestamp = "2026-08-08T13:00:00Z"
	a.deliver(context.Background(), []Event{fresh}, true)

	if !a.sources["src"].cursor.Equal(before) {
		t.Fatalf("cursor advanced despite a failed post — events would be lost: %v", a.sources["src"].cursor)
	}
}

// One bad source must not take the others down with it.
func TestInvalidConfigReportsNotReadyAndOthersKeepServing(t *testing.T) {
	mgr := newFakeManager(
		SourceInfo{Name: "broken", Config: json.RawMessage(`{"severities":["Critical"]}`)},
		SourceInfo{Name: "healthy", Config: json.RawMessage(`{"severities":["Warning"]}`)},
	)
	a := newTestAdapter(mgr.start(t), nil, "unused", mustFilter(t, `{}`))
	a.sources = map[string]*servedSource{}

	a.refreshSources(context.Background())

	if _, served := a.sources["broken"]; served {
		t.Fatal("a source with invalid config must not be served")
	}
	if _, served := a.sources["healthy"]; !served {
		t.Fatal("a valid source must keep being served alongside a broken one")
	}
	waitFor(t, func() bool { return len(mgr.allStatuses()) >= 2 })

	var sawInvalid, sawReady bool
	for _, s := range mgr.allStatuses() {
		if s.Source == "broken" && !s.Ready && s.Reason == "InvalidConfig" && strings.Contains(s.Message, "Critical") {
			sawInvalid = true
		}
		if s.Source == "healthy" && s.Ready {
			sawReady = true
		}
	}
	if !sawInvalid {
		t.Fatalf("broken source must report InvalidConfig naming the value: %+v", mgr.allStatuses())
	}
	if !sawReady {
		t.Fatalf("healthy source must report Ready: %+v", mgr.allStatuses())
	}
}

// Editing a source's config must not replay its history.
func TestConfigEditPreservesCursor(t *testing.T) {
	mgr := newFakeManager(SourceInfo{Name: "src", Config: json.RawMessage(`{"severities":["Warning"]}`)})
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	cursor := mustTime(t, "2026-08-08T12:00:00Z")
	a.sources["src"].cursor = cursor

	a.refreshSources(context.Background())
	if !a.sources["src"].cursor.Equal(cursor) {
		t.Fatalf("cursor must survive a config refresh: %v", a.sources["src"].cursor)
	}
}

// A first-time source starts at "now": opening a conversation for every warning
// already sitting in the backlog would be a very expensive first impression.
func TestUnseenSourceSeedsCursorAtNow(t *testing.T) {
	mgr := newFakeManager()
	m := mgr.start(t)
	a := newTestAdapter(m, nil, "src", mustFilter(t, `{}`))

	got := a.loadCursor(context.Background(), "brand-new")
	if time.Since(got) > time.Minute {
		t.Fatalf("a new source must start at now, got %v", got)
	}
	if mgr.state["brand-new/"+cursorKey] == "" {
		t.Fatal("the seeded cursor must be persisted immediately")
	}
}

// One cluster-wide source makes per-namespace watches redundant.
func TestDesiredScopesCollapseToClusterWide(t *testing.T) {
	a := newTestAdapter(nil, nil, "a", mustFilter(t, `{"namespaces":["prod"]}`))
	if got := a.desiredScopes(); len(got) != 1 || got[0] != "prod" {
		t.Fatalf("namespaced source: %v", got)
	}
	a.sources["b"] = &servedSource{filter: mustFilter(t, `{}`)}
	if got := a.desiredScopes(); len(got) != 1 || got[0] != "" {
		t.Fatalf("a cluster-wide source must subsume the namespaced ones: %v", got)
	}
}

// ---- helpers -----------------------------------------------------------------

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v.UTC()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}
