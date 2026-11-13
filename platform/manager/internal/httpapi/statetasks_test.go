package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
)

func stateTasksReq(s *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+s.AdapterToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestStateTasksSurfaceRefusesWithoutABus(t *testing.T) {
	s := &Server{AdapterToken: "master"}
	for _, path := range []string{"/state/tasks", "/state/tasks/x", "/state/dead-letters"} {
		if rec := stateTasksReq(s, "GET", path); rec.Code != 503 {
			t.Fatalf("%s: want 503 with no Bus configured, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestStateTasksListReportsPerQueueStats(t *testing.T) {
	s, _, bus := busServer(t)
	if _, err := bus.Submit(context.Background(), commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "c1", Key: "root",
		Payload: convtask.Payload{Verb: convtask.VerbDelete, Conversation: "x"}.Encode(),
	}); err != nil {
		t.Fatal(err)
	}
	rec := stateTasksReq(s, "GET", "/state/tasks")
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Stats []struct {
			Queue   string `json:"queue"`
			Pending int    `json:"pending"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, st := range out.Stats {
		if st.Queue == "conv" {
			found = true
			if st.Pending < 1 {
				t.Fatalf("want at least one pending task on conv, got %d", st.Pending)
			}
		}
	}
	if !found {
		t.Fatalf("want a stats row for the conv queue, got %+v", out.Stats)
	}
}

func TestStateTaskGetServesOneTaskByID(t *testing.T) {
	s, _, bus := busServer(t)
	task, err := bus.Submit(context.Background(), commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "c1", Key: "root",
		Payload: convtask.Payload{Verb: convtask.VerbDelete, Conversation: "x"}.Encode(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := stateTasksReq(s, "GET", "/state/tasks/"+task.UID)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		UID   string `json:"uid"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.UID != task.UID || out.State != "queued" {
		t.Fatalf("want uid=%q state=queued, got %+v", task.UID, out)
	}
}

func TestStateTaskGetReports404ForAnUnknownID(t *testing.T) {
	s, _, _ := busServer(t)
	rec := stateTasksReq(s, "GET", "/state/tasks/does-not-exist")
	if rec.Code != 404 {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStateDeadLettersListsADeadTask(t *testing.T) {
	s, _, bus := busServer(t)
	task, err := bus.Submit(context.Background(), commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "c1", Key: "root",
		Payload: convtask.Payload{Verb: convtask.VerbReopen, Conversation: "nope"}.Encode(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := bus.Claim(context.Background(), ConvQueue, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Fail(context.Background(), claimed.UID, claimed.Lease.Token,
		commandbus.CodePreconditionFailed, "refused"); err != nil {
		t.Fatal(err)
	}
	rec := stateTasksReq(s, "GET", "/state/dead-letters")
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []struct {
			UID   string `json:"uid"`
			State string `json:"state"`
			Code  string `json:"code"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].UID != task.UID || out.Items[0].State != "dead" || out.Items[0].Code != "precondition_failed" {
		t.Fatalf("want one dead task %q, got %+v", task.UID, out.Items)
	}
}

// handleStateTasksStream is the same resumable-SSE shape state.go's own
// streams use — a submission after the stream opens arrives as one "put"
// frame, carrying the Sequence as its id. A REAL httptest.Server, read over
// a real socket, same as state_nats_test.go's own streaming tests: reading
// the body concurrently with the handler's writes to an
// httptest.ResponseRecorder's shared buffer would race.
func TestStateTasksStreamDeliversAPutFrameForANewSubmission(t *testing.T) {
	s, _, bus := busServer(t)
	ts := httptest.NewServer(http.HandlerFunc(s.handleStateTasksStream))
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	frames := sseFrames(resp.Body)

	if _, err := bus.Submit(context.Background(), commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "c1", Key: "root",
		Payload: convtask.Payload{Verb: convtask.VerbDelete, Conversation: "x"}.Encode(),
	}); err != nil {
		t.Fatal(err)
	}
	waitForPut(t, frames, "the new submission", `"queue":"conv"`)
}
