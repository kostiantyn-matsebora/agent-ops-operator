package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func postUnread(t *testing.T, h http.Handler, who, body string) (int, readResponse) {
	t.Helper()
	rec := identified(t, h, "POST", "/api/conversations/unread", body, who)
	var out readResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unread body %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

// Marking unread rewinds the reader's own watermark to just before the
// newest counted message, bringing exactly one message back — for that
// reader, and the fake manager's own records show the request it sent
// carried a rewind, keyed on the reader's own opaque key.
func TestMarkUnreadBringsOneMessageBack(t *testing.T) {
	keyer := NewAdapter(nil, nil, nil, "console")
	withSalt(keyer, "pepper")
	aliceKey := keyer.ReaderKey("alice@example.com")

	// alice has already read up to the answer
	conv := convWithReaders("read-already", tLate, map[string]string{aliceKey: tLate})
	api, f, _, _ := apiWithOptions(t, "tok", true, conv)
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	if got := unreadFor(t, h, "alice@example.com"); got != 0 {
		t.Fatalf("fixture must start read for alice: got %d unread", got)
	}

	code, out := postUnread(t, h, "alice@example.com", `{"names":["read-already"]}`)
	if code != http.StatusOK || out.Marked != 1 {
		t.Fatalf("mark unread: %d %+v", code, out)
	}
	reported := f.reported()
	if len(reported) != 1 || !reported[0].Rewind || reported[0].Reader != aliceKey {
		t.Fatalf("the report must ask for a rewind on alice's own key: %+v", reported)
	}

	// Apply the rewound watermark by hand — the fake manager only RECORDS
	// the report — and confirm exactly one message comes back for alice.
	rewound := convWithReaders("read-already", tLate, map[string]string{aliceKey: reported[0].ReadAt})
	api2, _, _, _ := apiWithOptions(t, "tok", true, rewound)
	withSalt(api2.adapter, "pepper")
	if got := unreadFor(t, api2.Handler(http.NotFoundHandler()), "alice@example.com"); got != 1 {
		t.Fatalf("the rewound watermark must bring exactly one message back, got %d", got)
	}
}

// With no reader salt projected, mark unread is refused outright — there is
// no per-person mark to rewind, and the channel-wide one must never move.
func TestMarkUnreadRefusedWithNoReader(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		convWithRuns("a", tLate, [2]string{"result", tLate}))
	h := api.Handler(http.NotFoundHandler()) // no salt set

	rec := authed(t, h, "POST", "/api/conversations/unread", `{"names":["a"]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("mark unread with no reader salt must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.reported()) != 0 {
		t.Fatal("a refused mark-unread reported something")
	}
}

// Bounded exactly as mark read is.
func TestMarkUnreadBoundedAtOnePage(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithRuns("a", "", [2]string{"r", tLate}))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	names := make([]string, conversationPageSize+1)
	for i := range names {
		names[i] = `"a` + strconv.Itoa(i) + `"`
	}
	rec := identified(t, h, "POST", "/api/conversations/unread",
		`{"names":[`+strings.Join(names, ",")+`]}`, "alice@example.com")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized batch: %d", rec.Code)
	}
	rec = identified(t, h, "POST", "/api/conversations/unread", `{"names":[]}`, "alice@example.com")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty batch: %d", rec.Code)
	}
}

// A conversation with no counted message yet has nothing to rewind to.
func TestMarkUnreadSkipsAConversationWithNothingToCount(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithRuns("never-answered", ""))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	code, out := postUnread(t, h, "alice@example.com", `{"names":["never-answered"]}`)
	if code != http.StatusOK || out.Skipped != 1 {
		t.Fatalf("a conversation with nothing counted must be skipped: %d %+v", code, out)
	}
}

// Marking unread is authenticated.
func TestMarkUnreadRequiresAuth(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithRuns("a", "", [2]string{"r", tLate}))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	w := httptest.NewRecorder()
	h.ServeHTTP(w, jsonReq("POST", "/api/conversations/unread", `{"names":["a"]}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mark-unread must be refused: %d", w.Code)
	}
}
