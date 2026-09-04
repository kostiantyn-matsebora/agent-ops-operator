package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestManagerSourcesRoundTrip closes the gap on Sources/do's success path: a
// real HTTP round trip against httptest.Server, asserting the adapter query
// param, the bearer header and the decoded body — none of this was exercised
// before (manager.go was at 0%).
func TestManagerSourcesRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/signal/sources" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("adapter"); got != "cron" {
			t.Errorf("adapter query = %q, want cron", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"s1","config":{"schedule":"* * * * *"},"credentialEnvPrefix":"P_"}]`))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "tok123")
	out, err := m.Sources(context.Background(), "cron")
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if len(out) != 1 || out[0].Name != "s1" || out[0].CredentialEnvPrefix != "P_" {
		t.Fatalf("unexpected sources: %+v", out)
	}
	if string(out[0].Config) != `{"schedule":"* * * * *"}` {
		t.Fatalf("unexpected raw config: %s", out[0].Config)
	}
}

// TestManagerSourcesErrorStatus closes the >=400 branch of do(): the body is
// read, trimmed and folded into the returned error.
func TestManagerSourcesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("  boom  \n"))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	_, err := m.Sources(context.Background(), "cron")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error missing status/body: %v", err)
	}
}

// TestManagerInboundPostsBody closes Inbound's POST path: verifies the
// request body shape, the Content-Type header set only when a body is
// present, and a 204 response that skips decoding entirely (out==nil path).
func TestManagerInboundPostsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/signal/inbound" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body struct {
			Source  string   `json:"source"`
			Signals []Signal `json:"signals"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Source != "src1" || len(body.Signals) != 1 || body.Signals[0].Fingerprint != "src1@t1" {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	err := m.Inbound(context.Background(), "src1", []Signal{{Fingerprint: "src1@t1", Kind: "job"}})
	if err != nil {
		t.Fatalf("Inbound: %v", err)
	}
}

// TestManagerGetStateDecodes closes GetState's decode path (out != nil,
// status 200).
func TestManagerGetStateDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/signal/state/src 1/last-fire" {
			t.Fatalf("unexpected path: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"value":"2026-08-06T06:00:00Z"}`))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	got, err := m.GetState(context.Background(), "src 1", "last-fire")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got != "2026-08-06T06:00:00Z" {
		t.Fatalf("got %q", got)
	}
}

// TestManagerGetStateBadJSON closes the JSON-decode-error branch of do(): a
// 2xx response whose body cannot be unmarshaled into `out`.
func TestManagerGetStateBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	if _, err := m.GetState(context.Background(), "s", "k"); err == nil {
		t.Fatal("expected a decode error")
	}
}

// TestManagerPutStatePutsBody closes PutState's PUT path and its URL-escaping
// of source/key.
func TestManagerPutStatePutsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/signal/state/s1/last-fire" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Value string `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Value != "2026-08-06T06:00:00Z" {
			t.Fatalf("unexpected value: %q", body.Value)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	if err := m.PutState(context.Background(), "s1", "last-fire", "2026-08-06T06:00:00Z"); err != nil {
		t.Fatalf("PutState: %v", err)
	}
}

// TestManagerReportStatusPostsBody closes ReportStatus's POST path.
func TestManagerReportStatusPostsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/signal/sources/s1/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body struct {
			Ready   bool   `json:"ready"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Ready || body.Reason != "InvalidConfig" || body.Message != "bad" {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "t")
	if err := m.ReportStatus(context.Background(), "s1", false, "InvalidConfig", "bad"); err != nil {
		t.Fatalf("ReportStatus: %v", err)
	}
}

// TestManagerDoMarshalError closes do()'s json.Marshal error branch by
// calling the unexported method directly (same package) with a value the
// encoder rejects — a real marshal failure, not a stand-in.
func TestManagerDoMarshalError(t *testing.T) {
	m := NewManager("http://unused.invalid", "t")
	err := m.do(context.Background(), "POST", "/x", make(chan int), nil)
	if err == nil {
		t.Fatal("expected a marshal error")
	}
}

// TestManagerDoBadMethod closes do()'s http.NewRequestWithContext error
// branch: a method string containing a space is rejected by net/http itself.
func TestManagerDoBadMethod(t *testing.T) {
	m := NewManager("http://unused.invalid", "t")
	err := m.do(context.Background(), "BAD METHOD", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected a request-construction error")
	}
}

// TestManagerDoConnectionRefused closes do()'s HTTP-client transport-error
// branch: a real closed listener refuses the connection immediately.
func TestManagerDoConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now nothing is listening on this port

	m := NewManager(url, "t")
	if err := m.do(context.Background(), "GET", "/x", nil, nil); err == nil {
		t.Fatal("expected a connection error")
	}
}
