package nats

import (
	mathrand "math/rand"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// reapTransform applies, to a value copy of t, the two transitions design
// D-4/D-6 say happen without any caller asking for them: a Running task
// whose lease has lapsed fails as CodeUnavailable, and a Queued or
// Retrying task past its queue's TTL expires. Sequential, not exclusive —
// a lapse long enough to also outlive the TTL (counted from the task's
// original SubmittedAt) expires the task instead of handing it back for
// redelivery, mirroring commandbus/memory's own reapLocked and the system
// it models: a restart must not deliver a now-stale notice.
//
// Returning one combined result (rather than writing the intermediate
// Retrying state the way commandbus/memory's two-statement reapLocked
// does) costs this backend one feed event commandbus/memory emits and this
// one never will — the intermediate state is never durably observed by
// any contract the conformance suite checks, only the final one.
func reapTransform(t commandbus.Task, inv []commandbus.Declaration, now time.Time) (commandbus.Task, bool) {
	changed := false
	if t.State == commandbus.StateRunning && t.Lease != nil && now.After(t.Lease.Until) {
		t = applyFail(t, inv, now, commandbus.CodeUnavailable, "lease lapsed")
		changed = true
	}
	if t.State == commandbus.StateQueued || t.State == commandbus.StateRetrying {
		if decl, ok := commandbus.Resolve(inv, t.Queue, t.Kind); ok && decl.Policy.TTL > 0 && now.After(t.SubmittedAt.Add(decl.Policy.TTL)) {
			t.State = commandbus.StateExpired
			t.UpdatedAt = now
			changed = true
		}
	}
	return t, changed
}

// applyFail is the pure state transition one failed attempt makes (design
// D-6): retrying with backoff while the code is retryable and attempts
// remain, dead otherwise. Shared by Fail and reapTransform's lease-lapse
// path, which is exactly a Fail(CodeUnavailable) the consumer never got to
// make.
func applyFail(t commandbus.Task, inv []commandbus.Declaration, now time.Time, code commandbus.ErrorCode, detail string) commandbus.Task {
	code = commandbus.ParseErrorCode(string(code))
	decl, _ := commandbus.Resolve(inv, t.Queue, t.Kind)
	t.Code, t.Detail, t.Lease = code, detail, nil
	if code.Retryable() && t.Attempt < t.MaxAttempts {
		t.State = commandbus.StateRetrying
		t.NotBefore = now.Add(backoffFor(decl.Policy.Backoff, t.Attempt))
	} else {
		t.State = commandbus.StateDead
		t.DeadAt = now
		t.ParkedUntil = now.Add(decl.Policy.ParkCoolOff)
	}
	t.UpdatedAt = now
	return t
}

// backoffFor mirrors commandbus/memory's own backoffFor exactly — the same
// "full jitter" choice, for the same reason (AWS's own backoff writeup).
// Duplicated rather than shared: design D-1 keeps every backend
// independent, and the function is a dozen lines of pure arithmetic with
// no state to drift.
func backoffFor(b commandbus.Backoff, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := b.Min
	for i := 1; i < attempt && d < b.Max; i++ {
		d *= 2
	}
	if d > b.Max {
		d = b.Max
	}
	if d < b.Min {
		d = b.Min
	}
	if b.Jitter && d > 0 {
		return time.Duration(mathrand.Int63n(int64(d))) + 1
	}
	return d
}
