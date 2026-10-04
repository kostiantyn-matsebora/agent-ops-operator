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

// Unread is a property of the MESSAGES on the thread THIS console holds, by
// kind — never raw activity. These tests pin the derivation table, that the
// count is taken before the filter, and that marking read reaches only as
// far as the console's own threads do.

const (
	tEarly = "2026-08-13T10:00:00Z"
	tLate  = "2026-08-13T11:00:00Z"
)

// convWithRuns builds a conversation bound to the console, its thread read up
// to readAt ("" for never read), with one recorded run per {result, finishedAt}
// pair — the durable half of the new counting rule.
func convWithRuns(name, readAt string, results ...[2]string) *Object {
	binding := `{"channel":"console","threadId":"console-uid-` + name + `","readTracked":true`
	if readAt != "" {
		binding += `,"readAt":"` + readAt + `"`
	}
	binding += `}`
	var runs []string
	last := ""
	for i, r := range results {
		runs = append(runs, `{"runId":"r`+strconv.Itoa(i)+`","status":"succeeded","result":"`+r[0]+`","finishedAt":"`+r[1]+`"}`)
		last = r[1]
	}
	status := `{"phase":"Idle","threads":[` + binding + `],"runs":[` + strings.Join(runs, ",") + `]`
	if last != "" {
		status += `,"lastActivity":"` + last + `"`
	}
	status += `}`
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`, status)
}

// A new answer marks its conversation unread, and the count tracks how many.
func TestUnreadCountsAnswersByKind(t *testing.T) {
	answered := convWithRuns("answered", tEarly, [2]string{"the fix", tLate})
	s := summarize(answered, nil, nil, "console", "", nil)
	if s.UnreadCount != 1 || !s.Unread {
		t.Fatalf("a fresh answer must count once: %+v", s)
	}
	if s.LastMessage == nil || s.LastMessage.Kind != MsgAgent || s.LastMessage.Text != "the fix" {
		t.Fatalf("lastMessage not the agent's answer: %+v", s.LastMessage)
	}

	const tBefore = "2026-08-13T09:00:00Z"
	two := convWithRuns("two-answers", tBefore, [2]string{"first", tEarly}, [2]string{"second", tLate})
	if s := summarize(two, nil, nil, "console", "", nil); s.UnreadCount != 2 {
		t.Fatalf("two answers after the watermark must count twice, got %d", s.UnreadCount)
	}

	read := convWithRuns("read", tLate, [2]string{"done", tLate})
	if s := summarize(read, nil, nil, "console", "", nil); s.Unread || s.UnreadCount != 0 {
		t.Fatalf("a conversation read up to its latest answer must not be unread: %+v", s)
	}

	never := convWithRuns("never-answered", "")
	if s := summarize(never, nil, nil, "console", "", nil); s.Unread {
		t.Fatal("a bound conversation with no counted message yet must not be unread")
	}
}

// The acknowledgement never counts: it lives only in the live buffer, and its
// KIND excludes it exactly as a console user's own words are.
func TestUnreadExcludesTheAck(t *testing.T) {
	conv := convWithRuns("acked", tEarly)
	tr := NewTranscripts()
	tr.AppendOp("ack1", "console-uid-acked", &OpMessage{Kind: "notice", Body: "🔧 On it…"}, "console")
	if s := summarize(conv, nil, nil, "console", "", tr); s.Unread || s.UnreadCount != 0 {
		t.Fatalf("an acknowledgement must never count: %+v", s)
	}
}

// An observed conversation — no console thread — carries no count however
// new its activity, and reading it on another channel never clears the
// console: the watermark is per thread.
func TestUnreadIsScopedToTheConsoleThread(t *testing.T) {
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55","readTracked":true}],"lastActivity":"`+tLate+`"}`)
	if s := summarize(observed, nil, nil, "console", "", nil); s.Unread || s.UnreadCount != 0 {
		t.Fatal("a conversation with no console thread must never be unread")
	}

	both := obj("conversations", "both", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"},{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55","readTracked":true,"readAt":"`+tLate+`"},`+
			`{"channel":"console","threadId":"console-uid-both","readTracked":true}],`+
			`"runs":[{"runId":"r1","status":"succeeded","result":"hi","finishedAt":"`+tLate+`"}],`+
			`"lastActivity":"`+tLate+`"}`)
	if s := summarize(both, nil, nil, "console", "", nil); !s.Unread {
		t.Fatal("reading a conversation on another channel must not clear the console's mark")
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
	closed := convWithRuns("closed-unread", tEarly, [2]string{"result", tLate})
	closed.Status = json.RawMessage(strings.Replace(string(closed.Status), `"phase":"Idle"`, `"phase":"Closed"`, 1))
	api, _, _, _ := apiWithOptions(t, "tok", true,
		convWithRuns("unread-1", tEarly, [2]string{"result", tLate}),
		convWithRuns("unread-2", "", [2]string{"result", tLate}),
		convWithRuns("read-1", tLate, [2]string{"result", tEarly}),
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
		convWithRuns("a", tEarly, [2]string{"result", tLate}), observed, convWithRuns("b", "", [2]string{"result", tLate}))
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
		// the watermark is the conversation's own latest counted message,
		// never a client-generated "now"
		if e.ReadAt != tLate {
			t.Fatalf("reported watermark %q, want the latest answer's time %q", e.ReadAt, tLate)
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
	api, _, _, _ := apiWithOptions(t, "tok", true, convWithRuns("a", "", [2]string{"result", tLate}))
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
	api, f, _, _ := apiWithOptions(t, "tok", false, convWithRuns("a", "", [2]string{"result", tLate}))
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

// convWithReaders builds a conversation whose console binding already carries
// a per-identity overlay, with one answer so there is something to count.
func convWithReaders(name, answeredAt string, readers map[string]string) *Object {
	entries := []string{}
	for k, at := range readers {
		entries = append(entries, `{"key":"`+k+`","readAt":"`+at+`"}`)
	}
	sort.Strings(entries) // stable fixture
	binding := `{"channel":"console","threadId":"console-uid-` + name + `","readTracked":true,` +
		`"readers":[` + strings.Join(entries, ",") + `]}`
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Idle","threads":[`+binding+`],"runs":[{"runId":"r1","status":"succeeded","result":"hi",`+
			`"finishedAt":"`+answeredAt+`"}],"lastActivity":"`+answeredAt+`"}`)
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
	api, f, _, _ := apiWithOptions(t, "tok", true, convWithRuns("c1", "", [2]string{"result", tLate}))
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
