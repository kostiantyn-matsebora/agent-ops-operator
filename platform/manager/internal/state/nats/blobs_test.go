package nats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// TestBlobLargerThanTheServerMaxPayloadReadsBackByteIdentical is task 2.6's
// own verify clause: the server's default max_payload is 1 MiB, and a
// queued input's payload can exceed a single KV value — design D-7's whole
// reason to put Blobs in an Object Store, which chunks on the client's
// behalf, rather than a KV bucket.
func TestBlobLargerThanTheServerMaxPayloadReadsBackByteIdentical(t *testing.T) {
	ctx := context.Background()
	b := openBackend(t, sharedTestConn(t), newScope(t))

	const oneMiB = 1 << 20
	large := strings.Repeat("agent-ops-nats-blob-content-", (oneMiB*3)/len("agent-ops-nats-blob-content-")+1)
	if len(large) <= oneMiB {
		t.Fatalf("fixture payload is only %d bytes, want more than %d", len(large), oneMiB)
	}

	key := state.Key("large-blob")
	want := state.Blob{ConversationName: "conv-a", Type: "task", Payload: large, Labels: map[string]string{"k": "v"}, Severity: "critical"}
	if err := b.Stores.Blobs.Put(ctx, key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := b.Stores.Blobs.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Payload != want.Payload {
		t.Fatalf("payload round-trip mismatch: got %d bytes, want %d bytes identical", len(got.Payload), len(want.Payload))
	}
	if got.ConversationName != want.ConversationName || got.Type != want.Type || got.Severity != want.Severity {
		t.Fatalf("metadata round-trip mismatch: got %+v", struct {
			ConversationName, Type, Severity string
		}{got.ConversationName, got.Type, got.Severity})
	}
}

func TestBlobGetOfAMissingKeyIsNotFoundAndDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	b := openBackend(t, sharedTestConn(t), newScope(t))
	key := state.Key("gone")

	if _, err := b.Stores.Blobs.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a missing blob = %v, want ErrNotFound", err)
	}
	if err := b.Stores.Blobs.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of a missing blob must not fail: %v", err)
	}
	if err := b.Stores.Blobs.Put(ctx, key, state.Blob{Payload: "x"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := b.Stores.Blobs.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.Stores.Blobs.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestExpiringExpiryReadsTheDeclaredInstant(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	exp, ok := b.Stores.Signals.(*Expiring[state.SignalRecord])
	if !ok {
		t.Fatalf("Signals is %T, want *Expiring[SignalRecord]", b.Stores.Signals)
	}
	at := time.Now().Add(time.Hour)
	if got := exp.Expiry(state.SignalRecord{ExpiresAt: &at}); got == nil || !got.Equal(at) {
		t.Fatalf("Expiry = %v, want %v", got, at)
	}
	if got := exp.Expiry(state.SignalRecord{}); got != nil {
		t.Fatalf("Expiry of a record with none = %v, want nil", got)
	}
}
