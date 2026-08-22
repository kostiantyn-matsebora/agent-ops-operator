package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newStatusManager returns a Manager pointed at a stub serving a fixed
// /status — the manager-internal state exists nowhere else, so a fixture is the
// only way to test the views built on it.
func newStatusManager(t *testing.T, status *ManagerStatus) *Manager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(status)
	}))
	t.Cleanup(srv.Close)
	return NewManager(srv.URL, "tok")
}

// contextWithCancel makes a request's context cancellable, so a streaming
// handler can be stopped from the test rather than run to its keep-alive.
func contextWithCancel(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithCancel(r.Context())
}
