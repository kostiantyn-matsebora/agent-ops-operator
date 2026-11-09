package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// casRetryMaxAttempts bounds Delete's own read-modify-write loop (it carries
// no caller revision to CAS against, unlike DeleteAt) and the Appended
// retries of appended.go — the same purpose kube's statusUpdateMaxAttempts
// serves, sized the same.
const casRetryMaxAttempts = 20

// collection is this backend's state.Collection[T]: one KV bucket, mapped
// through Layout and kept visible-by-expiry at Get/List time (design D-1
// through D-4). The mirror (mirror.go) feeds List only; every other method
// reads the server directly, so a precondition is never a guess (D-4).
type collection[T any] struct {
	kv         jetstream.KeyValue
	layout     Layout[T]
	mirror     *mirror[T]
	js         jetstream.JetStream
	bucket     string // the scoped bucket name, read only by Watch (rawwatch.go)
	streamName string // "KV_<bucket>" — the stream backing kv, read by latestSeq and Watch
}

// newCollection opens a collection over an already-bootstrapped bucket. js,
// bucket and streamName exist for two needs no capability of KeyValue meets:
// List's fence against the bucket's own latest write (latestSeq — the
// mirror's watch is otherwise asynchronous, and "the first catch-up
// completed" long before a later write is not "caught up with every write so
// far"), and Watch's own resumable ordered consumer (rawwatch.go).
func newCollection[T any](js jetstream.JetStream, kv jetstream.KeyValue, bucket, streamName string, l Layout[T]) *collection[T] {
	return &collection[T]{kv: kv, layout: l, mirror: newMirror[T](kv), js: js, bucket: bucket, streamName: streamName}
}

// latestSeq reads the bucket's own last stream sequence directly from the
// server — never from the mirror, which is what List fences the mirror
// against below.
func (c *collection[T]) latestSeq(ctx context.Context) (uint64, error) {
	s, err := c.js.Stream(ctx, c.streamName)
	if err != nil {
		return 0, fmt.Errorf("nats: stream info %q: %w", c.streamName, err)
	}
	return s.CachedInfo().State.LastSeq, nil
}

// expiry reads rec's expiry through the declaration, nil for a kind that
// declares none.
func (c *collection[T]) expiry(rec *T) *time.Time {
	if c.layout.Decl.Expiry == nil {
		return nil
	}
	return c.layout.Decl.Expiry(rec)
}

// visible reports whether e's record has not yet passed its expiry.
func (c *collection[T]) visible(e *envelope[T], now time.Time) bool {
	rec := e.Record
	at := c.expiry(&rec)
	return at == nil || !at.Before(now)
}

// decorated returns e's record with the envelope's own identity/hold fields
// applied and its own Name set to key — what every read hands back to a
// caller.
func (c *collection[T]) decorated(key state.Key, e *envelope[T]) T {
	rec := e.Record
	c.layout.apply(&rec, e)
	c.layout.setName(&rec, key)
	return rec
}

// Get reads straight through to the server (design D-4).
func (c *collection[T]) Get(ctx context.Context, key state.Key) (T, state.Revision, error) {
	var zero T
	entry, err := c.kv.Get(ctx, wireKey(string(key)))
	if err != nil {
		if isNotFound(err) {
			return zero, "", state.ErrNotFound
		}
		return zero, "", fmt.Errorf("nats: get %s %q: %w", c.layout.Decl.Name, key, err)
	}
	e, err := decodeEnvelope[T](entry.Value())
	if err != nil {
		return zero, "", err
	}
	if !c.visible(&e, time.Now()) {
		return zero, "", state.ErrNotFound
	}
	return c.decorated(key, &e), formatRevision(entry.Revision()), nil
}

// Create writes a new entry. The backend mints ID and CreatedAt regardless
// of what rec carries; Finalizers is read from rec first (caller-owned, and
// a caller may create an already-held record, as the conformance fixtures
// do) then cleared from the value stored under "record" (design D-3).
func (c *collection[T]) Create(ctx context.Context, key state.Key, rec T) (state.Revision, error) {
	finalizers := c.layout.finalizersOf(&rec)
	c.layout.clear(&rec)
	e := envelope[T]{ID: nuid.Next(), CreatedAt: time.Now().UTC(), Finalizers: finalizers, Record: rec}
	data, err := e.encode()
	if err != nil {
		return "", err
	}
	seq, err := c.kv.Create(ctx, wireKey(string(key)), data)
	if err != nil {
		if isWrongLastSequence(err) {
			return "", state.ErrAlreadyExists
		}
		return "", fmt.Errorf("nats: create %s %q: %w", c.layout.Decl.Name, key, err)
	}
	return formatRevision(seq), nil
}

