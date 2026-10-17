package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// manager.go's contract client (do, Sources, Inbound, ReportStatus) is
// exercised indirectly, end to end, by every test in main_test.go and
// register_test.go through the adapter's real HTTP calls — that already
// covers the happy paths. This file closes do()'s error branches, which
// none of those paths reach because the fake manager server they drive
// never fails.

// TestManagerDoMarshalError closes do()'s json.Marshal error branch: an
// unmarshalable body must fail before any request is attempted.
func TestManagerDoMarshalError(t *testing.T) {
	m := NewManager("http://unused", "t")
	if err := m.do(context.Background(), "POST", "/x", make(chan int), nil); err == nil {
		t.Fatal("unmarshalable input should error before any request is sent")
	}
}

// TestManagerDoInvalidMethodError closes do()'s http.NewRequestWithContext
// error branch with a real invalid HTTP method.
func TestManagerDoInvalidMethodError(t *testing.T) {
	m := NewManager("http://unused", "t")
	if err := m.do(context.Background(), "BAD METHOD", "/x", nil, nil); err == nil {
		t.Fatal("invalid HTTP method should be rejected before any request is sent")
	}
}

// TestManagerDoConnectionError closes do()'s m.HTTP.Do error branch against
// a real refused TCP connection, not a mocked transport.
func TestManagerDoConnectionError(t *testing.T) {
	m := &Manager{BaseURL: "http://127.0.0.1:1", Token: "t", HTTP: &http.Client{Timeout: 2 * time.Second}}
	if err := m.do(context.Background(), "GET", "/x", nil, nil); err == nil {
		t.Fatal("a refused connection should surface as an error")
	}
}

// TestManagerDoHTTPErrorStatus closes do()'s status>=400 branch against a
// real server returning a real error body.
func TestManagerDoHTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	m := NewManager(srv.URL, "t")
	err := m.do(context.Background(), "GET", "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("an error status should carry the code and body: %v", err)
	}
}
