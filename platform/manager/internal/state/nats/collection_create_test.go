package nats

import (
	"context"
	"math"
	"testing"
)

// unencodableRecord is a record no backend can round-trip through JSON: a
// NaN float64 is refused by encoding/json, by design, so Create must report
// it rather than write something it could never read back.
type unencodableRecord struct {
	Bad float64
}

// TestCreatePropagatesAnEncodeFailureWithoutTouchingTheServer proves the
// encode check in Create (envelope.go's own doc: "failing loudly rather than
// silently losing a write") runs, and fails the call, before anything is
// sent to the KV bucket — the collection here carries a nil kv, so a Create
// that got as far as c.kv.Create would panic instead of just erroring.
func TestCreatePropagatesAnEncodeFailureWithoutTouchingTheServer(t *testing.T) {
	c := &collection[unencodableRecord]{layout: Layout[unencodableRecord]{}}
	if _, _, err := c.Create(context.Background(), "k", unencodableRecord{Bad: math.NaN()}); err == nil {
		t.Fatal("Create succeeded encoding a record with a NaN float64")
	}
}