// Update writes rec in place of the record at rev (design D-2's CAS: the
// caller's own Update IS the compare-and-swap, carried by rev as the
// expected last sequence). The current entry is read first only to carry
// forward the backend-minted identity and to apply the finalizer/deletion
// dance of store.go's own doc; the actual conflict detection is the CAS
// write below, which fails if anything — including a write this read never
// saw — moved the key since rev.
func (c *collection[T]) Update(ctx context.Context, key state.Key, rev state.Revision, rec T) (state.Revision, error) {
	wk := wireKey(string(key))
	_, cur, err := c.getVisible(ctx, wk, key, "update")
	if err != nil {
		return "", err
	}
	revUint, err := parseRevision(rev)
	if err != nil {
		return "", state.ErrConflict
	}

	deleting := cur.DeletionRequestedAt != nil
	next := c.layout.finalizersOf(&rec)
	if deleting && addsFinalizer(cur.Finalizers, next) {
		return "", state.ErrDeleting
	}
	if deleting && len(next) == 0 {
		if err := c.kv.Delete(ctx, wk, jetstream.LastRevision(revUint)); err != nil {
			return "", c.conflictOr(err, "release", key)
		}
		return "", nil
	}

	c.layout.clear(&rec)
	updated := envelope[T]{ID: cur.ID, CreatedAt: cur.CreatedAt, Finalizers: next, DeletionRequestedAt: cur.DeletionRequestedAt, Record: rec}
	data, err := updated.encode()
	if err != nil {
		return "", err
	}
	seq, err := c.kv.Update(ctx, wk, data, revUint)
	if err != nil {
		return "", c.conflictOr(err, "update", key)
	}
	return formatRevision(seq), nil
}

// getVisible reads key's current envelope straight from the server, answering
// ErrNotFound for an absent or expired record.
func (c *collection[T]) getVisible(ctx context.Context, wk string, key state.Key, op string) (jetstream.KeyValueEntry, envelope[T], error) {
	entry, err := c.kv.Get(ctx, wk)
	if err != nil {
		if isNotFound(err) {
			return nil, envelope[T]{}, state.ErrNotFound
		}
		return nil, envelope[T]{}, fmt.Errorf("nats: get %s %q before %s: %w", c.layout.Decl.Name, key, op, err)
	}
	cur, err := decodeEnvelope[T](entry.Value())
	if err != nil {
		return nil, envelope[T]{}, err
	}
	if !c.visible(&cur, time.Now()) {
		return nil, envelope[T]{}, state.ErrNotFound
	}
	return entry, cur, nil
}

// markDeleting stamps cur's deletion time under entry's revision. retry is
// true when another writer raced the mark.
func (c *collection[T]) markDeleting(ctx context.Context, wk string, key state.Key, entry jetstream.KeyValueEntry, cur envelope[T]) (retry bool, err error) {
	now := time.Now().UTC()
	cur.DeletionRequestedAt = &now
	data, err := cur.encode()
	if err != nil {
		return false, err
	}
	if _, err := c.kv.Update(ctx, wk, data, entry.Revision()); err != nil {
		if isWrongLastSequence(err) {
			return true, nil
		}
		return false, fmt.Errorf("nats: mark %s %q for deletion: %w", c.layout.Decl.Name, key, err)
	}
	return false, nil
}

