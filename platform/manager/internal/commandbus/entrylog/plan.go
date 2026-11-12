package entrylog

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	mathrand "math/rand"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// MaxClaimCandidates bounds how many eligible tasks PlanClaim offers in one
// pass. A stale view can lose the race for the first, never for all of them
// at once, so a handful is enough and the rest wait for the next pass.
const MaxClaimCandidates = 16

// Submission is what PlanSubmit decides: either the task already suppressing
// the command id, or the message entry to append.
type Submission struct {
	// Existing is set when the submit is suppressed (design D-4).
	Existing *commandbus.Task
	// Append is the message entry when Existing is nil.
	Append Append
	// Task is the task as it reads once Append lands.
	Task commandbus.Task
}

// PlanSubmit validates req and decides it against the entries applied so far.
// The task's identity is minted here, before the append, and carries no order.
func (l *Log) PlanSubmit(req commandbus.SubmitRequest, now time.Time) (Submission, error) {
	if req.CommandID == "" || req.Key == "" {
		return Submission{}, fmt.Errorf("%w: CommandID and Key are both required", commandbus.ErrInvalidRequest)
	}
	decl, ok := commandbus.Resolve(l.inventory, req.Queue, req.Kind)
	if !ok {
		return Submission{}, fmt.Errorf("%w: queue=%q kind=%q", commandbus.ErrUnknownQueue, req.Queue, req.Kind)
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	recs := l.byCmd[cmdRef{req.Queue, req.CommandID}]
	if t, ok := l.settled[settledRef{req.Queue, req.Kind, req.CommandID}]; ok && t.Suppresses(now, decl.Policy.DedupWindow) {
		// A consumed task has no entries left to fold; its mark holds its id for
		// the dedup window, unless a newer task of the id has come since.
		if len(recs) == 0 || !recs[len(recs)-1].msg.At.After(t.SubmittedAt) {
			return Submission{Existing: &t}, nil
		}
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].msg.Kind != req.Kind {
			continue
		}
		if t := l.fold(recs[i], now); t.Suppresses(now, decl.Policy.DedupWindow) {
			return Submission{Existing: &t}, nil
		}
		break // only the latest task of the id can suppress it
	}
	msg := Entry{
		Type: TypeMsg, UID: NewUID(), CommandID: req.CommandID, Queue: req.Queue, Kind: req.Kind, Key: req.Key,
		InvocationID: req.InvocationID, At: now, Payload: req.Payload, MaxAttempts: decl.Policy.Attempts,
	}
	rec := &record{msg: msg}
	return Submission{
		Append: Append{Entry: msg, ExpectLast: tail(l.lastMsg[keyRef{req.Queue, req.Key}])},
		Task:   l.fold(rec, now),
	}, nil
}

// Claim is one offered claim: the entry to append and the task as it reads
// once it lands.
type Claim struct {
	Append Append
	Task   commandbus.Task
}

// PlanClaim offers the oldest eligible tasks of queue, oldest first. A task is
// eligible when it is queued, or retrying with its backoff elapsed, no earlier
// task of its key is unsettled, and its invocation is under its in-flight
// cap. The first that lands wins; the append is conditional on the key's last
// entry, so of two racing claims only one does.
func (l *Log) PlanClaim(queue commandbus.Queue, holder string, now time.Time, lease time.Duration) []Claim {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []Claim
	for _, c := range l.eligible(queue, now, MaxClaimCandidates) {
		out = append(out, l.planClaim(c.rec, c.t, holder, now, lease))
	}
	return out
}

// candidate is an eligible task and its record.
type candidate struct {
	rec *record
	t   commandbus.Task
}

// eligible lists up to limit tasks of queue that may be handed out now, oldest
// first. The caller holds the read lock.
func (l *Log) eligible(queue commandbus.Queue, now time.Time, limit int) []candidate {
	live := l.live[queue]
	folded := make(map[*record]commandbus.Task, len(live))
	running := map[string]int{}
	for _, rec := range live {
		t := l.fold(rec, now)
		folded[rec] = t
		if t.State == commandbus.StateRunning && t.InvocationID != "" {
			running[t.InvocationID]++
		}
	}
	var out []candidate
	for _, rec := range live {
		t := folded[rec]
		if !claimable(t, now) || l.blockedByKey(rec, folded) || l.atCap(t, running) {
			continue
		}
		out = append(out, candidate{rec, t})
		if len(out) == limit {
			break
		}
	}
	return out
}

