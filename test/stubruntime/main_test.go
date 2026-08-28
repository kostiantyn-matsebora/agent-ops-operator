package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func withHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// Identical input, identical report. The stub is an instrument; a result that
// varied between runs would turn every failure into a question about variance.
func TestSameInputSameReport(t *testing.T) {
	withHome(t)
	u := unit{RunID: "r1", Convo: "c", PromptText: "some prose\n\necho hello there\n"}
	a, _ := perform(u)
	b, _ := perform(u)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("reports differ:\n%+v\n%+v", a, b)
	}
	if a.Result != "[stub] hello there" || a.Status != "succeeded" || a.Continuity != "new" || a.RuntimeContextID != "stub-ctx-c" {
		t.Fatalf("echo report: %+v", a)
	}
}

// Each directive guards a named mechanism, so its report shape is pinned.
func TestDirectives(t *testing.T) {
	withHome(t)
	first := func(text string) report {
		r, ok := perform(unit{RunID: "r", Convo: "c", PromptVars: map[string]string{"USER_TASK": text}})
		if !ok {
			t.Fatalf("%q must report", text)
		}
		return r
	}
	if r := first("fail now"); r.Status != "failed" || r.ExitCode == nil || *r.ExitCode != 1 {
		t.Fatalf("fail: %+v", r)
	}
	if r := first("no-context"); r.RuntimeContextID != "" || r.Continuity != "new" || r.Status != "succeeded" {
		t.Fatalf("no-context: %+v", r)
	}
	if r := first("storage-outage"); r.Continuity != "unavailable" || r.Status != "failed" || r.ContinuityReason == "" {
		t.Fatalf("storage-outage: %+v", r)
	}
	if r := first("plain words with no directive"); r.Result != "[stub] plain words with no directive" {
		t.Fatalf("default is echo: %+v", r)
	}
	for _, d := range []string{"die", "stall"} {
		if _, ok := perform(unit{RunID: "r", Convo: "c", PromptVars: map[string]string{"USER_TASK": d}}); ok {
			t.Fatalf("%s must not report", d)
		}
	}
}

// A handle that names nothing FAILS the next run instead of being carried
// forward: promised-and-lost. echo continues a handle whose file exists.
func TestStaleHandleFailsTheNextRun(t *testing.T) {
	withHome(t)
	stale, _ := perform(unit{RunID: "r1", Convo: "c", PromptVars: map[string]string{"USER_TASK": "stale-context"}})
	if stale.Status != "succeeded" || stale.RuntimeContextID != "stub-stale-c" {
		t.Fatalf("stale-context: %+v", stale)
	}
	next, _ := perform(unit{RunID: "r2", Convo: "c", RuntimeContextID: stale.RuntimeContextID, PromptVars: map[string]string{"USER_TASK": "echo again"}})
	if next.Status != "failed" || next.Continuity != "unavailable" {
		t.Fatalf("continuing a stale handle must fail the run: %+v", next)
	}
	// A real handle continues, and survives a "restart" (a new perform) as a file.
	ok1, _ := perform(unit{RunID: "r3", Convo: "c", PromptVars: map[string]string{"USER_TASK": "echo one"}})
	ok2, _ := perform(unit{RunID: "r4", Convo: "c", RuntimeContextID: ok1.RuntimeContextID, PromptVars: map[string]string{"USER_TASK": "echo two"}})
	if ok2.Status != "succeeded" || ok2.Continuity != "continued" || ok2.RuntimeContextID != ok1.RuntimeContextID {
		t.Fatalf("a kept handle must continue: %+v", ok2)
	}
	if _, err := os.Stat(contextsDir() + "/" + ok1.RuntimeContextID); err != nil {
		t.Fatalf("the handle must be a file on the context volume: %v", err)
	}
}

// The loop is a conforming client: poll with convo/pod/wait, report the run,
// and `die` exits WITHOUT reporting.
func TestContractLoopAndDie(t *testing.T) {
	withHome(t)
	var mu sync.Mutex
	var polls, reports []string
	units := []string{"echo hi", "die"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/work":
			polls = append(polls, r.URL.RawQuery)
			if len(units) == 0 {
				w.WriteHeader(204)
				return
			}
			u := unit{RunID: "r" + string(rune('0'+len(polls))), Convo: "c", PromptVars: map[string]string{"USER_TASK": units[0]}}
			units = units[1:]
			_ = json.NewEncoder(w).Encode(u)
		case "/work/done":
			var rep report
			_ = json.NewDecoder(r.Body).Decode(&rep)
			reports = append(reports, rep.Result)
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	exited := make(chan int, 1)
	done := make(chan struct{})
	go func() {
		_ = run(srv.URL, "c", "pod-1", time.Minute, func(code int) { exited <- code; close(done); select {} })
	}()
	select {
	case code := <-exited:
		if code != 3 {
			t.Fatalf("die must exit 3, got %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop never reached die")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 1 || reports[0] != "[stub] hi" {
		t.Fatalf("exactly the echo run is reported, die reports nothing: %v", reports)
	}
	if polls[0] != "convo=c&pod=pod-1&wait=25" {
		t.Fatalf("poll query: %q", polls[0])
	}
}
