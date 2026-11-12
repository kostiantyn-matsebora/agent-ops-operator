package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// defaultPollInterval bounds how long Claim ever sleeps with nothing
// scheduled before rechecking — a safety net behind the clock's own wake
// channel (bus.go), not the normal wake path, mirroring commandbus/memory's
// own constant of the same name and purpose.
const defaultPollInterval = 200 * time.Millisecond

// Claim implements commandbus.Bus (design D-2). Every decision — which task is
// next, whether an earlier sibling blocks it, whether an invocation's cap is
// full — reads the mirror, never the server; only the conditional append that
// actually claims a task reaches it, and only for the candidate chosen.
func (b *Bus) Claim(ctx context.Context, queue commandbus.Queue, consumerID string) (commandbus.Task, error) {
	if err := b.mirror.waitReady(ctx); err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: claim: %w", err)
	}
	for {
		now := b.clock.now()
		claimed, ok, err := b.tryClaim(ctx, queue, consumerID, now)
		if err != nil {
			return commandbus.Task{}, err
		}
		if ok {
			return claimed, nil
		}

		wait := b.mirror.log.NextWake(queue, now, defaultPollInterval)
		waitCh := b.clock.wait()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return commandbus.Task{}, fmt.Errorf("commandbus/nats: claim on %q canceled: %w", queue, ctx.Err())
		case <-waitCh:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// tryClaim is one non-blocking pass: offer the oldest eligible, unblocked
// candidates and append a claim for the first whose conditional append lands.
func (b *Bus) tryClaim(ctx context.Context, queue commandbus.Queue, consumerID string, now time.Time) (commandbus.Task, bool, error) {
	// Fence on this Bus value's own last append before trusting the mirror for
	// the blocking checks — the watch that feeds it is asynchronous even
	// against this same process's own prior writes, and two Claim calls made
	// back to back (the in-flight-cap test's own shape) must see the first's
	// effect on the second.
	if err := b.mirror.waitCaughtUpTo(ctx, b.selfSeq.Load()); err != nil {
		return commandbus.Task{}, false, fmt.Errorf("commandbus/nats: claim: %w", err)
	}
	for _, c := range b.mirror.log.PlanClaim(queue, consumerID, now, b.leaseDuration) {
		if _, err := b.append(ctx, c.Append); err != nil {
			if isWrongLastSeq(err) {
				// A stale view only delays a claim (design D-2): another claim
				// won this key. Try the next candidate; a later pass sees
				// whatever actually won.
				continue
			}
			return commandbus.Task{}, false, fmt.Errorf("commandbus/nats: claim: %w", err)
		}
		return c.Task, true, nil
	}
	return commandbus.Task{}, false, nil
}
