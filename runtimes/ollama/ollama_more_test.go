package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newHTTPClient was entirely untested (0%): it is the one place the client
// timeout is set.
func TestNewHTTPClientSetsTheTimeout(t *testing.T) {
	c := newHTTPClient(42 * time.Second)
	if c.Timeout != 42*time.Second {
		t.Errorf("got %s, want 42s", c.Timeout)
	}
}

// newToolDef falls back to an empty-object schema when none is supplied --
// the other branch (a real schema passed through) is already exercised by
// the mcp tool-registration tests.
func TestNewToolDefFillsInAnEmptySchema(t *testing.T) {
	d := newToolDef("Foo", "does foo", nil)
	if string(d.Function.Parameters) != `{"type":"object","properties":{}}` {
		t.Errorf("got %s", d.Function.Parameters)
	}
}

// readErr's fallback path: a non-JSON (or JSON-without-"error") body is
// returned trimmed, verbatim -- the "error" field extraction is already
// covered by TestChatFailuresNameTheEndpoint.
func TestReadErrFallsBackToTheRawBodyWhenThereIsNoErrorField(t *testing.T) {
	if got := readErr(strings.NewReader("  plain failure text  \n")); got != "plain failure text" {
		t.Errorf("got %q", got)
	}
	// valid JSON, but no "error" key: still falls back to the raw (trimmed) body
	if got := readErr(strings.NewReader(`{"other":"x"}`)); got != `{"other":"x"}` {
		t.Errorf("got %q", got)
	}
}

// Check surfaces a transport failure against /api/tags as "<url> unreachable"
// -- distinct from the non-2xx-status and decode-failure branches other
// tests already cover.
func TestCheckUnreachableEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now genuinely unreachable
	o := &Ollama{URL: url, HTTP: srv.Client()}
	_, err := o.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("want an unreachable error, got %v", err)
	}
}

// Check: /api/tags answering a non-2xx status is a distinct failure from an
// unreachable endpoint or a decode error.
func TestCheckTagsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, HTTP: srv.Client()}
	_, err := o.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "/api/tags") {
		t.Errorf("want an /api/tags error, got %v", err)
	}
}

// Check: a malformed /api/tags body must fail to decode rather than being
// silently treated as zero models.
func TestCheckTagsMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not json")
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, HTTP: srv.Client()}
	_, err := o.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("want a decode error, got %v", err)
	}
}

// Check: /api/show answering neither 404 nor 2xx is a distinct failure.
func TestCheckShowNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[]}`)
		case "/api/show":
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"error":"boom"}`)
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", HTTP: srv.Client()}
	_, err := o.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "/api/show") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("want an /api/show error naming the reason, got %v", err)
	}
}

// Check: a malformed /api/show body must fail to decode.
func TestCheckShowMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[]}`)
		case "/api/show":
			io.WriteString(w, "not json")
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", HTTP: srv.Client()}
	_, err := o.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("want a decode error, got %v", err)
	}
}

// Chat: a blank line mid-stream is skipped, and a stream chunk carrying its
// own "error" field fails the whole call -- neither is exercised by the
// happy-path or HTTP-status-failure tests.
func TestChatStreamBlankLineAndInlineError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, line := range []string{
			``, // blank keep-alive line: must be skipped, not decoded
			`{"message":{"role":"assistant","content":"partial"},"done":false}`,
			`{"error":"the model crashed mid-stream"}`,
		} {
			io.WriteString(w, line+"\n")
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", HTTP: srv.Client()}
	_, err := o.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "the model crashed mid-stream") {
		t.Errorf("want the stream's own error, got %v", err)
	}
}

// Chat: a malformed line mid-stream must fail to decode rather than being
// skipped or panicking.
func TestChatStreamMalformedLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not json at all\n")
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", HTTP: srv.Client()}
	_, err := o.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("want a decode error, got %v", err)
	}
}
