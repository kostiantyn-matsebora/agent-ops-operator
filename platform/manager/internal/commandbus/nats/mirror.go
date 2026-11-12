package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// mirrorRetryDelay bounds how fast a mirror whose watch dropped (server
// restart, network blip) tries again — mirrors internal/state/nats's own
// mirrorRetryDelay, restated rather than imported (design D-1).
var mirrorRetryDelay = time.Second

// mirrorEntry is one cached task and the KV revision it was read at — the
// revision Claim's CAS write and Fail/Settle/GetTask's reap write both
// carry forward as their expected last sequence.
type mirrorEntry struct {
	Task     commandbus.Task
	Revision uint64
}

// taskMirror is the watch-fed, in-memory read model task 3.3 asks for: one
// background watch over the whole "tasks" bucket keeps a map of every task
// by UID, so Claim's candidate search, its per-key ordering check and
// Stats never read the server (design D-2's own "the transport only stores
// and wakes" — the mirror is this backend's model of that). Settle, Fail
// and GetTask still read the server directly for the one UID they act on:
// a lease decision is never a guess (the same split
// internal/state/nats/collection.go draws between Get/Update and List).
type taskMirror struct {
	kv jetstream.KeyValue
	// onChange is called after every applied update — the nats Bus wires
	// it to its clock's notify, so a Claim blocked on nothing eligible
	// wakes the instant the mirror sees why.
	onChange func()

	mu      sync.RWMutex
	byUID   map[string]mirrorEntry
	lastSeq uint64
	ready   chan struct{}
	lastErr error
	closed  chan struct{}
}

func newTaskMirror(kv jetstream.KeyValue, onChange func()) *taskMirror {
	m := &taskMirror{kv: kv, onChange: onChange, byUID: map[string]mirrorEntry{}, ready: make(chan struct{}), closed: make(chan struct{})}
	go m.run()
	return m
}

// waitReady blocks until the mirror's first full replay of the bucket
// completes, however many reconnect attempts that takes.
func (m *taskMirror) waitReady(ctx context.Context) error {
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

// caughtUpPollInterval bounds how often waitCaughtUpTo re-checks once it
// has to wait at all — mirrors internal/state/nats's own constant of the
// same name.
const caughtUpPollInterval = 2 * time.Millisecond

// waitCaughtUpTo blocks until the mirror has processed a sequence at least
// as high as target — Stats' own fence (stats.go) against a write that
// landed on the server a moment before the call but has not yet reached
// this mirror's watch.
func (m *taskMirror) waitCaughtUpTo(ctx context.Context, target uint64) error {
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

func (m *taskMirror) run() {
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
		case <-time.After(mirrorRetryDelay):
		}
	}
}

func (m *taskMirror) runOnce(ctx context.Context) error {
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
				return errWatchEnded
			}
			if entry == nil {
				m.markReady()
				continue
			}
			m.apply(entry)
		}
	}
}

func (m *taskMirror) markReady() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.ready:
	default:
		close(m.ready)
	}
}

func (m *taskMirror) apply(entry jetstream.KeyValueEntry) {
	uid := entry.Key()
	m.mu.Lock()
	if entry.Revision() > m.lastSeq {
		m.lastSeq = entry.Revision()
	}
	switch entry.Operation() {
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		delete(m.byUID, uid)
		m.mu.Unlock()
	default:
		var t commandbus.Task
		if err := json.Unmarshal(entry.Value(), &t); err != nil {
			m.mu.Unlock()
			return // an entry this process never wrote — dropped, not crashed
		}
		m.byUID[uid] = mirrorEntry{Task: t, Revision: entry.Revision()}
		m.mu.Unlock()
	}
	if m.onChange != nil {
		m.onChange()
	}
}

// snapshot returns every cached task alongside the revision it was last
// seen at.
func (m *taskMirror) snapshot() map[string]mirrorEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]mirrorEntry, len(m.byUID))
	for k, v := range m.byUID {
		out[k] = v
	}
	return out
}

// get returns one cached task by UID.
func (m *taskMirror) get(uid string) (mirrorEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.byUID[uid]
	return e, ok
}

func (m *taskMirror) stop() {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
}
