package nats

import (
	"context"
	"strings"
	"testing"

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
