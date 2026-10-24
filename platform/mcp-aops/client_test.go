package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostReturnsAManagerErrorOnANon2xxStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"out of scope"}`))
	}))
	defer ts.Close()
	c := newManagerClient(ts.URL)

	err := c.post(context.Background(), "/coordinate/invoke", "tok", map[string]any{}, nil)
	me, ok := err.(*managerError)
	if !ok {
		t.Fatalf("want a *managerError, got %T: %v", err, err)
	}
	if me.Status != 403 {
		t.Fatalf("want status 403, got %d", me.Status)
	}
}

func TestPostDecodesA2xxBodyIntoOut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"member":"m-1","status":"created"}`))
	}))
	defer ts.Close()
	c := newManagerClient(ts.URL)

	var out map[string]string
	if err := c.post(context.Background(), "/coordinate/invoke", "tok", map[string]any{}, &out); err != nil {
		t.Fatal(err)
	}
	if out["member"] != "m-1" || out["status"] != "created" {
		t.Fatalf("got %+v", out)
	}
}
