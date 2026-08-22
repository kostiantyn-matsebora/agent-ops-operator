package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The browser surface. Two things are worth pinning hard: nothing is readable
// without the token, and an unconfigured console is closed rather than open.

func apiUnderTest(t *testing.T, staticToken string, objs ...*Object) (*API, *fakeManager, *Transcripts) {
	api, f, tr, _ := apiWithOptions(t, staticToken, true, objs...)
	return api, f, tr
}

// apiWithOptions builds the browser surface over a fixture cache. writeEnabled
// is explicit because the read-only posture is a behaviour with its own tests,
// not a variant.
func apiWithOptions(t *testing.T, staticToken string, writeEnabled bool, objs ...*Object) (*API, *fakeManager, *Transcripts, *Cache) {
	t.Helper()
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	adapter, tr, cache := consoleUnderTest(t, f, objs...)
	adapter.refreshChannels(context.Background())
	cfg := &Config{
		Namespace: "agent-ops", AdapterName: "console", UIToken: staticToken,
		WriteEnabled: writeEnabled, SignalSourceName: "console",
	}
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500),
		Manager:  adapter.mgr, Config: cfg,
	})
	return api, f, tr, cache
}

func TestEveryDataEndpointRequiresAuth(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok")
	h := api.Handler(http.NotFoundHandler())
	for _, path := range []string{
		"/api/topology", "/api/config", "/api/config/pipelines", "/api/config/pipelines/x",
		"/api/conversations", "/api/conversations/x", "/api/stream",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: want 401, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "pipeline") {
			t.Fatalf("%s leaked data in the 401 body: %s", path, rec.Body.String())
		}
	}
}

// A console with no token configured must authorize NOBODY. "Unset" reading as
// "open" is the failure mode worth a test of its own.
func TestUnconfiguredConsoleAuthorizesNobody(t *testing.T) {
	api, _, _ := apiUnderTest(t, "")
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/topology", nil)
	req.Header.Set("Authorization", "Bearer ")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty bearer must not pass: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/login", `{"token":""}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login against an unconfigured console must fail: %d", rec.Code)
	}
}

func jsonReq(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestLoginIssuesSessionCookie(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok",
		obj("pipelines", "p", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")))
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/login", `{"token":"wrong"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/login", `{"token":"tok"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie || !cookies[0].HttpOnly {
		t.Fatalf("session cookie missing or not HttpOnly: %+v", cookies)
	}

	// the cookie now authorizes reads
	req := httptest.NewRequest("GET", "/api/topology", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session cookie rejected: %d", rec.Code)
	}
	var payload struct {
		Topology Topology `json:"topology"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if findNode(payload.Topology, "pipelines", "p") == nil {
		t.Fatalf("topology missing the pipeline: %s", rec.Body.String())
	}
}

func authed(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = jsonReq(method, path, body)
	}
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestConversationListDistinguishesJoinedFromObserved(t *testing.T) {
	joined := obj("conversations", "joined", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}],"title":"joined one"}`,
		`{"phase":"Working","threads":[{"channel":"console","threadId":"console-uid-1"}],"inflight":{"runId":"r1"}}`)
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"phase":"Idle","threads":[{"channel":"telegram","threadId":"55"}],"runs":[{"runId":"r0","status":"Succeeded","result":"all good"}]}`)
	api, _, _ := apiUnderTest(t, "tok", joined, observed)
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/conversations", "")
	var page struct {
		Items  []ConversationSummary `json:"items"`
		Total  int                   `json:"total"`
		Offset int                   `json:"offset"`
		Limit  int                   `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	byName := map[string]ConversationSummary{}
	for _, c := range page.Items {
		byName[c.Name] = c
	}
	if !byName["joined"].Joined || byName["joined"].ConsoleThread != "console-uid-1" {
		t.Fatalf("joined conversation not detected: %+v", byName["joined"])
	}
	if byName["observed"].Joined {
		t.Fatalf("telegram-only conversation must be observed: %+v", byName["observed"])
	}
	// list rows carry the COUNT, not the results — those are detail-view sized
	if byName["observed"].RunCount != 1 || len(byName["observed"].Runs) != 0 {
		t.Fatalf("list row should summarize runs, not carry them: %+v", byName["observed"])
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("page counts wrong: %+v", page)
	}

	// ...and the detail view has the durable record in full
	rec = authed(t, h, "GET", "/api/conversations/observed", "")
	var detail struct {
		Conversation ConversationSummary `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Conversation.Runs) != 1 || detail.Conversation.Runs[0].Result != "all good" {
		t.Fatalf("detail must carry runs history: %+v", detail.Conversation.Runs)
	}
}

