package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Contract tests for the browser API, against fixture caches.

// fixtureInstall is a small but complete namespace: a wired pipeline, a
// dangling ref, an unclaimed source, an unserved channel, and a failing pod.
func fixtureInstall() []*Object {
	return []*Object{
		obj("pipelines", "k8s-ops", "1",
			`{"profileRef":{"name":"k8s-engineer"},"signalSourceRefs":[{"name":"cluster-events"}],`+
				`"channelRefs":[{"name":"console"}],"toolsets":{"mode":"merge","refs":[{"name":"observe"}]}}`,
			cond("Ready", "True", "")),
		obj("pipelines", "broken", "1",
			`{"profileRef":{"name":"ghost"},"channelRefs":[{"name":"nowhere"}]}`,
			cond("Ready", "False", "MissingProfile")),
		obj("agentprofiles", "k8s-engineer", "1", `{"runtimeRef":{"name":"default"}}`, "{}"),
		obj("agentruntimes", "default", "1", `{"image":"example/runtime:1"}`, "{}"),
		obj("mcptoolsets", "observe", "1", `{"tools":["Read","Grep"]}`, "{}"),
		obj("signalsources", "cluster-events", "1", `{"adapter":"k8s-events"}`,
			`{"conditions":[{"type":"Served","status":"True"},{"type":"Wired","status":"True","reason":"PipelineClaim"}]}`),
		obj("signalsources", "orphan", "1", `{"adapter":"k8s-events"}`,
			`{"conditions":[{"type":"Served","status":"True"},`+
				`{"type":"Wired","status":"False","reason":"NoPipelineClaim","message":"no Ready Pipeline references this source"}]}`),
		obj("channels", "console", "1", `{"adapter":"console"}`, cond("Served", "True", "")),
		obj("channels", "slack", "1", `{"adapter":"slack"}`,
			`{"conditions":[{"type":"Served","status":"False","reason":"NoServingImplementation"}]}`),
		obj("channeladapters", "console", "1", `{"image":"example/console:1"}`, cond("Ready", "True", "")),
		obj("signaladapters", "k8s-events", "1", `{"image":"example/k8s-events:1"}`, cond("Ready", "True", "")),
		obj("deployments", "agentops-manager", "1",
			`{"replicas":1,"template":{"spec":{"containers":[{"name":"manager","image":"example/manager:0.22.0"}]}}}`,
			`{"replicas":1,"readyReplicas":1}`),
		obj("pods", "agentops-manager-abc-123", "1",
			`{"containers":[{"name":"manager","image":"example/manager:0.22.0"}],"nodeName":"node-1"}`,
			`{"phase":"Running","containerStatuses":[{"name":"manager","ready":true,"restartCount":0,`+
				`"imageID":"docker.io/example/manager@sha256:deadbeef","state":{"running":{}}}]}`),
		obj("deployments", "agentops-adapter-slack", "1",
			`{"replicas":1,"template":{"spec":{"containers":[{"name":"a","image":"example/slack:1"}]}}}`,
			`{"replicas":1,"readyReplicas":0}`),
		obj("pods", "agentops-adapter-slack-def-456", "1",
			`{"containers":[{"name":"a","image":"example/slack:1"}]}`,
			`{"phase":"Running","containerStatuses":[{"name":"a","ready":false,"restartCount":7,`+
				`"state":{"waiting":{"reason":"CrashLoopBackOff","message":"back-off"}}}]}`),
	}
}

func getJSON(t *testing.T, h http.Handler, path string, into any) *httptest.ResponseRecorder {
	t.Helper()
	rec := authed(t, h, "GET", path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("%s: %v (%s)", path, err, rec.Body.String())
		}
	}
	return rec
}

