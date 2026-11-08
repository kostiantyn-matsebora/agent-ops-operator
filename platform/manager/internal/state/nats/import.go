package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// This file is the one-time import from an older backend (design D-10):
// on a first start over an install that already holds records, every kind
// is copied once, by existence, and never overwritten once the live
// manager has written its own value. cmd/manager (task 2.8) supplies the
// source — the kube backend, opened read-only for this — and gates when
// Run is called (leader-elected) and what OldReplicaPresent answers (its
// own pod list); nothing in this file names a Kubernetes type or reaches a
// Kubernetes client, so it is exercised in this package's own tests
// against a fake Source.

// importMarkerKey is the "_meta/imported" key design D-10 names, stored as a
// raw entry in the adapterstate bucket (never through the generic
// AdapterState collection, whose key grammar it does not follow — it is
// this backend's own bookkeeping, not an adapter's).
const importMarkerKey = "_meta.imported"

// Imported reports whether the one-time import has already completed.
func (b *Backend) Imported(ctx context.Context) (bool, error) {
	_, err := b.adapterStateKV.Get(ctx, importMarkerKey)
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("nats: check import marker: %w", err)
}

// markImported writes the marker last, as design D-10 requires: the manager
// serves nothing until Imported reports true.
func (b *Backend) markImported(ctx context.Context) error {
	if _, err := b.adapterStateKV.Put(ctx, importMarkerKey, []byte(time.Now().UTC().Format(time.RFC3339Nano))); err != nil {
		return fmt.Errorf("nats: write import marker: %w", err)
	}
	return nil
}

// importMark is this backend's own bookkeeping of one imported record,
// alongside the real record, under the _import namespace of the
// adapterstate bucket (never the generic AdapterState collection, same
// reasoning as the marker above). DestRevision is this record's own
// revision in THIS backend immediately after the import wrote it: a later
// pass compares it against the record's CURRENT revision to tell "nothing
// has written since" from "the live manager already has its own value" —
// design D-10's own re-copy rule, step 3.
type importMark struct {
	SourceVersion string `json:"sourceVersion"`
	DestRevision  string `json:"destRevision"`
}

func importTrackKey(kind string, key state.Key) string {
	return "_import." + kind + "." + wireKey(string(key))
}

func (b *Backend) readImportMark(ctx context.Context, kind string, key state.Key) (importMark, bool, error) {
	entry, err := b.adapterStateKV.Get(ctx, importTrackKey(kind, key))
	if err != nil {
		if isNotFound(err) {
			return importMark{}, false, nil
		}
		return importMark{}, false, fmt.Errorf("nats: read import mark %s %q: %w", kind, key, err)
	}
	var m importMark
	if err := json.Unmarshal(entry.Value(), &m); err != nil {
		return importMark{}, false, fmt.Errorf("nats: decode import mark %s %q: %w", kind, key, err)
	}
	return m, true, nil
}

func (b *Backend) writeImportMark(ctx context.Context, kind string, key state.Key, m importMark) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("nats: encode import mark %s %q: %w", kind, key, err)
	}
	if _, err := b.adapterStateKV.Put(ctx, importTrackKey(kind, key), data); err != nil {
		return fmt.Errorf("nats: write import mark %s %q: %w", kind, key, err)
	}
	return nil
}

// copyRecord implements design D-10's step 3 for one Collection-shaped
// record: create what is missing; re-copy a record whose source version
// differs from the mark, but only while this backend's own revision has not
// moved past the one the mark recorded (proof nothing else has written
// since); otherwise leave it untouched, because the live manager has
// already written its own value. sourceVersion is whatever the source's own
// Get/List reported as the record's Revision — a real resourceVersion for
// kube, or (AdapterState's own import, below) the value itself, which this
// function never interprets, only compares.
func copyRecord[T any](ctx context.Context, b *Backend, kind string, key state.Key, sourceVersion state.Revision, rec T, dest state.Collection[T]) error {
	mark, found, err := b.readImportMark(ctx, kind, key)
	if err != nil {
		return err
	}
	_, destRev, err := dest.Get(ctx, key)
	switch {
	case errors.Is(err, state.ErrNotFound):
		newRev, cerr := dest.Create(ctx, key, rec)
		if cerr != nil {
			if errors.Is(cerr, state.ErrAlreadyExists) {
				return nil // another pass, or the live manager, got there first
			}
			return fmt.Errorf("nats: import create %s %q: %w", kind, key, cerr)
		}
		return b.writeImportMark(ctx, kind, key, importMark{SourceVersion: string(sourceVersion), DestRevision: string(newRev)})
	case err != nil:
		return fmt.Errorf("nats: import get %s %q: %w", kind, key, err)
	default:
		if !found || string(destRev) != mark.DestRevision {
			return nil // no mark, or the live manager has already written: never overwritten
		}
		if mark.SourceVersion == string(sourceVersion) {
			return nil // already current
		}
		newRev, uerr := dest.Update(ctx, key, destRev, rec)
		if uerr != nil {
			if errors.Is(uerr, state.ErrConflict) {
				return nil // raced a concurrent write; the next pass tries again
			}
			return fmt.Errorf("nats: import update %s %q: %w", kind, key, uerr)
		}
		return b.writeImportMark(ctx, kind, key, importMark{SourceVersion: string(sourceVersion), DestRevision: string(newRev)})
	}
}

