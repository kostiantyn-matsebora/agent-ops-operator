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
}
