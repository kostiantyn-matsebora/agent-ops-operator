package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// orphanGrace is how old a blob must be before the orphan sweep may remove it
// for want of a conversation. A blob is written BEFORE the input that names it
// is queued, so a young blob without a conversation is a write in flight, not
// an orphan.
const orphanGrace = 10 * time.Minute

// children is what a conversation owns outside its own record: its runs (the
// runs bucket) and the out-of-line bodies of its queued inputs (the blobs
// store). The kube backend keeps runs inside the Conversation object, so
// deleting the object deletes them. On this backend they are separate
// entries, so removing a conversation has to remove them too, and a sweep has
// to catch what a crash between the two steps left behind.
type children struct {
	conversations *collection[state.ConversationRecord]
	runsKV        jetstream.KeyValue
	blobs         jetstream.ObjectStore
	now           func() time.Time
}

// removed deletes everything conversation key owned. It is called after the
// record itself is gone, and is safe to repeat: both halves are no-ops on
// nothing.
func (c *children) removed(ctx context.Context, key state.Key) error {
	return errors.Join(c.purgeRuns(ctx, key), c.deleteBlobsOf(ctx, key, 0))
}

// purgeRuns removes every run item under conversation. Purge, not delete: the
// payloads of the earlier revisions go too, instead of waiting for the key's
// history to roll.
func (c *children) purgeRuns(ctx context.Context, conversation state.Key) error {
	keys, err := c.runKeys(ctx, runsPrefixFilter(string(conversation)))
	if err != nil {
		return fmt.Errorf("nats: list runs of %q to remove: %w", conversation, err)
	}
	var errs []error
	for _, k := range keys {
		if err := c.runsKV.Purge(ctx, k); err != nil {
			errs = append(errs, fmt.Errorf("nats: purge run %q: %w", k, err))
		}
	}
	return errors.Join(errs...)
}

// runKeys lists the run item keys matching filter, empty when there are none.
func (c *children) runKeys(ctx context.Context, filter string) ([]string, error) {
	lister, err := c.runsKV.ListKeysFiltered(ctx, filter)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for k := range lister.Keys() {
		keys = append(keys, k)
	}
	return keys, nil
}

// deleteBlobsOf removes every blob that names conversation, older than minAge.
// Zero minAge removes them all.
func (c *children) deleteBlobsOf(ctx context.Context, conversation state.Key, minAge time.Duration) error {
	return c.eachBlob(ctx, func(info *jetstream.ObjectInfo, blob state.Blob) (bool, error) {
		return blob.ConversationName == string(conversation) && c.oldEnough(info, minAge), nil
	})
}

// oldEnough reports whether info was last modified at least minAge ago.
func (c *children) oldEnough(info *jetstream.ObjectInfo, minAge time.Duration) bool {
	return minAge <= 0 || c.now().Sub(info.ModTime) >= minAge
}

// eachBlob walks the blobs store and deletes the ones remove selects. A blob
// that cannot be read or decoded is left alone: unreadable is not orphaned.
func (c *children) eachBlob(ctx context.Context, remove func(*jetstream.ObjectInfo, state.Blob) (bool, error)) error {
	infos, err := c.blobs.List(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return nil
		}
		return fmt.Errorf("nats: list blobs: %w", err)
	}
	var errs []error
	for _, info := range infos {
		data, err := c.blobs.GetBytes(ctx, info.Name)
		if err != nil {
			continue
		}
		var blob state.Blob
		if json.Unmarshal(data, &blob) != nil {
			continue
		}
		del, err := remove(info, blob)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !del {
			continue
		}
		if err := c.blobs.Delete(ctx, info.Name); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
			errs = append(errs, fmt.Errorf("nats: delete blob %q: %w", info.Name, err))
		}
	}
	return errors.Join(errs...)
}

// Sweep removes what no conversation owns any more: run items whose parent
// record is gone, and blobs (past the grace) whose conversation is gone. It is
// the net under removed, for a crash between deleting the record and deleting
// its children.
func (c *children) Sweep(ctx context.Context) error {
	gone := map[string]bool{}
	absent := func(name string) (bool, error) {
		if v, ok := gone[name]; ok {
			return v, nil
		}
		_, _, err := c.conversations.Get(ctx, state.Key(name))
		switch {
		case err == nil:
			gone[name] = false
		case errors.Is(err, state.ErrNotFound):
			gone[name] = true
		default:
			return false, err
		}
		return gone[name], nil
	}

	var errs []error
	keys, err := c.runKeys(ctx, ">")
	if err != nil {
		errs = append(errs, fmt.Errorf("nats: list runs to sweep: %w", err))
	}
	for _, k := range keys {
		parent, err := parentOfItemKey(k)
		if err != nil {
			continue
		}
		missing, err := absent(parent)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if missing {
			if err := c.runsKV.Purge(ctx, k); err != nil {
				errs = append(errs, fmt.Errorf("nats: purge orphan run %q: %w", k, err))
			}
		}
	}

	errs = append(errs, c.eachBlob(ctx, func(info *jetstream.ObjectInfo, blob state.Blob) (bool, error) {
		if blob.ConversationName == "" || !c.oldEnough(info, orphanGrace) {
			return false, nil
		}
		return absent(blob.ConversationName)
	}))
	return errors.Join(errs...)
}

// Run sweeps every interval until ctx ends, and once at the start.
func (c *children) Run(ctx context.Context, interval time.Duration) {
	_ = c.Sweep(ctx) // a failed pass is retried by the next; the sweep is idempotent
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = c.Sweep(ctx)
		}
	}
}
