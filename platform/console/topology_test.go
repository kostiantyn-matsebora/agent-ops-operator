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

// A Pipeline's icon reaches the graph as it reaches the list: verbatim, and
// only where one is declared.
func TestTopologyPublishesThePipelineIcon(t *testing.T) {
	c := staticCache(
		obj("pipelines", "iconed", "1", `{"profileRef":{"name":"p"},"icon":"aops:kubernetes"}`, "{}"),
		obj("pipelines", "plain", "1", `{"profileRef":{"name":"p"}}`, "{}"),
	)
	topo := BuildTopology(c)
	if got := findNode(topo, "pipelines", "iconed").Icon; got != "aops:kubernetes" {
		t.Fatalf("icon = %q, want aops:kubernetes", got)
	}
	if got := findNode(topo, "pipelines", "plain").Icon; got != "" {
		t.Fatalf("icon = %q, want none", got)
	}
}

// A Pipeline naming capabilityRef draws ONE edge to the AgentCapability, never
// the inline profile/toolset/runtime edges — the two are mutually exclusive on
// the CRD, and the capability itself carries the rest transitively.
func TestTopologyPipelineCapabilityRefDrawsOneEdge(t *testing.T) {
	c := staticCache(
		obj("agentprofiles", "ops", "1", `{}`, cond("Ready", "True", "")),
		obj("agentcapabilities", "shared", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")),
		obj("pipelines", "viaref", "1", `{"capabilityRef":{"name":"shared"}}`, cond("Ready", "True", "")),
	)
	topo := BuildTopology(c)
	if e := hasEdge(topo, "pipelines/viaref", "agentcapabilities/shared"); e == nil || e.Kind != "capability" {
		t.Fatalf("pipeline -> capability edge missing: %+v", e)
	}
	if e := hasEdge(topo, "pipelines/viaref", "agentprofiles/ops"); e != nil {
		t.Fatalf("pipeline must not draw the inline profile edge when capabilityRef is set: %+v", e)
	}
	if e := hasEdge(topo, "agentcapabilities/shared", "agentprofiles/ops"); e == nil || e.Kind != "answers" {
		t.Fatalf("capability -> profile edge missing: %+v", e)
	}
	if n := findNode(topo, "agentcapabilities", "shared"); n == nil || n.Detached {
		t.Fatalf("a referenced capability must not be detached: %+v", n)
	}
}

// An AgentCapability nothing references is DETACHED (the ordinary state of a
// shareable object) but that is independent of its Health, which comes only
// from its own Ready condition.
func TestTopologyUnwiredCapabilityIsDistinctFromMisconfigured(t *testing.T) {
	c := staticCache(
		obj("agentcapabilities", "unused", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")),
		obj("agentcapabilities", "broken", "1", `{"profileRef":{"name":"missing"}}`, cond("Ready", "False", "MissingReference")),
	)
	topo := BuildTopology(c)
	unused := findNode(topo, "agentcapabilities", "unused")
	if unused == nil || !unused.Detached || unused.Health != HealthOK {
		t.Fatalf("unused capability: want detached+ok, got %+v", unused)
	}
	broken := findNode(topo, "agentcapabilities", "broken")
	if broken == nil || !broken.Detached || broken.Health != HealthBad {
		t.Fatalf("broken, unreferenced capability: want detached+bad, got %+v", broken)
	}
}

// A Coordinator claims sources, escalates to channels (never an ordinary
// post), and invokes its agents[] entries — an AgentCapability member and a
// nested Coordinator alike.
func TestTopologyCoordinatorEdges(t *testing.T) {
	c := staticCache(
		obj("signalsources", "alerts", "1", `{"adapter":"cron"}`, cond("Served", "True", "")),
		obj("channels", "ops-room", "1", `{"adapter":"telegram"}`, cond("Served", "True", "")),
		obj("agentprofiles", "lead", "1", `{}`, cond("Ready", "True", "")),
		obj("agentcapabilities", "member-cap", "1", `{"profileRef":{"name":"lead"}}`, cond("Ready", "True", "")),
		obj("coordinators", "nested", "1", `{"profileRef":{"name":"lead"}}`, cond("Ready", "True", "")),
		obj("coordinators", "incident", "1",
			`{"signalSourceRefs":[{"name":"alerts"}],"channelRefs":[{"name":"ops-room"}],`+
				`"profileRef":{"name":"lead"},"agents":[`+
				`{"name":"triage","capabilityRef":{"name":"member-cap"},"description":"triage"},`+
				`{"name":"escalation","coordinatorRef":{"name":"nested"},"description":"nest"}]}`,
			cond("Ready", "True", "")),
	)
	topo := BuildTopology(c)
	if e := hasEdge(topo, "signalsources/alerts", "coordinators/incident"); e == nil || e.Kind != "feeds" {
		t.Fatalf("source -> coordinator feeds edge missing: %+v", e)
	}
	if e := hasEdge(topo, "coordinators/incident", "channels/ops-room"); e == nil || e.Kind != "escalates-to" {
		t.Fatalf("coordinator -> channel escalates-to edge missing: %+v", e)
	}
	if e := hasEdge(topo, "coordinators/incident", "agentprofiles/lead"); e == nil || e.Kind != "answers" {
		t.Fatalf("coordinator's own inline capability edge missing: %+v", e)
	}
	if e := hasEdge(topo, "coordinators/incident", "agentcapabilities/member-cap"); e == nil || e.Kind != "invokes" {
		t.Fatalf("coordinator -> member capability invokes edge missing: %+v", e)
	}
	if e := hasEdge(topo, "coordinators/incident", "coordinators/nested"); e == nil || e.Kind != "invokes" {
		t.Fatalf("coordinator -> nested coordinator invokes edge missing: %+v", e)
	}
	if n := findNode(topo, "coordinators", "incident"); n == nil || n.Detached {
		t.Fatalf("a coordinator must never be treated as a detached leaf: %+v", n)
	}
}

// A Coordinator-rooted conversation records spec.coordinatorRef directly —
// no inference, unlike AttributePipeline's fallback for conversations that
// predate spec.pipelineRef.
func TestAttributeCoordinator(t *testing.T) {
	root := obj("coordinators", "incident", "1", `{}`, "")
	conv := obj("conversations", "c", "1", `{"coordinatorRef":{"name":"incident"}}`, "")
	if got := AttributeCoordinator(conv, []*Object{root}); got != "incident" {
		t.Fatalf("got %q, want incident", got)
	}
	plain := obj("conversations", "c2", "1", `{}`, "")
	if got := AttributeCoordinator(plain, []*Object{root}); got != "" {
		t.Fatalf("a conversation with no coordinatorRef must attribute to none, got %q", got)
	}
}

// coordinatorRef is provenance, snapshotted once at creation — it SHALL
// survive the Coordinator being edited or deleted, the same guarantee
// escalate() and the budget snapshot already carry. A conversation whose
// Coordinator is gone (deleted, or never synced into this list) must still
// attribute by name, never silently fall back to "".
func TestAttributeCoordinatorSurvivesCoordinatorDeletion(t *testing.T) {
	conv := obj("conversations", "c", "1", `{"coordinatorRef":{"name":"incident"}}`, "")
	if got := AttributeCoordinator(conv, nil); got != "incident" {
		t.Fatalf("attribution must not depend on the Coordinator still existing, got %q", got)
	}
	if got := AttributeCoordinator(conv, []*Object{obj("coordinators", "unrelated", "1", `{}`, "")}); got != "incident" {
		t.Fatalf("attribution must not depend on the live list naming it, got %q", got)
	}
}
