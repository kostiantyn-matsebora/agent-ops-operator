package commandbus

import "time"

// TaskState is one node of the state machine design D-4 draws. There are no
// others — "parked" is not a seventh state, it is the Dead task before
// ParkedUntil (see Task.Parked), because the diagram itself names six nodes
// and a park is a cool-off on the dead node, not a place a task visits.
type TaskState string

const (
	StateQueued    TaskState = "queued"
	StateRunning   TaskState = "running"
	StateSucceeded TaskState = "succeeded"
	StateRetrying  TaskState = "retrying"
	StateDead      TaskState = "dead"
	StateExpired   TaskState = "expired"
)

// Settled reports whether s is terminal for ordering purposes: a task past
// it never blocks the next task sharing its Key (design D-4 — "expired
// counts as settled for ordering, so a task that never ran cannot block its
// key"). Retrying is NOT settled: the task still owns its Key until it
// either succeeds or goes dead.
func (s TaskState) Settled() bool {
	switch s {
	case StateSucceeded, StateDead, StateExpired:
		return true
	default:
		return false
	}
}

// Lease is held by whichever consumer currently owns a running task's next
// move (design D-2): a token and the instant it lapses. Token, not identity,
// is the credential Settle and Fail check, which is what lets a claim made
// on one replica complete on another.
type Lease struct {
	Token string
	// Holder is the consumerID Claim was given, recorded for a human reading
	// the task back — never consulted by Settle or Fail.
	Holder string
	Until  time.Time
}

// Task is one ledger record, read back from Submit, Claim, GetTask and
// DeadLetters. A single Task persists across every attempt of one
// submission's life — Attempt grows and Lease changes underneath it — and a
// NEW Task (a new UID, the same CommandID) is created only when a fresh
// Submit finds the old one no longer suppressing (succeeded past its window,
// dead past its cool-off, or expired).
type Task struct {
	UID       string
	CommandID CommandID
	Queue     Queue
	Kind      Kind
	Key       Key
	Payload   []byte
	// InvocationID is the batch tag Submit was given, or "".
	InvocationID string

	State TaskState
	// Attempt counts claims that reached Running, starting at 0 before the
	// first. It grows across Retrying, never resets.
	Attempt int
	// MaxAttempts is resolved from the queue's declared policy at Submit
	// time and frozen on the task — a later chart change narrowing a
	// queue's attempts must not retroactively dead-letter work already in
	// flight under the old number.
	MaxAttempts int

	// Code and Detail are the last Fail's own code and message. Zero value
	// before any failure.
	Code   ErrorCode
	Detail string

	// NotBefore is the earliest instant this task becomes claimable again.
	// Zero means "now" (true of a freshly queued task and of Running, which
	// Claim never offers regardless).
	NotBefore time.Time

	// Lease is non-nil exactly while State is Running.
	Lease *Lease

	// Order is this task's own position in its queue's FIFO, assigned once
	// at Submit and never touched again — the fact that lets a retry keep
	// its place rather than re-entering at the back (design D-14, 2.6).
	Order uint64

	SubmittedAt time.Time
	UpdatedAt   time.Time
	// SucceededAt is set on the Settle that reached StateSucceeded.
	SucceededAt time.Time
	// DeadAt is set on the Fail that reached StateDead.
	DeadAt time.Time
	// ParkedUntil is valid only while State is StateDead: the CommandID
	// keeps suppressing a fresh Submit until this instant (design D-4). A
	// zero value with State Dead means the queue's ParkCoolOff is zero —
	// D-5's "never parked" — so the id was never suppressed to begin with.
	ParkedUntil time.Time
}

// Parked reports whether t is a dead task still within its cool-off, as of
// now.
func (t Task) Parked(now time.Time) bool {
	return t.State == StateDead && now.Before(t.ParkedUntil)
}

// Suppresses reports whether t is the reason a fresh Submit under the same
// CommandID is refused, as of now (design D-4): queued, running or retrying
// unconditionally; succeeded within its dedup window; dead within its
// park cool-off. Expired, and succeeded or dead past their window, do not
// suppress.
func (t Task) Suppresses(now time.Time, dedupWindow time.Duration) bool {
	switch t.State {
	case StateQueued, StateRunning, StateRetrying:
		return true
	case StateSucceeded:
		return now.Before(t.SucceededAt.Add(dedupWindow))
	case StateDead:
		return t.Parked(now)
	default: // StateExpired
		return false
	}
}
