package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// The whole flow must animate, not just its first hop.
//
// A first look at the live graph showed only signal-source → pipeline moving,
// while every other hop travelled in silence. Three separate reasons, each
// pinned below: direction, multi-edge paths, and endpoints that are not wiring
// nodes at all.

func trafficFixture() []*Object {
	return []*Object{
		obj("pipelines", "k8s-ops", "1",
			`{"profileRef":{"name":"k8s-engineer"},"signalSourceRefs":[{"name":"cluster-events"}],`+
				`"channelRefs":[{"name":"console"}]}`,
			cond("Ready", "True", "")),
		obj("agentprofiles", "k8s-engineer", "1", `{"runtimeRef":{"name":"default"}}`, "{}"),
		obj("agentruntimes", "default", "1", `{"image":"x:1"}`, "{}"),
		obj("signalsources", "cluster-events", "1", `{"adapter":"k8s-events"}`, cond("Served", "True", "")),
		obj("signaladapters", "k8s-events", "1", `{"image":"x:1"}`, cond("Ready", "True", "")),
		obj("channels", "console", "1", `{"adapter":"console"}`, cond("Served", "True", "")),
		obj("channeladapters", "console", "1", `{"image":"x:1"}`, cond("Ready", "True", "")),
		obj("conversations", "chat-1", "1",
			`{"profileRef":{"name":"k8s-engineer"},"channelRefs":[{"name":"console"}]}`,
			`{"phase":"Idle"}`),
	}
}

// trafficOn renders the topology and returns event counts by drawn edge.
func trafficOn(t *testing.T, api *API) map[string]int {
	t.Helper()
	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/topology?windowSeconds=60", "")
	if rec.Code != 200 {
		t.Fatalf("topology: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Topology Topology `json:"topology"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, e := range out.Topology.Edges {
		if e.Traffic != nil {
			got[e.From+"->"+e.To] = e.Traffic.Events
		}
	}
	return got
}

func TestEveryHopLightsTheEdgeItCrossed(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, trafficFixture()...)
	now := time.Now().UTC()
	seq := 0
	emit := func(kind string, from, to *NodeRef, pipeline, conv string) {
		seq++
		api.activity.add(ActivityEvent{
			Cursor: pad(seq), TS: now, Kind: kind, Status: "ok",
			From: from, To: to, Pipeline: pipeline, Conversation: conv,
		})
	}

	// the real order of a conversation, as the manager emits it
	emit("signal.received", &NodeRef{"signal-adapter", "k8s-events"}, &NodeRef{"signal-source", "cluster-events"}, "", "")
	emit("signal.claimed", &NodeRef{"signal-source", "cluster-events"}, &NodeRef{"pipeline", "k8s-ops"}, "k8s-ops", "")
	emit("run.dispatched", &NodeRef{"pipeline", "k8s-ops"}, &NodeRef{"runtime", "default"}, "k8s-ops", "chat-1")
	emit("run.completed", &NodeRef{"runtime", "default"}, &NodeRef{"pipeline", "k8s-ops"}, "k8s-ops", "chat-1")
	emit("channel.op.enqueued", &NodeRef{"conversation", "chat-1"}, &NodeRef{"channel", "console"}, "", "chat-1")
	emit("channel.op.completed", &NodeRef{"channel-adapter", "console"}, &NodeRef{"channel", "console"}, "", "chat-1")

	got := trafficOn(t, api)

	// 1. DIRECTION: the signal travelled adapter → source; the graph draws
	//    source → adapter. Undirected matching is what makes that edge light.
	if got["signalsources/cluster-events->signaladapters/k8s-events"] == 0 {
		t.Fatalf("served-by edge did not light for signal.received: %+v", got)
	}
	// the claim, which already worked
	if got["signalsources/cluster-events->pipelines/k8s-ops"] == 0 {
		t.Fatalf("feeds edge did not light: %+v", got)
	}
	// 2. PATHS: a run is one hop pipeline↔runtime, but the wiring reaches the
	//    runtime through the profile, so BOTH legs carry the run.
	if got["pipelines/k8s-ops->agentprofiles/k8s-engineer"] == 0 {
		t.Fatalf("answers edge did not light for the run: %+v", got)
	}
	if got["agentprofiles/k8s-engineer->agentruntimes/default"] == 0 {
		t.Fatalf("uses edge did not light for the run: %+v", got)
	}
	// 3. NON-WIRING ENDPOINTS: an op names a conversation, which the wiring
	//    graph has no node for. It is credited to that conversation's pipeline.
	if got["pipelines/k8s-ops->channels/console"] == 0 {
		t.Fatalf("posts edge did not light for the channel op: %+v", got)
	}
	// ...and the adapter's delivery lights the channel's served-by edge
	if got["channels/console->channeladapters/console"] == 0 {
		t.Fatalf("channel served-by edge did not light for the delivery: %+v", got)
	}

	// Nothing invented: every lit edge is one the wiring actually contains.
	for id := range got {
		switch id {
		case "signalsources/cluster-events->signaladapters/k8s-events",
			"signalsources/cluster-events->pipelines/k8s-ops",
			"pipelines/k8s-ops->agentprofiles/k8s-engineer",
			"agentprofiles/k8s-engineer->agentruntimes/default",
			"pipelines/k8s-ops->channels/console",
			"channels/console->channeladapters/console":
		default:
			t.Fatalf("traffic credited to an edge no hop crossed: %s", id)
		}
	}
}

// A hop that resolves to no wiring pair at all stays uncredited — inventing an
// edge for it would draw wiring that does not exist.
func TestUnattributableHopLightsNothing(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, trafficFixture()...)
	api.activity.add(ActivityEvent{
		Cursor: pad(1), TS: time.Now(), Kind: "conversation.created", Status: "ok",
		// a conversation nothing in the cache attributes, and the manager node
		From: &NodeRef{"manager", "manager"}, To: &NodeRef{"conversation", "ghost"},
	})
	if got := trafficOn(t, api); len(got) != 0 {
		t.Fatalf("an unattributable hop must credit no edge: %+v", got)
	}
}

func pad(n int) string {
	s := "0000000000000000" + itoa(n)
	return s[len(s)-16:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
