package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Delete and reopen, the two verbs that exist because closing stopped deleting.
//
// A CLOSED conversation holds no thread, so neither can be a command posted on
// one. Both are manager verbs the console CALLS — the console still performs no
// Kubernetes write, and these tests pin that its own decisions come from cached
// CR state rather than from the request.

type deleteResponse struct {
	Results []CloseResult `json:"results"`
	Deleted int           `json:"deleted"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
}

func closedConversation(name string) *Object {
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"phase":"Closed","threads":[{"channel":"console","threadId":"console-uid-`+name+`"}]}`)
}

func postDelete(t *testing.T, h http.Handler, body string) (*httptest.ResponseRecorder, deleteResponse) {
	t.Helper()
	rec := authed(t, h, "POST", "/api/conversations/delete", body)
	var out deleteResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("delete body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, out
}

// The ordinary case: closed conversations are reclaimed, one manager call each.
func TestBulkDeleteReclaimsClosedConversations(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		closedConversation("a"), closedConversation("b"))
	h := api.Handler(http.NotFoundHandler())

	rec, out := postDelete(t, h, `{"names":["a","b"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	if out.Deleted != 2 || out.Skipped != 0 || out.Failed != 0 {
		t.Fatalf("totals: %+v", out)
	}
	if got := f.calledVerbs(); len(got) != 2 || got[0] != "delete:a" || got[1] != "delete:b" {
		t.Fatalf("one delete per conversation: %v", got)
	}
	// and NOT through /channel/inbound: there is no thread to post on
	if len(f.inbounds()) != 0 {
		t.Fatalf("deleting must not post a message: %v", f.inbounds())
	}
}

// THE safety property. A live conversation named in a delete batch is skipped
// with "close it first" — never closed on the way through, because one call
// doing the irreversible thing to a conversation that was still working, behind
// a confirmation naming only the delete, is what the two-step prevents.
func TestBulkDeleteRefusesAConversationThatIsNotClosed(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		closedConversation("done"), joinedConversation("live"))
	h := api.Handler(http.NotFoundHandler())

	_, out := postDelete(t, h, `{"names":["done","live"]}`)
	if out.Deleted != 1 || out.Skipped != 1 || out.Failed != 0 {
		t.Fatalf("a live conversation must not fail the batch: %+v", out)
	}
	res := outcomes(out.Results)
	if res["live"].Outcome != closeOutcomeSkipped || !strings.Contains(res["live"].Reason, "close it first") {
		t.Fatalf("the skip must name the missing step: %+v", res["live"])
	}
	// nothing at all happened to the live one
	for _, v := range f.calledVerbs() {
		if strings.HasSuffix(v, ":live") {
			t.Fatalf("a refused delete must not touch the conversation: %v", f.calledVerbs())
		}
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("a refused delete must not close it either")
	}
}

// The bound is the page size, enforced server-side: the blast radius equals one
// screen regardless of what the client sends.
func TestBulkDeleteBoundIsServerEnforced(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true)
	h := api.Handler(http.NotFoundHandler())

	names := make([]string, conversationPageSize+1)
	for i := range names {
		names[i] = `"c` + strconv.Itoa(i) + `"`
	}
	rec, _ := postDelete(t, h, `{"names":[`+strings.Join(names, ",")+`]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an oversized batch must be refused, got %d", rec.Code)
	}
	if rec2, _ := postDelete(t, h, `{"names":[]}`); rec2.Code != http.StatusBadRequest {
		t.Fatalf("an empty batch must be refused, got %d", rec2.Code)
	}
}

// A batch never aborts: a failing item is recorded and the walk continues.
func TestBulkDeleteReportsPerItemAndNeverAborts(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		closedConversation("a"), closedConversation("boom"), closedConversation("b"))
	f.failVerb = map[string]bool{"boom": true}
	h := api.Handler(http.NotFoundHandler())

	_, out := postDelete(t, h, `{"names":["a","boom","b","ghost"]}`)
	if out.Deleted != 2 || out.Failed != 2 {
		t.Fatalf("the walk must continue past a failure: %+v", out)
	}
	res := outcomes(out.Results)
	if res["boom"].Outcome != closeOutcomeFailed {
		t.Fatalf("a manager refusal is a failure, not a skip: %+v", res["boom"])
	}
	if res["ghost"].Outcome != closeOutcomeFailed {
		t.Fatalf("an unknown name is reported, not ignored: %+v", res["ghost"])
	}
}

// Reopen is per conversation, and only for a closed one.
func TestReopenOnlyAppliesToAClosedConversation(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true,
		closedConversation("done"), joinedConversation("live"))
	h := api.Handler(http.NotFoundHandler())

	if rec := authed(t, h, "POST", "/api/conversations/done/reopen", ""); rec.Code != http.StatusOK {
		t.Fatalf("reopening a closed conversation: %d %s", rec.Code, rec.Body.String())
	}
	if got := f.calledVerbs(); len(got) != 1 || got[0] != "reopen:done" {
		t.Fatalf("one reopen call: %v", got)
	}

	rec := authed(t, h, "POST", "/api/conversations/live/reopen", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("only a closed conversation can be reopened, got %d", rec.Code)
	}
	if len(f.calledVerbs()) != 1 {
		t.Fatalf("a refused reopen must reach the manager not at all: %v", f.calledVerbs())
	}
}

