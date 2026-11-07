// Task 2.5: PutBlob/GetBlob/DeleteBlob.
package kube

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func fixtureBlob() state.Blob {
	return state.Blob{
		ConversationName: "conv-a",
		Type:             "alert",
		Payload:          "the full out-of-line payload",
		Labels:           map[string]string{"alertname": "Test", "namespace": "ns-a"},
		Severity:         "critical",
	}
}

func TestBlobRoundTrip(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	key := state.Key("blob-1")
	want := fixtureBlob()
	if err := s.PutBlob(ctx, key, want); err != nil {
		t.Fatalf("PutBlob: %v", err)
	}

	got, err := s.GetBlob(ctx, key)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetBlob: got %+v, want %+v", got, want)
	}
}

// TestPutBlobUpsertsInPlace: PutBlob on a key that already carries a value
// replaces it (store.go's Store.PutBlob: "replacing any value already
// there"), never creating a second record.
func TestPutBlobUpsertsInPlace(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	key := state.Key("blob-upsert")
	first := fixtureBlob()
	if err := s.PutBlob(ctx, key, first); err != nil {
		t.Fatalf("PutBlob first: %v", err)
	}

	second := first
	second.Payload = "a replacement payload"
	second.Severity = "warning"
	if err := s.PutBlob(ctx, key, second); err != nil {
		t.Fatalf("PutBlob second: %v", err)
	}

	got, err := s.GetBlob(ctx, key)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("GetBlob after upsert: got %+v, want %+v", got, second)
	}
}

func TestGetBlobAfterDeleteIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	key := state.Key("blob-delete")
	if err := s.PutBlob(ctx, key, fixtureBlob()); err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if err := s.DeleteBlob(ctx, key); err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}

	_, err := s.GetBlob(ctx, key)
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("GetBlob after delete: got %v, want ErrNotFound", err)
	}

	// Not an error when the key is already absent (store.go's
	// Store.DeleteBlob: "a deleted key reads absent", not an error).
	if err := s.DeleteBlob(ctx, key); err != nil {
		t.Fatalf("second DeleteBlob on an already-absent key: %v", err)
	}
}

func TestGetBlobOnMissingKeyIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	_, err := s.GetBlob(ctx, state.Key("never-put"))
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("GetBlob on missing key: got %v, want ErrNotFound", err)
	}
}
