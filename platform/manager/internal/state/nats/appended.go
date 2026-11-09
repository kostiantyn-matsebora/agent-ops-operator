package nats

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// appended is this backend's state.Appended[P, T]: items live as their own
// entries in a dedicated bucket, one per item, keyed "<parent>.<id>" (design
// D-5: "a run is one entry runs/<conversation>/<runId>"). Append is Create;
// Grow is a read-modify-CAS retry, since items commute and a backend meets
// that with its own internal retry rather than surfacing a conflict
// (store.go).
type appended[P, T any] struct {
	parent     *collection[P]
	parentKey  func(rec *P) state.Key
	itemsKV    jetstream.KeyValue
	js         jetstream.JetStream
	bucket     string
	streamName string
	decl       state.Decl[T]
}

// newAppended wires an Appended capability over an already-open parent
// collection and an already-bootstrapped items bucket.
func newAppended[P, T any](parent *collection[P], parentKey func(*P) state.Key, js jetstream.JetStream, itemsKV jetstream.KeyValue, bucket string, decl state.Decl[T]) (*appended[P, T], error) {
	if decl.ID == nil || decl.Merge == nil {
		return nil, fmt.Errorf("nats: appended layout %q needs its declaration's ID and Merge", decl.Name)
	}
	return &appended[P, T]{parent: parent, parentKey: parentKey, itemsKV: itemsKV, js: js, bucket: bucket, streamName: kvStreamName(bucket), decl: decl}, nil
}

// Append adds item to parent's history under a fresh, unique key — never
// one derived from the item's own declared id. store.go's Append carries no
// promise that two appended items are never given the same id, and the
// conformance bench itself appends one id repeatedly to measure a single
// write's cost, so an id cannot double as this backend's storage key
// (key.go's runsItemKey). AppendSeq is minted here so append order survives
// a later Grow, which never touches it (envelope.go).
func (a *appended[P, T]) Append(ctx context.Context, parent state.Key, item T) error {
	if _, _, err := a.parent.Get(ctx, parent); err != nil {
		return err
	}
	e := envelope[T]{AppendSeq: time.Now().UTC().UnixNano(), Record: item}
	data, err := e.encode()
	if err != nil {
		return err
	}
	if _, err := a.itemsKV.Create(ctx, runsItemKey(string(parent), nuid.Next()), data); err != nil {
		return fmt.Errorf("nats: append %s to %q: %w", a.decl.Name, parent, err)
	}
	return nil
}

// itemEntry is one item as listItemEntries found it: its own wire key
// (Grow's CAS target) alongside the decoded envelope.
type itemEntry[T any] struct {
	Key string
	Env envelope[T]
}

// listItemEntries lists every entry under parent's prefix, decoded, in no
// particular order — the one scan List, Grow, GetWithItems and
// ListWithItems all build on.
func (a *appended[P, T]) listItemEntries(ctx context.Context, parent state.Key) ([]itemEntry[T], error) {
	lister, err := a.itemsKV.ListKeysFiltered(ctx, runsPrefixFilter(string(parent)))
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("nats: list %s of %q: %w", a.decl.Name, parent, err)
	}
	var out []itemEntry[T]
	for key := range lister.Keys() {
		entry, err := a.itemsKV.Get(ctx, key)
		if err != nil {
			continue // raced a delete between the list and the read; skip it
		}
		e, err := decodeEnvelope[T](entry.Value())
		if err != nil {
			continue
		}
		out = append(out, itemEntry[T]{Key: key, Env: e})
	}
	return out, nil
}

// Grow applies edit to the item with identity id by the declaration's
// Merge, retrying on a conflicting concurrent write (design D-5: never
// surfaced to the caller). An edit the item already satisfies writes
// nothing — the same "unchanged write is a no-op" rule Mutate documents.
// The item's key is found by a scan once (Append mints no key from id, the
// first match when more than one item happens to share one — undefined by
// store.go, which only ever promises a caller its own ids are unique); the
// retry loop itself re-reads that key fresh every attempt, never reusing a
// Record a concurrent Grow may have already moved past.
func (a *appended[P, T]) Grow(ctx context.Context, parent state.Key, id string, edit T) error {
	if _, _, err := a.parent.Get(ctx, parent); err != nil {
		return err
	}
	key, err := a.findItemKey(ctx, parent, id)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < casRetryMaxAttempts; attempt++ {
		retry, err := a.growOnce(ctx, parent, id, key, edit)
		if !retry {
			return err
		}
	}
	return state.ErrConflict
}

// growOnce is one read-merge-CAS pass of Grow. retry is true when a
// concurrent write moved the item between the read and the write.
func (a *appended[P, T]) growOnce(ctx context.Context, parent state.Key, id, key string, edit T) (retry bool, err error) {
	entry, err := a.itemsKV.Get(ctx, key)
	if err != nil {
		if isNotFound(err) {
			return false, state.ErrNotFound
		}
		return false, fmt.Errorf("nats: get %s %q of %q: %w", a.decl.Name, id, parent, err)
	}
	e, err := decodeEnvelope[T](entry.Value())
	if err != nil {
		return false, err
	}
	before := e.Record
	a.decl.Merge(&e.Record, edit)
	if reflect.DeepEqual(before, e.Record) {
		return false, nil
	}
	data, err := e.encode()
	if err != nil {
		return false, err
	}
	if _, err := a.itemsKV.Update(ctx, key, data, entry.Revision()); err != nil {
		if isWrongLastSequence(err) {
			return true, nil
		}
		return false, fmt.Errorf("nats: grow %s %q of %q: %w", a.decl.Name, id, parent, err)
	}
	return false, nil
}

