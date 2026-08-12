package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Bulk close is a FAN-OUT OF `/close` over the console's own threads: what these
// tests pin is that every decision is taken server-side from CR state, that no
// item can fail the batch, and that the console's reach ends exactly where its
// threads do.

// closeResponse mirrors POST /api/conversations/close.
type closeResponse struct {
	Results []CloseResult `json:"results"`
	Closed  int           `json:"closed"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
}

func postClose(t *testing.T, h http.Handler, body string) (*httptest.ResponseRecorder, closeResponse) {
	t.Helper()
	rec := authed(t, h, "POST", "/api/conversations/close", body)
	var out closeResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("close body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, out
}

// outcomes indexes a result set by conversation name.
func outcomes(res []CloseResult) map[string]CloseResult {
	out := map[string]CloseResult{}
	for _, r := range res {
		out[r.Name] = r
	}
	return out
}

// closes lists the conversations `/close` was actually posted for, in order.
func closes(f *fakeManager) []string {
	var out []string
	for _, in := range f.inbounds() {
		if in["text"] == "/close" {
			thread, _ := in["threadId"].(string)
			out = append(out, thread)
		}
	}
	return out
}

// 3.1: the ordinary case — every joined, idle conversation in the batch gets
// `/close` on its own console thread, and the totals say so.
func TestBulkCloseClosesJoinedConversations(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		joinedConversation("a"), joinedConversation("b"), joinedConversation("c"))
	h := api.Handler(http.NotFoundHandler())

	rec, out := postClose(t, h, `{"names":["a","b","c"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	if out.Closed != 3 || out.Skipped != 0 || out.Failed != 0 {
		t.Fatalf("totals: %+v", out)
	}
	if got := closes(f); len(got) != 3 {
		t.Fatalf("one /close per conversation, got %v", got)
	}
	// the text is the COMMAND, not a paraphrase: the manager intercepts it on
	// the reply path, and anything else becomes an input to the agent
	for _, in := range f.inbounds() {
		if in["text"] != "/close" {
			t.Fatalf("bulk close must post the literal command, got %v", in["text"])
		}
	}
}

// 3.2: reach ends at the threads the console holds. An observed conversation is
// reported with the binding that would extend that reach — and its joined
// siblings in the same batch still close.
func TestBulkCloseSkipsObservedAndClosesTheRest(t *testing.T) {
	observed := obj("conversations", "observed", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"telegram"}]}`,
		`{"threads":[{"channel":"telegram","threadId":"55"}]}`)
	api, f, _, _ := apiWithOptions(t, "tok", true,
		joinedConversation("a"), observed, joinedConversation("b"))
	h := api.Handler(http.NotFoundHandler())

	_, out := postClose(t, h, `{"names":["a","observed","b"]}`)
	if out.Closed != 2 || out.Skipped != 1 || out.Failed != 0 {
		t.Fatalf("an observed conversation must not fail the batch: %+v", out)
	}
	res := outcomes(out.Results)
	if res["observed"].Outcome != closeOutcomeSkipped ||
		!strings.Contains(res["observed"].Reason, "channels[]") {
		t.Fatalf("the skip must name the missing binding: %+v", res["observed"])
	}
	if got := closes(f); len(got) != 2 {
		t.Fatalf("only the joined conversations may be closed, got %v", got)
	}
}

// 3.3: `/close` abandoning an inflight run is right for a deliberate single
// close and wrong as the silent default of a batch — so Working is excluded
// until the request says otherwise.
func TestBulkCloseExcludesWorkingUnlessOptedIn(t *testing.T) {
	working := obj("conversations", "busy", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Working","inflight":{"runId":"r1"},"threads":[{"channel":"console","threadId":"console-uid-busy"}]}`)
	api, f, _, _ := apiWithOptions(t, "tok", true, working)
	h := api.Handler(http.NotFoundHandler())

	_, out := postClose(t, h, `{"names":["busy"]}`)
	if out.Closed != 0 || out.Skipped != 1 {
		t.Fatalf("a working conversation must be excluded by default: %+v", out)
	}
	if reason := outcomes(out.Results)["busy"].Reason; !strings.Contains(reason, "working") {
		t.Fatalf("the skip must say it is working, got %q", reason)
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing may be posted for a skipped conversation")
	}

	_, out = postClose(t, h, `{"names":["busy"],"includeWorking":true}`)
	if out.Closed != 1 {
		t.Fatalf("opting in must close it: %+v", out)
	}
	if got := closes(f); len(got) != 1 {
		t.Fatalf("the opt-in close was not posted: %v", got)
	}
}

