package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The composer's typeahead listing. Its whole job is to agree with `/agents`:
// both filter to Ready pipelines, so a person choosing from the menu and a
// person typing the command are told the same thing.

func agentsUnderTest(t *testing.T, objs ...*Object) []Agent {
	t.Helper()
	api, _, _ := apiUnderTest(t, "tok", objs...)
	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/agents", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/agents: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Agents []Agent `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Agents
}

func TestAgentsListingOffersReadyPipelinesWithTheirProfile(t *testing.T) {
	agents := agentsUnderTest(t,
		obj("pipelines", "ha-ops", "1", `{"profileRef":{"name":"ha-admin"}}`, cond("Ready", "True", "")),
		obj("pipelines", "ha-control", "1", `{"profileRef":{"name":"ha-user"}}`, cond("Ready", "True", "")),
	)
	if len(agents) != 2 {
		t.Fatalf("want both Ready pipelines, got %+v", agents)
	}
	// sorted, so the menu does not reshuffle between renders
	if agents[0].Name != "ha-control" || agents[1].Name != "ha-ops" {
		t.Fatalf("listing must be sorted by name: %+v", agents)
	}
	// the profile is what tells two agents apart when their names do not
	if agents[0].Profile != "ha-user" || agents[1].Profile != "ha-admin" {
		t.Fatalf("each entry must carry its answering profile: %+v", agents)
	}
}

func TestAgentsListingExcludesUnreadyPipelines(t *testing.T) {
	// An unready pipeline names wiring that does not resolve. Offering it would
	// invite a request nothing can serve — and `/agents` already hides it, so
	// showing it here would make one surface answer one question two ways.
	agents := agentsUnderTest(t,
		obj("pipelines", "good", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")),
		obj("pipelines", "broken", "1", `{"profileRef":{"name":"ops"}}`,
			cond("Ready", "False", "MissingReferences")),
		obj("pipelines", "unreconciled", "1", `{"profileRef":{"name":"ops"}}`, ""),
	)
	if len(agents) != 1 || agents[0].Name != "good" {
		t.Fatalf("only Ready pipelines may be offered: %+v", agents)
	}
}

func TestAgentsListingIsEmptyRatherThanNull(t *testing.T) {
	// A surface with nothing addressable degrades to no menu, not a menu with
	// nothing in it — and the client must not have to defend against null.
	if agents := agentsUnderTest(t); len(agents) != 0 {
		t.Fatalf("want an empty listing, got %+v", agents)
	}
}

// A shared source is the case the typeahead exists for: both pipelines serve
// the console surface, both are addressable, and neither is a conflict.
func TestAgentsListingOffersEveryServerOfASharedSource(t *testing.T) {
	agents := agentsUnderTest(t,
		obj("signalsources", "console", "1", `{"adapter":"console"}`, cond("Wired", "True", "PipelineClaim")),
		obj("pipelines", "ha-control", "1",
			`{"profileRef":{"name":"ha-user"},"signalSourceRefs":[{"name":"console"}]}`,
			cond("Ready", "True", "")),
		obj("pipelines", "ha-ops", "1",
			`{"profileRef":{"name":"ha-admin"},"signalSourceRefs":[{"name":"console"}]}`,
			cond("Ready", "True", "")),
	)
	if len(agents) != 2 {
		t.Fatalf("both servers of a shared source must be addressable: %+v", agents)
	}
}
