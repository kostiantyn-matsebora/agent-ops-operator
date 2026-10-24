package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func hasInbound(refs []InboundRef, kind, name, field string) bool {
	for _, r := range refs {
		if r.Kind == kind && r.Name == name && r.Field == field {
			return true
		}
	}
	return false
}

// The coordination kinds are referenced from Pipelines and Coordinators, and
// each pointer shape gets its own row in the detail view's "used by" panel.
func TestInboundRefsCoverCoordinationPointers(t *testing.T) {
	pipeline := obj("pipelines", "p", "1", `{"capabilityRef":{"name":"cap"},"signalSourceRefs":[{"name":"src"}]}`, "{}")
	coord := obj("coordinators", "co", "1", `{
		"profileRef":{"name":"prof"},
		"signalSourceRefs":[{"name":"src"}],
		"channelRefs":[{"name":"chan"}],
		"toolsets":{"refs":[{"name":"ts"}]},
		"mcpConfigs":{"refs":[{"name":"mc"}]},
		"agents":[
			{"name":"a","capabilityRef":{"name":"cap"}},
			{"name":"b","coordinatorRef":{"name":"inner"}}
		]
	}`, "{}")
	inline := obj("coordinators", "inner", "1", `{"capabilityRef":{"name":"cap2"}}`, "{}")
	capability := obj("agentcapabilities", "cap", "1", `{"profileRef":{"name":"prof"},"runtimeRef":{"name":"rt"}}`, "{}")
	targets := []*Object{
		obj("agentprofiles", "prof", "1", "{}", "{}"),
		obj("signalsources", "src", "1", "{}", "{}"),
		obj("channels", "chan", "1", "{}", "{}"),
		obj("mcptoolsets", "ts", "1", "{}", "{}"),
		obj("mcpconfigs", "mc", "1", "{}", "{}"),
		obj("agentcapabilities", "cap2", "1", "{}", "{}"),
		obj("agentruntimes", "rt", "1", "{}", "{}"),
	}
	api, _, _ := apiUnderTest(t, "tok", append([]*Object{pipeline, coord, inline, capability}, targets...)...)

	if refs := api.inboundRefs("agentcapabilities", "cap"); !hasInbound(refs, "pipelines", "p", "capabilityRef") ||
		!hasInbound(refs, "coordinators", "co", "agents[a].capabilityRef") {
		t.Fatalf("agentcapabilities: %+v", refs)
	}
	if refs := api.inboundRefs("agentcapabilities", "cap2"); !hasInbound(refs, "coordinators", "inner", "capabilityRef") {
		t.Fatalf("agentcapabilities (inner): %+v", refs)
	}
	if refs := api.inboundRefs("coordinators", "inner"); !hasInbound(refs, "coordinators", "co", "agents[b].coordinatorRef") {
		t.Fatalf("coordinators: %+v", refs)
	}
	if refs := api.inboundRefs("agentprofiles", "prof"); !hasInbound(refs, "coordinators", "co", "profileRef") {
		t.Fatalf("agentprofiles: %+v", refs)
	}
	if refs := api.inboundRefs("signalsources", "src"); !hasInbound(refs, "coordinators", "co", "signalSourceRefs") {
		t.Fatalf("signalsources: %+v", refs)
	}
	if refs := api.inboundRefs("channels", "chan"); !hasInbound(refs, "coordinators", "co", "channelRefs") {
		t.Fatalf("channels: %+v", refs)
	}
	if refs := api.inboundRefs("mcptoolsets", "ts"); !hasInbound(refs, "coordinators", "co", "toolsets.refs") {
		t.Fatalf("mcptoolsets: %+v", refs)
	}
	if refs := api.inboundRefs("mcpconfigs", "mc"); !hasInbound(refs, "coordinators", "co", "mcpConfigs.refs") {
		t.Fatalf("mcpconfigs: %+v", refs)
	}
	if refs := api.inboundRefs("agentruntimes", "rt"); !hasInbound(refs, "agentcapabilities", "cap", "runtimeRef") {
		t.Fatalf("agentruntimes: %+v", refs)
	}
}

