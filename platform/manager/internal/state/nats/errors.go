package nats

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// isNotFound reports whether err is the KV's own "nothing under this key"
// (ErrKeyNotFound) or "a delete marker is there" (ErrKeyDeleted) — both read
// as state.ErrNotFound, since a tombstone is exactly as absent to a caller
// as a key that never existed.
// errCorrupt marks a stored value this backend cannot decode. It is a fact
// about that one entry, permanent, so a feed skips it where it would end on a
// failure that may pass.
var errCorrupt = errors.New("nats: stored value is corrupt")

func isNotFound(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

// isWrongLastSequence reports whether err is the server's refusal of a
// Create, Update or conditional Delete whose expected last sequence did not
// match (design D-2): the one NATS-side signal both ErrAlreadyExists and
// ErrConflict are derived from, distinguished only by which operation the
// caller attempted, exactly as design D-2 states ("Both map their
// wrong-last-sequence error to state.ErrConflict or ErrAlreadyExists").
func isWrongLastSequence(err error) bool {
	return errors.Is(err, jetstream.ErrKeyExists)
}

// formatRevision renders a KV sequence as the opaque state.Revision this
// backend hands back to a caller.
func formatRevision(seq uint64) state.Revision {
	return state.Revision(strconv.FormatUint(seq, 10))
}

// parseRevision reverses formatRevision. A revision this backend never
// minted (a caller's bug, never a legitimate race) is refused by name rather
// than silently treated as sequence zero, which would read as "create".
func parseRevision(rev state.Revision) (uint64, error) {
	n, err := strconv.ParseUint(string(rev), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("nats: %q is not a revision this backend minted: %w", rev, err)
	}
	return n, nil
}

// addsFinalizer reports whether next holds a finalizer cur does not — the
// kube backend's own rule (internal/state/kube/store.go), restated here
// because a collection marked for deletion refuses exactly this (ErrDeleting).
func addsFinalizer(cur, next []string) bool {
	for _, f := range next {
		if !contains(cur, f) {
			return true
		}
	}
	return false
}

// errNotExpiring names the kind that was asked to open as an Expiring
// collection without declaring an expiry.
func errNotExpiring(name string) error {
	return fmt.Errorf("nats: %q is not an expiring collection: its declaration carries no expiry", name)
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
