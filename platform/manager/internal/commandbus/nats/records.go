package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// taskDoc is one task's record: the whole of its state. It is written by the
// submitter, then by whoever holds the task, one at a time.
type taskDoc struct {
	Task commandbus.Task `json:"task"`
	// Seq is the stream sequence of the task's message: its place in line.
	Seq     uint64 `json:"seq"`
	Subject string `json:"subject,omitempty"`
}

// taskKey is the record key of the latest task of a command id.
func taskKey(q commandbus.Queue, k commandbus.Kind, id commandbus.CommandID) string {
	return escapeToken(string(q)) + "." + escapeToken(string(k)) + "." + escapeToken(string(id))
}

// archiveKey is where a task that a newer task of its command id replaced is
// kept, so it still reads back by its uid.
func archiveKey(uid string) string { return "dead." + uid }

// newUID makes a task's identity: the key it is recorded under, then a nonce.
func newUID(rk string) string { return rk + "." + nuid.Next() }

// keyOfUID recovers the record key from a uid.
func keyOfUID(uid string) (string, bool) {
	i := strings.LastIndexByte(uid, '.')
	if i <= 0 {
		return "", false
	}
	return uid[:i], true
}

var errConflict = errors.New("commandbus/nats: the record changed under this write")

func (b *Bus) getDoc(ctx context.Context, rk string) (*taskDoc, uint64, error) {
	e, err := b.tasks.Get(ctx, rk)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("commandbus/nats: read task: %w", err)
	}
	var d taskDoc
	if err := json.Unmarshal(e.Value(), &d); err != nil {
		return nil, 0, fmt.Errorf("commandbus/nats: decode task %q: %w", rk, err)
	}
	return &d, e.Revision(), nil
}

// putDoc writes a record: create when rev is 0, otherwise an update that holds
// only if the record is still at rev. A lost race is errConflict.
func (b *Bus) putDoc(ctx context.Context, rk string, d taskDoc, rev uint64) (uint64, error) {
	body, err := json.Marshal(d)
	if err != nil {
		return 0, err
	}
	var next uint64
	if rev == 0 {
		next, err = b.tasks.Create(ctx, rk, body)
	} else {
		next, err = b.tasks.Update(ctx, rk, body, rev)
	}
	if err != nil {
		var api *jetstream.APIError
		if errors.Is(err, jetstream.ErrKeyExists) || (errors.As(err, &api) && api.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence) {
			return 0, errConflict
		}
		return 0, fmt.Errorf("commandbus/nats: write task: %w", err)
	}
	b.mirror.apply(rk, d, next)
	return next, nil
}

// viewOf is a task as of now: a lease that has passed reads as a failed
// attempt, and a task still waiting past its TTL as expired. Nothing is
// written; the sweep makes these facts of the record.
func (b *Bus) viewOf(t commandbus.Task, now time.Time) commandbus.Task {
	decl, _ := commandbus.Resolve(b.cfg.inventory, t.Queue, t.Kind)
	if t.State == commandbus.StateRunning && t.Lease != nil && now.After(t.Lease.Until) {
		until := t.Lease.Until
		t.Code, t.Detail, t.Lease, t.UpdatedAt = commandbus.CodeUnavailable, "lease lapsed", nil, until
		if t.Attempt < t.MaxAttempts && decl.Policy.DeliveryTTL <= 0 {
			t.State, t.NotBefore = commandbus.StateRetrying, until
		} else {
			t.State, t.DeadAt, t.ParkedUntil = commandbus.StateDead, until, until.Add(decl.Policy.ParkCoolOff)
		}
	}
	if ttl := decl.Policy.TTL; ttl > 0 && (t.State == commandbus.StateQueued || t.State == commandbus.StateRetrying) {
		if deadline := t.SubmittedAt.Add(ttl); now.After(deadline) {
			t.State, t.UpdatedAt = commandbus.StateExpired, deadline
		}
	}
	return t
}

func claimable(t commandbus.Task, now time.Time) bool {
	switch t.State {
	case commandbus.StateQueued:
		return true
	case commandbus.StateRetrying:
		return !t.NotBefore.After(now)
	}
	return false
}

// mirrored is a record with the revision it was seen at.
type mirrored struct {
	doc taskDoc
	rev uint64
	key string
}

