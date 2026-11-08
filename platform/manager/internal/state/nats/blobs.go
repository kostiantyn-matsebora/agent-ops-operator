package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// blobs is this backend's state.Blobs, over an Object Store (design D-7): a
// queued input's payload can exceed the server's maximum message size,
// which an Object Store chunks for a caller — a KV value cannot.
type blobs struct {
	store jetstream.ObjectStore
}

// newBlobs wires the Blobs capability over an already-bootstrapped store.
func newBlobs(store jetstream.ObjectStore) *blobs {
	return &blobs{store: store}
}

// Put stores blob, replacing any value already there (an Object Store's own
// Put already does exactly this).
func (b *blobs) Put(ctx context.Context, key state.Key, blob state.Blob) error {
	data, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("nats: encode blob %q: %w", key, err)
	}
	if _, err := b.store.PutBytes(ctx, wireKey(string(key)), data); err != nil {
		return fmt.Errorf("nats: put blob %q: %w", key, err)
	}
	return nil
}

// Get returns the blob stored under key, or ErrNotFound.
func (b *blobs) Get(ctx context.Context, key state.Key) (state.Blob, error) {
	data, err := b.store.GetBytes(ctx, wireKey(string(key)))
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) || errors.Is(err, jetstream.ErrObjectAlreadyExists) {
			return state.Blob{}, state.ErrNotFound
		}
		return state.Blob{}, fmt.Errorf("nats: get blob %q: %w", key, err)
	}
	var blob state.Blob
	if err := json.Unmarshal(data, &blob); err != nil {
		return state.Blob{}, fmt.Errorf("nats: decode blob %q: %w", key, err)
	}
	return blob, nil
}

// Delete removes a blob. Not an error when the key is already absent.
func (b *blobs) Delete(ctx context.Context, key state.Key) error {
	if err := b.store.Delete(ctx, wireKey(string(key))); err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil
		}
		return fmt.Errorf("nats: delete blob %q: %w", key, err)
	}
	return nil
}

var _ state.Blobs = (*blobs)(nil)