// 4.2: the overview names every image, reports pod-level failure, and rolls up
// every non-True condition. An operations console that cannot see a
// CrashLoopBackOff is not one — which is why the pod read grant exists.
func TestOverviewReportsInstallFactsAndProblems(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var out Overview
	getJSON(t, h, "/api/overview", &out)

	byName := map[string]WorkloadInfo{}
	for _, wl := range out.Workloads {
		byName[wl.Name] = wl
	}
	mgr := byName["agentops-manager"]
	if mgr.Image != "example/manager:0.22.0" || !strings.Contains(mgr.Digest, "sha256:") {
		t.Fatalf("manager image/digest missing: %+v", mgr)
	}
	slack := byName["agentops-adapter-slack"]
	if slack.Restarts != 7 || !strings.Contains(slack.Problem, "CrashLoopBackOff") {
		t.Fatalf("pod-level failure not surfaced: %+v", slack)
	}
	if len(out.Runtimes) != 1 || out.Runtimes[0].Image != "example/runtime:1" {
		t.Fatalf("runtime images missing: %+v", out.Runtimes)
	}

	// every non-True condition, plus the pod, plus derived findings — each
	// labelled with WHERE it came from
	sources := map[string]int{}
	for _, p := range out.Problems {
		sources[p.Source]++
	}
	if sources[SourceReported] == 0 || sources[SourcePod] == 0 || sources[SourceDerived] == 0 {
		t.Fatalf("problem rollup must carry all three sources: %+v", out.Problems)
	}
	found := false
	for _, p := range out.Problems {
		if p.Kind == "signalsources" && p.Name == "orphan" && p.Type == "Wired" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the unclaimed source must appear in the rollup: %+v", out.Problems)
	}
}

// 4.3: cross-object findings, and the source distinction. A condition a
// reconciler wrote and a cross-reference the console derived carry different
// authority.
func TestFindingsAreDerivedAndMarked(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var findings []Finding
	getJSON(t, h, "/api/findings", &findings)

	byCheck := map[string][]Finding{}
	for _, f := range findings {
		byCheck[f.Check] = append(byCheck[f.Check], f)
	}
	// the broken pipeline names a profile and a channel that do not exist
	if len(byCheck[CheckDanglingRef]) != 2 {
		t.Fatalf("want 2 dangling refs, got %+v", byCheck[CheckDanglingRef])
	}
	// the slack channel names an adapter nothing serves
	if len(byCheck[CheckUnservedChannel]) != 1 || byCheck[CheckUnservedChannel][0].Name != "slack" {
		t.Fatalf("unserved channel: %+v", byCheck[CheckUnservedChannel])
	}
	// The unclaimed source already REPORTS Wired=False, so the console does not
	// derive a duplicate — the reported condition is authoritative.
	if len(byCheck[CheckUnclaimedSource]) != 0 {
		t.Fatalf("a reported condition must not be duplicated as a finding: %+v", byCheck[CheckUnclaimedSource])
	}
}

// 4.3: detail carries conditions, YAML, inbound references and per-object
// findings.
func TestDetailCarriesReferencesAndYAML(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var detail Detail
	getJSON(t, h, "/api/config/channels/console", &detail)
	if detail.Health != HealthOK {
		t.Fatalf("health: %v", detail.Health)
	}
	if len(detail.UsedBy) != 1 || detail.UsedBy[0].Name != "k8s-ops" || detail.UsedBy[0].Field != "channelRefs" {
		t.Fatalf("inbound references wrong: %+v", detail.UsedBy)
	}
	if !strings.Contains(detail.YAML, "adapter: console") {
		t.Fatalf("YAML missing spec:\n%s", detail.YAML)
	}

	// a toolset knows which pipelines bind it — the "is it safe to delete this"
	// answer
	var ts Detail
	getJSON(t, h, "/api/config/mcptoolsets/observe", &ts)
	if len(ts.UsedBy) != 1 || ts.UsedBy[0].Field != "toolsets.refs" {
		t.Fatalf("toolset references wrong: %+v", ts.UsedBy)
	}
}