// taskMirror is a read cache of the task records, fed by a watch. It answers
// listings only: Submit, Claim, Settle and Fail read and write the record
// itself, so a lagging cache never holds up work. A process's own writes are
// applied at once, so it reads what it wrote.
type taskMirror struct {
	mu    sync.RWMutex
	docs  map[string]mirrored
	ready chan struct{}
	once  sync.Once
	wake  chan struct{}
	// applied is the highest stream sequence the watch has delivered, which is
	// how a listing knows the cache has caught up with a given write.
	applied atomic.Uint64
}

func newTaskMirror() *taskMirror {
	return &taskMirror{docs: map[string]mirrored{}, ready: make(chan struct{}), wake: make(chan struct{})}
}

func (m *taskMirror) apply(key string, d taskDoc, rev uint64) {
	m.mu.Lock()
	if cur, ok := m.docs[key]; !ok || rev >= cur.rev {
		m.docs[key] = mirrored{doc: d, rev: rev, key: key}
	}
	close(m.wake)
	m.wake = make(chan struct{})
	m.mu.Unlock()
}

func (m *taskMirror) remove(key string, rev uint64) {
	m.mu.Lock()
	if cur, ok := m.docs[key]; ok && rev >= cur.rev {
		delete(m.docs, key)
	}
	close(m.wake)
	m.wake = make(chan struct{})
	m.mu.Unlock()
}

func (m *taskMirror) waiter() <-chan struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.wake
}

func (m *taskMirror) waitReady(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run keeps the cache fed until ctx ends, restarting a watch that ends.
func (m *taskMirror) run(ctx context.Context, kv jetstream.KeyValue) {
	for ctx.Err() == nil {
		w, err := kv.WatchAll(ctx)
		if err == nil {
			m.feed(ctx, w)
			_ = w.Stop()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (m *taskMirror) feed(ctx context.Context, w jetstream.KeyWatcher) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-w.Updates():
			if !ok {
				return
			}
			if e == nil {
				m.once.Do(func() { close(m.ready) })
				continue
			}
			if e.Operation() != jetstream.KeyValuePut {
				m.remove(e.Key(), e.Revision())
			} else {
				var d taskDoc
				if json.Unmarshal(e.Value(), &d) == nil {
					m.apply(e.Key(), d, e.Revision())
				}
			}
			for cur := m.applied.Load(); e.Revision() > cur && !m.applied.CompareAndSwap(cur, e.Revision()); cur = m.applied.Load() {
			}
		}
	}
}

// queueDocs lists the records of a queue, in line order.
func (m *taskMirror) queueDocs(queue commandbus.Queue) []mirrored {
	m.mu.RLock()
	var out []mirrored
	for _, d := range m.docs {
		if d.doc.Task.Queue == queue {
			out = append(out, d)
		}
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].doc.Seq < out[j].doc.Seq })
	return out
}

func (m *taskMirror) all() []mirrored {
	m.mu.RLock()
	out := make([]mirrored, 0, len(m.docs))
	for _, d := range m.docs {
		out = append(out, d)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].doc.Seq < out[j].doc.Seq })
	return out
}

// eligible lists up to limit tasks of a hand-over queue that may be handed out
// now, oldest first: waiting, or retrying with its delay over, and with no
// earlier task of the same key unsettled. A task being held, or waiting out a
// delay, keeps its key closed behind it.
func (b *Bus) eligible(queue commandbus.Queue, now time.Time, limit int) []mirrored {
	closed := map[commandbus.Key]bool{}
	var out []mirrored
	for _, d := range b.mirror.queueDocs(queue) {
		t := b.viewOf(d.doc.Task, now)
		if t.State.Settled() || closed[t.Key] {
			continue
		}
		closed[t.Key] = true
		if claimable(t, now) {
			d.doc.Task = t
			out = append(out, d)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

// fresh waits, a bounded time, until the cache has seen every write the stream
// held when this was called, so a listing includes what other replicas wrote a
// moment ago. It never holds up Submit, Claim, Settle or Fail, which read the
// record itself, and it gives up quietly: a listing may be a little stale, and
// is never refused.
func (b *Bus) fresh(ctx context.Context) {
	s, err := b.js.Stream(ctx, b.tasksStream)
	if err != nil {
		return
	}
	info, err := s.Info(ctx)
	if err != nil {
		return
	}
	target := info.State.LastSeq
	deadline := time.Now().Add(2 * time.Second)
	for b.mirror.applied.Load() < target && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-b.mirror.waiter():
		case <-time.After(20 * time.Millisecond):
		}
	}
}
