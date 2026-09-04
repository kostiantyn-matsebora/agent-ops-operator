package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Manager.do had three untested branches (json.Marshal, request
// construction, and the transport error) plus its two callers (Sources,
// ReportStatus) at 0% -- refreshSources is the only production caller and it
// was itself untested. These exercise the client directly, the same way
// channel-telegram's errorpaths_test.go drives its Manager against real
// httptest servers rather than a mock transport.

// do's json.Marshal branch: every real caller passes a marshalable payload,
// so this is the only way to reach it -- a channel value fails to encode.
func TestDoReturnsAnErrorWhenTheBodyCannotBeMarshaled(t *testing.T) {
	m := NewManager("http://example.invalid", "tok")
	if err := m.do(context.Background(), "POST", "/x", make(chan int), nil); err == nil {
		t.Fatal("want an error for an unmarshalable body")
	}
}

// do's http.NewRequestWithContext branch: a method containing a space is not
// a valid HTTP token and is rejected before any network I/O.
func TestDoReturnsAnErrorWhenTheRequestCannotBeBuilt(t *testing.T) {
	m := NewManager("http://example.invalid", "tok")
	if err := m.do(context.Background(), "BAD METHOD", "/x", nil, nil); err == nil {
		t.Fatal("want an error for an invalid method")
	}
}

// do's HTTP.Do branch: nothing listens on this loopback port, so the dial
// itself fails -- a real network error, not a stand-in for one.
func TestDoReturnsAnErrorWhenTheServerIsUnreachable(t *testing.T) {
	m := NewManager("http://127.0.0.1:1", "tok")
	if err := m.do(context.Background(), "GET", "/x", nil, nil); err == nil {
		t.Fatal("want a connection error")
	}
}

// do's out!=nil-but-204 branch: a 204 with a decode target must be skipped
// rather than fed to json.Decode against an empty body.
func TestDoSkipsDecodingOnNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m := NewManager(srv.URL, "tok")
	var out []SourceInfo
	if err := m.do(context.Background(), "GET", "/x", nil, &out); err != nil {
		t.Fatalf("204 with a decode target must not error: %v", err)
	}
}

// Sources is the GET half of the contract -- untested at 0% because only
// refreshSources calls it. Pins the query string, the auth header and the
// decoded shape.
func TestSourcesBuildsTheRequestAndDecodesTheList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/signal/sources" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("adapter"); got != "telegram" {
			t.Fatalf("adapter query = %q, want telegram", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SourceInfo{{Name: "tg-chat", Config: json.RawMessage(`{"chatId":"1"}`)}})
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "tok")
	out, err := m.Sources(context.Background(), "telegram")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "tg-chat" {
		t.Fatalf("out = %+v", out)
	}
}

// do's resp.StatusCode>=400 branch, through the public Sources call so the
// error text is what a real caller actually sees.
func TestSourcesSurfacesTheServersErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "tok")
	if _, err := m.Sources(context.Background(), "telegram"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want it to name the 403 status", err)
	}
}

// ReportStatus is the POST half of the contract, also 0% covered before this
// -- only refreshSources called it. Pins the path and the JSON body shape.
func TestReportStatusPostsTheCondition(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/signal/sources/tg-chat/status" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	m := NewManager(srv.URL, "tok")
	if err := m.ReportStatus(context.Background(), "tg-chat", true, "AdapterReady", "ok"); err != nil {
		t.Fatal(err)
	}
	if gotBody["ready"] != true || gotBody["reason"] != "AdapterReady" || gotBody["message"] != "ok" {
		t.Fatalf("body = %v", gotBody)
	}
}
