package conformance

// memStore is a throwaway, in-memory state.Store used only to prove this
// suite is internally consistent before any real backend exists (tasks.md
// 3.1/3.2) — not a mock: every guarantee below (CAS, the three declared
// indexes, a resumable feed, blobs, append-only runs, expiry) is honestly
// computed from state this type actually holds, never a hardcoded return
// value.
//
// It exists for this package's own tests only (hence the _test.go name)
// and is not exported. internal/state/kube is the real backend, built
// separately.

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

type memRecord struct {
	rec state.ConversationRecord
	rev uint64
}

type memStore struct {
	mu      sync.Mutex
	records map[state.Key]*memRecord
	runs    map[state.Key][]state.Run
	blobs   map[state.Key]state.Blob
	seq     uint64
	ids     uint64
	log     []state.Event // every ConversationRecord Put/Delete, in seq order
}

// newMemStore returns a fresh, empty, isolated Store — a new map set every
// call, so two calls never share a record (the scope-isolation scenario).
func newMemStore() state.Store {
	return &memStore{
		records: map[state.Key]*memRecord{},
		runs:    map[state.Key][]state.Run{},
		blobs:   map[state.Key]state.Blob{},
	}
}

func revisionOf(seq uint64) state.Revision {
	return state.Revision(strconv.FormatUint(seq, 10))
}

