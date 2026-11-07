package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const (
	releaseMaxAttempts  = 5
	releaseBackoffStart = 50 * time.Millisecond
)

// LiveSourceID reads the identity of the source that holds name right now,
// from the authoritative store of configuration and never a cache. held is
// false when no source holds the name.
type LiveSourceID func(ctx context.Context, name string) (id string, held bool, err error)

// SourceReleaser releases a deleted source's signals and cooldown window.
// It is a helper over the contract, not an operation of it: the decision
// reads the live source, which is not state-store state.
type SourceReleaser struct {
	Signals   state.Collection[state.SignalRecord]
	Cooldowns state.Collection[state.CooldownWindow]
	Live      LiveSourceID
	// Backoff is the first retry delay, doubled each attempt. Zero is the default.
	Backoff time.Duration
}

// Release deletes every signal of the named source that does not belong to
// the source holding the name now, then clears the window if no source holds
// it. The caller supplies no identity: a successor created at any point
// during the release keeps its signals. Idempotent.
func (r *SourceReleaser) Release(ctx context.Context, name string) error {
	listed, err := r.Signals.List(ctx, state.SignalBySource, name)
	if err != nil {
		return fmt.Errorf("list signals of source %q: %w", name, err)
	}
	for i := range listed {
		if err := r.releaseSignal(ctx, name, state.Key(listed[i].Name)); err != nil {
			return err
		}
	}
	_, held, err := r.Live(ctx, name)
	if err != nil {
		return fmt.Errorf("read source %q to release it: %w", name, err)
	}
	if held {
		return nil
	}
	// A clear creates nothing: a source with no window has nothing to clear.
	_, err = state.Mutate(ctx, r.Cooldowns, state.Key(name), func(w *state.CooldownWindow) error {
		w.Entries = nil
		return nil
	})
	return state.IgnoreNotFound(err)
}

// releaseSignal decides one signal on the version read last and the source
// holding the name at that moment, retrying a conflict with back-off.
func (r *SourceReleaser) releaseSignal(ctx context.Context, source string, key state.Key) error {
	delay := r.Backoff
	if delay <= 0 {
		delay = releaseBackoffStart
	}
	var lastErr error
	for attempt := 0; attempt < releaseMaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
		err := r.tryRelease(ctx, source, key)
		if err == nil {
			return nil
		}
		if !errors.Is(err, state.ErrConflict) {
			return fmt.Errorf("release signal %q of source %q: %w", key, source, err)
		}
		lastErr = err
	}
	return fmt.Errorf("release signal %q of source %q: exhausted %d attempts: %w", key, source, releaseMaxAttempts, lastErr)
}

// tryRelease re-reads the signal and the live source and deletes the signal
// under the revision it read, unless it was recorded under the identity of
// the source holding the name. Already gone is success.
func (r *SourceReleaser) tryRelease(ctx context.Context, source string, key state.Key) error {
	rec, rev, err := r.Signals.Get(ctx, key)
	if errors.Is(err, state.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	id, held, err := r.Live(ctx, source)
	if err != nil {
		return fmt.Errorf("read source %q: %w", source, err)
	}
	if held && rec.SourceID != "" && rec.SourceID == id {
		return nil
	}
	// Conditional on the revision just decided on: a signal rewritten since
	// conflicts, and the caller re-reads and decides again.
	err = r.Signals.DeleteAt(ctx, key, rev)
	if errors.Is(err, state.ErrNotFound) {
		return nil
	}
	return err
}
