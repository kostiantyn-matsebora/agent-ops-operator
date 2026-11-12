package commandbus

import "time"

// QueueStats is one (queue, kind) pair's snapshot (design: "per-queue
// statistics").
type QueueStats struct {
	Queue Queue
	Kind  Kind

	// Pending counts tasks Queued and eligible now (NotBefore <= now).
	Pending int
	// Scheduled counts tasks Queued or Retrying with NotBefore in the
	// future — backoff or a delayed start, not yet claimable.
	Scheduled int
	Running   int
	Dead      int
	// Parked counts Dead tasks still within their park cool-off (a subset
	// of Dead: a parked id still suppresses a resubmit, a released one
	// does not).
	Parked int

	// OldestPendingAge is how long the oldest Pending task has waited, zero
	// when Pending is zero.
	OldestPendingAge time.Duration
	// OldestPendingID and OldestPendingKey identify the task
	// OldestPendingAge measures — a count alone cannot tell an operator
	// WHICH op or WHICH conversation is stuck. Added for
	// async-command-bus task 6 (internal/chat.OpQueue.Stats, which keyed
	// its own old in-memory QueueStat on exactly these two identities —
	// an op id and a conversation name — and has no other way to recover
	// them from the bus). Zero value (both empty) when Pending is zero.
	OldestPendingID  CommandID
	OldestPendingKey Key

	// OldestRunningAge, OldestRunningID and OldestRunningKey mirror the
	// three fields above for the oldest Running task — "claimed and never
	// completed" in OpQueue's own vocabulary, the other half of the same
	// task 6 need. Zero value when Running is zero.
	OldestRunningAge time.Duration
	OldestRunningID  CommandID
	OldestRunningKey Key
}
