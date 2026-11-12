package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Stats implements commandbus.Bus, reading taskMirror exactly as Claim's
// candidate search does (task 3.3). It fences on the tasks bucket's own
// latest sequence first — the same technique internal/state/nats's own
// List uses against its mirror — so a write that lands on the server a
// moment before this call is never missed because the watch had not yet
// echoed it back.
//
// A Dead task the ledger retention sweep has already purged (sweep.go)
// stops counting here: the ledger is declared bounded and lossy
// telemetry (invariants.md), and DeadLetters — not Stats — is this
// backend's durable dead-letter accounting.
func (b *Bus) Stats(ctx context.Context, queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	if queue == "" {
		return commandbus.QueueStats{}, fmt.Errorf("%w: Stats needs a queue", commandbus.ErrInvalidRequest)
	}
	target, err := b.tasksLatestSeq(ctx)
	if err != nil {
		return commandbus.QueueStats{}, fmt.Errorf("commandbus/nats: stats: %w", err)
	}
	if err := b.mirror.waitCaughtUpTo(ctx, target); err != nil {
		return commandbus.QueueStats{}, fmt.Errorf("commandbus/nats: stats: %w", err)
	}

	now := b.clock.now()
	out := commandbus.QueueStats{Queue: queue, Kind: kind}
	var oldestPending, oldestRunning time.Time
	markPending := func(t commandbus.Task) {
		out.Pending++
		if oldestPending.IsZero() || t.SubmittedAt.Before(oldestPending) {
			oldestPending = t.SubmittedAt
			out.OldestPendingID, out.OldestPendingKey = t.CommandID, t.Key
		}
	}
	// UpdatedAt, not SubmittedAt: a Running task's age is how long it has
	// been CLAIMED (tryClaim stamps UpdatedAt at the moment of claim), the
	// same "claimed and never completed" clock internal/chat.OpQueue's own
	// old Op.ClaimedAt() kept — see commandbus/memory's identical twin.
	markRunning := func(t commandbus.Task) {
		out.Running++
		if oldestRunning.IsZero() || t.UpdatedAt.Before(oldestRunning) {
			oldestRunning = t.UpdatedAt
			out.OldestRunningID, out.OldestRunningKey = t.CommandID, t.Key
		}
	}
	for _, e := range b.mirror.snapshot() {
		t := e.Task
		if t.Queue != queue || t.Kind != kind {
			continue
		}
		tallyTask(&out, t, now, markPending, markRunning)
	}
	if !oldestPending.IsZero() {
		out.OldestPendingAge = now.Sub(oldestPending)
	}
	if !oldestRunning.IsZero() {
		out.OldestRunningAge = now.Sub(oldestRunning)
	}
	return out, nil
}

// tallyTask counts one task of the queue and kind Stats is reading into out.
func tallyTask(out *commandbus.QueueStats, t commandbus.Task, now time.Time, markPending, markRunning func(commandbus.Task)) {
	switch t.State {
	case commandbus.StateQueued:
		markPending(t)
	case commandbus.StateRetrying:
		if t.NotBefore.After(now) {
			out.Scheduled++
		} else {
			markPending(t)
		}
	case commandbus.StateRunning:
		markRunning(t)
	case commandbus.StateDead:
		out.Dead++
		if t.Parked(now) {
			out.Parked++
		}
	}
}

// tasksLatestSeq reads the "tasks" bucket's own last stream sequence
// directly from the server — never from the mirror, which is what Stats
// fences the mirror against above.
func (b *Bus) tasksLatestSeq(ctx context.Context) (uint64, error) {
	s, err := b.js.Stream(ctx, b.tasksStream)
	if err != nil {
		return 0, fmt.Errorf("stream info %q: %w", b.tasksStream, err)
	}
	return s.CachedInfo().State.LastSeq, nil
}
