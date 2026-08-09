package main

import (
	"testing"
)

// Topology is derived, never invented: every test here asserts that what the
// graph shows is exactly what some CR already says.

// staticCache builds a cache pre-populated without any API traffic.
func staticCache(objs ...*Object) *Cache {
	c := NewCache(&fakeSource{lists: []listResult{{rv: "1"}}, watches: []watchScript{{block: true}}}, Kinds)
	for _, o := range objs {
		c.apply("ADDED", o)
	}
	return c
}

func cond(t, status, reason string) string {
	return `{"conditions":[{"type":"` + t + `","status":"` + status + `","reason":"` + reason + `"}]}`
}

func findNode(topo Topology, kind, name string) *Node {
	for i := range topo.Nodes {
		if topo.Nodes[i].Kind == kind && topo.Nodes[i].Name == name {
			return &topo.Nodes[i]
		}
	}
	return nil
}

func hasEdge(topo Topology, from, to string) *Edge {
	for i := range topo.Edges {
		if topo.Edges[i].From == from && topo.Edges[i].To == to {
			return &topo.Edges[i]
		}
	}
	return nil
}

func TestTopologyHealthyPipelineRendersConnected(t *testing.T) {
	c := staticCache(
		obj("signalsources", "cron", "1", `{"adapter":"cron"}`,
			`{"conditions":[{"type":"Served","status":"True"},{"type":"Wired","status":"True"}]}`),
		obj("signaladapters", "cron", "1", `{"image":"x"}`, cond("Ready", "True", "")),
		obj("channels", "tg", "1", `{"adapter":"telegram"}`, cond("Served", "True", "")),
		obj("channels", "console", "1", `{"adapter":"console"}`, cond("Served", "True", "")),
		obj("channeladapters", "telegram", "1", `{"image":"x"}`, cond("Ready", "True", "")),
		obj("channeladapters", "console", "1", `{"image":"x"}`, cond("Ready", "True", "")),
		obj("agentprofiles", "ops", "1", `{"runtimeRef":{"name":"claude"}}`, cond("Ready", "True", "")),
		obj("pipelines", "nightly", "1",
			`{"signalSourceRefs":[{"name":"cron"}],"channelRefs":[{"name":"tg"},{"name":"console"}],"profileRef":{"name":"ops"}}`,
			cond("Ready", "True", "")),
	)
	topo := BuildTopology(c)

	for _, tc := range []struct {
		kind, name string
		want       Health
	}{
		{"signalsources", "cron", HealthOK}, {"pipelines", "nightly", HealthOK},
		{"channels", "tg", HealthOK}, {"channels", "console", HealthOK},
		// the profile asserts no health — nothing writes its conditions
		{"agentprofiles", "ops", HealthNone},
	} {
		n := findNode(topo, tc.kind, tc.name)
		if n == nil || n.Health != tc.want {
			t.Fatalf("%s/%s health: want %s, got %+v", tc.kind, tc.name, tc.want, n)
		}
		if n.Detached {
			t.Fatalf("%s/%s should be wired into the graph", tc.kind, tc.name)
		}
	}
	for _, e := range [][2]string{
		{"signalsources/cron", "pipelines/nightly"},
		{"pipelines/nightly", "agentprofiles/ops"},
		{"pipelines/nightly", "channels/tg"},
		{"pipelines/nightly", "channels/console"},
		{"channels/tg", "channeladapters/telegram"},
		{"signalsources/cron", "signaladapters/cron"},
	} {
		if edge := hasEdge(topo, e[0], e[1]); edge == nil || edge.Dangling {
			t.Fatalf("edge %s -> %s missing or dangling: %+v", e[0], e[1], edge)
		}
	}
}

func TestTopologyUnclaimedSourceIsVisiblyDropped(t *testing.T) {
	c := staticCache(
		obj("signalsources", "orphan", "1", `{"adapter":"cron"}`,
			`{"conditions":[{"type":"Served","status":"True"},`+
				`{"type":"Wired","status":"False","reason":"NoPipeline","message":"no Ready Pipeline claims this source; signals are dropped"}]}`),
		obj("signaladapters", "cron", "1", `{"image":"x"}`, cond("Ready", "True", "")),
	)
	topo := BuildTopology(c)
	n := findNode(topo, "signalsources", "orphan")
	if n == nil {
		t.Fatal("unclaimed source must still appear")
	}
	if n.Health != HealthBad {
		t.Fatalf("Wired=False must render unhealthy: %+v", n)
	}
	if !n.Detached {
		t.Fatal("a source no pipeline references is detached — that IS the state")
	}
	// the console shows the cluster's reason verbatim, it does not write one
	if n.Reason != "NoPipeline" || n.Message == "" {
		t.Fatalf("condition reason not surfaced: %+v", n)
	}
}

func TestTopologyUnservedAdapterReferenceIsDiagnosable(t *testing.T) {
	c := staticCache(
		obj("channels", "typo", "1", `{"adapter":"slak"}`,
			`{"conditions":[{"type":"Served","status":"False","reason":"NoAdapter","message":"no ChannelAdapter named slak"}]}`),
	)
	topo := BuildTopology(c)
	ch := findNode(topo, "channels", "typo")
	if ch == nil || ch.Health != HealthBad || ch.Reason != "NoAdapter" {
		t.Fatalf("channel should carry Served=False: %+v", ch)
	}
	edge := hasEdge(topo, "channels/typo", "channeladapters/slak")
	if edge == nil || !edge.Dangling {
		t.Fatalf("reference to a missing adapter must render as a dangling edge: %+v", edge)
	}
	if placeholder := findNode(topo, "channeladapters", "slak"); placeholder == nil || placeholder.Reason != "NotFound" {
		t.Fatalf("missing adapter needs a placeholder node: %+v", placeholder)
	}
}

