package nats

import (
	"context"
	"fmt"
	mathrand "math/rand"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// appendAttempts bounds how many times one operation re-plans after a lost
// race before giving up. Every lost race means another writer landed, so the
// bound is only reached under sustained contention on one key.
const appendAttempts = 100

// raceBackoff is a short, growing, jittered pause after a lost race, so
// writers contending for one key do not re-plan in lockstep.
func raceBackoff(attempt int) time.Duration {
	return time.Duration(mathrand.Int63n(int64(time.Millisecond)*int64(min(attempt, 10)+1))) + 1
}

func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Submit implements commandbus.Bus (design D-4/D-9). It is ONE append: the
// message entry, conditional on the key's message subject still ending where
// the plan saw it. The dedup lookup reads the mirror after fencing on the
// server's last sequence — Submit is the one call D-9 requires cheap, not
// zero-network — and a submit that loses the race for its key's subject
// (another message landed, possibly the very command id being submitted)
// re-plans from a fresh view, so of several replicas submitting one command id
// exactly one creates a task and every caller gets its uid.
//
// Nothing is written after the append, so a consumer claiming the task the
// instant it is visible cannot fail the submit.
func (b *Bus) Submit(ctx context.Context, req commandbus.SubmitRequest) (commandbus.Task, error) {
	for attempt := 0; attempt < appendAttempts; attempt++ {
		if err := b.sync(ctx); err != nil {
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: %w", err)
		}
		sub, err := b.mirror.log.PlanSubmit(req, b.clock.now())
		if err != nil {
			return commandbus.Task{}, err
		}
		if sub.Existing != nil {
			return *sub.Existing, nil
		}
		_, err = b.append(ctx, sub.Append)
		if err == nil {
			return sub.Task, nil
		}
		if !isWrongLastSeq(err) {
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: append message: %w", err)
		}
		if err := pause(ctx, raceBackoff(attempt)); err != nil {
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit: %w", err)
		}
	}
	return commandbus.Task{}, fmt.Errorf("commandbus/nats: submit %q: still losing the race for key %q after %d attempts", req.CommandID, req.Key, appendAttempts)
}