// Offered lists up to limit tasks of queue a consumer may be handed next,
// oldest first, as they read now: a read, never an entry.
func (l *Log) Offered(queue commandbus.Queue, limit int, now time.Time) []commandbus.Task {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []commandbus.Task
	for _, c := range l.eligible(queue, now, limit) {
		out = append(out, c.t)
	}
	return out
}

// PlanAcknowledge decides the hand-over of the message of command id on queue
// to holder. Exactly one of the results is meaningful: Existing when the message
// is already acknowledged and its lease holds (idempotent), a Claim to append
// otherwise. ErrNotFound when the queue holds no such message, ErrNotOffered when
// it is not currently offered, ErrInvalidRequest on a queue with no DeliveryTTL.
func (l *Log) PlanAcknowledge(queue commandbus.Queue, id commandbus.CommandID, holder string, now time.Time) (Claim, *commandbus.Task, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	recs := l.byCmd[cmdRef{queue, id}]
	if len(recs) == 0 {
		return Claim{}, nil, commandbus.ErrNotFound
	}
	rec := recs[len(recs)-1]
	decl, ok := commandbus.Resolve(l.inventory, rec.msg.Queue, rec.msg.Kind)
	if !ok || decl.Policy.DeliveryTTL <= 0 {
		return Claim{}, nil, fmt.Errorf("%w: queue %q kind %q is not a hand-over queue", commandbus.ErrInvalidRequest, queue, rec.msg.Kind)
	}
	t := l.fold(rec, now)
	if t.State == commandbus.StateRunning && t.Lease != nil {
		return Claim{}, &t, nil
	}
	for _, c := range l.eligible(queue, now, int(^uint(0)>>1)) {
		if c.rec == rec {
			return l.planClaim(rec, t, holder, now, 0), nil, nil
		}
	}
	return Claim{}, nil, fmt.Errorf("%w: %q on %q is %s", commandbus.ErrNotOffered, id, queue, t.State)
}

func claimable(t commandbus.Task, now time.Time) bool {
	switch t.State {
	case commandbus.StateQueued:
		return true
	case commandbus.StateRetrying:
		return !t.NotBefore.After(now)
	default:
		return false
	}
}

func (l *Log) blockedByKey(cand *record, folded map[*record]commandbus.Task) bool {
	for _, rec := range l.byKey[keyRef{cand.msg.Queue, cand.msg.Key}] {
		if rec == cand {
			return false
		}
		if t, ok := folded[rec]; ok && !t.State.Settled() {
			return true
		}
	}
	return false
}

func (l *Log) atCap(t commandbus.Task, running map[string]int) bool {
	if t.InvocationID == "" {
		return false
	}
	decl, ok := commandbus.Resolve(l.inventory, t.Queue, t.Kind)
	return ok && decl.Policy.InFlightCapPerInvocation > 0 && running[t.InvocationID] >= decl.Policy.InFlightCapPerInvocation
}

func (l *Log) planClaim(rec *record, t commandbus.Task, holder string, now time.Time, lease time.Duration) Claim {
	if decl, ok := commandbus.Resolve(l.inventory, rec.msg.Queue, rec.msg.Kind); ok && decl.Policy.DeliveryTTL > 0 {
		lease = decl.Policy.DeliveryTTL // a claim here is an acknowledgement (design D-17)
	}
	e := l.descriptor(rec, TypeClaimed, now)
	e.Attempt, e.Token, e.Holder, e.Until = t.Attempt+1, NewLeaseToken(), holder, now.Add(lease)
	claimed := t
	foldEntry(&claimed, e)
	return Claim{Append: Append{Entry: e, ExpectLast: tail(l.lastEv[keyRef{e.Queue, e.Key}])}, Task: claimed}
}

// descriptor starts an entry of kind typ for rec's task, carrying the
// descriptors every entry repeats.
func (l *Log) descriptor(rec *record, typ Type, now time.Time) Entry {
	m := rec.msg
	return Entry{Type: typ, UID: m.UID, CommandID: m.CommandID, Queue: m.Queue, Kind: m.Kind, Key: m.Key, InvocationID: m.InvocationID, At: now}
}