// 4.4: the graph carries all nine kinds and the capability edges, and health
// comes from conditions only.
func TestTopologyCarriesEveryKindAndCapabilityEdges(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Topology Topology `json:"topology"`
	}
	getJSON(t, h, "/api/topology", &out)

	for _, want := range []struct{ kind, name string }{
		{"pipelines", "k8s-ops"}, {"agentprofiles", "k8s-engineer"}, {"agentruntimes", "default"},
		{"channels", "console"}, {"channeladapters", "console"},
		{"signalsources", "cluster-events"}, {"signaladapters", "k8s-events"},
		{"mcptoolsets", "observe"},
	} {
		if findNode(out.Topology, want.kind, want.name) == nil {
			t.Fatalf("node %s/%s missing from the graph", want.kind, want.name)
		}
	}
	kinds := map[string]int{}
	for _, e := range out.Topology.Edges {
		kinds[e.Kind]++
	}
	if kinds["uses"] < 2 {
		t.Fatalf("capability + runtime edges missing: %+v", kinds)
	}
	if kinds["feeds"] == 0 || kinds["answers"] == 0 || kinds["posts"] == 0 || kinds["served-by"] == 0 {
		t.Fatalf("wiring edges missing: %+v", kinds)
	}
	// Every unresolvable ref is drawn as a broken edge to a placeholder rather
	// than silently omitted: the broken pipeline's profile and channel, and the
	// slack channel's missing adapter.
	broken := map[string]bool{}
	for _, e := range out.Topology.Edges {
		if e.Dangling {
			broken[e.To] = true
		}
	}
	for _, want := range []string{"agentprofiles/ghost", "channels/nowhere", "channeladapters/slack"} {
		if !broken[want] {
			t.Fatalf("missing broken edge to %s (got %v)", want, broken)
		}
	}
	if len(broken) != 3 {
		t.Fatalf("want exactly 3 dangling edges, got %v", broken)
	}
	// ...and each placeholder is on the graph, so the picture shows WHAT failed
	// to resolve instead of a hole
	if n := findNode(out.Topology, "agentprofiles", "ghost"); n == nil || n.Health != HealthBad {
		t.Fatalf("placeholder node missing: %+v", n)
	}
	// the unclaimed source renders detached — that IS its state
	if n := findNode(out.Topology, "signalsources", "orphan"); n == nil || !n.Detached {
		t.Fatalf("unclaimed source must render detached: %+v", n)
	}
}

// 4.4: traffic comes from RECORDED hops, and an edge with no events is idle
// rather than absent.
func TestTopologyTrafficComesFromActivity(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, fixtureInstall()...)
	now := time.Now().UTC()
	api.activity.add(ActivityEvent{
		Cursor: "0000000000000001", TS: now, Kind: "signal.claimed", Status: "ok",
		From: &NodeRef{"signal-source", "cluster-events"}, To: &NodeRef{"pipeline", "k8s-ops"},
	})
	api.activity.add(ActivityEvent{
		Cursor: "0000000000000002", TS: now, Kind: "signal.claimed", Status: "error",
		From: &NodeRef{"signal-source", "cluster-events"}, To: &NodeRef{"pipeline", "k8s-ops"},
	})
	h := api.Handler(http.NotFoundHandler())

	var out struct {
		Topology Topology `json:"topology"`
	}
	getJSON(t, h, "/api/topology?windowSeconds=60", &out)

	var feeds, posts *Edge
	for i := range out.Topology.Edges {
		e := &out.Topology.Edges[i]
		if e.Kind == "feeds" && strings.HasSuffix(e.From, "cluster-events") {
			feeds = e
		}
		if e.Kind == "posts" && strings.HasSuffix(e.To, "console") {
			posts = e
		}
	}
	if feeds == nil || feeds.Traffic == nil {
		t.Fatalf("the fed edge should carry traffic: %+v", feeds)
	}
	if feeds.Traffic.Events != 2 || feeds.Traffic.Errors != 1 || feeds.Traffic.RatePerMin != 2 {
		t.Fatalf("edge traffic wrong: %+v", feeds.Traffic)
	}
	if posts == nil || posts.Traffic != nil {
		t.Fatalf("an edge with no events must be idle, not fabricated: %+v", posts)
	}
	if out.Topology.WindowSeconds != 60 {
		t.Fatalf("window not reported: %v", out.Topology.WindowSeconds)
	}
}

