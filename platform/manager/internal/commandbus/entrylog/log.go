package entrylog

import (
	"sync"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

type keyRef struct {
	queue commandbus.Queue
	key   commandbus.Key
}

type cmdRef struct {
	queue commandbus.Queue
	id    commandbus.CommandID
}

// record is one task's entries: its message and every entry appended for it
// since. Only Apply writes it, and only by appending.
type record struct {
	msg    Entry
	seq    uint64
	events []Entry
	// seqs is every sequence the task's entries hold, message first: what a
	// backend removes from the log once the task is consumed.
	seqs []uint64
	// done is set once an entry made the task terminal: it never blocks its
	// key again and is never claimable.
	done bool
}

// Log is the in-memory index of the entries applied so far. It is a read
// model of the log, never a second source of truth: Apply is its only writer
// and it holds nothing the entries do not say.
type Log struct {
	mu        sync.RWMutex
	inventory []commandbus.Declaration

	records map[string]*record
	// live holds the records not yet terminal by entry, per queue, and
	// byKey the same per ordering key; both in append order. They are what
	// claim-time ordering reads, so a long history of finished tasks costs
	// nothing.
	live  map[commandbus.Queue][]*record
	byKey map[keyRef][]*record
	byCmd map[cmdRef][]*record
	// lastMsg and lastEv hold, per ordering key, the sequences the key's message
	// and event subjects still hold, ascending. Their tails are what an append
	// is conditional on: the server's own last sequence for the subject.
	lastMsg map[keyRef][]uint64
	lastEv  map[keyRef][]uint64
	lastSeq uint64

	// A consumed (succeeded) task is no longer in the log's read model: its
	// entries are removed from the log, so nothing of it is left to fold.
	// settled remembers the latest success per command id for the queue's dedup
	// window, so a resubmit inside it is still suppressed; consumed lists the
	// sequences awaiting that removal.
	settled  map[settledRef]commandbus.Task
	consumed []consumedTask
}

type settledRef struct {
	queue commandbus.Queue
	kind  commandbus.Kind
	id    commandbus.CommandID
}

type consumedTask struct {
	key  keyRef
	seqs []uint64
	at   time.Time
}

// New returns an empty Log resolving policy against inventory.
func New(inventory []commandbus.Declaration) *Log {
	return &Log{
		inventory: inventory,
		records:   map[string]*record{},
		live:      map[commandbus.Queue][]*record{},
		byKey:     map[keyRef][]*record{},
		byCmd:     map[cmdRef][]*record{},
		lastMsg:   map[keyRef][]uint64{},
		lastEv:    map[keyRef][]uint64{},
		settled:   map[settledRef]commandbus.Task{},
	}
}

// LastSeq is the highest sequence applied.
func (l *Log) LastSeq() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastSeq
}

// Apply adds the entry the log gave sequence seq. A sequence at or below the
// last applied one is ignored, so a replayed watch cannot double an entry.
// An entry naming a task whose message is gone (aged out by the log's own
// limit) is kept out of the index; its sequence still counts.
func (l *Log) Apply(seq uint64, e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if seq <= l.lastSeq {
		return
	}
	l.lastSeq = seq
	k := keyRef{e.Queue, e.Key}
	if e.Type == TypeMsg {
		l.lastMsg[k] = append(l.lastMsg[k], seq)
		rec := &record{msg: e, seq: seq, seqs: []uint64{seq}}
		l.records[e.UID] = rec
		l.live[e.Queue] = append(l.live[e.Queue], rec)
		l.byKey[k] = append(l.byKey[k], rec)
		c := cmdRef{e.Queue, e.CommandID}
		l.byCmd[c] = append(l.byCmd[c], rec)
		return
	}
	l.lastEv[k] = append(l.lastEv[k], seq)
	rec := l.records[e.UID]
	if rec == nil {
		return
	}
	rec.events = append(rec.events, e)
	rec.seqs = append(rec.seqs, seq)
	switch {
	case e.Type == TypeSucceeded:
		l.consumeLocked(rec, e.At)
	case e.Type == TypeFailed && e.Dead:
		l.finishLocked(rec)
	}
}

