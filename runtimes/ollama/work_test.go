package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Control.Poll and Control.Report were entirely untested (0% coverage) --
// this file drives both against a real net/http server, exactly as the
// runtime does against the manager's /work endpoints.

func TestPollDecodesAWorkUnit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if got := r.URL.Query().Get("convo"); got != "c1" {
			t.Errorf("convo query: %q", got)
		}
		json.NewEncoder(w).Encode(WorkUnit{RunID: "r1", Convo: "c1", PromptText: "hi"})
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, Convo: "c1", Pod: "p1", HTTP: srv.Client()}
	unit, err := c.Poll(context.Background())
	if err != nil || unit == nil || unit.RunID != "r1" || unit.PromptText != "hi" {
		t.Fatalf("unit=%+v err=%v", unit, err)
	}
}

// A 204 means no work is due; Poll must return (nil, nil), not an error.
func TestPollNoContentMeansNoWork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	unit, err := c.Poll(context.Background())
	if err != nil || unit != nil {
		t.Fatalf("want (nil, nil) on 204, got %+v %v", unit, err)
	}
}

// A non-2xx status must surface as a readable error naming the status.
func TestPollNonOKStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "GET /work") {
		t.Fatalf("want a GET /work error, got %v", err)
	}
}

// A body that is not valid JSON must fail to decode rather than silently
// return a zero-value unit.
func TestPollMalformedBodyFailsToDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("want a decode error, got %v", err)
	}
}

// A transport failure (server unreachable) must surface as an error too.
func TestPollTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed before use: connections are refused
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Poll(context.Background()); err == nil {
		t.Fatal("want an error against an unreachable server")
	}
}

// Report succeeds on the first attempt and posts the runId/convo/result
// fields the manager's /work/done contract expects, plus the retired
// sessionId spelling for one release.
func TestReportPostsTheDoneBodyAndSucceeds(t *testing.T) {
	var got doneReport
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/work/done" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, Convo: "c1", HTTP: srv.Client()}
	res := RunResult{Status: "succeeded", RuntimeContextID: "oc-abc"}
	var slept []time.Duration
	if err := c.Report(context.Background(), "r1", res, func(d time.Duration) { slept = append(slept, d) }); err != nil {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 || len(slept) != 0 {
		t.Errorf("must succeed on the first attempt without sleeping: calls=%d slept=%v", calls, slept)
	}
	if got.Convo != "c1" || got.RunID != "r1" || got.SessionID != "oc-abc" {
		t.Errorf("done body: %+v", got)
	}
}

// A server that never answers 2xx is retried, ten seconds apart per the
// reference cadence, until the injected sleep exhausts the retry budget --
// exercised with an instant sleep so the test itself is fast.
func TestReportRetriesThenGivesUpNamingTheLastError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	var slept []time.Duration
	err := c.Report(context.Background(), "r1", RunResult{}, func(d time.Duration) { slept = append(slept, d) })
	if err == nil || !strings.Contains(err.Error(), "POST /work/done") {
		t.Fatalf("want the last HTTP error, got %v", err)
	}
	if len(slept) != 60 || slept[0] != 10*time.Second {
		t.Errorf("want 60 retries at 10s apart, got %d: %v", len(slept), slept)
	}
}

// A DEFINITIVE rejection -- a 4xx the manager will never accept unchanged --
// must be logged with its status and body and NOT retried: resending the
// same bytes cannot change the verdict, so the poll loop must resume at
// once rather than spend ten minutes re-sending a report that will never be
// accepted (1.5, pins the runtime-contract delta's rejection rule).
func TestReportDoesNotRetryOnADefinitiveRejection(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("result field exceeds the bound"))
	}))
	defer srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	var slept []time.Duration
	err := c.Report(context.Background(), "r1", RunResult{}, func(d time.Duration) { slept = append(slept, d) })
	if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "result field exceeds the bound") {
		t.Fatalf("want an error naming the status and body, got %v", err)
	}
	if calls != 1 || len(slept) != 0 {
		t.Errorf("a definitive rejection must not be retried: calls=%d slept=%v", calls, slept)
	}
}

// 408 and 429 are 4xx too, but both are transient by nature and must keep
// their bounded retry rather than being treated as definitive.
func TestIsDefinitiveRejectionExemptsTimeoutAndRateLimit(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests} {
		if isDefinitiveRejection(status) {
			t.Errorf("status %d must keep its retry, not be treated as definitive", status)
		}
	}
	if !isDefinitiveRejection(http.StatusBadRequest) {
		t.Error("an ordinary 400 must be definitive")
	}
	if isDefinitiveRejection(http.StatusServiceUnavailable) {
		t.Error("a 5xx must never be treated as definitive")
	}
}

// A transport-level failure (not an HTTP status) is retried the same way and
// its error is what's returned once the budget is exhausted.
func TestReportRetriesOnTransportFailureToo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	c := &Control{BaseURL: srv.URL, HTTP: srv.Client()}
	var attempts int
	err := c.Report(context.Background(), "r1", RunResult{}, func(time.Duration) { attempts++ })
	if err == nil {
		t.Fatal("want the transport error surfaced")
	}
	if attempts != 60 {
		t.Errorf("want 60 attempts, got %d", attempts)
	}
}
