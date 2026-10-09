package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// withOriginReader stamps a conversation's spec.originReader, as the manager
// does once at creation for whoever started it. TrimSuffix on the OUTER
// closing brace, never Replace: the spec already nests objects of its own
// (profileRef), so the first "}" in the string is never the outer one.
func withOriginReader(o *Object, channel, key string) *Object {
	o.Spec = json.RawMessage(strings.TrimSuffix(string(o.Spec), "}") +
		`,"originReader":{"channel":"` + channel + `","key":"` + key + `"}}`)
	return o
}

// withCoordinatorRef stamps a conversation's spec.coordinatorRef, marking it
// as that Coordinator's own root.
func withCoordinatorRef(o *Object, coordinator string) *Object {
	o.Spec = json.RawMessage(strings.TrimSuffix(string(o.Spec), "}") +
		`,"coordinatorRef":{"name":"` + coordinator + `"}}`)
	return o
}

// Mine narrows to conversations the requesting reader themselves started.
// There is no "Incidents" filter or scope — that concept was removed outright
// (item #26): a Coordinator's root is reached by clicking the Coordinator
// itself under PIPELINES & COORDINATORS, counted below by its OWN name
// (scopes[coordinator name]), which is the one count that survives.
func TestMineFilterAndPerCoordinatorScopeCount(t *testing.T) {
	// the reader key is a pure function of the salt, so it is computable
	// before the fixture that must carry it exists.
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	api, _, _, _ := apiWithOptions(t, "tok", true,
		withOriginReader(convWithRuns("started-by-alice", "", [2]string{"r", tLate}), "console", aliceKey),
		convWithRuns("started-by-nobody-in-particular", "", [2]string{"r", tLate}),
		withCoordinatorRef(convWithRuns("a-coordinator-root", "", [2]string{"r", tLate}), "triage"),
		obj("coordinators", "triage", "1", "{}", "{}"),
	)
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	mine := getListAs(t, h, "/api/conversations?mine=true", "alice@example.com")
	if mine.Total != 1 || len(mine.Items) != 1 || mine.Items[0].Name != "started-by-alice" {
		t.Fatalf("mine filter: %+v", mine)
	}
	if !mine.Items[0].Mine {
		t.Fatalf("the row itself must report mine=true: %+v", mine.Items[0])
	}

	// scope counts ride the count-only form, unread within each scope
	var scoped struct {
		Scopes map[string]int `json:"scopes"`
	}
	rec := identified(t, h, "GET", "/api/conversations?count=1", "", "alice@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("count-only: %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &scoped); err != nil {
		t.Fatal(err)
	}
	if scoped.Scopes["mine"] != 1 {
		t.Fatalf("scope count for mine: %+v", scoped.Scopes)
	}
	if _, stillThere := scoped.Scopes["incidents"]; stillThere {
		t.Fatalf("the retired 'incidents' scope must not be computed at all: %+v", scoped.Scopes)
	}
	if scoped.Scopes["triage"] != 1 {
		t.Fatalf("scope count for the Coordinator's own name: %+v", scoped.Scopes)
	}
}

// getListAs is getList, but as a named identity rather than the shared token.
func getListAs(t *testing.T, h http.Handler, path, who string) listResponse {
	t.Helper()
	rec := identified(t, h, "GET", path, "", who)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	var out listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s body %q: %v", path, rec.Body.String(), err)
	}
	return out
}
