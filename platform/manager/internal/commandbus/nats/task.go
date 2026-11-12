package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nuid"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// newUID mints a task's own UID. nuid's own alphabet is already a valid KV
// key (alphanumeric), so no escaping is needed — unlike dedupKey below,
// whose inputs are producer-chosen and not.
func newUID() string { return nuid.Next() }

// wireKeyAllowed is the character set a KV key may hold unescaped (nats.go's
// own doc: alphanumeric, dashes, underscores, equal signs and dots).
// Restated here rather than imported from internal/state/nats/key.go —
// design D-1 keeps the two backends independent.
func wireKeyAllowed(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-' || b == '=':
		return true
	}
	return false
}

// escapeIntroducer starts a two-hex-digit escape, reserved so a producer's
// own "_" can never be mistaken for one.
const escapeIntroducer = '_'

// emptySegment stands in for KindDefault's own "" (commandbus.go: "the
// kind of a queue that declares no distinct kinds"): dedupKey joins three
// segments with ".", and an empty one between two dots is not a valid
// NATS subject token — "conv..cmd-1" is rejected outright, silently
// timing out the Put rather than naming the problem. Unambiguous against
// an escaped segment: escapeKeySegment never emits a bare "_" on its
// own, only "_" followed by two hex digits.
const emptySegment = "_"

// escapeKeySegment maps an opaque producer-chosen string onto a valid KV
// key segment: every byte outside wireKeyAllowed, including the escape
// introducer itself and the "." dedupKey joins segments with, becomes
// "_XX" of its own byte value.
func escapeKeySegment(s string) string {
	if s == "" {
		return emptySegment
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if wireKeyAllowed(c) && c != escapeIntroducer {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "_%02X", c)
	}
	return b.String()
}

// dedupKey composes the "ids" bucket key for one (queue, kind, CommandID)
// triple (design D-4's dedup scope: a producer that retries its own work
// keeps all three the same).
func dedupKey(queue commandbus.Queue, kind commandbus.Kind, id commandbus.CommandID) string {
	return escapeKeySegment(string(queue)) + "." + escapeKeySegment(string(kind)) + "." + escapeKeySegment(string(id))
}

func encodeTask(t commandbus.Task) ([]byte, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: encode task %s: %w", t.UID, err)
	}
	return data, nil
}

func decodeTask(data []byte) (commandbus.Task, error) {
	var t commandbus.Task
	if err := json.Unmarshal(data, &t); err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: decode task: %w", err)
	}
	return t, nil
}

// getTaskLive reads uid straight from the "tasks" bucket — never the
// mirror, for the same reason internal/state/nats/collection.go's Get
// never reads its own mirror: a precondition (here, a lease token) is
// never a guess.
func (b *Bus) getTaskLive(ctx context.Context, uid string) (commandbus.Task, uint64, bool, error) {
	entry, err := b.tasks.Get(ctx, uid)
	if err != nil {
		if isNotFoundErr(err) {
			return commandbus.Task{}, 0, false, nil
		}
		return commandbus.Task{}, 0, false, fmt.Errorf("commandbus/nats: get task %s: %w", uid, err)
	}
	t, err := decodeTask(entry.Value())
	if err != nil {
		return commandbus.Task{}, 0, false, err
	}
	return t, entry.Revision(), true, nil
}

// casWriteTask writes next in place of uid's record at rev — the
// compare-and-swap design D-2 draws every lease and transition from.
// isWrongLastSeq on the result means another writer moved uid since rev;
// the caller decides what that means for it (claim.go skips the candidate,
// task.go's callers report ErrLeaseLost).
func (b *Bus) casWriteTask(ctx context.Context, uid string, rev uint64, next commandbus.Task) (uint64, error) {
	data, err := encodeTask(next)
	if err != nil {
		return 0, err
	}
	newRev, err := b.tasks.Update(ctx, uid, data, rev)
	if err != nil {
		if isWrongLastSeq(err) {
			return 0, err
		}
		return 0, fmt.Errorf("commandbus/nats: update task %s: %w", uid, err)
	}
	b.bumpSelfSeq(newRev)
	return newRev, nil
}

// casReapRetries bounds loadAndReap's read-reap-write loop against a
// genuine concurrent writer — the same purpose and size as
// internal/state/nats's own casRetryMaxAttempts.
const casReapRetries = 20

// loadAndReap reads uid's current record from the server and applies
// reapTransform before returning it, so no caller — GetTask, Submit's
// dedup check, Settle, Fail — ever sees state staler than the backend's
// own reaping (commandbus.Bus.GetTask's own doc: "applied before the
// read"). ok is false for a UID this bucket holds nothing under —
// unminted, or swept past the ledger's own retention (sweep.go).
func (b *Bus) loadAndReap(ctx context.Context, uid string, now time.Time) (commandbus.Task, uint64, bool, error) {
	for attempt := 0; attempt < casReapRetries; attempt++ {
		t, rev, ok, err := b.getTaskLive(ctx, uid)
		if err != nil || !ok {
			return t, rev, ok, err
		}
		next, changed := reapTransform(t, b.inventory, now)
		if !changed {
			return t, rev, true, nil
		}
		newRev, err := b.casWriteTask(ctx, uid, rev, next)
		if err == nil {
			if next.State == commandbus.StateDead {
				b.archiveDead(ctx, next)
			}
			return next, newRev, true, nil
		}
		if !isWrongLastSeq(err) {
			return commandbus.Task{}, 0, false, err
		}
		// Another writer moved uid between our read and our write — read
		// its result and decide again, rather than clobbering it.
	}
	return commandbus.Task{}, 0, false, fmt.Errorf("commandbus/nats: reap %s: exhausted %d attempts on conflict", uid, casReapRetries)
}

// archiveDead copies t into the "dead" bucket the instant it reaches
// StateDead, so DeadLetters (deadletters.go) never depends on the
// retention sweep having run yet. Best-effort: a failure here is not the
// caller's — the task is already recorded as Dead in "tasks" — and the
// sweep re-attempts the same write before it ever purges that record
// (sweep.go), so a crash between the two writes still heals.
func (b *Bus) archiveDead(ctx context.Context, t commandbus.Task) {
	data, err := encodeTask(t)
	if err != nil {
		return
	}
	_, _ = b.dead.Put(ctx, t.UID, data)
}