// 4.7: the per-conversation graph is built from what the CONVERSATION recorded,
// and reports when the pipeline has since been re-wired. Reading the live
// pipeline instead would silently rewrite history.
func TestConversationGraphUsesRecordedBindings(t *testing.T) {
	objs := append(fixtureInstall(),
		obj("conversations", "chat-1", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}],`+
				`"toolsets":{"mode":"merge","refs":[{"name":"retired-toolset"}]}}`,
			`{"phase":"Idle","threads":[{"channel":"console","threadId":"t1"}]}`))
	api, _, _ := apiUnderTest(t, "tok", objs...)
	h := api.Handler(http.NotFoundHandler())

	var g ConversationGraph
	getJSON(t, h, "/api/conversations/chat-1/graph", &g)

	// the toolset it RAN with is on the graph, even though it no longer exists
	n := findNode(g.Topology, "mcptoolsets", "retired-toolset")
	if n == nil {
		t.Fatalf("the graph must show the binding the run materialized: %+v", g.Nodes)
	}
	// ...and the current toolset, which it did NOT run with, is not
	if findNode(g.Topology, "mcptoolsets", "observe") != nil {
		t.Fatal("the conversation graph must not show the pipeline's CURRENT bindings")
	}
	if !g.Diverged || len(g.Drift) == 0 {
		t.Fatalf("a re-wired pipeline must be reported as diverged: %+v", g)
	}
	if !strings.Contains(strings.Join(g.Drift, " "), "retired-toolset") {
		t.Fatalf("drift must name what changed: %+v", g.Drift)
	}
}

// 4.8: origination is refused when the source is unclaimed, carrying the
// Wired=False reason — not a generic error.
func TestOriginationRefusedWhenSourceUnclaimed(t *testing.T) {
	objs := append(fixtureInstall(),
		obj("signalsources", "console", "1", `{"adapter":"console"}`,
			`{"conditions":[{"type":"Wired","status":"False","reason":"NoPipelineClaim",`+
				`"message":"no Ready Pipeline references this source — signals are dropped until one does"}]}`))
	api, _, _, _ := apiWithOptions(t, "tok", true, objs...)
	api.originator = NewOriginator("http://unused", "signal-token", "console")
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "POST", "/api/conversations", `{"task":"check the nodes"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NoPipelineClaim") ||
		!strings.Contains(rec.Body.String(), "signalSourceRefs") {
		t.Fatalf("the refusal must carry the reason and the fix: %s", rec.Body.String())
	}
}

// A console with no signal identity cannot originate, and says so with the fix.
func TestOriginationRefusedWithoutASignalIdentity(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok", fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())
	rec := authed(t, h, "POST", "/api/conversations", `{"task":"anything"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "servedBy") {
		t.Fatalf("want a 409 naming the fix, got %d %s", rec.Code, rec.Body.String())
	}
}

// 4.1/8.5: with writes disabled BOTH write paths are refused server-side. The
// UI hiding the buttons is presentation; this is the boundary.
func TestWriteDisabledRefusesBothWritePaths(t *testing.T) {
	objs := append(fixtureInstall(),
		obj("conversations", "chat-1", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"threads":[{"channel":"console","threadId":"t1"}]}`))
	api, f, _, _ := apiWithOptions(t, "tok", false, objs...)
	api.originator = NewOriginator("http://unused", "signal-token", "console")
	h := api.Handler(http.NotFoundHandler())

	for _, tc := range []struct{ path, body string }{
		{"/api/conversations", `{"task":"go"}`},
		{"/api/conversations/chat-1/messages", `{"text":"hi"}`},
	} {
		rec := authed(t, h, "POST", tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: want 403, got %d %s", tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "read-only") {
			t.Fatalf("%s: the refusal must name the reason: %s", tc.path, rec.Body.String())
		}
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing should reach the manager with writes disabled")
	}
	// and the session tells the SPA not to render the affordances at all
	var session struct {
		WriteEnabled bool `json:"writeEnabled"`
		CanOriginate bool `json:"canOriginate"`
	}
	rec := authed(t, h, "GET", "/api/session", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &session)
	if session.WriteEnabled {
		t.Fatalf("session must report writes disabled: %+v", session)
	}
}

// 4.10b: with no metrics backend, long windows are UNAVAILABLE with a reason —
// never an empty chart, which would read as "there was no traffic".
func TestHistoricalChartsUnavailableWithoutABackend(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok")
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/charts/throughput", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "console.metrics.url") {
		t.Fatalf("the response must name the value to set: %s", rec.Body.String())
	}
	var list struct {
		Available bool     `json:"available"`
		Charts    []string `json:"charts"`
	}
	getJSON(t, h, "/api/charts", &list)
	if list.Available || len(list.Charts) == 0 {
		t.Fatalf("charts listing wrong: %+v", list)
	}
}