// A namespace can hold thousands of conversations. The listing must stay
// bounded, ordered newest-first, and honest about what it left out.
func TestConversationListIsBoundedAndNewestFirst(t *testing.T) {
	var objs []*Object
	for i := 0; i < 12; i++ {
		// lastActivity ascending with i, so the newest is the last created
		o := obj("conversations", fmt.Sprintf("c%02d", i), "1",
			`{"profileRef":{"name":"ops"}}`,
			fmt.Sprintf(`{"phase":"Idle","lastActivity":"2026-08-09T10:%02d:00Z"}`, i))
		objs = append(objs, o)
	}
	api, _, _ := apiUnderTest(t, "tok", objs...)
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/conversations?limit=5", "")
	var page struct {
		Items  []ConversationSummary `json:"items"`
		Total  int                   `json:"total"`
		Offset int                   `json:"offset"`
		Limit  int                   `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 12 || len(page.Items) != 5 {
		t.Fatalf("listing not bounded: %+v", page)
	}
	if page.Items[0].Name != "c11" || page.Items[4].Name != "c07" {
		t.Fatalf("listing must be newest-activity-first: %s…%s", page.Items[0].Name, page.Items[4].Name)
	}

	// The total is the MATCH COUNT, so a second page continues rather than
	// restarts — an operator paging through a storm must not see c11 twice.
	rec = authed(t, h, "GET", "/api/conversations?limit=5&offset=5", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 12 || page.Offset != 5 || page.Items[0].Name != "c06" {
		t.Fatalf("second page wrong: %+v", page)
	}
}

// Filtering happens SERVER-side: shipping thousands of rows so the client can
// hide most is how a read-only viewer becomes an API-server problem.
func TestConversationListFiltersServerSide(t *testing.T) {
	working := obj("conversations", "busy", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Working","lastActivity":"2026-08-09T10:00:00Z","threads":[{"channel":"console","threadId":"t1"}]}`)
	idle := obj("conversations", "calm", "1",
		`{"profileRef":{"name":"other"}}`,
		`{"phase":"Idle","lastActivity":"2026-08-09T10:00:00Z","runs":[{"runId":"r","status":"failed"}]}`)
	api, _, _ := apiUnderTest(t, "tok", working, idle)
	h := api.Handler(http.NotFoundHandler())

	get := func(query string) []ConversationSummary {
		t.Helper()
		rec := authed(t, h, "GET", "/api/conversations?"+query, "")
		var page struct {
			Items []ConversationSummary `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page.Items
	}
	if got := get("phase=Working"); len(got) != 1 || got[0].Name != "busy" {
		t.Fatalf("phase filter: %+v", got)
	}
	if got := get("profile=other"); len(got) != 1 || got[0].Name != "calm" {
		t.Fatalf("profile filter: %+v", got)
	}
	if got := get("channel=console"); len(got) != 1 || got[0].Name != "busy" {
		t.Fatalf("channel filter: %+v", got)
	}
	// errored keys on the LAST run, so a conversation that recovered is not
	// flagged forever
	if got := get("errored=true"); len(got) != 1 || got[0].Name != "calm" {
		t.Fatalf("errored filter: %+v", got)
	}
	if got := get("q=bus"); len(got) != 1 || got[0].Name != "busy" {
		t.Fatalf("search filter: %+v", got)
	}
}

func TestSendingToAnObservedConversationIsRefused(t *testing.T) {
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55"}]}`)
	api, f, _ := apiUnderTest(t, "tok", observed)
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "POST", "/api/conversations/observed/messages", `{"text":"hi"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "channels[]") {
		t.Fatalf("the refusal should say how to join: %s", rec.Body.String())
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing should reach the manager")
	}
}

func TestUnknownKindIsRefused(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok")
	h := api.Handler(http.NotFoundHandler())
	// secrets are not in Kinds — the console has no path to any resource
	// outside the agentops group, and asking for one is a 404 here
	if rec := authed(t, h, "GET", "/api/config/secrets", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for an unwatched kind, got %d", rec.Code)
	}
}

func TestInventoryCarriesConditionsVerbatim(t *testing.T) {
	ch := obj("channels", "tg", "1", `{"adapter":"telegram"}`,
		`{"conditions":[{"type":"Served","status":"False","reason":"NoAdapter","message":"no ChannelAdapter named telegram"}]}`)
	api, _, _ := apiUnderTest(t, "tok", ch)
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/config/channels", "")
	var rows []InventoryRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Health != HealthBad || len(rows[0].Conditions) != 1 {
		t.Fatalf("inventory row wrong: %+v", rows)
	}
	if rows[0].Conditions[0].Message == "" || rows[0].Summary != "adapter telegram" {
		t.Fatalf("condition/summary not surfaced: %+v", rows[0])
	}
}

func TestDetailReturnsOpaqueConfigUntouched(t *testing.T) {
	ch := obj("channels", "tg", "1", `{"adapter":"telegram","config":{"chatId":"-100","approvers":[7]}}`, "{}")
	api, _, _ := apiUnderTest(t, "tok", ch)
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/config/channels/tg", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d", rec.Code)
	}
	var out Detail
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.Object.Spec), `"chatId":"-100"`) {
		t.Fatalf("opaque config must pass through verbatim: %s", out.Object.Spec)
	}
	// and the YAML view carries it too, so what an operator copies matches what
	// the cluster holds
	if !strings.Contains(out.YAML, "chatId") || !strings.Contains(out.YAML, "kind: Channel") {
		t.Fatalf("YAML view wrong:\n%s", out.YAML)
	}
}
