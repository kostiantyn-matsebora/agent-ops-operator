package nats

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// defaultPollInterval bounds how long Claim ever sleeps with nothing
// scheduled before rechecking — a safety net behind the clock's own wake
// channel (bus.go), not the normal wake path, mirroring commandbus/memory's
// own constant of the same name and purpose.
const defaultPollInterval = 200 * time.Millisecond

// Claim implements commandbus.Bus (design D-2). Every decision — which
// task is next, whether an earlier sibling blocks it, whether an
// invocation's cap is full — reads taskMirror (mirror.go), never the
// server; only the CAS write that actually claims a task reaches it, and
// only for the one candidate chosen.
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

		wait := b.nextWake(queue, now)
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

// tryClaim is one non-blocking pass: reap this queue's own lapsed leases
// first (so a redelivery or a now-stale TTL is visible to the candidate
// scan below without waiting for the mirror's watch to echo our own
// write), then offer the oldest eligible, unblocked candidate.
func (b *Bus) tryClaim(ctx context.Context, queue commandbus.Queue, consumerID string, now time.Time) (commandbus.Task, bool, error) {
	// Fence on this Bus value's own last write before trusting the mirror
	// for the blocking checks below — the watch that feeds it is
	// asynchronous even against this same process's own prior writes, and
	// two Claim calls made back to back (the in-flight-cap test's own
	// shape) must see the first's effect on the second.
	if err := b.mirror.waitCaughtUpTo(ctx, b.selfSeq.Load()); err != nil {
		return commandbus.Task{}, false, fmt.Errorf("commandbus/nats: claim: %w", err)
	}

	effective, revisions, err := b.reapQueueLocally(ctx, queue, now)
	if err != nil {
		return commandbus.Task{}, false, err
	}

	var candidates []commandbus.Task
	for _, t := range effective {
		if t.Queue != queue {
			continue
		}
		if t.State == commandbus.StateQueued || (t.State == commandbus.StateRetrying && !t.NotBefore.After(now)) {
			candidates = append(candidates, t)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Order < candidates[j].Order })

	for _, t := range candidates {
		if blockedByKey(effective, queue, t) || blockedByInFlightCap(effective, queue, t, b.inventory) {
			continue
		}
		claimed := t
		claimed.State = commandbus.StateRunning
		claimed.Attempt++
		claimed.Lease = &commandbus.Lease{Token: newLeaseToken(), Holder: consumerID, Until: now.Add(b.leaseDuration)}
		claimed.UpdatedAt = now
		if _, err := b.casWriteTask(ctx, t.UID, revisions[t.UID], claimed); err != nil {
			if isWrongLastSeq(err) {
				// A stale view only delays a claim (design D-2) — try the
				// next candidate; a later pass sees whatever actually won.
				continue
			}
			return commandbus.Task{}, false, fmt.Errorf("commandbus/nats: claim: %w", err)
		}
		return claimed, true, nil
	}
	return commandbus.Task{}, false, nil
}

// reapQueueLocally returns a local view of every mirrored task, with
// queue's own Running-but-lapsed tasks already reaped (loadAndReap writes
// the reap through to the server and returns its authoritative result,
// which this function folds into the view it hands back — the local
// equivalent of commandbus/memory's reapLocked running under the same
// lock the candidate scan uses).
func (b *Bus) reapQueueLocally(ctx context.Context, queue commandbus.Queue, now time.Time) (map[string]commandbus.Task, map[string]uint64, error) {
	snap := b.mirror.snapshot()
	effective := make(map[string]commandbus.Task, len(snap))
	revisions := make(map[string]uint64, len(snap))
	for uid, e := range snap {
		effective[uid], revisions[uid] = e.Task, e.Revision
	}
	for uid, t := range effective {
		if t.Queue != queue || t.State != commandbus.StateRunning || t.Lease == nil || !now.After(t.Lease.Until) {
			continue
		}
		next, rev, ok, err := b.loadAndReap(ctx, uid, now)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			effective[uid], revisions[uid] = next, rev
		} else {
			delete(effective, uid)
		}
	}
	return effective, revisions, nil
}

// blockedByKey reports whether an earlier, still-unsettled task on queue
// shares candidate's Key (design D-2's claim-time ordering rule, enforced
// over the whole task set rather than by partitioning).
func blockedByKey(view map[string]commandbus.Task, queue commandbus.Queue, candidate commandbus.Task) bool {
	for _, t := range view {
		if t.Queue == queue && t.Key == candidate.Key && t.Order < candidate.Order && !t.State.Settled() {
			return true
		}
	}
	return false
}

// blockedByInFlightCap reports whether candidate's invocation already has
// as many tasks Running on queue as its declaration allows (design D-5).
func blockedByInFlightCap(view map[string]commandbus.Task, queue commandbus.Queue, candidate commandbus.Task, inv []commandbus.Declaration) bool {
	if candidate.InvocationID == "" {
		return false
	}
	decl, ok := commandbus.Resolve(inv, queue, candidate.Kind)
	if !ok || decl.Policy.InFlightCapPerInvocation <= 0 {
		return false
	}
	running := 0
	for _, t := range view {
		if t.Queue == queue && t.InvocationID == candidate.InvocationID && t.State == commandbus.StateRunning {
			running++
		}
	}
	return running >= decl.Policy.InFlightCapPerInvocation
}

// nextWake bounds how long Claim should sleep when nothing is eligible
// yet: the earliest instant a Retrying task's backoff elapses or a Running
// task's lease lapses, floored at defaultPollInterval.
func (b *Bus) nextWake(queue commandbus.Queue, now time.Time) time.Duration {
	wait := defaultPollInterval
	for _, e := range b.mirror.snapshot() {
		t := e.Task
		if t.Queue != queue {
			continue
		}
		if t.State == commandbus.StateRetrying {
			wait = soonerWake(wait, t.NotBefore, now)
		}
		if t.State == commandbus.StateRunning && t.Lease != nil {
			wait = soonerWake(wait, t.Lease.Until, now)
		}
	}
	if wait <= 0 {
		wait = time.Millisecond
	}
	return wait
}

// soonerWake returns the shorter of wait and the time until at, ignoring an
// instant that has already passed.
func soonerWake(wait time.Duration, at, now time.Time) time.Duration {
	if !at.After(now) {
		return wait
	}
	if d := at.Sub(now); d < wait {
		return d
	}
	return wait
}

func newLeaseToken() string { return randomHex(16) }

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := cryptorand.Read(buf); err != nil {
		panic(fmt.Errorf("commandbus/nats: reading random bytes: %w", err))
	}
	return hex.EncodeToString(buf)
}
