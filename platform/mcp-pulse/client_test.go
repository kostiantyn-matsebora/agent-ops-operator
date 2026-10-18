package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetReturnsAPulseErrorOnANon2xxStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"out of scope"}`))
	}))
	defer ts.Close()
	c := newPulseClient(ts.URL)

	err := c.get(context.Background(), "/api/alerts", nil, "tok", nil)
	pe, ok := err.(*pulseError)
	if !ok {
		t.Fatalf("want a *pulseError, got %T: %v", err, err)
	}
	if pe.Status != 403 {
		t.Fatalf("want status 403, got %d", pe.Status)
	}
	if pe.Error() == "" {
		t.Fatal("pulseError must render")
	}
}

func TestGetDecodesA2xxBodyIntoOut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "resource=pve-1" {
			t.Fatalf("want the query forwarded, got %q", r.URL.RawQuery)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()
	c := newPulseClient(ts.URL)

	var out map[string]bool
	q := url.Values{"resource": {"pve-1"}}
	if err := c.get(context.Background(), "/api/metrics", q, "tok", &out); err != nil {
		t.Fatal(err)
	}
	if !out["ok"] {
		t.Fatalf("got %+v", out)
	}
}

func TestPostSendsAJSONBodyAndDecodesA2xxResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("want application/json, got %q", ct)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()
	c := newPulseClient(ts.URL)

	var out map[string]bool
	if err := c.post(context.Background(), "/api/operator-state/criticality-tag", "tok", map[string]any{"resource": "pve-1"}, &out); err != nil {
		t.Fatal(err)
	}
	if !out["ok"] {
		t.Fatalf("got %+v", out)
	}
}

func TestNoTokenSetsNoAuthorizationHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("want no Authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	c := newPulseClient(ts.URL)

	if err := c.get(context.Background(), "/api/resources", nil, "", nil); err != nil {
		t.Fatal(err)
	}
}
