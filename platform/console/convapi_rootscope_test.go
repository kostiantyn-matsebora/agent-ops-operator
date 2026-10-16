package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// withCausedBy stamps a conversation's spec.causedBy, marking it as a member
// invoked by the named parent — the one fact isMember reads.
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
		t.Fatalf("%s body %q: %v", path, rec.Body.String(), err)
	}
	return out
}

// The Archive badge is the one scope that is NOT an unread count: every
// CLOSED root, read or not — an archive a person could only browse while
// everything in it stayed unread would empty out the moment they had read
// it, defeating what an archive is for. It still excludes a member, which
// the Archive list never shows on its own either.
func TestClosedScopeCountIsRootOnlyAndUnconditional(t *testing.T) {
	closedRootRead := convWithPhase("closed-root-read", "Closed", tLate, "succeeded")
	closedRootUnread := convWithPhase("closed-root-unread", "Closed", "", "succeeded")
	closedMember := withCausedBy(convWithPhase("closed-member", "Closed", "", "succeeded"), "closed-root-unread", "k8s-ops")
	openRoot := convWithPhase("open-root", "Idle", "", "succeeded")

	api, _, _, _ := apiWithOptions(t, "tok", true, closedRootRead, closedRootUnread, closedMember, openRoot)
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["closed"] != 2 {
		t.Fatalf("closed scope must count every closed root regardless of read state, excluding members: %+v", got.Scopes)
	}
}

// showClosed mirrors the list's own display preference. A badge the reader
// cannot reach without first checking that box is not a count of anything
// they can act on — the exact confusion a closed-but-unread conversation
// produced: "Unread: 2" beside a list that matched nothing.
func TestUnreadTotalRespectsShowClosed(t *testing.T) {
	openUnread := convWithRuns("open-unread", "", [2]string{"x", tLate})
	closedUnread := convWithPhase("closed-unread", "Closed", "", "succeeded")

	api, _, _, _ := apiWithOptions(t, "tok", true, openUnread, closedUnread)
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Unread != 2 {
		t.Fatalf("absent (or true), showClosed must include the closed-but-unread row: %+v", got)
	}
	if got := getCounted(t, h, "/api/conversations?count=1&showClosed=true"); got.Unread != 2 {
		t.Fatalf("showClosed=true must include the closed-but-unread row: %+v", got)
	}
	if got := getCounted(t, h, "/api/conversations?count=1&showClosed=false"); got.Unread != 1 {
		t.Fatalf("showClosed=false must exclude the row the list is hiding: %+v", got)
	}
}

// A member never binds a console thread of its own (invariants.md), so it
// can never genuinely be Unread — but the explicit isMember guard in
// handleConversations must not rely on that alone, in case anything ever
// changes it from underneath.
func TestUnreadTotalExcludesAMemberEvenIfSomehowJoined(t *testing.T) {
	member := withCausedBy(convWithRuns("improbably-joined-member", "", [2]string{"x", tLate}), "some-root", "k8s-observe")

	api, _, _, _ := apiWithOptions(t, "tok", true, member)
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Unread != 0 {
		t.Fatalf("a member must never contribute to unreadTotal, even carrying unread content: %+v", got)
	}
}

// Working, Mine, Errored and the per-Pipeline/Coordinator badges are each an
// UNREAD count ANDed with their own predicate — a READ conversation must
// never inflate one, matching ChatView.tsx's TREE_PREDICATE (every entry ANDs
// `c.unread` too). A blanket "Mine: 1" that never cleared, however much of it
// you had read, is exactly the confusion that made a separate Unread scope
// feel redundant once every OTHER scope already promised the same thing.
func TestEveryScopeButClosedIsAnUnreadCount(t *testing.T) {
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	workingRead := convWithPhase("working-read", "Working", tLate, "succeeded")
	workingUnread := convWithPhase("working-unread", "Working", "", "succeeded")
	mineErroredRead := withOriginReader(convWithPhase("mine-errored-read", "Idle", tLate, "failed"), "console", aliceKey)
	mineErroredUnread := withOriginReader(convWithPhase("mine-errored-unread", "Idle", "", "failed"), "console", aliceKey)
	coordRead := withCoordinatorRef(convWithRuns("coord-read", tLate, [2]string{"x", tLate}), "triage")
	coordUnread := withCoordinatorRef(convWithRuns("coord-unread", "", [2]string{"x", tLate}), "triage")

	api, _, _, _ := apiWithOptions(t, "tok", true,
		workingRead, workingUnread, mineErroredRead, mineErroredUnread, coordRead, coordUnread,
		obj("coordinators", "triage", "1", "{}", "{}"))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Scopes["working"] != 1 {
		t.Fatalf("working must count only the unread one: %+v", got.Scopes)
	}
	if got.Scopes["mine"] != 1 {
		t.Fatalf("mine must count only the unread one: %+v", got.Scopes)
	}
	if got.Scopes["errored"] != 1 {
		t.Fatalf("errored must count only the unread one: %+v", got.Scopes)
	}
	if got.Scopes["triage"] != 1 {
		t.Fatalf("the coordinator's own scope must count only the unread one: %+v", got.Scopes)
	}
}

// showClosed gates these the same way it gates the aggregate Unread count —
// Mine and Errored can both be true on a CLOSED-and-unread conversation, and
// their own lists hide a closed row the same generic way every scope but
// Closed does.
func TestMineErroredAndCoordinatorScopesRespectShowClosed(t *testing.T) {
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	openUnread := withOriginReader(convWithPhase("open-unread", "Idle", "", "failed"), "console", aliceKey)
	closedUnread := withOriginReader(convWithPhase("closed-unread", "Closed", "", "failed"), "console", aliceKey)
	coordOpenUnread := withCoordinatorRef(convWithRuns("coord-open-unread", "", [2]string{"x", tLate}), "triage")
	coordClosedUnread := withCoordinatorRef(convWithPhase("coord-closed-unread", "Closed", "", "succeeded"), "triage")

	api, _, _, _ := apiWithOptions(t, "tok", true, openUnread, closedUnread, coordOpenUnread, coordClosedUnread,
		obj("coordinators", "triage", "1", "{}", "{}"))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Scopes["mine"] != 2 || got.Scopes["errored"] != 2 || got.Scopes["triage"] != 2 {
		t.Fatalf("absent showClosed must include the closed-but-unread row in all three: %+v", got.Scopes)
	}
	if got := getCounted(t, h, "/api/conversations?count=1&showClosed=false"); got.Scopes["mine"] != 1 || got.Scopes["errored"] != 1 || got.Scopes["triage"] != 1 {
		t.Fatalf("showClosed=false must exclude the closed one from all three, matching their own lists: %+v", got.Scopes)
	}
}