// The manager refuses a reopen whose wiring is gone and NAMES the missing
// object. Flattening that to "reopen failed" would send nobody anywhere.
func TestReopenPassesTheManagersReasonThrough(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", true, closedConversation("done"))
	f.failVerb = map[string]bool{"done": true}
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "POST", "/api/conversations/done/reopen", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "reopen refused for done") {
		t.Fatalf("the manager's own reason must survive: %s", rec.Body.String())
	}
}

// Both verbs are WRITES, so both sit behind the same gate as everything else.
func TestVerbsRequireTheWriteGate(t *testing.T) {
	api, f, _, _ := apiWithOptions(t, "tok", false, closedConversation("done"))
	h := api.Handler(http.NotFoundHandler())

	if rec, _ := postDelete(t, h, `{"names":["done"]}`); rec.Code == http.StatusOK {
		t.Fatalf("a read-only console must not delete, got %d", rec.Code)
	}
	if rec := authed(t, h, "POST", "/api/conversations/done/reopen", ""); rec.Code == http.StatusOK {
		t.Fatalf("a read-only console must not reopen, got %d", rec.Code)
	}
	if got := f.calledVerbs(); len(got) != 0 {
		t.Fatalf("nothing may reach the manager: %v", got)
	}
}

// A reopened conversation must be TYPEABLE again, and must say so.
//
// Found live: closing marks the console transcript archived, the UI derives
// "you cannot reply here" from that flag, and nothing ever cleared it. A
// reopened conversation therefore rendered alive with no composer — and with
// no line in the transcript explaining why it had gone quiet in the first
// place, or that it was back.
func TestReopenMakesTheConsoleThreadLiveAgain(t *testing.T) {
	f := newFakeManager(t)
	adapter, transcripts, _ := consoleUnderTest(t, f, closedConversation("conv-1"))
	tid := adapter.threadID("conv-1")

	// close: the thread is archived and says so
	adapter.execute(context.Background(), &Op{Kind: "close-topic", Conversation: "conv-1", ThreadID: &tid})
	if !transcripts.Archived(tid) {
		t.Fatal("close-topic must archive the console thread")
	}

	// reopen arrives as an ordinary ensure-topic
	got, opErr := adapter.execute(context.Background(), &Op{Kind: "ensure-topic", Conversation: "conv-1"})
	if opErr != "" || got != tid {
		t.Fatalf("reopen must return the same thread: %q %q", got, opErr)
	}
	if transcripts.Archived(tid) {
		t.Fatal("a reopened thread must not stay archived — the composer is derived from this")
	}
}

// ensure-topic is at-least-once and is delivered for threads that were never
// closed: it must not announce a reopen that did not happen.
func TestEnsureTopicOnALiveThreadSaysNothing(t *testing.T) {
	f := newFakeManager(t)
	adapter, transcripts, _ := consoleUnderTest(t, f, joinedConversation("conv-2"))
	tid := adapter.threadID("conv-2")

	adapter.execute(context.Background(), &Op{Kind: "ensure-topic", Conversation: "conv-2"})
	adapter.execute(context.Background(), &Op{Kind: "ensure-topic", Conversation: "conv-2"})
	if transcripts.Archived(tid) {
		t.Fatal("a live thread must stay live")
	}
}

// A CLOSED conversation is not typeable, and that must not depend on this
// console process having witnessed the close.
//
// The transcript's archived flag is per console SESSION. It was enough while
// closing deleted the conversation — the row vanished — but a closed
// conversation now persists, and a console restart put the composer back on
// every one of them. The phase is on the CR and survives everything.
func TestAClosedConversationIsNotTypeableAfterAConsoleRestart(t *testing.T) {
	// A fresh console that has never seen the close-topic op for this thread:
	// its transcript state is empty, exactly as after a restart.
	api, _, _, _ := apiWithOptions(t, "tok", true, closedConversation("done"))
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/conversations/done", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if archived, _ := out["archived"].(bool); !archived {
		t.Fatal("a Closed conversation must report archived from its PHASE, not from in-memory state")
	}
}

// ...and a live one stays typeable, or the fix would disable every composer.
func TestALiveConversationStaysTypeable(t *testing.T) {
	api, _, _, _ := apiWithOptions(t, "tok", true, joinedConversation("live"))
	h := api.Handler(http.NotFoundHandler())

	rec := authed(t, h, "GET", "/api/conversations/live", "")
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if archived, _ := out["archived"].(bool); archived {
		t.Fatal("a live conversation must not be reported archived")
	}
}

// A deleted conversation's console transcript keeps the tombstone and stops
// offering a composer. The CR is about to vanish from the watch cache; the
// transcript is not, so whoever was reading can still see how it ended.
func TestDeleteConversationLeavesAReadableArchivedTranscript(t *testing.T) {
	f := newFakeManager(t)
	adapter, transcripts, _ := consoleUnderTest(t, f, closedConversation("conv-1"))
	tid := adapter.threadID("conv-1")

	msg := OpMessage{Kind: "notice", Body: "This conversation has been deleted."}
	if _, opErr := adapter.execute(context.Background(), &Op{
		ID: "gone:conv-1:console", Kind: "delete-conversation", Conversation: "conv-1",
		ThreadID: &tid, Message: &msg,
	}); opErr != "" {
		t.Fatalf("delete-conversation: %s", opErr)
	}
	if !transcripts.Archived(tid) {
		t.Fatal("a deleted conversation's thread must be archived — there is nothing to reply to")
	}
	var tombstone bool
	for _, m := range transcripts.Thread(tid) {
		if strings.Contains(m.Text, "deleted") {
			tombstone = true
		}
	}
	if !tombstone {
		t.Fatal("the transcript must keep the notice saying the conversation was deleted")
	}
}
