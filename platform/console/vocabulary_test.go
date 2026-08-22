package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The composer's typeahead listing. Its whole job is to agree with the
// manager's own vocabulary — the same list a chat transport registers as its
// command menu — so a person choosing from the menu and a person typing the
// command are told the same thing.
//
// These exercise the FALLBACK path: no manager client is wired in the test
// harness, so the listing is built from the console's own Pipeline view. That
// path must offer exactly what the projected one does.

func entriesUnderTest(t *testing.T, objs ...*Object) []Entry {
	t.Helper()
	api, _, _ := apiUnderTest(t, "tok", objs...)
	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/vocabulary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/vocabulary: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Entries
}

func TestVocabularyListingOffersReadyPipelinesWithTheirProfile(t *testing.T) {
	entries := entriesUnderTest(t,
		obj("pipelines", "ha-ops", "1", `{"profileRef":{"name":"ha-admin"}}`, cond("Ready", "True", "")),
		obj("pipelines", "ha-control", "1", `{"profileRef":{"name":"ha-user"}}`, cond("Ready", "True", "")),
	)
	if len(entries) != 2 {
		t.Fatalf("want both Ready pipelines, got %+v", entries)
	}
	// sorted, so the menu does not reshuffle between renders
	if entries[0].Name != "ha-control" || entries[1].Name != "ha-ops" {
		t.Fatalf("listing must be sorted by name: %+v", entries)
	}
	// the profile is what tells two pipelines apart when their names do not
	if entries[0].Profile != "ha-user" || entries[1].Profile != "ha-admin" {
		t.Fatalf("each entry must carry its answering profile: %+v", entries)
	}
}

func TestVocabularyListingExcludesUnreadyPipelines(t *testing.T) {
	// An unready pipeline names wiring that does not resolve. Offering it would
	// invite a request nothing can serve — and the listing command already hides it, so
	// showing it here would make one surface answer one question two ways.
	entries := entriesUnderTest(t,
		obj("pipelines", "good", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")),
		obj("pipelines", "broken", "1", `{"profileRef":{"name":"ops"}}`,
			cond("Ready", "False", "MissingReferences")),
		obj("pipelines", "unreconciled", "1", `{"profileRef":{"name":"ops"}}`, ""),
	)
	if len(entries) != 1 || entries[0].Name != "good" {
		t.Fatalf("only Ready pipelines may be offered: %+v", entries)
	}
}

func TestVocabularyListingIsEmptyRatherThanNull(t *testing.T) {
	// A surface with nothing addressable degrades to no menu, not a menu with
	// nothing in it — and the client must not have to defend against null.
	if entries := entriesUnderTest(t); len(entries) != 0 {
		t.Fatalf("want an empty listing, got %+v", entries)
	}
}

// A shared source is the case the typeahead exists for: both pipelines serve
// the console surface, both are addressable, and neither is a conflict.
func TestVocabularyListingOffersEveryServerOfASharedSource(t *testing.T) {
	entries := entriesUnderTest(t,
		obj("signalsources", "console", "1", `{"adapter":"console"}`, cond("Wired", "True", "PipelineClaim")),
		obj("pipelines", "ha-control", "1",
			`{"profileRef":{"name":"ha-user"},"signalSourceRefs":[{"name":"console"}]}`,
			cond("Ready", "True", "")),
		obj("pipelines", "ha-ops", "1",
			`{"profileRef":{"name":"ha-admin"},"signalSourceRefs":[{"name":"console"}]}`,
			cond("Ready", "True", "")),
	)
	if len(entries) != 2 {
		t.Fatalf("both servers of a shared source must be addressable: %+v", entries)
	}
}

// ---- the projected path ---------------------------------------------------

// The console CAN read Pipelines and fetches the manager's vocabulary anyway.
// Two derivations of one fact drift, and this module cannot import the
// manager's — they are separate Go modules by design.
func TestVocabularyIsProjectedFromTheManagerWhenReachable(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"revision":"rev-9","entries":[
			{"kind":"builtin","name":"exit","description":"Release the runtime","position":"thread"},
			{"kind":"pipeline","name":"k8s-observe","description":"k8s-engineer","position":"general","profile":"k8s-engineer"}
		]}`))
	}))
	defer srv.Close()

	api, _, _ := apiUnderTest(t, "tok",
		// Deliberately DIFFERENT from what the manager returns: a projection
		// that quietly fell back to this would look identical otherwise.
		obj("pipelines", "from-the-cache", "1", `{"profileRef":{"name":"p"}}`, cond("Ready", "True", "")),
	)
	api.mgr = NewManager(srv.URL, "tok")

	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/vocabulary", "")
	var out struct {
		Revision string  `json:"revision"`
		Entries  []Entry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if path != "/channel/vocabulary" {
		t.Fatalf("fetched %q", path)
	}
	if out.Revision != "rev-9" {
		t.Fatalf("revision = %q", out.Revision)
	}
	byName := map[string]Entry{}
	for _, e := range out.Entries {
		byName[e.Name] = e
	}
	if _, fellBack := byName["from-the-cache"]; fellBack {
		t.Fatalf("projection fell back to the Pipeline cache: %+v", out.Entries)
	}
	// POSITION is what the console can honour that a chat transport cannot: it
	// has two composers, and they take disjoint sets.
	if byName["exit"].Position != "thread" || byName["k8s-observe"].Position != "general" {
		t.Fatalf("positions not carried: %+v", out.Entries)
	}
	if byName["k8s-observe"].Profile != "k8s-engineer" {
		t.Fatalf("profile not carried: %+v", byName["k8s-observe"])
	}
}

// An unreachable manager degrades to the console's own view rather than to an
// empty composer.
func TestVocabularyFallsBackWhenTheManagerIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	api, _, _ := apiUnderTest(t, "tok",
		obj("pipelines", "still-here", "1", `{"profileRef":{"name":"ops"}}`, cond("Ready", "True", "")),
	)
	api.mgr = NewManager(srv.URL, "tok")

	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/vocabulary", "")
	var out struct {
		Entries []Entry `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Entries) != 1 || out.Entries[0].Name != "still-here" {
		t.Fatalf("fallback must still offer what this console can see: %+v", out.Entries)
	}
	if out.Entries[0].Position != "general" {
		t.Fatalf("fallback entry must carry a position: %+v", out.Entries[0])
	}
}
