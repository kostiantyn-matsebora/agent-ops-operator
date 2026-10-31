package conformance

// memSignalStore is the throwaway in-memory state.SignalStore that proves the
// signal scenarios are internally consistent before a real backend runs them,
// as memStore does for Store. Every guarantee is computed from the state it
// holds.

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

type memSignal struct {
	rec state.SignalRecord
	rev uint64
}

type memSignalStore struct {
	mu       sync.Mutex
	records  map[state.Key]*memSignal
	cooldown map[string][]state.CooldownEntry
	seq, ids uint64
	log      []state.SignalEvent
}

func newMemSignalStore() state.SignalStore {
	return &memSignalStore{records: map[state.Key]*memSignal{}, cooldown: map[string][]state.CooldownEntry{}}
}

func (m *memSignalStore) mintLocked(typ state.EventType, key state.Key, rec state.SignalRecord) uint64 {
	m.seq++
	rev := revisionOf(m.seq)
	ev := state.SignalEvent{Type: typ, Key: key, Resume: rev}
	if typ == state.EventPut {
		ev.Revision, ev.Record = rev, rec
	}
	m.log = append(m.log, ev)
	return m.seq
}

func (m *memSignalStore) sweepLocked(now time.Time) {
	for key, ms := range m.records {
		if ms.rec.ExpiresAt != nil && !ms.rec.ExpiresAt.After(now) {
			delete(m.records, key)
			m.mintLocked(state.EventDelete, key, state.SignalRecord{})
		}
	}
}

func utcSignal(rec state.SignalRecord) state.SignalRecord {
	for _, p := range []**time.Time{&rec.FirstAttemptAt, &rec.CompletedAt, &rec.ConsumedAt, &rec.ExpiresAt} {
		if *p != nil {
			t := (*p).UTC()
			*p = &t
		}
	}
	return rec
}

func (m *memSignalStore) Get(ctx context.Context, key state.Key) (state.SignalRecord, state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	ms, ok := m.records[key]
	if !ok {
		return state.SignalRecord{}, "", state.ErrNotFound
	}
	return ms.rec, revisionOf(ms.rev), nil
}

func (m *memSignalStore) Create(ctx context.Context, key state.Key, rec state.SignalRecord) (state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	if _, exists := m.records[key]; exists {
		return "", state.ErrAlreadyExists
	}
	m.ids++
	rec.ID, rec.CreatedAt = "mem-sig-"+strconv.FormatUint(m.ids, 10), time.Now().UTC()
	rec = utcSignal(rec)
	seq := m.mintLocked(state.EventPut, key, rec)
	m.records[key] = &memSignal{rec: rec, rev: seq}
	return revisionOf(seq), nil
}

func (m *memSignalStore) Update(ctx context.Context, key state.Key, rev state.Revision, rec state.SignalRecord) (state.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	ms, ok := m.records[key]
	if !ok {
		return "", state.ErrNotFound
	}
	if revisionOf(ms.rev) != rev {
		return "", state.ErrConflict
	}
	next := ms.rec // publication fields, ID and CreatedAt stay
	next.Phase, next.Reason, next.Attempts = rec.Phase, rec.Reason, rec.Attempts
	next.FirstAttemptAt, next.CompletedAt, next.ConsumedBy, next.ConsumedAt = rec.FirstAttemptAt, rec.CompletedAt, rec.ConsumedBy, rec.ConsumedAt
	next.Claimants, next.ExpiresAt = append([]state.SignalClaimant(nil), rec.Claimants...), rec.ExpiresAt
	next = utcSignal(next)
	seq := m.mintLocked(state.EventPut, key, next)
	m.records[key] = &memSignal{rec: next, rev: seq}
	return revisionOf(seq), nil
}

func (m *memSignalStore) Delete(ctx context.Context, key state.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	if _, ok := m.records[key]; ok {
		delete(m.records, key)
		m.mintLocked(state.EventDelete, key, state.SignalRecord{})
	}
	return nil
}

func (m *memSignalStore) List(ctx context.Context, index state.SignalIndex, key state.SignalListKey) ([]state.SignalRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	var match func(state.SignalRecord) bool
	switch index {
	case state.SignalAll:
		match = func(state.SignalRecord) bool { return true }
	case state.SignalNonTerminalBySource:
		match = func(r state.SignalRecord) bool { return r.SourceRef == key.Source && !r.Terminal() }
	case state.SignalNonTerminalByInitiation:
		match = func(r state.SignalRecord) bool {
			return r.SourceRef == key.Source && r.InitiationID == key.InitiationID && !r.Terminal()
		}
	default:
		return nil, state.ErrUnknownIndex
	}
	var out []state.SignalRecord
	for _, ms := range m.records {
		if match(ms.rec) {
			out = append(out, ms.rec)
		}
	}
	return out, nil
}

func (m *memSignalStore) Watch(ctx context.Context, resume state.Revision) (<-chan state.SignalEvent, error) {
	m.mu.Lock()
	next := m.seq
	m.mu.Unlock()
	if resume != "" {
		next = seqOf(resume)
	}
	out := make(chan state.SignalEvent)
	go func() {
		defer close(out)
		for {
			m.mu.Lock()
			m.sweepLocked(time.Now())
			var batch []state.SignalEvent
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

func (m *memSignalStore) GetCooldown(ctx context.Context, source string) ([]state.CooldownEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]state.CooldownEntry(nil), m.cooldown[source]...), nil
}

func (m *memSignalStore) PutCooldown(ctx context.Context, source string, entries []state.CooldownEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]state.CooldownEntry, len(entries))
	for i, e := range entries {
		out[i] = state.CooldownEntry{InitiationID: e.InitiationID, At: e.At.UTC()}
	}
	m.cooldown[source] = out
	return nil
}

func (m *memSignalStore) ReleaseSource(ctx context.Context, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())
	for key, ms := range m.records {
		if ms.rec.SourceRef == source {
			delete(m.records, key)
			m.mintLocked(state.EventDelete, key, state.SignalRecord{})
		}
	}
	delete(m.cooldown, source)
	return nil
}

var _ state.SignalStore = (*memSignalStore)(nil)

// TestMemSignalStorePassesConformanceSuite proves the signal scenarios are
// satisfiable and do not contradict each other.
func TestMemSignalStorePassesConformanceSuite(t *testing.T) {
	RunSignals(t, newMemSignalStore)
}