func TestTopologyHealthIsUnknownWithoutConditions(t *testing.T) {
	c := staticCache(obj("pipelines", "fresh", "1", `{"profileRef":{"name":"p"}}`, "{}"))
	n := findNode(BuildTopology(c), "pipelines", "fresh")
	if n == nil || n.Health != HealthUnknown {
		t.Fatalf("a pipeline with no conditions is unknown, not healthy: %+v", n)
	}
}

// AgentProfile and AgentRuntime have a conditions field that no reconciler
// writes. Rendering them as "unknown" would park a pending-looking node on
// every graph forever, so they report no health instead — a distinction the
// live cluster made obvious.
func TestKindsThatAssertNoHealthRenderNeutral(t *testing.T) {
	c := staticCache(
		obj("agentprofiles", "k8s-engineer", "1", `{"runtimeRef":{"name":"default"}}`, ""),
		obj("pipelines", "p", "1", `{"profileRef":{"name":"k8s-engineer"}}`, cond("Ready", "True", "")),
	)
	topo := BuildTopology(c)
	profile := findNode(topo, "agentprofiles", "k8s-engineer")
	if profile == nil || profile.Health != HealthNone {
		t.Fatalf("a profile asserts no health: %+v", profile)
	}
	if profile.Reason != "" {
		t.Fatalf("no health means nothing to explain: %+v", profile)
	}
}

func TestActivityBadgesCountInflightConversations(t *testing.T) {
	pipeline := obj("pipelines", "busy", "1",
		`{"channelRefs":[{"name":"c"}],"profileRef":{"name":"ops"}}`, cond("Ready", "True", ""))
	idle := obj("pipelines", "idle", "1",
		`{"channelRefs":[{"name":"d"}],"profileRef":{"name":"other"}}`, cond("Ready", "True", ""))
	c := staticCache(pipeline, idle,
		obj("conversations", "conv1", "1",
			`{"channelRefs":[{"name":"c"}],"profileRef":{"name":"ops"}}`,
			`{"phase":"Working","inflight":{"runId":"r1"}}`),
		obj("conversations", "conv2", "1",
			`{"channelRefs":[{"name":"c"}],"profileRef":{"name":"ops"}}`,
			`{"phase":"Working","inflight":{"runId":"r2"}}`),
		obj("conversations", "conv3", "1",
			`{"channelRefs":[{"name":"c"}],"profileRef":{"name":"ops"}}`,
			`{"phase":"Idle"}`),
	)
	topo := BuildTopology(c)
	busy := findNode(topo, "pipelines", "busy")
	if busy == nil || busy.Active != 2 || busy.Recent != 3 {
		t.Fatalf("active/recent counts wrong: %+v", busy)
	}
	if n := findNode(topo, "pipelines", "idle"); n == nil || n.Active != 0 {
		t.Fatalf("idle pipeline must show no activity: %+v", n)
	}
}

// A Conversation carries no pipelineRef, so attribution is reconstructed from
// the bindings it materialized. These cases pin both the match and the refusal
// to guess.
func TestAttributePipeline(t *testing.T) {
	exact := obj("pipelines", "exact", "1",
		`{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"ops"}}`, "")
	twin := obj("pipelines", "twin", "1",
		`{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"ops"}}`, "")
	otherProfile := obj("pipelines", "other", "1",
		`{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"nope"}}`, "")
	subset := obj("pipelines", "subset", "1",
		`{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"ops"}}`, "")

	conv := obj("conversations", "c", "1", `{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"ops"}}`, "")

	if got := AttributePipeline(conv, []*Object{exact, otherProfile}); got != "exact" {
		t.Fatalf("exact binding match: got %q", got)
	}
	if got := AttributePipeline(conv, []*Object{exact, twin}); got != "" {
		t.Fatalf("two indistinguishable pipelines must stay unattributed, got %q", got)
	}
	// the router appends the originating channel, so a conversation's channel
	// set may be a superset of the pipeline's
	wider := obj("conversations", "c2", "1",
		`{"channelRefs":[{"name":"tg"},{"name":"console"}],"profileRef":{"name":"ops"}}`, "")
	if got := AttributePipeline(wider, []*Object{subset}); got != "subset" {
		t.Fatalf("superset channel set should still attribute: got %q", got)
	}
	// an explicit label wins when something eventually writes one
	labeled := obj("conversations", "c3", "1", `{"profileRef":{"name":"zzz"}}`, "")
	labeled.Metadata.Labels = map[string]string{LabelPipeline: "exact"}
	if got := AttributePipeline(labeled, []*Object{exact, twin}); got != "exact" {
		t.Fatalf("label must win: got %q", got)
	}
}

func TestUnjoinedPipelinesListsWhatNeedsAnEdit(t *testing.T) {
	c := staticCache(
		obj("pipelines", "joined", "1", `{"channelRefs":[{"name":"console"}],"profileRef":{"name":"a"}}`, ""),
		obj("pipelines", "solo", "1", `{"channelRefs":[{"name":"tg"}],"profileRef":{"name":"a"}}`, ""),
	)
	got := UnjoinedPipelines(c, "console")
	if len(got) != 1 || got[0] != "solo" {
		t.Fatalf("unjoined pipelines: %v", got)
	}
}
