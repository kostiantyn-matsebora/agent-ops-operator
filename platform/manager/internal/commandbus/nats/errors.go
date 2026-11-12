package nats

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// errWatchEnded is a watch that closed on its own — a dropped connection or
// a restarted server — never a terminal failure: taskMirror.run reopens it.
var errWatchEnded = errors.New("commandbus/nats: the bucket watch ended")

// isNotFoundErr reports whether err is the KV's own "nothing under this
// key", including a tombstone left by Delete — as absent to a caller as a
// key that never existed.
func isNotFoundErr(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

// isWrongLastSeq reports whether err is the server's refusal of a Create,
// Update or conditional Delete whose expected last sequence did not match —
// the one NATS-side signal a lost compare-and-swap is derived from,
// mirroring internal/state/nats's own isWrongLastSequence.
func isWrongLastSeq(err error) bool {
	return errors.Is(err, jetstream.ErrKeyExists)
}
