package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A grab-bag of small, real gaps left after the larger files (kube.go,
// metrics.go, uiserve.go, config.go) got their own test files — each closes
// one previously-untested function against real behavior, not a restated
// assertion.

// consoleError.Error() was never called anywhere: every caller compared the
// sentinel by identity (errors.Is/==) and never printed it.
func TestConsoleErrorMessage(t *testing.T) {
	err := &consoleError{"conversation is not joined to the console channel"}
	if err.Error() != "conversation is not joined to the console channel" {
		t.Fatalf("got %q", err.Error())
	}
}

// plural() renders the count-and-word pair summaryLine uses; nothing had
// called it directly, and the fixture-driven HTTP tests never happened to hit
// n>1.
func TestPlural(t *testing.T) {
	if plural(1, "source") != "1 source" {
		t.Fatalf("got %q", plural(1, "source"))
	}
	if plural(2, "source") != "2 sources" {
		t.Fatalf("got %q", plural(2, "source"))
	}
	if plural(0, "channel") != "0 channels" {
		t.Fatalf("got %q", plural(0, "channel"))
	}
}

// nextBackoff doubles and caps — the reconnect loop's own pacing logic, never
// called directly by any watch test (which use scripted, immediate results).
func TestNextBackoffDoublesAndCaps(t *testing.T) {
	d := time.Second
	d = nextBackoff(d)
	if d != 2*time.Second {
		t.Fatalf("want 2s, got %v", d)
	}
	d = nextBackoff(20 * time.Second)
	if d != 30*time.Second {
		t.Fatalf("must cap at 30s, got %v", d)
	}
}

// Sessions.drop is only reachable through handleLogout when a real session
// cookie is presented; the existing logout test sends none (it only checks
// the cleared cookie's flags), so the delete-from-the-map branch was never
// exercised.
func TestLogoutDropsTheSession(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok")
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/login", `{"token":"tok"}`))
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a cookie")
	}

	logoutReq := httptest.NewRequest("POST", "/api/logout", nil)
	logoutReq.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, logoutReq)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}

	// the dropped session must no longer authorize a read
	req := httptest.NewRequest("GET", "/api/topology", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a dropped session cookie must no longer authorize: %d", rec.Code)
	}
}

// handleActivity (GET /api/activity) was wired as a route but never requested
// by any test in the suite.
func TestHandleActivityEndpoint(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok")
	api.activity.add(ActivityEvent{Cursor: "0000000000000001", TS: time.Now(), Kind: "signal.claimed", Status: "ok"})
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Events []ActivityEvent `json:"events"`
		Cursor string          `json:"cursor"`
	}
	getJSON(t, h, "/api/activity", &out)
	if len(out.Events) != 1 || out.Events[0].Kind != "signal.claimed" {
		t.Fatalf("activity not served: %+v", out)
	}
	if out.Cursor == "" {
		t.Fatal("cursor must be reported so a client can page from it")
	}

	// the limit query param, and its clamp against a nonsense value
	rec := authed(t, h, "GET", "/api/activity?limit=abc", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("an unparseable limit must fall back rather than fail: %d", rec.Code)
	}
}

// handleOriginationSources (GET /api/sources) was wired but never requested.
// This exercises all three branches: wired-with-one-server (profile filled
// in), unwired-with-a-patch-suggestion, and unknown-to-the-cache.
func TestHandleOriginationSourcesEndpoint(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	adapter, tr, cache := consoleUnderTest(t, f, fixtureInstall()...)
	adapter.refreshChannels(context.Background())

	cfg := &Config{Namespace: "agent-ops", AdapterName: "console", UIToken: "tok", WriteEnabled: true}
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Originator: NewOriginator("http://mgr", "sig-tok", "cluster-events"),
		Config:     cfg,
	})
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Sources      []OriginationSource `json:"sources"`
		CanOriginate bool                `json:"canOriginate"`
	}
	getJSON(t, h, "/api/sources", &out)
	if !out.CanOriginate || len(out.Sources) != 1 {
		t.Fatalf("wrong shape: %+v", out)
	}
	src := out.Sources[0]
	if src.Name != "cluster-events" || !src.Wired || src.Pipeline != "k8s-ops" || src.Profile != "k8s-engineer" {
		t.Fatalf("wired source not reported correctly: %+v", src)
	}
	if src.Patch != "" {
		t.Fatalf("a wired source needs no patch suggestion: %+v", src)
	}

	// the unwired branch, with the suggested patch
	api2 := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Originator: NewOriginator("http://mgr", "sig-tok", "orphan"),
		Config:     cfg,
	})
	h2 := api2.Handler(http.NotFoundHandler())
	var out2 struct {
		Sources []OriginationSource `json:"sources"`
	}
	getJSON(t, h2, "/api/sources", &out2)
	if len(out2.Sources) != 1 || out2.Sources[0].Wired || out2.Sources[0].Patch == "" {
		t.Fatalf("unwired source must carry a patch suggestion: %+v", out2.Sources)
	}

	// a source name the cache has never heard of
	api3 := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Originator: NewOriginator("http://mgr", "sig-tok", "does-not-exist"),
		Config:     cfg,
	})
	h3 := api3.Handler(http.NotFoundHandler())
	var out3 struct {
		Sources []OriginationSource `json:"sources"`
	}
	getJSON(t, h3, "/api/sources", &out3)
	if len(out3.Sources) != 1 || out3.Sources[0].Reason != "NotFound" {
		t.Fatalf("an unknown source must report NotFound: %+v", out3.Sources)
	}
}

