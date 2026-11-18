package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// A page counts TREES: the last N roots and every conversation under them, so a
// coordinator is never listed without the members the list draws beneath it
// (a cut at N rows once separated the two).
func TestConversationListPagesByRootsKeepingEveryMember(t *testing.T) {
	var objs []*Object
	for i := 0; i < 4; i++ {
		root := fmt.Sprintf("root%d", i)
		// The root is OLDER than its members, which is what a row cut split on.
		objs = append(objs, obj("conversations", root, "1", `{"profileRef":{"name":"ops"}}`,
			fmt.Sprintf(`{"phase":"Idle","lastActivity":"2026-08-09T10:%02d:00Z"}`, i*10)))
		for m := 0; m < 3; m++ {
			objs = append(objs, withCausedBy(obj("conversations", fmt.Sprintf("%s-m%d", root, m), "1",
				`{"profileRef":{"name":"ops"}}`,
				fmt.Sprintf(`{"phase":"Closed","lastActivity":"2026-08-09T10:%02d:00Z"}`, i*10+m+1)), root, "triage"))
		}
	}
	api, _, _ := apiUnderTest(t, "tok", objs...)
	h := api.Handler(http.NotFoundHandler())

	get := func(q string) (names []string, total int) {
		var page struct {
			Items []ConversationSummary `json:"items"`
			Total int                   `json:"total"`
		}
		rec := authed(t, h, "GET", "/api/conversations?"+q, "")
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, c := range page.Items {
			names = append(names, c.Name)
		}
		return names, page.Total
	}

	names, total := get("limit=2")
	if len(names) != 8 || total != 16 {
		t.Fatalf("two trees are eight rows of sixteen, got %d of %d: %v", len(names), total, names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	for _, root := range []string{"root3", "root2"} {
		for m := 0; m < 3; m++ {
			if !seen[fmt.Sprintf("%s-m%d", root, m)] {
				t.Fatalf("%s lost a member to the page: %v", root, names)
			}
		}
	}
	if seen["root1"] || seen["root1-m0"] {
		t.Fatalf("the page held a third tree: %v", names)
	}

	// The next page continues by trees, not rows.
	names, _ = get("limit=2&offset=2")
	if len(names) != 8 || !contains(names, "root1") || !contains(names, "root0-m2") {
		t.Fatalf("second page wrong: %v", names)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