// consumeLocked takes a succeeded task out of the read model, leaving its
// dedup mark and its sequences for removal.
func (l *Log) consumeLocked(rec *record, at time.Time) {
	t := l.fold(rec, at)
	l.finishLocked(rec)
	delete(l.records, rec.msg.UID)
	c := cmdRef{rec.msg.Queue, rec.msg.CommandID}
	l.byCmd[c] = without(l.byCmd[c], rec)
	if len(l.byCmd[c]) == 0 {
		delete(l.byCmd, c)
	}
	l.settled[settledRef{rec.msg.Queue, rec.msg.Kind, rec.msg.CommandID}] = t
	l.consumed = append(l.consumed, consumedTask{key: keyRef{rec.msg.Queue, rec.msg.Key}, seqs: rec.seqs, at: at})
}

func tail(s []uint64) uint64 {
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

// dropSeqs removes the given sequences from a subject's list.
func dropSeqs(list, drop []uint64) []uint64 {
	gone := make(map[uint64]bool, len(drop))
	for _, s := range drop {
		gone[s] = true
	}
	kept := list[:0]
	for _, s := range list {
		if !gone[s] {
			kept = append(kept, s)
		}
	}
	return kept
}

// SetLast tells the log the server's last sequence on the subject e would be
// appended to is seq. The log's own belief can be wrong in either direction
// without it having been told: entries were removed (consumed, or aged out by
// the log's own limit), or a rival appended. The caller has applied everything
// up to seq first, so sequences the log holds above seq are ones the server no
// longer has and are forgotten, and seq itself becomes the tail the next plan is
// conditional on.
func (l *Log) SetLast(e Entry, seq uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := l.lastEv
	if e.Type == TypeMsg {
		m = l.lastMsg
	}
	k := keyRef{e.Queue, e.Key}
	list := m[k]
	for len(list) > 0 && list[len(list)-1] > seq {
		list = list[:len(list)-1]
	}
	if seq > 0 && tail(list) != seq {
		list = append(list, seq)
	}
	m[k] = list
}

// TakeConsumed returns, and forgets, the sequences of every task consumed at
// least grace before now: the entries a backend removes from its log. Removal
// is the only thing the bus ever deletes, and only of a task that succeeded.
func (l *Log) TakeConsumed(now time.Time, grace time.Duration) []uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []uint64
	kept := l.consumed[:0]
	for _, c := range l.consumed {
		if now.Sub(c.at) >= grace {
			out = append(out, c.seqs...)
			// The caller removes these from the log next, so the subjects'
			// last sequences stop counting them now.
			l.lastMsg[c.key] = dropSeqs(l.lastMsg[c.key], c.seqs)
			l.lastEv[c.key] = dropSeqs(l.lastEv[c.key], c.seqs)
			continue
		}
		kept = append(kept, c)
	}
	l.consumed = kept
	return out
}

func (l *Log) finishLocked(rec *record) {
	if rec.done {
		return
	}
	rec.done = true
	q, k := rec.msg.Queue, keyRef{rec.msg.Queue, rec.msg.Key}
	l.live[q] = without(l.live[q], rec)
	l.byKey[k] = without(l.byKey[k], rec)
	if len(l.byKey[k]) == 0 {
		delete(l.byKey, k)
	}
}

func without(s []*record, rec *record) []*record {
	for i, r := range s {
		if r == rec {
			return append(s[:i:i], s[i+1:]...)
		}
	}
	return s
}

// Prune forgets, in memory only, every task that settled before cutoff and no
// longer suppresses its command id. It is how the read model keeps pace with
// the log's own retention: the log's entries are removed by its limits, never
// by this call. It returns how many tasks were forgotten.
func (l *Log) Prune(now, cutoff time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for ref, t := range l.settled {
		if !t.Suppresses(now, l.window(ref.queue, ref.kind)) {
			delete(l.settled, ref)
		}
	}
	for uid, rec := range l.records {
		t := l.fold(rec, now)
		if !t.State.Settled() || !terminalAt(t).Before(cutoff) {
			continue
		}
		if t.Suppresses(now, l.window(t.Queue, t.Kind)) {
			continue
		}
		l.finishLocked(rec)
		delete(l.records, uid)
		c := cmdRef{rec.msg.Queue, rec.msg.CommandID}
		l.byCmd[c] = without(l.byCmd[c], rec)
		if len(l.byCmd[c]) == 0 {
			delete(l.byCmd, c)
		}
		n++
	}
	return n
}

