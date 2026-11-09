package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHoldBodyRestoresWhatTheProxyConsumed(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte("payload")))
	held, restore := holdBody(r)
	_, _ = io.ReadAll(r.Body) // the proxy forwards it
	restore(held)
	got, _ := io.ReadAll(r.Body)
	if string(got) != "payload" {
		t.Fatalf("restored body = %q", got)
	}
}

func TestHoldBodyLeavesARequestWithoutABodyAlone(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Body = http.NoBody
	held, restore := holdBody(r)
	restore(held)
	if r.Body != http.NoBody {
		t.Fatal("a bodyless request was given a body")
	}
	r.Body = nil
	if _, restore := holdBody(r); restore == nil {
		t.Fatal("restore must always be callable")
	}
}

func TestHoldBodyForwardsABodyTooLargeToHoldWhole(t *testing.T) {
	big := bytes.Repeat([]byte("a"), maxHeldBody+10)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(big))
	held, restore := holdBody(r)
	if held != nil {
		t.Fatal("a body over the limit must not be held")
	}
	restore(held)
	got, _ := io.ReadAll(r.Body)
	if len(got) != len(big) {
		t.Fatalf("the stream lost bytes: %d of %d", len(got), len(big))
	}
}
