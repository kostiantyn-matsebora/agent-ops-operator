package main

import (
	"context"
	"math"
	"strings"
	"testing"
)

// Manager.do()'s own error paths — request construction and body encoding —
// which every existing test reaches only through payloads this adapter itself
// builds (always marshalable, always a valid method), so neither branch was
// ever exercised.

// json.Marshal can fail on the caller's "in" value (e.g. a NaN/Inf float);
// do() must return that error rather than send a malformed body.
func TestDoRejectsAnUnmarshalableBody(t *testing.T) {
	m := NewManager("http://unused.invalid", "t")
	err := m.do(context.Background(), "POST", "/x", math.Inf(1), nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported value") {
		t.Fatalf("expected a JSON marshal error, got %v", err)
	}
}

// http.NewRequestWithContext rejects a malformed method before ever touching
// the network — do() must surface that rather than panic or hang.
func TestDoRejectsAnInvalidMethod(t *testing.T) {
	m := NewManager("http://unused.invalid", "t")
	err := m.do(context.Background(), "BAD METHOD", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an invalid-method error")
	}
}
