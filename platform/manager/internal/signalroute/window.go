package signalroute

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The helpers below are the cooldown and receipt operations the SignalStore
// used to carry, moved above the contract: each is a read or a Mutate over the
// typed collection, keyed by the source's name.

// GetCooldownWindow reads a source's recorded cooldown window. A source that
// is gone, or holds nothing, reads as an empty window.
func GetCooldownWindow(ctx context.Context, c state.Collection[state.CooldownWindow], source string) ([]state.CooldownEntry, error) {
	w, _, err := c.Get(ctx, state.Key(source))
	if errors.Is(err, state.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cooldown of source %q: %w", source, err)
	}
	return w.Entries, nil
}

// PutCooldownWindow replaces a source's whole window, last writer wins. A
// source with no window yet gets one created; a source that is gone yields
// state.ErrNotFound, as the old replace did.
func PutCooldownWindow(ctx context.Context, c state.Collection[state.CooldownWindow], source string, entries []state.CooldownEntry) error {
	key := state.Key(source)
	replace := func(w *state.CooldownWindow) error {
		w.Entries = entries
		return nil
	}
	_, err := state.Mutate(ctx, c, key, replace)
	if errors.Is(err, state.ErrNotFound) {
		_, err = c.Create(ctx, key, state.CooldownWindow{Entries: entries})
		if errors.Is(err, state.ErrAlreadyExists) { // another writer created it first
			_, err = state.Mutate(ctx, c, key, replace)
		}
	}
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("record cooldown of source %q: %w", source, err)
	}
	return err
}

// CountReceipt counts one routed signal at the given instant. state.Mutate
// retries a lost race, so no increment is lost. A source that is gone yields
// state.ErrNotFound; a source with no counter yet starts from zero.
func CountReceipt(ctx context.Context, c state.Collection[state.Received], source string, at time.Time) error {
	key := state.Key(source)
	last := at.UTC()
	bump := func(r *state.Received) error {
		r.Total++
		r.Last = &last
		return nil
	}
	_, err := state.Mutate(ctx, c, key, bump)
	if errors.Is(err, state.ErrNotFound) {
		_, err = c.Create(ctx, key, state.Received{Total: 1, Last: &last})
		if errors.Is(err, state.ErrAlreadyExists) {
			_, err = state.Mutate(ctx, c, key, bump)
		}
	}
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("count a receipt on source %q: %w", source, err)
	}
	return err
}

// GetReceipts reads a source's counters; a source that is gone or has none
// reads as zero.
func GetReceipts(ctx context.Context, c state.Collection[state.Received], source string) (state.Received, error) {
	r, _, err := c.Get(ctx, state.Key(source))
	if errors.Is(err, state.ErrNotFound) {
		return state.Received{}, nil
	}
	if err != nil {
		return state.Received{}, fmt.Errorf("get receipts of source %q: %w", source, err)
	}
	return r, nil
}
