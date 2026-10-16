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
	Total     int            `json:"total"`
	RootTotal int            `json:"rootTotal"`
	Unread    int            `json:"unreadTotal"`
	Scopes    map[string]int `json:"scopes"`
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

// A member is counted in `total`/`items` — the tree is rebuilt client-side
// from the full flat page (ChatView.tsx's withTreeContext), so a member must
// stay in it — but never in `rootTotal`, the number the All badge reads.
// Before this, every invoked member inflated All right alongside its root:
// an install with ten things a person could actually open showed 21.
func TestRootTotalExcludesMembers(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true,
		convWithRuns("root-a", "", [2]string{"x", tLate}),
		convWithRuns("root-b", "", [2]string{"x", tLate}),
		withCausedBy(convWithRuns("member-of-a", "", [2]string{"x", tLate}), "root-a", "k8s-observe"),
	)
	h := api.Handler(http.NotFoundHandler())

	got := getCounted(t, h, "/api/conversations?count=1")
	if got.Total != 3 {
		t.Fatalf("total must still count the member, for the tree: %+v", got)
	}
	if got.RootTotal != 2 {
		t.Fatalf("rootTotal must exclude the member: %+v", got)
	}
}

// The Archive badge promises the list it opens — every CLOSED conversation,
// read or not — not "closed conversations that also happen to be unread"
// (what `scopes["closed"]` meant before this) and not a member, which the
// Archive list never shows on its own either.
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

// The per-Pipeline/Coordinator badge is a TOTAL, not an unread count: neither
// list has a TREE_PREDICATE entry for 'pipeline'/'coordinator' (ChatView.tsx)
// — selecting one shows every root attributed to it, read or unread — so a
// READ root proves the point a pair of unread fixtures could not. It DOES
// honor showClosed, the one filter both lists still apply: "agentops-
// coordinator: 2" beside a list showing 1 was wrong for exactly the reason
// "Unread: 2" beside an empty list was, one level down.
func TestPerCoordinatorScopeIsATotalThatRespectsShowClosed(t *testing.T) {
	openRead := withCoordinatorRef(convWithRuns("open-read", tLate, [2]string{"x", tLate}), "triage")
	closedRead := withCoordinatorRef(convWithPhase("closed-read", "Closed", tLate, "succeeded"), "triage")

	api, _, _, _ := apiWithOptions(t, "tok", true, openRead, closedRead,
		obj("coordinators", "triage", "1", "{}", "{}"))
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Scopes["triage"] != 2 {
		t.Fatalf("a READ root must still count — this badge is a total, not an unread count: %+v", got.Scopes)
	}
	if got := getCounted(t, h, "/api/conversations?count=1&showClosed=false"); got.Scopes["triage"] != 1 {
		t.Fatalf("showClosed=false must exclude the closed one, matching what the list under this scope shows: %+v", got.Scopes)
	}
}

// Mine and Errored can both be true on a CLOSED conversation, and their own
// lists hide a closed row the same generic way every scope but Closed does
// — so their badges must honor showClosed too, exactly like Coordinator's.
// Working needs no such test: Working and Closed are different phases, so a
// closed row was never counted there in the first place.
func TestMineAndErroredScopesRespectShowClosed(t *testing.T) {
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	openMineErrored := withOriginReader(convWithPhase("open-mine-errored", "Idle", tLate, "failed"), "console", aliceKey)
	closedMineErrored := withOriginReader(convWithPhase("closed-mine-errored", "Closed", tLate, "failed"), "console", aliceKey)

	api, _, _, _ := apiWithOptions(t, "tok", true, openMineErrored, closedMineErrored)
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	if got := getCounted(t, h, "/api/conversations?count=1"); got.Scopes["mine"] != 2 || got.Scopes["errored"] != 2 {
		t.Fatalf("absent showClosed must include the closed row in both badges: %+v", got.Scopes)
	}
	if got := getCounted(t, h, "/api/conversations?count=1&showClosed=false"); got.Scopes["mine"] != 1 || got.Scopes["errored"] != 1 {
		t.Fatalf("showClosed=false must exclude the closed row from both, matching their own lists: %+v", got.Scopes)
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
