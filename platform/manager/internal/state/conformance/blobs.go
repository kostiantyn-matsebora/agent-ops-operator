package conformance

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// RunBlobs executes every state.Blobs scenario against a Blobs open
// constructs fresh for each leaf.
func RunBlobs(t *testing.T, open func() state.Blobs) {
	t.Helper()

	t.Run("PutGetDeleteRoundTrip", func(t *testing.T) { testBlobRoundTrip(t, open()) })
	t.Run("PutUpsertsInPlace", func(t *testing.T) { testBlobUpsert(t, open()) })
	t.Run("GetAfterDeleteIsNotFoundThenAPrunedKeyMayBeReused", func(t *testing.T) { testBlobLifecycle(t, open()) })
	t.Run("GetOnMissingKeyIsNotFound", func(t *testing.T) { testBlobGetMissing(t, open()) })
}

func fixtureBlob() state.Blob {
	return state.Blob{ConversationName: "conv-a", Type: "alert", Payload: "the full out-of-line payload", Labels: map[string]string{"alertname": "Test"}, Severity: "critical"}
}

func testBlobRoundTrip(t *testing.T, s state.Blobs) {
	ctx := context.Background()
	key := state.Key("blob-1")
	want := fixtureBlob()
	if err := s.Put(ctx, key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get: got %+v, want %+v", got, want)
	}
}

func testBlobUpsert(t *testing.T, s state.Blobs) {
	ctx := context.Background()
	key := state.Key("blob-upsert")
	first := fixtureBlob()
	if err := s.Put(ctx, key, first); err != nil {
		t.Fatalf("Put first: %v", err)
	}
	second := first
	second.Payload = "a replacement payload"
	if err := s.Put(ctx, key, second); err != nil {
		t.Fatalf("Put second: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("Get after upsert: got %+v, want %+v", got, second)
	}
}

func testBlobLifecycle(t *testing.T, s state.Blobs) {
	ctx := context.Background()
	key := state.Key("blob-delete")
	if err := s.Put(ctx, key, fixtureBlob()); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after delete: got %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("second Delete on an already-absent key: %v", err)
	}
	second := fixtureBlob()
	second.Payload = "second life"
	if err := s.Put(ctx, key, second); err != nil {
		t.Fatalf("Put (re-put after delete): %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil || got.Payload != "second life" {
		t.Fatalf("Get (after re-put) = %+v, %v", got, err)
	}
}

func testBlobGetMissing(t *testing.T, s state.Blobs) {
	if _, err := s.Get(context.Background(), state.Key("never-put")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on missing key: got %v, want ErrNotFound", err)
	}
}