// A Coordinator referencing a capability carries no inline profile, toolsets
// or MCP configs, so those must not appear as references.
func TestInboundRefsIgnoreInlineFieldsBehindACapabilityRef(t *testing.T) {
	coord := obj("coordinators", "co", "1", `{
		"capabilityRef":{"name":"cap"},
		"profileRef":{"name":"prof"},
		"toolsets":{"refs":[{"name":"ts"}]},
		"mcpConfigs":{"refs":[{"name":"mc"}]}
	}`, "{}")
	api, _, _ := apiUnderTest(t, "tok", coord)
	for _, k := range []string{"agentprofiles", "mcptoolsets", "mcpconfigs"} {
		name := map[string]string{"agentprofiles": "prof", "mcptoolsets": "ts", "mcpconfigs": "mc"}[k]
		if refs := api.inboundRefs(k, name); len(refs) != 0 {
			t.Fatalf("%s: %+v", k, refs)
		}
	}
}

func TestInventoryColumnsCoverCoordinationKinds(t *testing.T) {
	byCap := obj("pipelines", "p1", "1", `{"capabilityRef":{"name":"cap"},"signalSourceRefs":[{"name":"src"}]}`, "{}")
	capability := obj("agentcapabilities", "cap", "1", `{
		"profileRef":{"name":"prof"},"runtimeRef":{"name":"rt"},
		"toolsets":{"refs":[{"name":"ts"}]},"mcpConfigs":{"refs":[{"name":"mc"}]}
	}`, "{}")
	coordCap := obj("coordinators", "co1", "1", `{"capabilityRef":{"name":"cap"},"signalSourceRefs":[{"name":"src"}],"channelRefs":[{"name":"chan"}],"agents":[{"name":"a"},{"name":"b"}]}`, "{}")
	coordInline := obj("coordinators", "co2", "1", `{"profileRef":{"name":"prof"}}`, "{}")
	convo := obj("conversations", "c", "1", `{"profileRef":{"name":"prof"},"coordinatorRef":{"name":"co1"},"causedBy":{"parent":"root","entry":"a"}}`, `{"phase":"Working"}`)
	api, _, _ := apiUnderTest(t, "tok", byCap, capability, coordCap, coordInline, convo)
	h := api.Handler(http.NotFoundHandler())

	columns := func(kind string) []map[string]string {
		t.Helper()
		rec := authed(t, h, "GET", "/api/config/"+kind, "")
		var rows []InventoryRow
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		var out []map[string]string
		for _, r := range rows {
			out = append(out, r.Columns)
		}
		return out
	}

	if c := columns("pipelines")[0]; c["capability"] != "cap" || c["profile"] != "" {
		t.Fatalf("pipelines: %+v", c)
	}
	if c := columns("agentcapabilities")[0]; c["profile"] != "prof" || c["runtime"] != "rt" || c["toolsets"] != "ts" || c["mcpConfigs"] != "mc" {
		t.Fatalf("agentcapabilities: %+v", c)
	}
	coords := columns("coordinators")
	if len(coords) != 2 {
		t.Fatalf("coordinators: %+v", coords)
	}
	if c := coords[0]; c["capability"] != "cap" || c["sources"] != "src" || c["escalatesTo"] != "chan" || c["agents"] != "a, b" {
		t.Fatalf("coordinators[0]: %+v", c)
	}
	if c := coords[1]; c["profile"] != "prof" || c["capability"] != "" {
		t.Fatalf("coordinators[1]: %+v", c)
	}
	if c := columns("conversations")[0]; c["coordinator"] != "co1" || c["causedBy"] != "root" {
		t.Fatalf("conversations: %+v", c)
	}
}

// A source only a Coordinator claims is still claimed.
func TestClaimedSourcesIncludesCoordinators(t *testing.T) {
	api, _, _ := apiUnderTest(t, "tok",
		obj("pipelines", "p", "1", `{"profileRef":{"name":"x"},"signalSourceRefs":[{"name":"a"}]}`, "{}"),
		obj("coordinators", "co", "1", `{"profileRef":{"name":"x"},"signalSourceRefs":[{"name":"b"}]}`, "{}"))
	got := api.claimedSources()
	if !got["a"] || !got["b"] || got["c"] {
		t.Fatalf("claimed: %+v", got)
	}
}
