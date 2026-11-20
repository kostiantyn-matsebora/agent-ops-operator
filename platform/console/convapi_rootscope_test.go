package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// withCausedBy stamps a conversation's spec.causedBy, marking it as a member
// invoked by the named parent — the one fact isMember (and memberParent)
// read.
func withCausedBy(o *Object, parent, entry string) *Object {
	o.Spec = json.RawMessage(strings.TrimSuffix(string(o.Spec), "}") +
		`,"causedBy":{"parent":"` + parent + `","entry":"` + entry + `"}}`)
	return o
}

type countedResponse struct {
	Total  int            `json:"total"`
	Unread int            `json:"unreadTotal"`
	Scopes map[string]int `json:"scopes"`
}

func getCounted(t *testing.T, h http.Handler, path string) countedResponse {
	t.Helper()
	rec := identified(t, h, "GET", path, "", "alice@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	var out countedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Open is the one scope every OTHER non-Closed scope is a further predicate
// on: UNREAD roots with phase≠Closed, excluding a member and excluding a
// closed-but-unread row (that one is Closed's own). unreadTotal is this same
// number, not a separate computation.
func TestOpenScopeCountsUnreadNonClosedRootsExcludingClosedAndMembers(t *testing.T) {
	openUnread := convWithPhase("open-unread", "Idle", "", "succeeded")
	openRead := convWithPhase("open-read", "Idle", tLate, "succeeded")
	closedUnread := convWithPhase("closed-unread", "Closed", "", "succeeded")
	member := withCausedBy(convWithRuns("member-of-open-unread", "", [2]string{"x", tLate}), "open-unread", "k8s-observe")

	api, _, _, _ := apiWithOptions(t, "tok", true, openUnread, openRead, closedUnread, member)
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["open"] != 1 {
		t.Fatalf("open must count only the one unread, non-Closed root: %+v", got.Scopes)
	}
	if got.Unread != got.Scopes["open"] {
		t.Fatalf("unreadTotal must equal scopes.open exactly, got unreadTotal=%d scopes.open=%d", got.Unread, got.Scopes["open"])
	}
}

// 1.2: Closed counts unread roots with phase=Closed — a read closed row
// belongs in the Closed LIST (it's the archive) but no longer inflates this
// badge, which now promises exactly the unread rows the Closed scope itself
// would highlight.
func TestClosedScopeCountsOnlyUnreadClosedRoots(t *testing.T) {
	objs := []*Object{
		convWithPhase("closed-unread-1", "Closed", "", "succeeded"),
		convWithPhase("closed-unread-2", "Closed", "", "succeeded"),
		convWithPhase("closed-read-1", "Closed", tLate, "succeeded"),
		convWithPhase("closed-read-2", "Closed", tLate, "succeeded"),
		convWithPhase("closed-read-3", "Closed", tLate, "succeeded"),
		convWithPhase("closed-read-4", "Closed", tLate, "succeeded"),
	}
	api, _, _, _ := apiWithOptions(t, "tok", true, objs...)
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["closed"] != 2 {
		t.Fatalf("two of six closed roots are unread, want closed=2: %+v", got.Scopes)
	}
}

// 1.3: neither a member nor a closed root ever contributes to Open or to
// unreadTotal — a member by isMember's own rule, a closed root because Open
// requires phase≠Closed. The closed root still belongs to its OWN scope,
// which this test also pins so the exclusion is not mistaken for "closed
// rows are dropped everywhere."
func TestMemberAndAClosedRootCountInNeitherOpenNorUnreadTotal(t *testing.T) {
	member := withCausedBy(convWithRuns("an-unread-member", "", [2]string{"x", tLate}), "some-root", "k8s-observe")
	closedRoot := convWithPhase("an-unread-closed-root", "Closed", "", "succeeded")

	api, _, _, _ := apiWithOptions(t, "tok", true, member, closedRoot)
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["open"] != 0 || got.Unread != 0 {
		t.Fatalf("neither the member nor the closed root may count toward open/unreadTotal: %+v", got)
	}
	if got.Scopes["closed"] != 1 {
		t.Fatalf("the closed root must still count in its own scope: %+v", got.Scopes)
	}
}

// 1.4: showClosed is gone from the server's vocabulary entirely, not merely
// ignored by coincidence — a request carrying it must answer identically to
// one that does not, for every count the response carries.
func TestShowClosedParameterNoLongerChangesAnyCount(t *testing.T) {
	openUnread := convWithPhase("open-unread", "Idle", "", "succeeded")
	closedUnread := convWithPhase("closed-unread", "Closed", "", "succeeded")
	api, _, _, _ := apiWithOptions(t, "tok", true, openUnread, closedUnread)
	h := api.Handler(http.NotFoundHandler())

	plain := getCounted(t, h, "/api/conversations?count=1")
	withTrue := getCounted(t, h, "/api/conversations?count=1&showClosed=true")
	withFalse := getCounted(t, h, "/api/conversations?count=1&showClosed=false")

	for _, got := range []countedResponse{withTrue, withFalse} {
		if got.Total != plain.Total || got.Unread != plain.Unread ||
			got.Scopes["open"] != plain.Scopes["open"] || got.Scopes["closed"] != plain.Scopes["closed"] {
			t.Fatalf("a showClosed value must not move any count: plain=%+v got=%+v", plain, got)
		}
	}
}

// 1.6: the count-only response shape, with rootTotal gone for good — a
// stray key here is as much a contract break as a wrong number.
func TestCountOnlyResponseShapeHasNoRootTotal(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithPhase("open-unread", "Idle", "", "succeeded"))
	h := api.Handler(http.NotFoundHandler())

	rec := identified(t, h, "GET", "/api/conversations?count=1", "", "alice@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("count-only: %d %s", rec.Code, rec.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, present := raw["rootTotal"]; present {
		t.Fatalf("rootTotal must not appear on the wire at all: %+v", raw)
	}
	for _, field := range []string{"total", "unreadTotal", "scopes", "items", "offset", "limit"} {
		if _, present := raw[field]; !present {
			t.Fatalf("count-only response is missing %q: %+v", field, raw)
		}
	}
	items, _ := raw["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("count-only must carry no rows: %+v", raw["items"])
	}
}

// A member is never Joined — it binds no console thread of its own
// (invariants.md) — so s.Unread is already always false for one; the
// explicit isMember guard in accumulateScopeCounts must not rely on that
// alone, in case anything ever changes it from underneath.
func TestUnreadTotalExcludesAMemberEvenIfSomehowJoined(t *testing.T) {
	member := withCausedBy(convWithRuns("improbably-joined-member", "", [2]string{"x", tLate}), "some-root", "k8s-observe")

	api, _, _, _ := apiWithOptions(t, "tok", true, member)
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Unread != 0 {
		t.Fatalf("a member must never contribute to unreadTotal, even carrying unread content: %+v", got)
	}
}

// Working, Mine, Errored and the per-Pipeline/Coordinator badges are each an
// UNREAD, non-Closed count ANDed with their own predicate — a READ
// conversation must never inflate one, matching ChatView.tsx's
// TREE_PREDICATE (every entry ANDs `c.unread` too, and the Open scope's own
// phase≠Closed test).
func TestEveryScopeButClosedIsAnUnreadNonClosedCount(t *testing.T) {
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	workingRead := convWithPhase("working-read", "Working", tLate, "succeeded")
	workingUnread := convWithPhase("working-unread", "Working", "", "succeeded")
	mineErroredRead := withOriginReader(convWithPhase("mine-errored-read", "Idle", tLate, "failed"), "console", aliceKey)
	mineErroredUnread := withOriginReader(convWithPhase("mine-errored-unread", "Idle", "", "failed"), "console", aliceKey)
	coordRead := withCoordinatorRef(convWithRuns("coord-read", tLate, [2]string{"x", tLate}), "triage")
	coordUnread := withCoordinatorRef(convWithRuns("coord-unread", "", [2]string{"x", tLate}), "triage")
	// closed-but-unread siblings of each predicate, which must likewise be
	// excluded — they belong to the Closed scope, not these.
	mineErroredClosedUnread := withOriginReader(convWithPhase("mine-errored-closed-unread", "Closed", "", "failed"), "console", aliceKey)
	coordClosedUnread := withCoordinatorRef(convWithPhase("coord-closed-unread", "Closed", "", "succeeded"), "triage")

	api, _, _, _ := apiWithOptions(t, "tok", true,
		workingRead, workingUnread, mineErroredRead, mineErroredUnread, coordRead, coordUnread,
		mineErroredClosedUnread, coordClosedUnread,
		obj("coordinators", "triage", "1", "{}", "{}"))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["working"] != 1 {
		t.Fatalf("working must count only the unread, non-Closed one: %+v", got.Scopes)
	}
	if got.Scopes["mine"] != 1 {
		t.Fatalf("mine must count only the unread, non-Closed one: %+v", got.Scopes)
	}
	if got.Scopes["errored"] != 1 {
		t.Fatalf("errored must count only the unread, non-Closed one: %+v", got.Scopes)
	}
	if got.Scopes["triage"] != 1 {
		t.Fatalf("the coordinator's own scope must count only the unread, non-Closed one: %+v", got.Scopes)
	}
	if got.Scopes["closed"] != 2 {
		t.Fatalf("the two closed-but-unread siblings must land in Closed instead: %+v", got.Scopes)
	}
}