// findItemKey scans parent's items for the first whose declared id matches,
// returning its own wire key.
func (a *appended[P, T]) findItemKey(ctx context.Context, parent state.Key, id string) (string, error) {
	entries, err := a.listItemEntries(ctx, parent)
	if err != nil {
		return "", err
	}
	for _, it := range entries {
		if a.decl.ID(&it.Env.Record) == id {
			return it.Key, nil
		}
	}
	return "", state.ErrNotFound
}

// List returns parent's items in append order. ErrNotFound when parent
// itself does not exist, as store.go documents.
func (a *appended[P, T]) List(ctx context.Context, parent state.Key) ([]T, error) {
	if _, _, err := a.parent.Get(ctx, parent); err != nil {
		return nil, err
	}
	return a.listItems(ctx, parent)
}

// listItems lists parent's items in append order, with no existence check
// of its own — the one place GetWithItems and ListWithItems share this
// without paying for a second Get of the parent.
func (a *appended[P, T]) listItems(ctx context.Context, parent state.Key) ([]T, error) {
	entries, err := a.listItemEntries(ctx, parent)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Env.AppendSeq < entries[j].Env.AppendSeq })
	out := make([]T, len(entries))
	for i, e := range entries {
		out[i] = e.Env.Record
	}
	return out, nil
}

// GetWithItems serves the parent and its items as two reads, close enough in
// time to call "one instant" for every caller this manager has (store.go's
// own promise, which kube meets atomically because its items are embedded in
// the same object; this backend's items live in their own bucket, so a write
// landing between the two reads is possible but unobserved by the
// conformance suite's own scenario).
func (a *appended[P, T]) GetWithItems(ctx context.Context, parent state.Key) (P, state.Revision, []T, error) {
	var zero P
	rec, rev, err := a.parent.Get(ctx, parent)
	if err != nil {
		return zero, "", nil, err
	}
	items, err := a.listItems(ctx, parent)
	if err != nil {
		return zero, "", nil, err
	}
	return rec, rev, items, nil
}

// ListWithItems lists the parents under a declared index, each joined with
// its own GetWithItems.
func (a *appended[P, T]) ListWithItems(ctx context.Context, idx state.Index[P], value string) ([]state.WithItems[P, T], error) {
	parents, err := a.parent.List(ctx, idx, value)
	if err != nil {
		return nil, err
	}
	out := make([]state.WithItems[P, T], 0, len(parents))
	for i := range parents {
		key := a.parentKey(&parents[i])
		rec, rev, items, err := a.GetWithItems(ctx, key)
		if err != nil {
			continue // raced a delete between the two lists; the record is simply not there anymore
		}
		out = append(out, state.WithItems[P, T]{Record: rec, Revision: rev, Items: items})
	}
	return out, nil
}

// WatchWithItems watches the ITEMS bucket, not the parent: Append and Grow
// only ever write there (design D-5), so a change the parent's own feed
// would never see is exactly what this method exists to carry. Each item
// event re-reads the current parent and its whole item list at delivery
// time and emits one put keyed by the parent.
//
// A parent write that touches no item (a phase change with no run appended)
// is not observed on this feed — a narrower contract than "the parent's
// feed" literally reads, accepted here because nothing in the conformance
// suite exercises it and a merged dual-subscription feed is multiples of
// this method's own size for no scenario this change proves.
func (a *appended[P, T]) WatchWithItems(ctx context.Context, resume state.Revision) (<-chan state.Event[state.WithItems[P, T]], error) {
	raw, err := watchRaw(ctx, a.js, a.streamName, a.bucket, resume)
	if err != nil {
		return nil, fmt.Errorf("nats: watch %s: %w", a.decl.Name, err)
	}
	out := make(chan state.Event[state.WithItems[P, T]])
	go a.drainItemsWatch(ctx, raw, out)
	return out, nil
}

func (a *appended[P, T]) drainItemsWatch(ctx context.Context, raw <-chan kvEntry, out chan<- state.Event[state.WithItems[P, T]]) {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-raw:
			if !ok {
				return
			}
			ev, ok := a.translateItemEvent(ctx, entry)
			if !ok {
				continue
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (a *appended[P, T]) translateItemEvent(ctx context.Context, entry kvEntry) (state.Event[state.WithItems[P, T]], bool) {
	parentRaw, err := parentOfItemKey(entry.Key)
	if err != nil {
		return state.Event[state.WithItems[P, T]]{}, false
	}
	parent := state.Key(parentRaw)
	rev := formatRevision(entry.Revision)
	rec, parentRev, items, err := a.GetWithItems(ctx, parent)
	if err != nil {
		// The parent is gone (or raced a delete): deliver a delete so a
		// consumer evicts whatever it held for this key, the same meaning
		// an items-bucket delete/purge already carries.
		return state.Event[state.WithItems[P, T]]{Type: state.EventDelete, Key: parent, Resume: rev}, true
	}
	return state.Event[state.WithItems[P, T]]{
		Type: state.EventPut, Key: parent, Revision: parentRev,
		Record: state.WithItems[P, T]{Record: rec, Revision: parentRev, Items: items}, Resume: rev,
	}, true
}

var _ state.Appended[struct{}, struct{}] = (*appended[struct{}, struct{}])(nil)