// 4.2b: the two queues are reported apart, and a wedged adapter is distinguished
// from one that is simply not polling — they look identical from outside and
// need different fixes.
func TestQueuesSeparatesStuckFromBusy(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, fixtureInstall()...)
	api.mgr = newStatusManager(t, &ManagerStatus{
		Queues: []QueueStat{
			{Adapter: "console", Queued: 3, OldestQueuedOpID: "send:1", OldestQueuedAgeSeconds: 600},
			{Adapter: "slack", Claimed: 1, OldestClaimedOpID: "topic:x", OldestClaimedAgeSeconds: 900},
			{Adapter: "telegram", Queued: 2, OldestQueuedOpID: "send:9", OldestQueuedAgeSeconds: 3},
		},
	})
	h := api.Handler(http.NotFoundHandler())

	var q Queues
	getJSON(t, h, "/api/queues", &q)
	byAdapter := map[string]DeliveryRow{}
	for _, row := range q.Delivery {
		byAdapter[row.Adapter] = row
	}
	if byAdapter["console"].Stuck != StuckNothingClaiming {
		t.Fatalf("queued-and-old means nothing is claiming: %+v", byAdapter["console"])
	}
	if byAdapter["slack"].Stuck != StuckAdapterWedged {
		t.Fatalf("claimed-and-old means an adapter is wedged: %+v", byAdapter["slack"])
	}
	if byAdapter["telegram"].Stuck != "" {
		t.Fatalf("a young backlog is busy, not stuck: %+v", byAdapter["telegram"])
	}
	if byAdapter["console"].OldestQueuedOpID != "send:1" {
		t.Fatalf("the row must name WHICH op is oldest: %+v", byAdapter["console"])
	}
}

// Without /status the delivery queue is UNAVAILABLE, not empty: it exists in no
// Kubernetes object, so "no rows" would read as "no ops pending".
func TestQueuesReportsManagerUnreachable(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true)
	api.mgr = NewManager("http://127.0.0.1:1", "tok")
	h := api.Handler(http.NotFoundHandler())

	var q Queues
	getJSON(t, h, "/api/queues", &q)
	if q.Error == "" {
		t.Fatal("an unreachable manager must be reported, not rendered as an empty queue")
	}
}

// 4.11: snapshot/stream convergence — applying deltas with the stream
// disconnected and re-reading gives the same state as a cold fetch. This is the
// property that makes a missed event cost one stale second rather than a wrong
// screen.
func TestSnapshotConvergesAfterMissedDeltas(t *testing.T) {
	api, _, _, cache := apiWithOptions(t, "tok", true, fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	var before struct {
		Topology Topology `json:"topology"`
	}
	getJSON(t, h, "/api/topology", &before)

	// N deltas arrive with nothing subscribed — the browser is "asleep"
	for i := 0; i < 5; i++ {
		cache.apply("ADDED", obj("channels", "late-"+string(rune('a'+i)), "2", `{"adapter":"console"}`, "{}"))
	}
	cache.apply("DELETED", obj("channels", "slack", "3", `{"adapter":"slack"}`, "{}"))

	var after struct {
		Topology Topology `json:"topology"`
	}
	getJSON(t, h, "/api/topology", &after)

	if len(after.Topology.Nodes) == len(before.Topology.Nodes) {
		t.Fatal("the re-fetched snapshot did not reflect the deltas")
	}
	if findNode(after.Topology, "channels", "slack") != nil {
		t.Fatal("a deleted object must be gone from the re-fetched snapshot")
	}

	// A COLD read of the same cache must equal the re-fetch: the snapshot is
	// authoritative, so there is no delta-derived state to drift.
	cold := BuildTopology(cache)
	if len(cold.Nodes) != len(after.Topology.Nodes) || len(cold.Edges) != len(after.Topology.Edges) {
		t.Fatalf("snapshot and cold fetch disagree: %d/%d vs %d/%d",
			len(after.Topology.Nodes), len(after.Topology.Edges), len(cold.Nodes), len(cold.Edges))
	}
}

// The stream opens with a resync so reconnect and first connect are the same
// code path, and multiplexes CR deltas AND activity events.
func TestStreamOpensWithResyncAndMultiplexes(t *testing.T) {
	api, _, _, cache := apiWithOptions(t, "tok", true, fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/stream", nil)
	req.Header.Set("Authorization", "Bearer tok")
	ctx, cancel := contextWithCancel(req)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { h.ServeHTTP(rec, req); close(done) }()

	waitForNamed(t, "the opening resync", func() bool { return strings.Contains(rec.Body.String(), "event: resync") })
	cache.apply("MODIFIED", obj("channels", "console", "9", `{"adapter":"console"}`, "{}"))
	api.activity.add(ActivityEvent{Cursor: "0000000000000001", TS: time.Now(), Kind: "run.dispatched"})
	waitForNamed(t, "a CR delta", func() bool { return strings.Contains(rec.Body.String(), "event: delta") })
	waitForNamed(t, "an activity event", func() bool { return strings.Contains(rec.Body.String(), "event: activity") })

	cancel()
	<-done
}

// deltaEvents pulls the `delta` events out of an SSE body.
func deltaEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, block := range strings.Split(body, "\n\n") {
		if !strings.HasPrefix(block, "event: delta\n") {
			continue
		}
		data := strings.TrimPrefix(strings.SplitN(block, "\n", 2)[1], "data: ")
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("delta event is not JSON: %v (%s)", err, data)
		}
		out = append(out, m)
	}
	return out
}

