package nats

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

var errWatchEnded = errors.New("commandbus/nats: the stream watch ended")

// isWrongLastSeq reports the server's refusal of a conditional append: the
// subject moved since the plan was made. It is the lost side of a race, never
// a failure of the operation.
func isWrongLastSeq(err error) bool {
	var apiErr *jetstream.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence
}
