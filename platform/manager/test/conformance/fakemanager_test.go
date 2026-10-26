//go:build conformance

package conformance

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Direct HTTP exercise of FakeManager's work-contract additions (GET /work,
// POST /work/done), the same way a runtime's own Control would call them —
// no built binary involved. These pin the FAKE's own behavior; runtime_test.go
// pins the real runtimes/ollama binary against it.

func fakeManagerGet(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestFakeManagerServesAQueuedWorkUnitOnceThenNothing(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	mgr.QueueWork(map[string]any{"runId": "r1", "convo": "c1", "promptText": "hi"})

	code, body := fakeManagerGet(t, mgr.URL()+"/work?convo=c1&pod=p1&wait=0")
	if code != 200 || !strings.Contains(body, `"runId":"r1"`) || !strings.Contains(body, `"convo":"c1"`) {
		t.Fatalf("first poll: %d %s", code, body)
	}

	code, _ = fakeManagerGet(t, mgr.URL()+"/work?convo=c1&pod=p1&wait=0")
	if code != 204 {
		t.Fatalf("a second poll with nothing queued must be 204, got %d", code)
	}

	reqs := mgr.WorkRequests()
	if len(reqs) != 2 || reqs[0].Convo != "c1" || reqs[0].Pod != "p1" {
		t.Fatalf("every GET /work must be recorded: %+v", reqs)
	}
}

func TestFakeManagerWorkPollWithoutConvoIsRefused(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	code, body := fakeManagerGet(t, mgr.URL()+"/work?wait=0")
	if code != 400 || !strings.Contains(body, "missing convo") {
		t.Fatalf("an empty convo must be refused naming it: %d %s", code, body)
	}
}

func TestFakeManagerWorkPollLongPollsUntilAUnitArrives(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	start := time.Now()
	done := make(chan struct{})
	var code int
	go func() {
		code, _ = fakeManagerGet(t, mgr.URL()+"/work?convo=c1&wait=5")
		close(done)
	}()
	time.Sleep(200 * time.Millisecond)
	mgr.QueueWork(map[string]any{"runId": "r2", "convo": "c1"})
	<-done
	if code != 200 {
		t.Fatalf("the long poll must return the unit once queued, got %d", code)
	}
	if time.Since(start) >= 5*time.Second {
		t.Fatalf("the unit must be delivered as soon as it is queued, not after the full wait")
	}
}

func TestFakeManagerWorkDoneRecordsTheCompletion(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	code, body := postJSON(t, mgr.URL()+"/work/done",
		[]byte(`{"convo":"c1","runId":"r1","status":"succeeded","runtimeContextId":"ctx-1"}`), nil)
	if code != 200 || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("POST /work/done: %d %s", code, body)
	}
	cs := mgr.WorkCompletions()
	if len(cs) != 1 || cs[0].RunID != "r1" || cs[0].Convo != "c1" || cs[0].Raw["status"] != "succeeded" {
		t.Fatalf("the completion must be recorded with its decoded body: %+v", cs)
	}
	if ctxID, _ := mgr.WorkState("c1"); ctxID != "ctx-1" {
		t.Fatalf("a reported runtimeContextId must update the latest-wins state, got %q", ctxID)
	}
}

func TestFakeManagerWorkDoneCanBeRejected(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	mgr.RejectWorkDone(500)
	code, _ := postJSON(t, mgr.URL()+"/work/done", []byte(`{"convo":"c1","runId":"r1","status":"succeeded"}`), nil)
	if code != 500 {
		t.Fatalf("a rejected /work/done must answer the configured status, got %d", code)
	}
	if len(mgr.WorkCompletions()) != 0 {
		t.Fatalf("a rejected report must not be recorded as a completion")
	}
}

func TestFakeManagerWorkStateLeavesOmittedFieldsUntouched(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	postJSON(t, mgr.URL()+"/work/done",
		[]byte(`{"convo":"c1","runId":"r1","status":"succeeded","runtimeContextId":"ctx-1","brief":"first brief"}`), nil)
	// A second report for the same conversation that omits both fields must
	// leave the previously recorded ones exactly as they were (API-5).
	postJSON(t, mgr.URL()+"/work/done", []byte(`{"convo":"c1","runId":"r2","status":"failed"}`), nil)
	ctxID, brief := mgr.WorkState("c1")
	if ctxID != "ctx-1" || brief != "first brief" {
		t.Fatalf("omitted fields on a later report must not clear the earlier ones: ctx=%q brief=%q", ctxID, brief)
	}
}
