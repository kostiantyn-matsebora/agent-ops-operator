package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A manager that dies without closing its connection (a node lost, a rollout
// behind a proxy that keeps the socket) leaves the stream open and silent. The
// console cannot tell that from a quiet system unless silence has a limit, so
// a stream that says nothing, not even a keep-alive, for the limit is dead.
func TestAStreamThatGoesSilentIsGivenUpOnSoTheCacheReconnects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // says nothing, ever
	}))
	t.Cleanup(srv.Close)
	s := newStateSource(&Config{ManagerURL: srv.URL})
	s.idleTimeout = 200 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- s.Watch(context.Background(), "conversations", "5", func(string, *Object) {})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errStreamIdle) {
			t.Fatalf("Watch = %v, want errStreamIdle so the cache reconnects", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a silent stream was never given up on")
	}
}

func TestKeepAlivesHoldAQuietStreamOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		for i := 0; i < 8; i++ {
			_, _ = w.Write([]byte(": keep-alive\n\n"))
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	s := newStateSource(&Config{ManagerURL: srv.URL})
	s.idleTimeout = 250 * time.Millisecond
	// ends cleanly when the server finishes, never idle: keep-alives arrive
	// well inside the limit although the whole stream outlasts it.
	if err := s.Watch(context.Background(), "conversations", "", func(string, *Object) {}); err != nil {
		t.Fatalf("Watch = %v, want a clean end", err)
	}
}

func TestTheDefaultIdleLimitIsSeveralKeepAlives(t *testing.T) {
	s := newStateSource(&Config{ManagerURL: "http://x"})
	if s.idleTimeout < 3*20*time.Second {
		t.Fatalf("idle limit %v must span several of the manager's 20s keep-alives", s.idleTimeout)
	}
	_ = strings.TrimSpace
}
