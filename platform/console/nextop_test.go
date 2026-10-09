package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	op, err := mgr.NextOp(context.Background(), "console", 0)
	if err != nil {
		t.Fatalf("503 must not surface as an error: %v", err)
	}
	if op != nil {
		t.Fatalf("503 must not be read as a delivered op: %+v", op)
	}
}
