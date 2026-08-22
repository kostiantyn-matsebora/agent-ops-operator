package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The live defect this exists for, verbatim from Home Assistant's own
// `tools/list`: `domain` is a union whose first branch is an EMPTY schema, so
// the parameter has no declared type and a model writes `{"domain": sensor}` —
// not valid JSON, discarded by the client, never sent. Its neighbour `name`,
// typed `string`, comes out quoted in the same call.
const haListing = `{"jsonrpc":"2.0","id":2,"result":{"tools":[
 {"name":"GetLiveContext","description":"Provides real-time information...",
  "inputSchema":{"type":"object","properties":{
    "name":{"description":"Filter entities by name or alias.","type":"string"},
    "domain":{"anyOf":[{},{"type":"array","items":{"type":"string"}}],
              "description":"Filter entities by domain. Accepts a single domain or a list."},
    "area":{"description":"Filter entities by area.","type":"string"}}}},
 {"name":"HassTurnOn","inputSchema":{"type":"object","properties":{
    "name":{"type":"string"}}}}]}}`

func toolsOf(t *testing.T, body []byte) map[string]map[string]any {
	t.Helper()
	var msg struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	out := map[string]map[string]any{}
	for _, tool := range msg.Result.Tools {
		out[tool["name"].(string)] = tool
	}
	return out
}

func propOf(t *testing.T, tool map[string]any, name string) map[string]any {
	t.Helper()
	schema, _ := tool["inputSchema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	p, _ := props[name].(map[string]any)
	if p == nil {
		t.Fatalf("no property %q", name)
	}
	return p
}

func TestATypelessBranchIsDroppedSoTheSchemaCanBeSatisfied(t *testing.T) {
	state := &policy{}
	state.set([]string{"*"})

	out, changed := filterListing([]byte(haListing), "homeassistant", state)
	if !changed {
		t.Fatal("a listing carrying an unsatisfiable schema must be rewritten")
	}
	domain := propOf(t, toolsOf(t, out)["GetLiveContext"], "domain")

	if _, still := domain["anyOf"]; still {
		t.Fatalf("the union survived: %v", domain)
	}
	// What is left is the branch the SERVER published — an array of strings —
	// and nothing was invented to replace the one that said nothing.
	if domain["type"] != "array" {
		t.Fatalf("want the server's own surviving branch inlined, got %v", domain)
	}
	items, _ := domain["items"].(map[string]any)
	if items["type"] != "string" {
		t.Fatalf("the surviving branch lost its items: %v", domain)
	}
	// The description is the only guidance a model has about what a domain IS.
	if !strings.Contains(domain["description"].(string), "Accepts a single domain") {
		t.Fatalf("the description was dropped: %v", domain)
	}
}

func TestTypedPropertiesAreUntouched(t *testing.T) {
	state := &policy{}
	state.set([]string{"*"})

	out, _ := filterListing([]byte(haListing), "homeassistant", state)
	tools := toolsOf(t, out)
	for _, prop := range []string{"name", "area"} {
		p := propOf(t, tools["GetLiveContext"], prop)
		if p["type"] != "string" {
			t.Fatalf("%s changed: %v", prop, p)
		}
	}
	if n := propOf(t, tools["HassTurnOn"], "name"); n["type"] != "string" {
		t.Fatalf("a tool with no union was rewritten: %v", n)
	}
}

// A union of typed branches is a schema a model CAN satisfy. Rewriting it would
// be the proxy deciding what a server meant, which is the thing this repair is
// careful not to do.
func TestAUnionOfTypedBranchesIsLeftAlone(t *testing.T) {
	body := `{"result":{"tools":[{"name":"x","inputSchema":{"type":"object","properties":{
	  "p":{"anyOf":[{"type":"string"},{"type":"array","items":{"type":"string"}}]}}}}]}}`
	state := &policy{}
	state.set([]string{"*"})

	_, changed := filterListing([]byte(body), "srv", state)
	if changed {
		t.Fatal("a satisfiable union must pass through untouched")
	}
}

// Nothing left to say is worse than something useless: a property whose ONLY
// branch is empty keeps it, rather than becoming a silently empty schema.
func TestAWhollyEmptyUnionIsLeftAlone(t *testing.T) {
	body := `{"result":{"tools":[{"name":"x","inputSchema":{"type":"object","properties":{
	  "p":{"anyOf":[{},{}],"description":"anything"}}}}]}}`
	state := &policy{}
	state.set([]string{"*"})

	_, changed := filterListing([]byte(body), "srv", state)
	if changed {
		t.Fatal("with every branch empty there is nothing to repair toward")
	}
}

// The repair must not become a way to see tools the wiring did not grant, and
// filtering must not become a way to skip the repair.
func TestRepairAndFilterBothApply(t *testing.T) {
	state := &policy{}
	state.set([]string{"mcp__homeassistant__GetLiveContext"})

	out, changed := filterListing([]byte(haListing), "homeassistant", state)
	if !changed {
		t.Fatal("an ungranted tool must be removed")
	}
	tools := toolsOf(t, out)
	if _, present := tools["HassTurnOn"]; present {
		t.Fatal("an ungranted tool was advertised")
	}
	if _, still := propOf(t, tools["GetLiveContext"], "domain")["anyOf"]; still {
		t.Fatal("the surviving tool kept its unsatisfiable schema")
	}
}

// oneOf carries the same defect and the same fix.
func TestOneOfIsRepairedToo(t *testing.T) {
	body := `{"result":{"tools":[{"name":"x","inputSchema":{"type":"object","properties":{
	  "p":{"oneOf":[{},{"type":"number"}]}}}}]}}`
	state := &policy{}
	state.set([]string{"*"})

	out, changed := filterListing([]byte(body), "srv", state)
	if !changed {
		t.Fatal("oneOf with an empty branch is as unsatisfiable as anyOf")
	}
	if propOf(t, toolsOf(t, out)["x"], "p")["type"] != "number" {
		t.Fatal("the typed branch was not inlined")
	}
}
