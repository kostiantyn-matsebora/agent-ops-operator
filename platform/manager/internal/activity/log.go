package activity

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// DefaultSize is the ring capacity when none is configured. Roughly a quarter
// hour of a busy namespace; the window is a value because the right number is a
// per-cluster measurement, not a constant anyone can derive.
const DefaultSize = 10000

// subscriberBuffer bounds each streaming subscriber. Beyond it the subscriber
// is marked lagged and told to resync — the emitter is never made to wait for a
// browser.
const subscriberBuffer = 256

// Observer sees every emitted event. It exists so that telemetry has exactly
// ONE instrumentation pass: the Prometheus registry is an observer, so a metric
// observation and its event cannot occur independently or drift apart.
//
// An Observer MUST NOT block — it runs on the emitting goroutine, which is a
// dispatch or reconcile path.
type Observer interface {
	Observe(Event)
}

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(Event)

// Observe implements Observer.
func (f ObserverFunc) Observe(e Event) { f(e) }

type subscriber struct {
	ch     chan Event
	lagged bool
}

// Log is the bounded activity ring with a subscriber fan-out.
//
// A nil *Log is usable and ignores every emission, so call sites need no nil
// checks and telemetry can be switched off without threading a flag through
// dispatch.
type Log struct {
	mu   sync.Mutex
	buf  []Event // len == capacity; index = (seq-1) % capacity
	next uint64  // sequence of the NEXT event (1-based)

	subs   map[uint64]*subscriber
	subSeq uint64

	observers []Observer
}

// New builds a log with the given capacity (<=0 uses DefaultSize).
func New(size int) *Log {
	if size <= 0 {
		size = DefaultSize
	}
	return &Log{buf: make([]Event, size), next: 1, subs: map[uint64]*subscriber{}}
}

// AddObserver registers a fan-out target. Call before serving; observers are
// not removable, because the one caller is process wiring.
func (l *Log) AddObserver(o Observer) {
	if l == nil || o == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.observers = append(l.observers, o)
}

// Cursor renders a sequence number as a cursor: zero-padded so lexicographic
// comparison is numeric comparison, which keeps clients from having to parse.
func Cursor(seq uint64) string {
	return fmt.Sprintf("%016d", seq)
}

// parseCursor reads a cursor back. An unparseable cursor is treated as absent.
func parseCursor(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Emit records one event. It NEVER blocks: the ring overwrites its oldest
// entry, and a subscriber that cannot keep up is marked lagged rather than
// waited on.
//
// Cursor, TS and Status are normalized here so every emission site can stay a
// literal.
func (l *Log) Emit(e Event) {
	if l == nil {
		return
	}
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	if e.Status == "" {
		e.Status = StatusOK
	}

	l.mu.Lock()
	seq := l.next
	l.next++
	e.Cursor = Cursor(seq)
	l.buf[(seq-1)%uint64(len(l.buf))] = e
	subs := make([]*subscriber, 0, len(l.subs))
	for _, s := range l.subs {
		subs = append(subs, s)
	}
	observers := l.observers
	l.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- e:
		default:
			// Full: the subscriber is behind. Marking it lagged turns a silent
			// gap into an explicit resync, which is the whole difference
			// between a console that is stale and one that is wrong.
			l.mu.Lock()
			s.lagged = true
			l.mu.Unlock()
		}
	}
	for _, o := range observers {
		o.Observe(e)
	}
}

// oldestSeq is the sequence of the oldest event still held (0 when empty).
func (l *Log) oldestSeq() uint64 {
	if l.next == 1 {
		return 0
	}
	capacity := uint64(len(l.buf))
	if l.next-1 <= capacity {
		return 1
	}
	return l.next - capacity
}

// Since returns events after a cursor, oldest first, at most limit of them.
//
// resync reports that the caller's cursor cannot be served, so the gap is real
// and it must re-read snapshots instead of assuming continuity. Two ways that
// happens, and the ring is only one of them:
//
//   - the cursor was EVICTED — the buffer wrapped past it;
//   - the cursor PREDATES this process — the manager restarted, and the ring is
//     in-memory, so it started counting again from 1.
//
// An empty cursor is a first connect, not a gap: it gets the tail of the buffer
// and no resync.
func (l *Log) Since(cursor string, limit int) (events []Event, resync bool) {
	if l == nil {
		return nil, false
	}
	if limit <= 0 || limit > len(l.buf) {
		limit = len(l.buf)
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	seq, hasCursor := parseCursor(cursor)
	// A cursor this process never issued. The sequence space restarts at 1 on
	// every start, so a cursor from the PREVIOUS process reads as "ahead of us"
	// — and answering it with an empty list would be a viewer's evidence that
	// nothing happened, when what actually happened is that the history holding
	// it is gone. This is the restart case; eviction is handled below.
	if hasCursor && seq >= l.next {
		return nil, true
	}
	oldest := l.oldestSeq()
	if oldest == 0 {
		return nil, false
	}
	from := oldest
	if hasCursor {
		if seq+1 < oldest {
			resync = true
		} else {
			from = seq + 1
		}
		if from >= l.next {
			return nil, resync
		}
	} else {
		// First connect: hand back the newest `limit` events rather than the
		// oldest, so a fresh console starts current.
		if l.next-from > uint64(limit) {
			from = l.next - uint64(limit)
		}
	}

	capacity := uint64(len(l.buf))
	for seq := from; seq < l.next && len(events) < limit; seq++ {
		events = append(events, l.buf[(seq-1)%capacity])
	}
	return events, resync
}

// Latest returns the cursor of the most recent event ("" when empty).
func (l *Log) Latest() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.next == 1 {
		return ""
	}
	return Cursor(l.next - 1)
}

// Subscription is a live feed of events.
//
// The delivery channel is deliberately NEVER closed: Emit copies the subscriber
// set under the lock and then sends outside it, so closing on unsubscribe would
// race an in-flight send and panic the emitting goroutine — which is a dispatch
// path. Readers select on their own context instead, which is what an SSE
// handler does anyway.
type Subscription struct {
	log *Log
	id  uint64
	sub *subscriber
}

// Events is the delivery channel.
func (s *Subscription) Events() <-chan Event { return s.sub.ch }

// Lagged reports whether this subscriber dropped events because it could not
// keep up. A lagged subscriber must resync — the stream cannot fill the gap.
func (s *Subscription) Lagged() bool {
	if s == nil || s.log == nil {
		return false
	}
	s.log.mu.Lock()
	defer s.log.mu.Unlock()
	return s.sub.lagged
}

// Close removes the subscription. Safe to call more than once.
func (s *Subscription) Close() {
	if s == nil || s.log == nil {
		return
	}
	s.log.mu.Lock()
	delete(s.log.subs, s.id)
	s.log.mu.Unlock()
}

// Subscribe starts a live feed. Returns nil for a nil log, so a caller with
// telemetry disabled gets no stream rather than a panic.
func (l *Log) Subscribe() *Subscription {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subSeq++
	sub := &subscriber{ch: make(chan Event, subscriberBuffer)}
	l.subs[l.subSeq] = sub
	return &Subscription{log: l, id: l.subSeq, sub: sub}
}
