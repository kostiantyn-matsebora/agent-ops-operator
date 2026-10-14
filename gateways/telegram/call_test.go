package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// callServer builds a router wired to a fake Bot API and returns a handler
// for POST /telegram/call/{method}, exactly as main() wires it.
func callServer(t *testing.T, botAPI http.Handler) (*router, http.Handler) {
	t.Helper()
	srv := httptest.NewServer(botAPI)
	t.Cleanup(srv.Close)
	r := &router{cfg: config{Token: "bot-token", APIBase: srv.URL, CallToken: "call-token"},
		tg: NewTelegram("bot-token", srv.URL)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /telegram/call/{method}", r.handleCall)
	return r, mux
}

// The endpoint forwards to the real Bot API path and relays a success
// response verbatim — a caller parsing tgResponse needs nothing new.
func TestHandleCallForwardsAndRelaysSuccess(t *testing.T) {
	var gotPath, gotBody, gotCT string
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_thread_id":7}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/createForumTopic",
		bytes.NewReader([]byte(`{"chat_id":"-100","name":"t"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer call-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotPath != "/botbot-token/createForumTopic" {
		t.Fatalf("forwarded path = %q, want the real Bot API path for the method", gotPath)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type not relayed, got %q", gotCT)
	}
	if gotBody != `{"chat_id":"-100","name":"t"}` {
		t.Fatalf("body not relayed verbatim, got %q", gotBody)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true,"result":{"message_thread_id":7}}` {
		t.Fatalf("response not relayed verbatim, got %q", rec.Body.String())
	}
}

// A Telegram-side error (ok:false, with retry_after) must reach the caller
// byte for byte -- the caller's own retry budget depends on reading it.
func TestHandleCallRelaysATelegramErrorVerbatim(t *testing.T) {
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":5}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/sendMessage", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer call-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 -- the gateway relays Telegram's OWN status, and a 429 was already consumed upstream here", rec.Code)
	}
	if rec.Body.String() != `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":5}}` {
		t.Fatalf("error body not relayed verbatim, got %q", rec.Body.String())
	}
}

// Multipart content (sendDocument) passes through with no gateway-side
// knowledge of the encoding -- it is forwarded with the same Content-Type it
// arrived with.
func TestHandleCallRelaysMultipartContentType(t *testing.T) {
	var gotCT string
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/sendDocument", bytes.NewReader([]byte("boundary-body")))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	req.Header.Set("Authorization", "Bearer call-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotCT != "multipart/form-data; boundary=xyz" {
		t.Fatalf("multipart content-type not relayed, got %q", gotCT)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// No bearer token at all: refused. Unlike /updates (pure network trust),
// this endpoint can SEND with the live credential, so it carries a shared
// secret -- see validCallToken's own comment for why.
func TestHandleCallRefusesAMissingToken(t *testing.T) {
	called := false
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/sendMessage", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("the Bot API must never be reached without a valid token")
	}
}

// A wrong token is refused exactly like a missing one.
func TestHandleCallRefusesAWrongToken(t *testing.T) {
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/sendMessage", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer not-the-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// A Call error (transport failure, not a Telegram-level "ok":false) is
// reported as 502 -- distinguishable from a relayed Telegram error, which
// always carries 200 with ok:false.
func TestHandleCallReports502OnATransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed at once: every call to it fails at the transport

	r := &router{cfg: config{Token: "bot-token", APIBase: srv.URL, CallToken: "call-token"},
		tg: NewTelegram("bot-token", srv.URL)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /telegram/call/{method}", r.handleCall)

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/sendMessage", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer call-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// An empty method path segment never reaches the Bot API at all -- the
// ServeMux pattern itself has no route for it, so this is 404 rather than
// handleCall's own (otherwise unreachable, and kept as a defensive check
// against a future route that forwards PathValue without going through this
// exact pattern) empty-method guard.
func TestHandleCallRequiresAMethod(t *testing.T) {
	_, h := callServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("must not reach the Bot API with no method")
	}))

	req := httptest.NewRequest(http.MethodPost, "/telegram/call/", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer call-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no route matches an empty method segment)", rec.Code)
	}
}
