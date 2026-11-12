package nats

import (
	"encoding/json"
	"fmt"
	"time"
)

// envelope is the JSON value stored under every KV key (design D-3): the
// backend-minted identity and the caller-owned hold/deletion marker at the
// top level, then the record. id, createdAt, finalizers and
// deletionRequestedAt are ignored on write and re-minted/carried-forward by
// the backend, exactly as store.go documents for Create and Update; a kind
// whose Layout carries no Identity/Hold leaves the matching fields zero and
// never reads them.
//
// importedFrom is design D-10's own field, empty for every record this
// backend ever wrote itself: the source object's resourceVersion at the
// instant the one-time import copied it, read back only by the import's own
// re-copy rule. importRevision is this record's own KV revision as of the
// import's last write, so a later pass can tell "nothing has written since"
// from "the live manager already has its own value" without a second
// round-trip.
type envelope[T any] struct {
	ID                  string     `json:"id,omitempty"`
	CreatedAt           time.Time  `json:"createdAt,omitempty"`
	Finalizers          []string   `json:"finalizers,omitempty"`
	DeletionRequestedAt *time.Time `json:"deletionRequestedAt,omitempty"`
	// AppendSeq orders an Appended item (appended.go) by its own append
	// instant, minted once at Append and carried forward untouched by every
	// Grow: a KV revision cannot serve this, since Grow's own write moves it.
	AppendSeq int64 `json:"appendSeq,omitempty"`
	Record    T     `json:"record"`
}

// encode marshals e, failing loudly rather than silently losing a write: a
// record this backend cannot round-trip through JSON is a bug in a Layout or
// a caller, never a transient condition worth retrying.
func (e *envelope[T]) encode() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("nats: encode envelope: %w", err)
	}
	return b, nil
}

// decodeEnvelope unmarshals a stored value.
func decodeEnvelope[T any](data []byte) (envelope[T], error) {
	var e envelope[T]
	if err := json.Unmarshal(data, &e); err != nil {
		return envelope[T]{}, fmt.Errorf("%w: decode envelope: %w", errCorrupt, err)
	}
	return e, nil
}