// handleKinds (GET /api/config) was wired but never requested — every other
// config test goes straight for a per-kind listing.
func TestHandleKindsEndpoint(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var out []KindInfo
	getJSON(t, h, "/api/config", &out)
	if len(out) != len(Kinds) {
		t.Fatalf("want one row per agentops.dev kind, got %d", len(out))
	}
	byKind := map[string]KindInfo{}
	for _, k := range out {
		byKind[k.Kind] = k
	}
	if byKind["pipelines"].Count != 2 || byKind["pipelines"].Title != "Pipeline" {
		t.Fatalf("pipelines row wrong: %+v", byKind["pipelines"])
	}
	if !byKind["pipelines"].Synced {
		t.Fatal("a fully-loaded fixture cache must report every kind synced")
	}
}

// podHealth is reached only through a conversation graph whose recorded
// runtimePod resolves to a live pod object — no existing test names one.
func TestConversationGraphReportsPodHealth(t *testing.T) {
	objs := append(fixtureInstall(),
		obj("conversations", "crashed", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Working","runtimePod":"agentops-adapter-slack-def-456",`+
				`"threads":[{"channel":"console","threadId":"t1"}]}`),
		obj("conversations", "healthy", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Working","runtimePod":"agentops-manager-abc-123",`+
				`"threads":[{"channel":"console","threadId":"t2"}]}`),
	)
	api, _, _ := apiUnderTest(t, "tok", objs...)
	h := api.Handler(http.NotFoundHandler())

	var crashed ConversationGraph
	getJSON(t, h, "/api/conversations/crashed/graph", &crashed)
	pod := findNode(crashed.Topology, "pods", "agentops-adapter-slack-def-456")
	if pod == nil || pod.Health != HealthBad {
		t.Fatalf("a crash-looping pod must report bad health: %+v", pod)
	}

	var healthy ConversationGraph
	getJSON(t, h, "/api/conversations/healthy/graph", &healthy)
	pod = findNode(healthy.Topology, "pods", "agentops-manager-abc-123")
	if pod == nil || pod.Health != HealthOK {
		t.Fatalf("a ready pod must report good health: %+v", pod)
	}
}

// Manager.Resolved reads the pipeline's server-side capability resolution —
// no existing test names the endpoint at all.
func TestManagerResolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pipelines/k8s-ops/resolved" {
			t.Fatalf("wrong path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"pipeline":"k8s-ops","profile":"k8s-engineer","allowedTools":["Read"],"toolsMode":"merge"}`))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "tok")
	res, err := m.Resolved(context.Background(), "k8s-ops")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pipeline != "k8s-ops" || res.Profile != "k8s-engineer" || len(res.AllowedTools) != 1 {
		t.Fatalf("wrong resolution: %+v", res)
	}
}

// The queues Work loop — a conversation with queued inputs, one with an
// inflight run stuck past the threshold, and one at the capacity ceiling —
// was entirely untested; every existing queues test only ever supplied
// Delivery rows.
func TestQueuesWorkRowsAndStuckReasons(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	old := time.Now().Add(-20 * time.Minute).Format(time.RFC3339)
	objs := append(fixtureInstall(),
		obj("conversations", "waiting", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"},{"name":"console"}],"inputs":[{"text":"hi"}]}`,
			`{"phase":"Pending"}`),
		obj("conversations", "hung", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Working","inflight":{"runId":"r1","dispatchedAt":"`+old+`"}}`),
		obj("conversations", "idle", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Idle"}`),
	)
	adapter, tr, cache := consoleUnderTest(t, f, objs...)
	adapter.refreshChannels(context.Background())
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Config: &Config{Namespace: "agent-ops", AdapterName: "console", UIToken: "tok", WriteEnabled: true},
	})
	// at the capacity ceiling: InUse >= Max makes the Pending row report
	// StuckAtCeiling instead of nothing
	st := &ManagerStatus{}
	st.RuntimeSlots.InUse, st.RuntimeSlots.Max = 5, 5
	api.mgr = newStatusManager(t, st)
	h := api.Handler(http.NotFoundHandler())

	var q Queues
	getJSON(t, h, "/api/queues", &q)
	byName := map[string]WorkRow{}
	for _, row := range q.Work {
		byName[row.Conversation] = row
	}
	if _, ok := byName["idle"]; ok {
		t.Fatal("a conversation with nothing queued and nothing inflight must not appear in the work queue")
	}
	if r := byName["waiting"]; r.Queued != 1 || r.Stuck != StuckAtCeiling {
		t.Fatalf("waiting-at-ceiling row wrong: %+v", r)
	}
	if r := byName["hung"]; r.Inflight != "r1" || r.Stuck != StuckRuntimeHung {
		t.Fatalf("a long-inflight run must report StuckRuntimeHung: %+v", r)
	}
}
