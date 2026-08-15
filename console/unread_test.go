package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Unread is a property of the thread THIS console holds. What these tests pin
// is the derivation table, that the count is taken before the filter, and that
// marking read reaches only as far as the console's own threads do.

// convWithRead builds a conversation bound to the console with an explicit
// watermark on its binding.
func convWithRead(name, lastActivity, readAt string, tracked bool) *Object {
	binding := `{"channel":"console","threadId":"console-uid-` + name + `"`
	if readAt != "" {
		binding += `,"readAt":"` + readAt + `"`
	}
	if tracked {
		binding += `,"readTracked":true`
	}
	binding += `}`
	status := `{"phase":"Idle","threads":[` + binding + `]`
	if lastActivity != "" {
		status += `,"lastActivity":"` + lastActivity + `"`
	}
	status += `}`
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`, status)
}

const (
	tEarly = "2026-08-13T10:00:00Z"
	tLate  = "2026-08-13T11:00:00Z"
)

// The table in design decision 4, one case per row.
func TestUnreadDerivation(t *testing.T) {
	cases := []struct {
		name string
		obj  *Object
		want bool
	}{
		{"untracked binding predates the mechanism", convWithRead("a", tLate, "", false), false},
		{"tracked and never read", convWithRead("b", tLate, "", true), true},
		{"activity after the watermark", convWithRead("c", tLate, tEarly, true), true},
		{"read up to the latest activity", convWithRead("d", tEarly, tEarly, true), false},
		{"watermark ahead of activity", convWithRead("e", tEarly, tLate, true), false},
	}
	for _, tc := range cases {
		if got := summarize(tc.obj, nil, "console", "").Unread; got != tc.want {
			t.Fatalf("%s: unread=%v, want %v", tc.name, got, tc.want)
		}
	}

	// An OBSERVED conversation — no console thread — is never unread, however
	// new it is. The console holds no watermark on it and no standing to call
	// it new.
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55","readTracked":true}],"lastActivity":"`+tLate+`"}`)
	if summarize(observed, nil, "console", "").Unread {
		t.Fatal("a conversation with no console thread must never be unread")
	}
	// …and reading it on Telegram does not clear the console, nor the reverse:
	// the watermark is per thread.
	both := obj("conversations", "both", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"},{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55","readTracked":true,"readAt":"`+tLate+`"},`+
			`{"channel":"console","threadId":"console-uid-both","readTracked":true,"readAt":"`+tEarly+`"}],`+
			`"lastActivity":"`+tLate+`"}`)
	if !summarize(both, nil, "console", "").Unread {
		t.Fatal("reading a conversation on another channel must not clear the console's mark")
	}

	// A conversation that never ran has no lastActivity: unreadness falls back
	// to creation, the same key the list sorts on, so the mark and the ordering
	// cannot disagree.
	never := convWithRead("never", "", "", true)
	never.Metadata.CreationTimestamp = tLate
	if !summarize(never, nil, "console", "").Unread {
		t.Fatal("a bound, never-read conversation must be unread even before its first run")
	}
}

type listResponse struct {
	Items       []ConversationSummary `json:"items"`
	Total       int                   `json:"total"`
	UnreadTotal int                   `json:"unreadTotal"`
}

func getList(t *testing.T, h http.Handler, path string) listResponse {
	t.Helper()
	rec := authed(t, h, "GET", path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	var out listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s body %q: %v", path, rec.Body.String(), err)
	}
	return out
}

// The filter narrows server-side; the COUNT is computed before it, so
// narrowing the view never moves the badge.
func TestUnreadFilterAndPreFilterCount(t *testing.T) {
	closed := convWithRead("closed-unread", tLate, tEarly, true)
	closed.Status = json.RawMessage(strings.Replace(string(closed.Status), `"phase":"Idle"`, `"phase":"Closed"`, 1))
	api, _, _, _ := apiWithOptions(t, "tok", true,
		convWithRead("unread-1", tLate, tEarly, true),
		convWithRead("unread-2", tLate, "", true),
		convWithRead("read-1", tEarly, tEarly, true),
		closed,
	)
	h := api.Handler(http.NotFoundHandler())

	all := getList(t, h, "/api/conversations")
	if all.Total != 4 || all.UnreadTotal != 3 {
		t.Fatalf("unfiltered: total=%d unread=%d", all.Total, all.UnreadTotal)
	}
	unread := getList(t, h, "/api/conversations?unread=true")
	if unread.Total != 3 || len(unread.Items) != 3 {
		t.Fatalf("the unread filter must narrow server-side: %+v", unread)
	}
	for _, it := range unread.Items {
		if !it.Unread {
			t.Fatalf("%s is not unread but was returned by the filter", it.Name)
		}
	}
	// a filter that hides two of the three unread rows must not move the count
	narrowed := getList(t, h, "/api/conversations?phase=Closed")
	if narrowed.Total != 1 || narrowed.UnreadTotal != 3 {
		t.Fatalf("the count moved when the view narrowed: total=%d unread=%d", narrowed.Total, narrowed.UnreadTotal)
	}
	// count-only: the numbers with no rows
	only := getList(t, h, "/api/conversations?count=1")
	if len(only.Items) != 0 || only.UnreadTotal != 3 || only.Total != 4 {
		t.Fatalf("count-only form: %+v", only)
	}
}

type readResponse struct {
	Results []ReadResult `json:"results"`
	Marked  int          `json:"marked"`
	Skipped int          `json:"skipped"`
	Failed  int          `json:"failed"`
}

func postRead(t *testing.T, h http.Handler, body string) (int, readResponse) {
	t.Helper()
	rec := authed(t, h, "POST", "/api/conversations/read", body)
	var out readResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("read body %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

// The selection is marked, the watermark is the one the SERVER read off each
// conversation, and observed conversations are skipped rather than sent.
func TestMarkReadMarksTheSelection(t *testing.T) {
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55"}],"lastActivity":"`+tLate+`"}`)
	api, f, _, _ := apiWithOptions(t, "tok", true,
		convWithRead("a", tLate, tEarly, true), observed, convWithRead("b", tLate, "", true))
	h := api.Handler(http.NotFoundHandler())

	code, out := postRead(t, h, `{"names":["a","observed","b"]}`)
	if code != http.StatusOK || out.Marked != 2 || out.Skipped != 1 || out.Failed != 0 {
		t.Fatalf("mark read: %d %+v", code, out)
	}
	byName := map[string]ReadResult{}
	for _, r := range out.Results {
		byName[r.Name] = r
	}
	if byName["observed"].Outcome != closeOutcomeSkipped ||
		!strings.Contains(byName["observed"].Reason, "channels[]") {
		t.Fatalf("an observed conversation must be skipped with the binding that would fix it: %+v", byName["observed"])
	}
	reported := f.reported()
	if len(reported) != 2 {
		t.Fatalf("only joined conversations may be reported: %+v", reported)
	}
	for _, e := range reported {
		// the watermark is the conversation's OWN activity, never a client now
		if e.ReadAt != tLate {
			t.Fatalf("reported watermark %q, want the conversation's last activity %q", e.ReadAt, tLate)
		}
	}

	// an unknown name fails without stopping the rest
	if _, out := postRead(t, h, `{"names":["a","ghost"]}`); out.Failed != 1 || out.Marked != 1 {
		t.Fatalf("an unknown name must not fail the batch: %+v", out)
	}
	// a watermark that would not advance comes back skipped, not marked
	f.skipReads["console-uid-a"] = true
	if _, out := postRead(t, h, `{"names":["a"]}`); out.Skipped != 1 {
		t.Fatalf("a report that would not advance must be skipped: %+v", out)
	}
}

