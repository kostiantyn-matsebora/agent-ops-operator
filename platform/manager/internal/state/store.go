// Package state is the only way the manager reaches a Conversation or a
// queued input's out-of-line payload (design D-1/D-2 of the state-store
// change, openspec/changes/state-store/). No Kubernetes type, no selector
// string and no transaction crosses this boundary — every operation here
// traces to a manager need recorded in design.md's provenance table.
//
// internal/state/kube is the one backend this change ships: today's
// Conversation/ConversationInput custom resources, behind this contract,
// byte-for-byte. internal/state/conformance is the suite every backend,
// kube included, must pass to exist.
package state

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Get, Update and GetBlob for a key that does not
// exist, or that exists but has passed ExpiresAt.
var ErrNotFound = errors.New("state: record not found")

// ErrConflict is returned by Update and AppendRun's internal retry loop when
// a write loses a compare-and-swap race. Distinguish it from any other
// failure with errors.Is(err, state.ErrConflict) — never by string-matching
// a backend's own error text.
var ErrConflict = errors.New("state: conflicting write")

// ErrAlreadyExists is returned by Create for a key that already has a
// record. Distinct from ErrConflict: a create names no revision to lose a
// race over, it simply refuses to create a second record under one key.
var ErrAlreadyExists = errors.New("state: record already exists")

// ErrUnknownIndex is returned by List for an Index outside DeclaredIndexes.
var ErrUnknownIndex = errors.New("state: unknown index")

// Index names one of the Store's declared, closed lookups (design D-2: "the
// declared index set, closed"). A backend's List implements every member of
// DeclaredIndexes or fails ErrUnknownIndex — there is no open-ended query.
type Index string

const (
	// ByConversationID looks up the equality match every signal re-delivery,
	// reopen's at-most-one-open check and member reuse perform today on
	// api/v1alpha1.IndexConversationID. ListKey.ConversationID is read.
	ByConversationID Index = "conversationId"
	// ByPipelineAndPhase looks up a Pipeline's own conversations in one
	// phase — the queue a Pipeline-rooted dispatch sweep reads.
	// ListKey.PipelineRef and ListKey.Phase are read.
	ByPipelineAndPhase Index = "pipelineRef+phase"
	// ByPhase looks up every conversation in one phase, the admission
	// gate's and housekeeping's own sweep. ListKey.Phase is read.
	ByPhase Index = "phase"
)

// DeclaredIndexes is the Store's closed index set. internal/state/conformance
// walks this slice, so adding an Index here without a kube implementation
// fails the suite on every backend rather than only the one left behind.
var DeclaredIndexes = []Index{ByConversationID, ByPipelineAndPhase, ByPhase}

// ListKey carries one index's lookup values. A backend's List reads only the
// fields the named Index declares above; the rest are ignored, so a caller
// may always populate every field it has without checking which index it is
// calling.
type ListKey struct {
	ConversationID string
	PipelineRef    string
	Phase          string
}

// EventType says what happened to a record on a Watch feed.
type EventType string

const (
	// EventPut is a create or an update — the feed does not distinguish
	// them, exactly as an informer's AddFunc/UpdateFunc collapse to one
	// reconcile request today.
	EventPut EventType = "put"
	// EventDelete is a Delete, or a record crossing its ExpiresAt.
	EventDelete EventType = "delete"
)

// Event is one change on a Watch feed: design D-3's "(key, revision,
// record)", plus the Type a consumer needs to tell a delete from a put and
// the Resume token for the point the feed has delivered through. Record is
// the zero value on EventDelete.
type Event struct {
	Type     EventType
	Key      Key
	Revision Revision
	Record   ConversationRecord
	// Resume is this event's own Revision, offered back from Watch's resume
	// parameter to continue strictly after it. Equal to Revision on
	// EventPut; a backend still mints one on EventDelete so a consumer that
	// stops there has somewhere to resume from.
	Resume Revision
}

// Store is the only way the manager reaches a Conversation's state. A Store
// is constructed bound to one namespace (design D-2: "one install per
// namespace") — no method here takes one, and no method takes a selector
// string.
type Store interface {
	// Get returns the current record and its revision, or ErrNotFound.
	Get(ctx context.Context, key Key) (ConversationRecord, Revision, error)

	// Create writes a new record and returns its revision. ErrAlreadyExists
	// when key already names a record — a create carries no revision to
	// lose a race over, it is refused outright.
	Create(ctx context.Context, key Key, rec ConversationRecord) (Revision, error)

	// Update writes rec in place of the record at rev, returning the new
	// revision. ErrConflict when rev is not the record's current revision —
	// the caller's own get-modify-put raced another writer (design D-2) and
	// must Get again before retrying.
	Update(ctx context.Context, key Key, rev Revision, rec ConversationRecord) (Revision, error)

	// Delete removes a record. Not an error when the key is already absent.
	Delete(ctx context.Context, key Key) error

	// List returns every non-expired record matching one declared Index.
	// ErrUnknownIndex for an Index outside DeclaredIndexes.
	List(ctx context.Context, index Index, key ListKey) ([]ConversationRecord, error)

	// Watch delivers an at-least-once feed of Event, resuming strictly after
	// resume (empty Revision starts from now). The channel closes when ctx
	// is done; a backend reports a terminal failure by closing the channel
	// early, with no error value crossing this contract (design: no
	// transactions, no backend-specific type leaks through).
	Watch(ctx context.Context, resume Revision) (<-chan Event, error)

	// AppendRun adds run to key's append-only run history. Takes no
	// revision: appends commute, and a backend meets this with its own
	// internal CAS retry loop (design D-5) rather than surfacing a conflict
	// the caller would have no reason to expect.
	AppendRun(ctx context.Context, key Key, run Run) error

	// ListRuns returns key's run history in append order.
	ListRuns(ctx context.Context, key Key) ([]Run, error)

	// PutBlob stores blob under key, replacing any value already there.
	PutBlob(ctx context.Context, key Key, blob Blob) error

	// GetBlob returns the blob stored under key, or ErrNotFound.
	GetBlob(ctx context.Context, key Key) (Blob, error)

	// DeleteBlob removes a blob. Not an error when the key is already
	// absent — a deleted key reads absent (task 2.5), it does not error.
	DeleteBlob(ctx context.Context, key Key) error
}