// 3.4: the phase is read from the conversation, never from the request. A phase
// the caller asserts is stale by definition and would make the caller the author
// of its own authorization.
func TestBulkCloseReadsThePhaseFromClusterState(t *testing.T) {
	working := obj("conversations", "busy", "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Working","threads":[{"channel":"console","threadId":"console-uid-busy"}]}`)
	api, f, _, _ := apiWithOptions(t, "tok", true, working)
	h := api.Handler(http.NotFoundHandler())

	// the client claims it is idle; the server reads Working and skips it
	_, out := postClose(t, h, `{"names":["busy"],"phase":"Idle","working":false}`)
	if out.Skipped != 1 || out.Closed != 0 {
		t.Fatalf("a client-asserted phase must not decide anything: %+v", out)
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing may be posted for a conversation the server judged working")
	}
}

// 3.5: the bound is the list page size, enforced server-side so it holds
// whatever the client sends — and an empty batch is a malformed request, not a
// no-op success.
func TestBulkCloseRefusesEmptyAndOversizedBatches(t *testing.T) {
	objs := []*Object{joinedConversation("a")}
	api, f, _, _ := apiWithOptions(t, "tok", true, objs...)
	h := api.Handler(http.NotFoundHandler())

	rec, _ := postClose(t, h, `{"names":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty names: want 400, got %d %s", rec.Code, rec.Body.String())
	}

	names := make([]string, 0, conversationPageSize+1)
	for i := 0; i <= conversationPageSize; i++ {
		names = append(names, "conv-"+strconv.Itoa(i))
	}
	body, _ := json.Marshal(map[string]any{"names": names})
	rec, _ = postClose(t, h, string(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d names: want 400, got %d %s", len(names), rec.Code, rec.Body.String())
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("a refused request must close nothing")
	}
}

// 3.6: a batch that stops on its first bad row is a batch nobody can reason
// about. A failure is recorded and the walk continues.
func TestBulkCloseContinuesPastAFailure(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		joinedConversation("a"), joinedConversation("bad"), joinedConversation("b"))
	f.failInbound["console-uid-bad"] = true
	h := api.Handler(http.NotFoundHandler())

	_, out := postClose(t, h, `{"names":["a","bad","b","gone"]}`)
	res := outcomes(out.Results)
	if res["bad"].Outcome != closeOutcomeFailed {
		t.Fatalf("a refused post must be reported failed: %+v", res["bad"])
	}
	// an unknown name is a failure too — it may already have been closed
	if res["gone"].Outcome != closeOutcomeFailed {
		t.Fatalf("an unknown conversation must be reported failed: %+v", res["gone"])
	}
	if res["a"].Outcome != closeOutcomeClosed || res["b"].Outcome != closeOutcomeClosed {
		t.Fatalf("the neighbours must still close: %+v", out.Results)
	}
	if out.Closed != 2 || out.Failed != 2 {
		t.Fatalf("totals: %+v", out)
	}
	// results come back in the order asked for, so the UI can line them up
	if len(out.Results) != 4 || out.Results[0].Name != "a" || out.Results[3].Name != "gone" {
		t.Fatalf("results must follow the requested order: %+v", out.Results)
	}
}

// 3.7: ending conversations is strictly more destructive than sending a
// message, and is gated exactly the same way.
func TestBulkCloseIsGatedLikeEveryOtherWrite(t *testing.T) {
	// read-only console: refused, and nothing reaches the manager
	api, f, _, _ := apiWithOptions(t, "tok", false, joinedConversation("a"))
	h := api.Handler(http.NotFoundHandler())
	rec, _ := postClose(t, h, `{"names":["a"]}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "read-only") {
		t.Fatalf("want a 403 naming the reason, got %d %s", rec.Code, rec.Body.String())
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing may reach the manager with writes disabled")
	}

	// unauthenticated
	api, f, _, _ = apiWithOptions(t, "tok", true, joinedConversation("a"))
	h = api.Handler(http.NotFoundHandler())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/conversations/close", `{"names":["a"]}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d %s", rec.Code, rec.Body.String())
	}

	// authenticated by a proxy that forwards no identity: there is nothing to
	// record the close against, so it is refused rather than attributed
	extAPI, extF := authAPI(t, "", false, "oauth2-proxy", joinedConversation("a"))
	h = extAPI.Handler(http.NotFoundHandler())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/conversations/close", `{"names":["a"]}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d %s", rec.Code, rec.Body.String())
	}
	if len(extF.inbounds()) != 0 {
		t.Fatal("nothing may be closed without an identity")
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("the unauthenticated request closed something")
	}
}

// 3.8: a conversation already held by its close-topics finalizer is skipped, so
// re-running the action over a batch cannot double-close.
func TestBulkCloseSkipsAConversationAlreadyClosing(t *testing.T) {
	closing := joinedConversation("going")
	closing.Metadata.DeletionTimestamp = "2026-08-12T10:00:00Z"
	api, f, _, _ := apiWithOptions(t, "tok", true, closing)
	h := api.Handler(http.NotFoundHandler())

	_, out := postClose(t, h, `{"names":["going"]}`)
	if out.Skipped != 1 || out.Closed != 0 {
		t.Fatalf("a closing conversation must be skipped: %+v", out)
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("no second /close may be posted")
	}
}