func seqOf(rev state.Revision) uint64 {
	if rev == "" {
		return 0
	}
	n, err := strconv.ParseUint(string(rev), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// sweepExpiredLocked removes every record past its ExpiresAt and emits an
// EventDelete for each. Called at the top of every public method, so
// removal eventually happens on the very next Store call after expiry
// (spec: "how the removal happens is the backend's own").
func (m *memStore) sweepExpiredLocked(now time.Time) {
	for key, mr := range m.records {
		if mr.rec.ExpiresAt != nil && !mr.rec.ExpiresAt.After(now) {
			delete(m.records, key)
			m.mintLocked(state.EventDelete, key, state.ConversationRecord{})
		}
	}
}

func (m *memStore) mintLocked(typ state.EventType, key state.Key, rec state.ConversationRecord) uint64 {
	m.seq++
	seq := m.seq
	rev := revisionOf(seq)
	evt := state.Event{Type: typ, Key: key, Resume: rev}
	if typ == state.EventPut {
		evt.Revision = rev
		evt.Record = rec
	}
	m.log = append(m.log, evt)
	return seq
}

func (m *memStore) Get(ctx context.Context, key state.Key) (state.ConversationRecord, state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	mr, ok := m.records[key]
	if !ok {
		return state.ConversationRecord{}, "", state.ErrNotFound
	}
	return mr.rec, revisionOf(mr.rev), nil
}

// utcRecord normalises the instants a scenario reads back to UTC, the
// contract's rule (record.go).
func utcRecord(rec state.ConversationRecord) state.ConversationRecord {
	if rec.LastActivity != nil {
		t := rec.LastActivity.UTC()
		rec.LastActivity = &t
	}
	if rec.Inflight != nil {
		in := *rec.Inflight
		in.DispatchedAt = in.DispatchedAt.UTC()
		rec.Inflight = &in
	}
	return rec
}

// mintIdentity gives rec the read-only fields a backend owns (record.go).
func (m *memStore) mintIdentity(rec state.ConversationRecord) state.ConversationRecord {
	m.ids++
	rec.ID = "mem-" + strconv.FormatUint(m.ids, 10)
	rec.CreatedAt = time.Now().UTC()
	rec.DeletionRequestedAt = nil
	return rec
}

func (m *memStore) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	if _, exists := m.records[key]; exists {
		return "", state.ErrAlreadyExists
	}
	rec = utcRecord(m.mintIdentity(rec))
	seq := m.mintLocked(state.EventPut, key, rec)
	m.records[key] = &memRecord{rec: rec, rev: seq}
	return revisionOf(seq), nil
}

func (m *memStore) Update(ctx context.Context, key state.Key, rev state.Revision, rec state.ConversationRecord) (state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	mr, ok := m.records[key]
	if !ok {
		return "", state.ErrNotFound
	}
	if revisionOf(mr.rev) != rev {
		return "", state.ErrConflict
	}
	rec = utcRecord(rec)
	rec.ID, rec.CreatedAt, rec.DeletionRequestedAt = mr.rec.ID, mr.rec.CreatedAt, mr.rec.DeletionRequestedAt
	if rec.DeletionRequestedAt != nil {
		for _, f := range rec.Finalizers {
			if !containsString(mr.rec.Finalizers, f) {
				return "", state.ErrDeleting
			}
		}
		if len(rec.Finalizers) == 0 {
			delete(m.records, key)
			return revisionOf(m.mintLocked(state.EventDelete, key, state.ConversationRecord{})), nil
		}
	}
	seq := m.mintLocked(state.EventPut, key, rec)
	m.records[key] = &memRecord{rec: rec, rev: seq}
	return revisionOf(seq), nil
}

func containsString(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func (m *memStore) Delete(ctx context.Context, key state.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	mr, ok := m.records[key]
	if !ok {
		return nil
	}
	if len(mr.rec.Finalizers) > 0 {
		if mr.rec.DeletionRequestedAt == nil {
			now := time.Now().UTC()
			mr.rec.DeletionRequestedAt = &now
			mr.rev = m.mintLocked(state.EventPut, key, mr.rec)
		}
		return nil
	}
	delete(m.records, key)
	m.mintLocked(state.EventDelete, key, state.ConversationRecord{})
	return nil
}

func (m *memStore) List(ctx context.Context, index state.Index, key state.ListKey) ([]state.ConversationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	var match func(state.ConversationRecord) bool
	switch index {
	case state.ByConversationID:
		match = func(r state.ConversationRecord) bool { return r.ConversationID == key.ConversationID }
	case state.ByPipelineAndPhase:
		match = func(r state.ConversationRecord) bool {
			return r.PipelineRef == key.PipelineRef && r.Phase == key.Phase
		}
	case state.ByPhase:
		match = func(r state.ConversationRecord) bool { return r.Phase == key.Phase }
	case state.ByCausedBy:
		match = func(r state.ConversationRecord) bool { return r.CausedBy != nil && r.CausedBy.Parent == key.CausedBy }
	case state.ByNameBase:
		match = func(r state.ConversationRecord) bool { return r.NameBase == key.NameBase }
	case state.ByAll:
		match = func(state.ConversationRecord) bool { return true }
	default:
		return nil, state.ErrUnknownIndex
	}
	var out []state.ConversationRecord
	for _, mr := range m.records {
		if match(mr.rec) {
			out = append(out, mr.rec)
		}
	}
	return out, nil
}

// Watch is a channel-fanout feed: each call gets its own goroutine and
// channel, polling the shared log from its own resume point, so concurrent
// watchers never interfere with each other and none can drop an event —
// the log is retained in full for the lifetime of this throwaway double.
func (m *memStore) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event, error) {
	m.mu.Lock()
	start := m.seq
	m.mu.Unlock()

	next := start
	if resume != "" {
		next = seqOf(resume)
	}

	out := make(chan state.Event)
	go func() {
		defer close(out)
		for {
			m.mu.Lock()
			m.sweepExpiredLocked(time.Now())
			var batch []state.Event
			for _, e := range m.log {
				if seqOf(e.Resume) > next {
					batch = append(batch, e)
				}
			}
			m.mu.Unlock()

			for _, e := range batch {
				select {
				case out <- e:
					next = seqOf(e.Resume)
				case <-ctx.Done():
					return
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Millisecond):
			}
		}
	}()
	return out, nil
}

func (m *memStore) AppendRun(ctx context.Context, key state.Key, run state.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run.StartedAt != nil {
		t := run.StartedAt.UTC()
		run.StartedAt = &t
	}
	m.runs[key] = append(m.runs[key], run)
	return nil
}

func (m *memStore) UpdateRun(ctx context.Context, key state.Key, runID string, upd state.RunUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[key]; !ok {
		return state.ErrNotFound
	}
	for i := range m.runs[key] {
		run := &m.runs[key][i]
		if run.RunID != runID {
			continue
		}
		run.DeliveryTracked = run.DeliveryTracked || upd.MarkDeliveryTracked
		run.RoutedToParent = run.RoutedToParent || upd.MarkRoutedToParent
		for _, ch := range upd.MarkDelivered {
			if !containsString(run.Delivered, ch) {
				run.Delivered = append(run.Delivered, ch)
			}
		}
		return nil
	}
	return state.ErrNotFound
}

func (m *memStore) ListRuns(ctx context.Context, key state.Key) ([]state.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]state.Run, len(m.runs[key]))
	copy(out, m.runs[key])
	return out, nil
}

func (m *memStore) PutBlob(ctx context.Context, key state.Key, blob state.Blob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[key] = blob
	return nil
}

func (m *memStore) GetBlob(ctx context.Context, key state.Key) (state.Blob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	blob, ok := m.blobs[key]
	if !ok {
		return state.Blob{}, state.ErrNotFound
	}
	return blob, nil
}

func (m *memStore) DeleteBlob(ctx context.Context, key state.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blobs, key)
	return nil
}

var _ state.Store = (*memStore)(nil)