// AdapterStateEntry is one adapter's persisted value, as SourceNames reads
// it off the source (design D-10's table row 6): the generic AdapterState
// collection declares no index, so nothing short of reading the owning
// Channel/SignalSource's own annotations (which only internal/state/kube,
// the one place the guarded annotation prefix may be named, can do) can
// enumerate "every key" — SourceNames is the bridge cmd/manager (task 2.8)
// supplies for exactly that reason.
type AdapterStateEntry struct {
	Key   state.Key
	Value string
}

// SourceNames supplies the identifiers the generic state.Stores contract
// cannot enumerate on its own: SignalSource and Channel are not state-store
// kinds at all (Cooldowns and Receipts are keyed by a source's name, never
// listed by one), and AdapterState declares no index. cmd/manager's own
// implementation (store.go) lists SignalSourceList/ChannelList directly —
// legitimate outside the kube backend, since neither type is guarded — and
// calls the one new export internal/state/kube gains for this,
// EnumerateAdapterState, for the second method.
type SourceNames interface {
	// SignalSourceNames lists every SignalSource's name, the set
	// Cooldowns' and Receipts' import probes by key.
	SignalSourceNames(ctx context.Context) ([]string, error)
	// AdapterStateEntries lists every adapter-state value any Channel or
	// SignalSource currently carries.
	AdapterStateEntries(ctx context.Context) ([]AdapterStateEntry, error)
}

// DefaultImportPassInterval is how often Importer.Run repeats a pass while
// an old-version replica may still be writing (design D-10, step 3).
const DefaultImportPassInterval = 30 * time.Second

// Importer runs the one-time import (design D-10). Source is read-only in
// practice: every field of it is only ever Get/List/Append/Grow'd from.
type Importer struct {
	Source       state.Stores
	SourceNames  SourceNames
	Dest         *Backend
	PassInterval time.Duration
	// OldReplicaPresent reports whether the manager's own pod list still
	// shows a replica of another version (design D-10, step 4). Required
	// for a rolling upgrade to be raced safely; nil means this process is
	// the only one that will ever write, so one pass already suffices —
	// true of a fresh install reusing this code path with nothing to copy,
	// and of any caller that has already waited out the roll itself.
	OldReplicaPresent func(ctx context.Context) (bool, error)
}