// PlanSettle decides a success. A token that is not the task's live lease,
// including a lapsed one and a task already settled, is ErrLeaseLost and plans
// nothing: a stale completion appends no entry.
func (l *Log) PlanSettle(uid, token string, now time.Time) (Append, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rec, t, err := l.leased(uid, token, now)
	if err != nil {
		return Append{}, err
	}
	e := l.descriptor(rec, TypeSucceeded, now)
	e.Attempt = t.Attempt
	return Append{Entry: e, ExpectLast: tail(l.lastEv[keyRef{e.Queue, e.Key}])}, nil
}

// Failure is what PlanFail decides: the entry and the task as it reads once
// the entry lands.
type Failure struct {
	Append Append
	Task   commandbus.Task
}

// PlanFail decides one failed attempt. Retryability is derived from code,
// never chosen; exhausting the queue's attempts, or a non-retryable code, ends
// the task dead instead of retrying. The token is checked as PlanSettle checks
// it.
func (l *Log) PlanFail(uid, token string, code commandbus.ErrorCode, detail string, now time.Time) (Failure, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rec, t, err := l.leased(uid, token, now)
	if err != nil {
		return Failure{}, err
	}
	return l.failure(rec, t, commandbus.ParseErrorCode(string(code)), detail, now), nil
}

func (l *Log) failure(rec *record, t commandbus.Task, code commandbus.ErrorCode, detail string, now time.Time) Failure {
	decl, _ := commandbus.Resolve(l.inventory, t.Queue, t.Kind)
	e := l.descriptor(rec, TypeFailed, now)
	e.Attempt, e.Code, e.Detail = t.Attempt, code, detail
	if code.Retryable() && t.Attempt < t.MaxAttempts && decl.Policy.DeliveryTTL <= 0 {
		e.NotBefore = now.Add(Backoff(decl.Policy.Backoff, t.Attempt))
	} else {
		e.Dead, e.ParkedUntil = true, now.Add(decl.Policy.ParkCoolOff)
	}
	after := t
	foldEntry(&after, e)
	return Failure{Append: Append{Entry: e, ExpectLast: tail(l.lastEv[keyRef{e.Queue, e.Key}])}, Task: after}
}

func (l *Log) leased(uid, token string, now time.Time) (*record, commandbus.Task, error) {
	rec := l.records[uid]
	if rec == nil {
		return nil, commandbus.Task{}, commandbus.ErrNotFound
	}
	t := l.fold(rec, now)
	if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != token {
		return nil, commandbus.Task{}, commandbus.ErrLeaseLost
	}
	return rec, t, nil
}

// PlanLapsedDeaths offers a dead entry for every task whose lease lapsed with
// its attempts spent. The fold already reads such a task as dead; the entry
// only makes that a fact of the log, so it reaches the dead log and stops
// being re-derived. Appended by a sweep, never needed to read the state.
func (l *Log) PlanLapsedDeaths(now time.Time) []Failure {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []Failure
	for _, rec := range l.records {
		if rec.done {
			continue
		}
		t := l.fold(rec, now)
		if t.State != commandbus.StateDead {
			continue
		}
		// fold marked it dead from a lapse, not from an entry, so the running
		// state it lapsed from is what the failure is planned against.
		running := l.foldEntries(rec)
		out = append(out, l.failure(rec, running, commandbus.CodeUnavailable, leaseLapsed, now))
	}
	return out
}

// foldEntries is the task as its entries alone read, with no clock applied.
func (l *Log) foldEntries(rec *record) commandbus.Task {
	m := rec.msg
	t := commandbus.Task{State: commandbus.StateQueued, MaxAttempts: m.MaxAttempts, Queue: m.Queue, Kind: m.Kind}
	for _, e := range rec.events {
		foldEntry(&t, e)
	}
	return t
}

// Backoff is the wait before attempt's retry: Min doubling to Max, jittered
// when the policy says so.
func Backoff(b commandbus.Backoff, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := b.Min
	for i := 1; i < attempt && d < b.Max; i++ {
		d *= 2
	}
	d = max(min(d, b.Max), b.Min)
	if b.Jitter && d > 0 {
		return time.Duration(mathrand.Int63n(int64(d))) + 1
	}
	return d
}

// NewUID mints a task's identity before its message is appended.
func NewUID() string { return randomHex(12) }

// NewLeaseToken mints the credential Settle and Fail check.
func NewLeaseToken() string { return randomHex(16) }

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		panic(fmt.Errorf("commandbus/entrylog: reading random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}