// The batch is bounded server-side at one page, exactly as bulk close is.
func TestMarkReadRefusesEmptyAndOversizedBatches(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithRead("a", tLate, "", true))
	h := api.Handler(http.NotFoundHandler())

	if code, _ := postRead(t, h, `{"names":[]}`); code != http.StatusBadRequest {
		t.Fatalf("empty batch: %d", code)
	}
	names := make([]string, conversationPageSize+1)
	for i := range names {
		names[i] = `"a` + strconv.Itoa(i) + `"`
	}
	if code, _ := postRead(t, h, `{"names":[`+strings.Join(names, ",")+`]}`); code != http.StatusBadRequest {
		t.Fatalf("oversized batch: %d", code)
	}
}

// Marking read is authenticated, but NOT behind the write gate: a read-only
// console that could show a backlog and never clear it would be broken in the
// way the unread mark exists to fix.
func TestMarkReadIsAuthenticatedButNotGatedByWrites(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", false, convWithRead("a", tLate, "", true))
	h := api.Handler(http.NotFoundHandler())

	w := httptest.NewRecorder()
	h.ServeHTTP(w, jsonReq("POST", "/api/conversations/read", `{"names":["a"]}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mark-read must be refused: %d", w.Code)
	}
	if len(f.reported()) != 0 {
		t.Fatal("an unauthenticated request reported a read")
	}

	code, out := postRead(t, h, `{"names":["a"]}`)
	if code != http.StatusOK || out.Marked != 1 {
		t.Fatalf("a read-only console must still mark read: %d %+v", code, out)
	}
	// the write gate is still doing its job on an actual write
	if rec := authed(t, h, "POST", "/api/conversations/close", `{"names":["a"]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("bulk close must stay gated: %d", rec.Code)
	}
}

// ---- per-identity read state --------------------------------------------------

// withSalt gives the adapter a projected reader salt, as the chart does.
func withSalt(a *Adapter, salt string) {
	a.mu.Lock()
	a.readerSalt = salt
	a.mu.Unlock()
}

// identified issues a request carrying a forward-auth identity, which is what
// an oauth2-proxy in front of the console supplies.
func identified(t *testing.T, h http.Handler, method, path, body, who string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = jsonReq(method, path, body)
	}
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Forwarded-Email", who)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func unreadFor(t *testing.T, h http.Handler, who string) int {
	t.Helper()
	rec := identified(t, h, "GET", "/api/conversations?count=1", "", who)
	if rec.Code != http.StatusOK {
		t.Fatalf("count for %s: %d", who, rec.Code)
	}
	var out listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.UnreadTotal
}

// convWithReaders builds a conversation whose console binding already carries a
// per-identity overlay.
func convWithReaders(name, lastActivity string, readers map[string]string) *Object {
	entries := []string{}
	for k, at := range readers {
		entries = append(entries, `{"key":"`+k+`","readAt":"`+at+`"}`)
	}
	sort.Strings(entries) // stable fixture
	binding := `{"channel":"console","threadId":"console-uid-` + name + `","readTracked":true,` +
		`"readers":[` + strings.Join(entries, ",") + `]}`
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Idle","threads":[`+binding+`],"lastActivity":"`+lastActivity+`"}`)
}

// One operator reading does not clear it for another.
func TestUnreadIsPerIdentity(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithReaders("shared", tLate, nil))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	if got := unreadFor(t, h, "alice@example.com"); got != 1 {
		t.Fatalf("alice: want 1 unread, got %d", got)
	}
	if rec := identified(t, h, "POST", "/api/conversations/read", `{"names":["shared"]}`, "alice@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("alice mark read: %d", rec.Code)
	}
	// bob is untouched — the fake manager records the report rather than
	// applying it, so what this pins is that alice's report carried HER key and
	// nobody else's.
	if got := unreadFor(t, h, "bob@example.com"); got != 1 {
		t.Fatalf("bob: alice's read must not clear his badge, got %d", got)
	}
}