// Run performs the import if it has not already completed, repeating a pass
// while OldReplicaPresent says an old replica may still write, then one
// final pass, writing the marker last. The caller gates when this runs on
// holding the leader Lease (design D-10, step 1) and reports not ready
// until it returns (step 5) — neither of which this package has a notion
// of.
func (im *Importer) Run(ctx context.Context) error {
	already, err := im.Dest.Imported(ctx)
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	interval := im.PassInterval
	if interval <= 0 {
		interval = DefaultImportPassInterval
	}
	for {
		if err := im.pass(ctx); err != nil {
			return err
		}
		present := false
		if im.OldReplicaPresent != nil {
			present, err = im.OldReplicaPresent(ctx)
			if err != nil {
				return fmt.Errorf("nats: check for an old-version replica: %w", err)
			}
		}
		if !present {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	if err := im.pass(ctx); err != nil { // the final pass, after the last old replica is gone
		return err
	}
	return im.Dest.markImported(ctx)
}

// pass copies every kind once, by existence and the re-copy rule
// (copyRecord), never erroring out of one kind's failure before the others
// have had their own chance — the next call to Run retries whatever this
// pass could not finish.
func (im *Importer) pass(ctx context.Context) error {
	if err := im.importConversationsAndRuns(ctx); err != nil {
		return fmt.Errorf("nats: import conversations: %w", err)
	}
	if err := im.importSignals(ctx); err != nil {
		return fmt.Errorf("nats: import signals: %w", err)
	}
	if err := im.importCooldownsAndReceipts(ctx); err != nil {
		return fmt.Errorf("nats: import cooldowns and receipts: %w", err)
	}
	if err := im.importAdapterState(ctx); err != nil {
		return fmt.Errorf("nats: import adapter state: %w", err)
	}
	return nil
}

func (im *Importer) importConversationsAndRuns(ctx context.Context) error {
	convs, err := im.Source.Conversations.List(ctx, state.ByAll, "")
	if err != nil {
		return err
	}
	for _, conv := range convs {
		key := state.Key(conv.Name)
		_, sourceRev, err := im.Source.Conversations.Get(ctx, key)
		if err != nil {
			continue // gone since the list; the next pass sees whatever is current
		}
		if err := copyRecord(ctx, im.Dest, "conversations", key, sourceRev, conv, im.Dest.Stores.Conversations); err != nil {
			return err
		}
		if err := im.importRuns(ctx, key); err != nil {
			return err
		}
		if err := im.importBlobs(ctx, conv); err != nil {
			return err
		}
	}
	return nil
}

// importRuns copies a conversation's runs: a run the destination has not
// seen is appended, one it already holds is grown by the same edit every
// live Grow uses — idempotent and commuting (stores.go's own mergeRunEdit),
// so re-applying it every pass is always safe and needs no mark of its own.
func (im *Importer) importRuns(ctx context.Context, parent state.Key) error {
	runs, err := im.Source.Runs.List(ctx, parent)
	if err != nil {
		return err
	}
	existing, err := im.Dest.Stores.Runs.List(ctx, parent)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(existing))
	for i := range existing {
		have[state.StoreDeclarations.Runs.ID(&existing[i])] = true
	}
	for _, run := range runs {
		id := state.StoreDeclarations.Runs.ID(&run)
		if have[id] {
			if err := im.Dest.Stores.Runs.Grow(ctx, parent, id, run); err != nil && !errors.Is(err, state.ErrNotFound) {
				return err
			}
			continue
		}
		if err := im.Dest.Stores.Runs.Append(ctx, parent, run); err != nil {
			return err
		}
	}
	return nil
}

// importBlobs copies a conversation's queued-input payloads: once a key
// exists in the destination it is never touched again (design D-10's
// general "treats already-exists as done" — Blobs carries no revision to
// mark a re-copy against).
func (im *Importer) importBlobs(ctx context.Context, conv state.ConversationRecord) error {
	for _, input := range conv.Inputs {
		if input.PayloadRef == "" {
			continue
		}
		if _, err := im.Dest.Stores.Blobs.Get(ctx, input.PayloadRef); err == nil {
			continue
		} else if !errors.Is(err, state.ErrNotFound) {
			return err
		}
		blob, err := im.Source.Blobs.Get(ctx, input.PayloadRef)
		if err != nil {
			if errors.Is(err, state.ErrNotFound) {
				continue
			}
			return err
		}
		if err := im.Dest.Stores.Blobs.Put(ctx, input.PayloadRef, blob); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) importSignals(ctx context.Context) error {
	sigs, err := im.Source.Signals.List(ctx, state.SignalAll, "")
	if err != nil {
		return err
	}
	for _, sig := range sigs {
		key := state.Key(sig.Name)
		_, sourceRev, err := im.Source.Signals.Get(ctx, key)
		if err != nil {
			continue
		}
		if err := copyRecord(ctx, im.Dest, "signals", key, sourceRev, sig, im.Dest.Stores.Signals); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) importCooldownsAndReceipts(ctx context.Context) error {
	names, err := im.SourceNames.SignalSourceNames(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		key := state.Key(name)
		if window, rev, err := im.Source.Cooldowns.Get(ctx, key); err == nil {
			if err := copyRecord(ctx, im.Dest, "cooldowns", key, rev, window, im.Dest.Stores.Cooldowns); err != nil {
				return err
			}
		} else if !errors.Is(err, state.ErrNotFound) {
			return err
		}
		if received, rev, err := im.Source.Receipts.Get(ctx, key); err == nil {
			if err := copyRecord(ctx, im.Dest, "receipts", key, rev, received, im.Dest.Stores.Receipts); err != nil {
				return err
			}
		} else if !errors.Is(err, state.ErrNotFound) {
			return err
		}
	}
	return nil
}

// importAdapterState imports every adapter's value, using the value itself
// as the "source version" copyRecord compares against: a plain string has
// no revision of its own, and the value IS the whole payload, so "the value
// changed" and "a re-copy is owed" are the same fact.
func (im *Importer) importAdapterState(ctx context.Context) error {
	entries, err := im.SourceNames.AdapterStateEntries(ctx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyRecord(ctx, im.Dest, "adapterState", e.Key, state.Revision(e.Value), e.Value, im.Dest.Stores.AdapterState); err != nil {
			return err
		}
	}
	return nil
}
