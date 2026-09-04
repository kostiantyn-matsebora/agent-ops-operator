package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDoReturnsMarshalError closes the gap on do()'s json.Marshal error
// branch — unreachable through GetOffset/PutOffset (both always pass a
// map[string]string), but a real path through the shared do() helper, called
// directly with a body type json.Marshal genuinely cannot encode.
func TestDoReturnsMarshalError(t *testing.T) {
	d := NewDownstream()
	err := d.do(context.Background(), http.MethodPost, "http://unused", make(chan int), nil)
	if err == nil {
		t.Fatal("expected a marshal error for an unmarshalable body")
	}
}

// TestGetOffsetReturnsRequestConstructionError closes the gap on do()'s
// http.NewRequestWithContext error branch: a malformed target URL fails at
// request construction, before any network call is attempted.
func TestGetOffsetReturnsRequestConstructionError(t *testing.T) {
	d := NewDownstream()
	_, err := d.GetOffset(context.Background(), "://bad-url")
	if err == nil {
		t.Fatal("expected an error for a malformed target URL")
	}
}

// TestForwardReturnsErrorOnBadStatus closes the gap on Forward()'s >=400
// branch — the existing route tests only ever exercise the success path.
func TestForwardReturnsErrorOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	d := NewDownstream()
	err := d.Forward(context.Background(), srv.URL, []byte(`{}`))
	if err == nil {
		t.Fatal("expected an error when the receiving adapter rejects the update")
	}
}