func terminalAt(t commandbus.Task) time.Time {
	switch t.State {
	case commandbus.StateSucceeded:
		return t.SucceededAt
	case commandbus.StateDead:
		return t.DeadAt
	default:
		return t.UpdatedAt
	}
}

func (l *Log) window(q commandbus.Queue, k commandbus.Kind) time.Duration {
	d, _ := commandbus.Resolve(l.inventory, q, k)
	return d.Policy.DedupWindow
}

// fold derives a task from its own entries as of now, the last one deciding.
// A lease that has passed needs no entry: the task reads as retryable (or
// dead, once its attempts are spent), and a task past its TTL reads as
// expired. Both are functions of the entries and the clock, never writes.
func (l *Log) fold(rec *record, now time.Time) commandbus.Task {
	m := rec.msg
	t := commandbus.Task{
		UID: m.UID, CommandID: m.CommandID, Queue: m.Queue, Kind: m.Kind, Key: m.Key,
		Payload: m.Payload, InvocationID: m.InvocationID,
		State: commandbus.StateQueued, MaxAttempts: m.MaxAttempts,
		SubmittedAt: m.At, UpdatedAt: m.At,
	}
	for _, e := range rec.events {
		foldEntry(&t, e)
	}
	decl, _ := commandbus.Resolve(l.inventory, m.Queue, m.Kind)
	if t.State == commandbus.StateRunning && t.Lease != nil && now.After(t.Lease.Until) {
		lapse(&t, decl.Policy)
	}
	if ttl := decl.Policy.TTL; ttl > 0 && (t.State == commandbus.StateQueued || t.State == commandbus.StateRetrying) {
		if deadline := t.SubmittedAt.Add(ttl); now.After(deadline) {
			t.State, t.UpdatedAt = commandbus.StateExpired, deadline
		}
	}
	return t
}

func foldEntry(t *commandbus.Task, e Entry) {
	switch e.Type {
	case TypeClaimed:
		if t.State == commandbus.StateRunning {
			// The earlier lease never ended in an entry: it lapsed.
			t.Code, t.Detail = commandbus.CodeUnavailable, leaseLapsed
		}
		t.State, t.Attempt, t.NotBefore, t.UpdatedAt = commandbus.StateRunning, e.Attempt, time.Time{}, e.At
		t.Lease = &commandbus.Lease{Token: e.Token, Holder: e.Holder, Until: e.Until}
	case TypeFailed:
		t.Code, t.Detail, t.Lease, t.UpdatedAt = e.Code, e.Detail, nil, e.At
		if e.Dead {
			t.State, t.DeadAt, t.ParkedUntil = commandbus.StateDead, e.At, e.ParkedUntil
			return
		}
		t.State, t.NotBefore = commandbus.StateRetrying, e.NotBefore
	case TypeSucceeded:
		t.State, t.SucceededAt, t.Lease, t.UpdatedAt = commandbus.StateSucceeded, e.At, nil, e.At
	}
}

const leaseLapsed = "lease lapsed"

// lapse reads a running task whose lease has passed as a failed attempt with
// code unavailable. It is claimable at once: the lease was already the wait.
func lapse(t *commandbus.Task, p commandbus.Policy) {
	until := t.Lease.Until
	t.Code, t.Detail, t.Lease, t.UpdatedAt = commandbus.CodeUnavailable, leaseLapsed, nil, until
	// A hand-over queue never redelivers: a message with no report by its
	// delivery TTL is failed, and failed is final (design D-17).
	if t.Attempt < t.MaxAttempts && p.DeliveryTTL <= 0 {
		t.State, t.NotBefore = commandbus.StateRetrying, until
		return
	}
	t.State, t.DeadAt, t.ParkedUntil = commandbus.StateDead, until, until.Add(p.ParkCoolOff)
}

// Task reads one task by uid as of now.
func (l *Log) Task(uid string, now time.Time) (commandbus.Task, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rec := l.records[uid]
	if rec == nil {
		return commandbus.Task{}, false
	}
	return l.fold(rec, now), true
}