// A delta CARRIES what changed, in the shapes the snapshots serve.
//
// It used to carry `{type, kind, name}`, so a browser told that something had
// changed had to ask what it now was — a request per change per browser, and a
// blank page while the answer came back. What is pinned here is that the answer
// is already on the wire: a create and an update arrive complete, and a delete
// carries its identity and type, which is all that dropping a row needs.
func TestDeltaCarriesTheChangedObject(t *testing.T) {
	api, _, _, cache := apiWithOptions(t, "tok", true, fixtureInstall()...)
	h := api.Handler(http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/stream", nil)
	req.Header.Set("Authorization", "Bearer tok")
	ctx, cancel := contextWithCancel(req)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { h.ServeHTTP(rec, req); close(done) }()
	waitForNamed(t, "the opening resync", func() bool { return strings.Contains(rec.Body.String(), "event: resync") })

	cache.apply("ADDED", obj("channels", "fresh", "2", `{"adapter":"console"}`, cond("Served", "True", "")))
	cache.apply("MODIFIED", obj("conversations", "conv-1", "3",
		`{"profileRef":{"name":"k8s-engineer"}}`, `{"phase":"Running"}`))
	cache.apply("ADDED", obj("pipelines", "extra", "2",
		`{"profileRef":{"name":"k8s-engineer"}}`, cond("Ready", "True", "")))
	cache.apply("DELETED", obj("channels", "slack", "4", `{"adapter":"slack"}`, "{}"))
	waitForNamed(t, "four deltas", func() bool { return len(deltaEvents(t, rec.Body.String())) >= 4 })

	cancel()
	<-done

	byName := map[string]map[string]any{}
	for _, d := range deltaEvents(t, rec.Body.String()) {
		byName[d["kind"].(string)+"/"+d["name"].(string)] = d
	}

	created := byName["channels/fresh"]
	if created["type"] != "ADDED" {
		t.Fatalf("a create must say so: %v", created["type"])
	}
	row, ok := created["row"].(map[string]any)
	if !ok || row["name"] != "fresh" || row["health"] == nil {
		t.Fatalf("a created object must arrive as the row a listing shows: %v", created["row"])
	}
	detail, ok := created["detail"].(map[string]any)
	if !ok || detail["object"] == nil || detail["yaml"] == "" {
		t.Fatalf("a created object must arrive as the detail a kind page shows: %v", created["detail"])
	}

	updated := byName["conversations/conv-1"]
	if updated["type"] != "MODIFIED" {
		t.Fatalf("an update must say so: %v", updated["type"])
	}
	cr, ok := updated["conversationRow"].(map[string]any)
	if !ok || cr["phase"] != "Running" {
		t.Fatalf("a conversation delta must carry its listing row: %v", updated["conversationRow"])
	}
	cv, ok := updated["conversationView"].(map[string]any)
	if !ok || cv["conversation"] == nil || cv["yaml"] == "" {
		t.Fatalf("a conversation delta must carry its page: %v", updated["conversationView"])
	}
	// The transcript is NOT in it: messages arrive once, as message events, and
	// repeating them inside every later delta is what would make this heavier
	// than the re-fetch it replaces.
	if _, present := cv["transcript"]; present {
		t.Fatal("a conversation delta must not repeat the transcript")
	}

	// Pipeline detail is the stated exception: it carries the manager's
	// resolved capabilities, which the console must not recompute.
	pipeline := byName["pipelines/extra"]
	if pipeline["row"] == nil {
		t.Fatal("a pipeline delta still carries its listing row")
	}
	if pipeline["detail"] != nil {
		t.Fatal("pipeline detail is fetched, not streamed — the resolved capabilities are the manager's answer")
	}

	deleted := byName["channels/slack"]
	if deleted["type"] != "DELETED" || deleted["name"] != "slack" || deleted["kind"] != "channels" {
		t.Fatalf("a delete must carry its identity and type: %v", deleted)
	}
	if deleted["row"] != nil || deleted["detail"] != nil {
		t.Fatalf("a delete carries no object — there is none left: %v", deleted)
	}
}

// An applied view must be a shape a re-fetch would also produce.
//
// This is what makes applying safe rather than a second source of truth: the
// stream and the snapshot endpoints run the SAME projection over the same
// object, so a browser writing a delta into its cache cannot end up rendering
// something the server would never have sent it.
func TestDeltaShapesEqualTheSnapshots(t *testing.T) {
	objs := append(fixtureInstall(),
		obj("conversations", "conv-1", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Running","threads":[{"channel":"console","threadId":"th-1"}]}`))
	api, _, _, _ := apiWithOptions(t, "tok", true, objs...)
	h := api.Handler(http.NotFoundHandler())

	// A CHANNEL: row against the listing, detail against the kind page.
	channel := api.cache.Get("channels", "console")
	ev := api.deltaEventFor(Delta{Type: "MODIFIED", Kind: "channels", Name: "console", Object: channel}, "")

	var rows []InventoryRow
	getJSON(t, h, "/api/config/channels", &rows)
	var fetchedRow InventoryRow
	for _, r := range rows {
		if r.Name == "console" {
			fetchedRow = r
		}
	}
	if !jsonEqual(t, ev.Row, fetchedRow) {
		t.Fatalf("the streamed row is not the row the listing serves:\n stream: %s\n fetch:  %s",
			mustJSON(t, ev.Row), mustJSON(t, fetchedRow))
	}

	var fetchedDetail Detail
	getJSON(t, h, "/api/config/channels/console", &fetchedDetail)
	if !jsonEqual(t, ev.Detail, fetchedDetail) {
		t.Fatalf("the streamed detail is not the detail the kind page serves:\n stream: %s\n fetch:  %s",
			mustJSON(t, ev.Detail), mustJSON(t, fetchedDetail))
	}

	// A CONVERSATION: the listing row, and the page minus the two parts that
	// arrive on streams of their own.
	conv := api.cache.Get("conversations", "conv-1")
	cev := api.deltaEventFor(Delta{Type: "MODIFIED", Kind: "conversations", Name: "conv-1", Object: conv}, "")

	var page struct {
		Items []ConversationSummary `json:"items"`
	}
	getJSON(t, h, "/api/conversations", &page)
	var fetchedSummary ConversationSummary
	for _, c := range page.Items {
		if c.Name == "conv-1" {
			fetchedSummary = c
		}
	}
	if !jsonEqual(t, cev.ConversationRow, fetchedSummary) {
		t.Fatalf("the streamed conversation row is not the row the listing serves:\n stream: %s\n fetch:  %s",
			mustJSON(t, cev.ConversationRow), mustJSON(t, fetchedSummary))
	}

	var fetchedPage map[string]any
	getJSON(t, h, "/api/conversations/conv-1", &fetchedPage)
	for key, streamed := range cev.ConversationView {
		if !jsonEqual(t, streamed, fetchedPage[key]) {
			t.Fatalf("conversation view %q differs from the page:\n stream: %s\n fetch:  %s",
				key, mustJSON(t, streamed), mustJSON(t, fetchedPage[key]))
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// jsonEqual compares two values as the browser sees them — over the wire.
func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(mustJSON(t, a)), &av); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := json.Unmarshal([]byte(mustJSON(t, b)), &bv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return reflect.DeepEqual(av, bv)
}