// The key sent upstream is an opaque hash — never the address, and never
// unsalted.
func TestReaderKeyCarriesNoIdentity(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true, convWithReaders("c1", tLate, nil))
	withSalt(api.adapter, "pepper")
	h := api.Handler(http.NotFoundHandler())

	if rec := identified(t, h, "POST", "/api/conversations/read", `{"names":["c1"]}`, "alice@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("mark read: %d", rec.Code)
	}
	reported := f.reported()
	if len(reported) != 1 {
		t.Fatalf("want one report, got %+v", reported)
	}
	key := reported[0].Reader
	if key == "" || !strings.HasPrefix(key, "sha256:") {
		t.Fatalf("reader key must be a prefixed hash, got %q", key)
	}
	if strings.Contains(key, "alice") || strings.Contains(key, "@") {
		t.Fatalf("the identity leaked into the key: %q", key)
	}
	// salted: the same address under a different salt is a different key
	other, _, _, _ := apiWithOptions(t, "tok", true, convWithReaders("c1", tLate, nil))
	withSalt(other.adapter, "different-pepper")
	if other.adapter.ReaderKey("alice@example.com") == key {
		t.Fatal("the key does not depend on the salt, so a known address is confirmable")
	}
}

// A per-identity mark answers the viewer it belongs to and nobody else.
func TestOverlayAnswersTheViewer(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true)
	withSalt(api.adapter, "pepper")
	aliceKey := api.adapter.ReaderKey("alice@example.com")

	c := convWithReaders("seen-by-alice", tLate, map[string]string{aliceKey: tLate})
	api2, _, _, _ := apiWithOptions(t, "tok", true, c)
	withSalt(api2.adapter, "pepper")
	h := api2.Handler(http.NotFoundHandler())

	if got := unreadFor(t, h, "alice@example.com"); got != 0 {
		t.Fatalf("alice has read it: want 0 unread, got %d", got)
	}
	if got := unreadFor(t, h, "bob@example.com"); got != 1 {
		t.Fatalf("bob has not: want 1 unread, got %d", got)
	}
}

