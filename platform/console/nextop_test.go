package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
)

// durable-chat-ops-broker: a manager replica that is not the current leader
// answers /channel/ops with 503, never a 204. NextOp must treat it exactly
// like an empty poll — never as a failure the caller logs and backs off
// from — so a poll pinned to the wrong replica is retried at once.
func TestNextOpTreats503AsNoOpRatherThanAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	mgr := NewManager(srv.URL, "tok")

	op, _, err := mgr.NextOp(context.Background(), "console", 0)
	if err != nil {
		t.Fatalf("503 must not surface as an error: %v", err)
	}
	if op != nil {
		t.Fatalf("503 must not be read as a delivered op: %+v", op)
	}
}

// "Retry at once" is theatre if the retry reuses the same connection: a
// Kubernetes Service picks a backend per TCP connection, not per request, so
// a keep-alive connection pinned to a non-leader would ask that same
// non-leader forever. NextOp must close the connection it just got a 503 on,
// so the next call dials fresh and has a new chance to land elsewhere.
func TestNextOpClosesTheConnectionOnANonLeaderAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	mgr := NewManager(srv.URL, "tok")

	if _, _, err := mgr.NextOp(context.Background(), "console", 0); err != nil {
		t.Fatalf("first poll: %v", err)
	}

	reused := true
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
	}
	ctx := httptrace.WithClientTrace(context.Background(), trace)
	if _, _, err := mgr.NextOp(ctx, "console", 0); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if reused {
		t.Fatal("the poll after a 503 reused the same connection — a pin to a non-leader would never clear")
	}
}

// ADA-6: the vocabulary revision rides every /channel/ops answer, 204 and 200
// alike, because that is the only dial a manager has into an otherwise idle
// adapter.
func TestNextOpCarriesTheVocabularyRevisionOnEveryStatus(t *testing.T) {
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(VocabularyRevisionHeader, "rev-7")
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	mgr := NewManager(srv.URL, "tok")

	_, rev, err := mgr.NextOp(context.Background(), "console", 0)
	if err != nil {
		t.Fatalf("204 poll: %v", err)
	}
	if rev != "rev-7" {
		t.Fatalf("revision not carried on 204: %q", rev)
	}

	status = http.StatusOK
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(VocabularyRevisionHeader, "rev-7")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ensure-topic:1","channel":"console","kind":"ensure-topic"}`))
	})
	op, rev, err := mgr.NextOp(context.Background(), "console", 0)
	if err != nil {
		t.Fatalf("200 poll: %v", err)
	}
	if op == nil || rev != "rev-7" {
		t.Fatalf("revision not carried on 200: op=%+v rev=%q", op, rev)
	}
}
