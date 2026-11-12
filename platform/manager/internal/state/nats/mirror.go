package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// mirrorRetryDelay bounds how fast a mirror whose watch dropped (server
// restart, network blip) tries again.
// A variable only so a test need not wait a second per retry.
var mirrorRetryDelay = time.Second

// mirror is the watch-fed, in-memory read model List serves from (design
// D-4): one background watch over the whole bucket keeps a map of every
// envelope, keyed by the original state.Key recovered from the wire key
// (key.go's wireKey/unwireKey pair — exactly reversible, so no second index
// is needed). Get, Create, Update, Delete and DeleteAt never read it: a
// precondition is never a guess, only a lookup is.
type mirror[T any] struct {
	kv jetstream.KeyValue

	mu      sync.RWMutex
	byKey   map[state.Key]mirrorEntry[T]
	lastSeq uint64 // the highest sequence this mirror has observed, any kind of event
	ready   chan struct{}
	lastErr error // the latest failure of the watch, for the message of a wait that gives up
	closed  chan struct{}

	retryDelay time.Duration // read once at construction, from mirrorRetryDelay
}

// mirrorEntry is one cached record: the envelope and the KV sequence it was
// read at, needed by the sweep (expiring.go) to delete under the exact
// revision it observed rather than blindly, which would risk destroying a
// write that landed between the snapshot and the delete.
type mirrorEntry[T any] struct {
	Env      envelope[T]
	Revision uint64
}

// newMirror starts the background watch and returns at once; a caller that
// needs the first catch-up waits on waitReady.
func newMirror[T any](kv jetstream.KeyValue) *mirror[T] {
	m := &mirror[T]{kv: kv, byKey: map[state.Key]mirrorEntry[T]{}, ready: make(chan struct{}), closed: make(chan struct{}), retryDelay: mirrorRetryDelay}
	go m.run()
	return m
}

// waitReady blocks until the mirror's first catch-up over the bucket
// completes, the "List blocks until the first catch-up completes" half of
// design D-4.
//
// A watch that fails or ends before the first catch-up is retried, not
// recorded: the mirror becomes ready when a watch has replayed the whole
// bucket, however many attempts that takes, and a caller that cannot wait
// that long leaves with its own context's error, naming the last failure.
func (m *mirror[T]) waitReady(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		m.mu.RLock()
		last := m.lastErr
		m.mu.RUnlock()
		if last != nil {
			return fmt.Errorf("%w (the watch last failed: %v)", ctx.Err(), last)
		}
		return ctx.Err()
	}
}

// caughtUpPollInterval bounds how often waitCaughtUpTo re-checks once it has
// to wait at all — the common case (already current) never sleeps.
const caughtUpPollInterval = 2 * time.Millisecond

// waitCaughtUpTo blocks until the mirror has processed a sequence at least
// as high as target: "the first catch-up" alone only proves the mirror was
// current ONCE, not that it has seen a write that landed after.
func (m *mirror[T]) waitCaughtUpTo(ctx context.Context, target uint64) error {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	for {
		m.mu.RLock()
		reached := m.lastSeq >= target
		m.mu.RUnlock()
		if reached {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(caughtUpPollInterval):
		}
	}
}

// run feeds the mirror for the life of the bucket handle it was built for. A
// watch that ends (a connection drop, a server restart) is reopened: losing
// the mirror would silently starve every future List rather than surface
// once and heal.
func (m *mirror[T]) run() {
	ctx := context.Background()
	for {
		select {
		case <-m.closed:
			return
		default:
		}
		if err := m.runOnce(ctx); err != nil {
			m.mu.Lock()
			m.lastErr = err
			m.mu.Unlock()
		}
		select {
		case <-m.closed:
			return
		case <-time.After(m.retryDelay):
		}
	}
}

// runOnce drains one watch to completion (or failure), replaying every key's
// current value then every future change — what "a list after a returned
// create includes it" needs.
func (m *mirror[T]) runOnce(ctx context.Context) error {
	w, err := m.kv.WatchAll(ctx)
	if err != nil {
		return err
	}
	defer w.Stop()
	for {
		select {
		case <-m.closed:
			return nil
		case entry, ok := <-w.Updates():
			if !ok {
				// The watch ended, however far it had got: reconnect and replay.
				return errWatchEnded
			}
			if entry == nil {
				m.markReady() // every initial value has now been replayed
				continue
			}
			m.apply(entry)
		}
	}
}

// markReady closes ready, once. It is called only when a watch has replayed
// every value the bucket held, so a ready mirror is a complete one.
func (m *mirror[T]) markReady() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.ready:
	default:
		close(m.ready)
	}
}

// errWatchEnded is a watch that closed on its own, a dropped connection or a
// restarted server.
var errWatchEnded = errors.New("nats: the bucket watch ended")

// apply folds one watch entry into the map: a put decodes and stores the
// envelope, a delete or purge removes it. The key is recovered from the wire
// key wireKey/unwireKey round-trip; an entry this backend never wrote (a
// corrupt escape) is dropped rather than crashing the mirror.
func (m *mirror[T]) apply(entry jetstream.KeyValueEntry) {
	key, err := unwireKey(entry.Key())
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.Revision() > m.lastSeq {
		m.lastSeq = entry.Revision()
	}
	switch entry.Operation() {
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		delete(m.byKey, state.Key(key))
		return
	}
	e, err := decodeEnvelope[T](entry.Value())
	if err != nil {
		return
	}
	m.byKey[state.Key(key)] = mirrorEntry[T]{Env: e, Revision: entry.Revision()}
}

// snapshot returns every cached entry alongside its own key. Visibility
// (expiry) is the caller's own decision — List and the sweep (expiring.go)
// each apply it their own way.
func (m *mirror[T]) snapshot() map[state.Key]mirrorEntry[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[state.Key]mirrorEntry[T], len(m.byKey))
	for k, e := range m.byKey {
		out[k] = e
	}
	return out
}

// stop ends the background watch. Not part of any capability interface —
// nothing in production ever calls it, since a bucket is opened once for the
// manager's whole life — but a test opening many isolated instances needs it
// to avoid leaking one goroutine and one subscription per scenario.
func (m *mirror[T]) stop() {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
}