// With no salt projected, and under a shared token, everyone is one reader —
// which is exactly the behaviour before per-identity marks existed.
func TestDegradesToChannelWideMarks(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true, convWithRead("c1", tLate, "", true))
	h := api.Handler(http.NotFoundHandler()) // no salt set

	if key := api.adapter.ReaderKey("alice@example.com"); key != "" {
		t.Fatalf("no salt must yield no key rather than an unsalted hash, got %q", key)
	}
	if rec := identified(t, h, "POST", "/api/conversations/read", `{"names":["c1"]}`, "alice@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("mark read: %d", rec.Code)
	}
	if r := f.reported(); len(r) != 1 || r[0].Reader != "" {
		t.Fatalf("without a salt the report must carry no reader: %+v", r)
	}

	// a shared token: no forwarded identity at all, so one key for everyone
	withSalt(api.adapter, "pepper")
	if api.adapter.ReaderKey("") != "" {
		t.Fatal("an unidentified request must resolve to no reader")
	}
}

// Starting a conversation sends the starter's own key, so the manager can mark
// it read for them when their thread is created — and for nobody else.
func TestOriginationCarriesTheStartersKey(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": 1, "conversations": 1})
	}))
	defer srv.Close()

	o := NewOriginator(srv.URL, "signal-token", "console")
	if _, err := o.Start(context.Background(), "console-chan", "alice@example.com", "sha256:opaque", "check the nodes"); err != nil {
		t.Fatal(err)
	}
	sig := body["signals"].([]any)[0].(map[string]any)
	if sig["reader"] != "sha256:opaque" {
		t.Fatalf("origination must carry the starter's own key, got %v", sig["reader"])
	}
	// the SENDER is attribution and stays as given; the READER is the opaque
	// half, and it is the only one that reaches the Conversation object
	labels := sig["labels"].(map[string]any)
	if labels["agentops.dev/sender"] != "alice@example.com" {
		t.Fatalf("sender attribution changed: %v", labels["agentops.dev/sender"])
	}
}
