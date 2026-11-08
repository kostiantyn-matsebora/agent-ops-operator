package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ListConversations against a fake manager: the request must carry the
// bearer token, and the response — the state read surface's own envelope
// shape, {"revision","items":[record]} (design D-5) — must be parsed
// including the dual-read fallback from the retired sessionId spelling.
func TestStateListConversationsParsesAndDualReads(t *testing.T) {
	var gotAuth, gotPath, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		fmt.Fprint(w, `{"revision":"42","items":[
			{"name":"conv-new","status":{"runtimeContextId":"ctx-new"}},
			{"name":"conv-old","status":{"sessionId":"ctx-old"}},
			{"name":"conv-none","status":{}}
		]}`)
	}))
	defer srv.Close()

	state := NewState(srv.URL, "sekrit-token")

	convs, err := state.ListConversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sekrit-token" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if gotPath != "/state/conversations" {
		t.Errorf("path = %q, want /state/conversations", gotPath)
	}

	want := map[string]string{"conv-new": "ctx-new", "conv-old": "ctx-old", "conv-none": ""}
	if len(convs) != len(want) {
		t.Fatalf("got %d conversations, want %d: %+v", len(convs), len(want), convs)
	}
	for _, c := range convs {
		if got, ok := want[c.Name]; !ok || got != c.ContextID {
			t.Errorf("conversation %+v: want ContextID %q", c, want[c.Name])
		}
	}
}

// A trailing slash on MANAGER_URL (a plausible chart value) must not become
// a double slash in the request path.
func TestStateBaseURLTrailingSlashIsTrimmed(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"revision":"","items":[]}`)
	}))
	defer srv.Close()

	state := NewState(srv.URL+"/", "tok")
	if _, err := state.ListConversations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/state/conversations" {
		t.Errorf("path = %q, want /state/conversations (no double slash)", gotPath)
	}
}

// A non-200 response must surface as an error naming the status, not be
// silently decoded as an empty conversation list — and the caller (main, via
// ReclaimWorkspaces/ReclaimSessions) must then remove nothing. See
// TestAFailedListingReclaimsNothing in reclaim_test.go for the removal half;
// this proves the client's own contribution to that guarantee.
func TestStateListConversationsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "state read token rejected")
	}))
	defer srv.Close()

	state := NewState(srv.URL, "tok")
	_, err := state.ListConversations(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want an error naming the status, got %v", err)
	}
	if !strings.Contains(err.Error(), "state read token rejected") {
		t.Fatalf("want the body's reason surfaced, got %v", err)
	}
}

// Malformed JSON must be reported as a decode error rather than an empty,
// misleadingly successful list.
func TestStateListConversationsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not json")
	}))
	defer srv.Close()

	state := NewState(srv.URL, "tok")
	if _, err := state.ListConversations(context.Background()); err == nil {
		t.Fatal("want a decode error for malformed JSON")
	}
}

// A connection-refused manager (no listener at all) must surface as an
// ordinary error, same as any other failed listing.
func TestStateListConversationsConnectionRefused(t *testing.T) {
	fastRetries(t)
	state := NewState("http://127.0.0.1:1", "tok")
	if _, err := state.ListConversations(context.Background()); err == nil {
		t.Fatal("want an error when the manager is unreachable")
	}
}

// fastRetries shrinks the connect-retry budget so a test never waits on it.
func fastRetries(t *testing.T) {
	t.Helper()
	b, f, m := connectRetryBudget, connectRetryFirst, connectRetryMax
	connectRetryBudget, connectRetryFirst, connectRetryMax = 300*time.Millisecond, 5*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { connectRetryBudget, connectRetryFirst, connectRetryMax = b, f, m })
}

// failingThenOK refuses the first n connections the way an unadmitted pod's
// are, then answers like the manager.
type failingThenOK struct {
	n     int
	calls int
}

func (f *failingThenOK) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.n {
		return nil, errors.New("dial tcp: connect: connection refused")
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[{"name":"c1"}]}`)), Header: http.Header{}}, nil
}

func TestStateListConversationsRetriesAConnectFailure(t *testing.T) {
	fastRetries(t)
	rt := &failingThenOK{n: 3}
	state := NewState("http://manager", "tok")
	state.HTTP = &http.Client{Transport: rt}
	got, err := state.ListConversations(context.Background())
	if err != nil || len(got) != 1 || rt.calls != 4 {
		t.Fatalf("got %v, %v after %d calls; want one conversation after 4", got, err, rt.calls)
	}
}

func TestStateListConversationsDoesNotRetryAnHTTPAnswer(t *testing.T) {
	fastRetries(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := NewState(srv.URL, "bad").ListConversations(context.Background()); err == nil || calls != 1 {
		t.Fatalf("err=%v after %d calls; want an error after exactly 1", err, calls)
	}
}

func TestStateListConversationsStopsRetryingWhenCancelled(t *testing.T) {
	connectRetryBudget = time.Minute
	t.Cleanup(func() { connectRetryBudget = 2 * time.Minute })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := NewState("http://127.0.0.1:1", "t").ListConversations(ctx); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %s; want a prompt error", err, time.Since(start))
	}
}