// Delete requests removal, carrying no revision (store.go: Delete takes
// none). A record holding a finalizer is marked instead, via a
// read-modify-CAS retry loop since there is no caller revision to carry the
// CAS on; a record already marked is left untouched (a second Delete
// changes nothing).
func (c *collection[T]) Delete(ctx context.Context, key state.Key) error {
	wk := wireKey(string(key))
	for attempt := 0; attempt < casRetryMaxAttempts; attempt++ {
		entry, cur, err := c.getVisible(ctx, wk, key, "delete")
		if errors.Is(err, state.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(cur.Finalizers) == 0 {
			if err := c.kv.Delete(ctx, wk); err != nil {
				return fmt.Errorf("nats: delete %s %q: %w", c.layout.Decl.Name, key, err)
			}
			return nil
		}
		if cur.DeletionRequestedAt != nil {
			return nil
		}
		retry, err := c.markDeleting(ctx, wk, key, entry, cur)
		if retry {
			continue // another writer raced the mark; look again
		}
		return err
	}
	return fmt.Errorf("nats: delete %s %q: exhausted %d attempts on conflict", c.layout.Decl.Name, key, casRetryMaxAttempts)
}

// DeleteAt is Delete conditional on rev (store.go). No retry: a lost race is
// reported as ErrConflict, as documented, not silently resolved.
func (c *collection[T]) DeleteAt(ctx context.Context, key state.Key, rev state.Revision) error {
	wk := wireKey(string(key))
	entry, cur, err := c.getVisible(ctx, wk, key, "deleteAt")
	if err != nil {
		return err
	}
	if formatRevision(entry.Revision()) != rev {
		return state.ErrConflict
	}
	if len(cur.Finalizers) == 0 {
		if err := c.kv.Delete(ctx, wk, jetstream.LastRevision(entry.Revision())); err != nil {
			return c.conflictOr(err, "deleteAt", key)
		}
		return nil
	}
	if cur.DeletionRequestedAt != nil {
		return nil
	}
	retry, err := c.markDeleting(ctx, wk, key, entry, cur)
	if retry {
		return state.ErrConflict
	}
	return err
}

// conflictOr maps a lost compare-and-set to ErrConflict and wraps anything else.
func (c *collection[T]) conflictOr(err error, op string, key state.Key) error {
	if isWrongLastSequence(err) {
		return state.ErrConflict
	}
	return fmt.Errorf("nats: %s %s %q: %w", op, c.layout.Decl.Name, key, err)
}

// List serves a declared index from the watch-fed mirror (design D-4):
// blocking until its first catch-up AND until the mirror has observed the
// bucket's own latest write as of this call (latestSeq) — "the first
// catch-up" alone only proves the mirror was current once, arbitrarily long
// before a write this very List is expected to see.
func (c *collection[T]) List(ctx context.Context, idx state.Index[T], value string) ([]T, error) {
	if !c.layout.Decl.Declares(idx) {
		return nil, state.ErrUnknownIndex
	}
	target, err := c.latestSeq(ctx)
	if err != nil {
		return nil, fmt.Errorf("nats: list %s: %w", c.layout.Decl.Name, err)
	}
	if err := c.mirror.waitCaughtUpTo(ctx, target); err != nil {
		return nil, fmt.Errorf("nats: list %s: %w", c.layout.Decl.Name, err)
	}
	now := time.Now()
	var out []T
	for key, m := range c.mirror.snapshot() {
		e := m.Env
		if !c.visible(&e, now) {
			continue
		}
		rec := c.decorated(key, &e)
		if v, listed := idx.Value(&rec); listed && v == value {
			out = append(out, rec)
		}
	}
	return out, nil
}

// Watch delivers an at-least-once feed, each call its own independent
// ordered consumer (design D-2, rawwatch.go): an empty resume starts
// strictly from now, a non-empty one resumes at the ordered sequence
// rev+1. A periodic bookmark carries the last delivered sequence forward so
// an idle consumer's resume point keeps moving.
func (c *collection[T]) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event[T], error) {
	raw, err := watchRaw(ctx, c.js, c.streamName, c.bucket, resume)
	if err != nil {
		return nil, fmt.Errorf("nats: watch %s: %w", c.layout.Decl.Name, err)
	}
	out := make(chan state.Event[T])
	go c.drainWatch(ctx, raw, out)
	return out, nil
}

// bookmarkInterval is how often an idle Watch feed emits a bookmark (design
// D-2).
const bookmarkInterval = 10 * time.Second

func (c *collection[T]) drainWatch(ctx context.Context, raw <-chan kvEntry, out chan<- state.Event[T]) {
	defer close(out)
	ticker := time.NewTicker(bookmarkInterval)
	defer ticker.Stop()
	var lastSeen state.Revision
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-raw:
			if !ok {
				return
			}
			ev, ok := c.translate(entry)
			if !ok {
				continue
			}
			lastSeen = ev.Resume
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		case <-ticker.C:
			if lastSeen == "" {
				continue
			}
			select {
			case out <- state.Event[T]{Type: state.EventBookmark, Resume: lastSeen}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// translate maps one raw stream entry to the contract's Event, trusting the
// raw payload and never re-reading the key (the same "no gap" reasoning
// internal/state/kube/base.go documents for its own watch translation).
func (c *collection[T]) translate(entry kvEntry) (state.Event[T], bool) {
	key, err := unwireKey(entry.Key)
	if err != nil {
		return state.Event[T]{}, false
	}
	rev := formatRevision(entry.Revision)
	if entry.IsDelete {
		return state.Event[T]{Type: state.EventDelete, Key: state.Key(key), Resume: rev}, true
	}
	e, err := decodeEnvelope[T](entry.Value)
	if err != nil {
		return state.Event[T]{}, false
	}
	return state.Event[T]{Type: state.EventPut, Key: state.Key(key), Revision: rev, Record: c.decorated(state.Key(key), &e), Resume: rev}, true
}

var _ state.Collection[struct{}] = (*collection[struct{}])(nil)
