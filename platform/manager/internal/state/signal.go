package state

import (
	"context"
	"time"
)

// This file is the Signal half of the state contract (design D-14 of the
// state-store change): the durable message of one admitted signal, the
// consumer's status on it, and the per-source cooldown window. Every operation
// below traces to a call site recorded in design.md's Signal provenance table.
//
// It reuses the contract's own pieces — Key, Revision, Ref, EventType, the
// errors, expiry, identity — and adds nothing a backend offers that the
// manager does not use: no finalizers (a Signal holds none), no sub-records,
// no blobs, no free-form query.

// Signal phases, mirroring api/v1alpha1.SignalPhase. An absent phase reads as
// SignalPending.
const (
	SignalPending         = "Pending"
	SignalRouted          = "Routed"
	SignalPartiallyRouted = "PartiallyRouted"
	SignalDropped         = "Dropped"
	SignalFailed          = "Failed"
)

// Claimant states, mirroring api/v1alpha1.ClaimantState.
const (
	ClaimantRouting = "Routing"
	ClaimantRouted  = "Routed"
	ClaimantFailed  = "Failed"
	ClaimantSkipped = "Skipped"
)

// Claimant kinds, mirroring api/v1alpha1.ClaimantKind.
const (
	ClaimantKindPipeline    = "Pipeline"
	ClaimantKindCoordinator = "Coordinator"
)

// TerminalSignalPhase reports whether a Signal phase is final: Routed,
// Dropped or Failed. Pending, PartiallyRouted and the absent phase are not.
func TerminalSignalPhase(phase string) bool {
	switch phase {
	case SignalRouted, SignalDropped, SignalFailed:
		return true
	default:
		return false
	}
}

// SignalClaimant is one claimant's own routing workflow for a Signal, the
// state machine the phase derives from. See api/v1alpha1.SignalClaimant.
type SignalClaimant struct {
	// Kind and Name identify the claiming Pipeline or Coordinator.
	Kind string
	Name string
	// State is Routing, Routed, Failed or Skipped.
	State string
	// ConversationRef names the conversation a Routed claimant routed onto.
	ConversationRef Ref
	// Reason explains a Failed or Skipped state.
	Reason string
}

// SignalRecord is the durable message of one admitted signal and the
// consumer's progress on it — every field of api/v1alpha1.SignalSpec and
// api/v1alpha1.SignalStatus.
//
// Publication fields (SourceRef through Payload, and SourceID) are written by
// Create and never rewritten: Update ignores whatever a caller puts in them,
// as it ignores ID and CreatedAt. Status fields (Phase through Claimants) and
// ExpiresAt are written by Update. The payload stays inline: a Signal has one
// payload and one arrival, and unlike a conversation's inputs it does not
// grow.
type SignalRecord struct {
	Name string

	// ID and CreatedAt are minted by the backend and read-only, exactly as on
	// ConversationRecord (design D-11). CreatedAt is the arrival instant the
	// consumer orders a conversation's messages by, then Name; see
	// SignalArrivesBefore.
	ID        string
	CreatedAt time.Time

	// SourceID is the opaque identity of the SignalSource this signal arrived
	// on, as the source's own object reports it. Supplied at Create so a
	// backend that links a record to its owner can (the kube backend turns it
	// into the owner reference), and read back. A backend with no such link
	// stores it and does nothing else with it: the manager releases a
	// deleted source's signals itself, with ReleaseSource.
	SourceID string

	// Publication fields.
	SourceRef    Ref
	Adapter      string
	InitiationID string
	SignalID     string
	Kind         string
	Title        string
	Severity     string
	Labels       map[string]string
	Reader       string
	Payload      string

	// Status fields, written by the consumer and housekeeping.
	Phase          string
	Reason         string
	Attempts       int32
	FirstAttemptAt *time.Time
	CompletedAt    *time.Time
	ConsumedBy     string
	ConsumedAt     *time.Time
	Claimants      []SignalClaimant

	// ExpiresAt, once passed, makes the record invisible to Get and List and
	// delivers it as a delete on the feed. The consumer sets it in the write
	// that makes the phase terminal: CompletedAt plus the phase's TTL, so a
	// terminal Signal removes itself and nothing deletes it by hand.
	ExpiresAt *time.Time
}

// Terminal reports whether the record's phase is final.
func (r *SignalRecord) Terminal() bool { return TerminalSignalPhase(r.Phase) }

// SignalArrivesBefore orders two signals by arrival: CreatedAt, then Name. The
// consumer processes the messages of one conversation in this order.
func SignalArrivesBefore(a, b *SignalRecord) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.Name < b.Name
}

// SignalIndex names one of the SignalStore's declared, closed lookups. There
// is no open-ended query: adding an index is a contract change with a
// conformance scenario, as for Index.
type SignalIndex string

const (
	// SignalAll lists every non-expired signal in the store's scope: the
	// consumer's initial pass after a leader change reads it. No
	// SignalListKey field is read.
	SignalAll SignalIndex = "all"
	// SignalNonTerminalBySource lists one source's signals that are not
	// terminal, the set the per-source cap counts. SignalListKey.Source is
	// read. A terminal record is bounded by rate times TTL and never
	// returned, so a flapping source does not slow the cap.
	SignalNonTerminalBySource SignalIndex = "nonTerminalBySource"
	// SignalNonTerminalByInitiation lists one conversation's non-terminal
	// signals on one source: the earlier messages a signal must wait for.
	// SignalListKey.Source and .InitiationID are read.
	SignalNonTerminalByInitiation SignalIndex = "nonTerminalByInitiation"
)

// DeclaredSignalIndexes is the SignalStore's closed index set.
// internal/state/conformance walks it.
var DeclaredSignalIndexes = []SignalIndex{SignalAll, SignalNonTerminalBySource, SignalNonTerminalByInitiation}

// SignalListKey carries one signal index's lookup values; a backend reads only
// the fields the named index declares.
type SignalListKey struct {
	Source       string
	InitiationID string
}

// SignalEvent is one change on a SignalStore feed, with the same meaning as
// Event: Record is the zero value on EventDelete and EventBookmark.
type SignalEvent struct {
	Type     EventType
	Key      Key
	Revision Revision
	Record   SignalRecord
	Resume   Revision
}

// Received is the count of signals a source has routed and the instant of the
// latest, the reported counters a configuration surface shows.
type Received struct {
	Total int64
	// Last is nil until a first signal is counted. UTC.
	Last *time.Time
}

// CooldownEntry records when an initiationId was last admitted on a source,
// so suppression survives a restart. See api/v1alpha1.CooldownEntry.
type CooldownEntry struct {
	InitiationID string
	At           time.Time
}

// SignalStore is the only way the manager reaches a Signal or a source's
// cooldown window. Like Store, it is bound to one install's scope at
// construction and takes no namespace, selector or transaction.
type SignalStore interface {
	// Get returns the current record and its revision, or ErrNotFound (also
	// for a record past its ExpiresAt).
	Get(ctx context.Context, key Key) (SignalRecord, Revision, error)

	// Create stores a new record under key and returns its revision.
	// ErrAlreadyExists when the key is taken, in any phase (a record past its
	// ExpiresAt is not taken: it is invisible, so a create replaces it):
	// publication
	// names a signal that carries a signalId by a key derived from it, and
	// this refusal is how a re-send is recognised as a duplicate. ID and
	// CreatedAt are minted here.
	Create(ctx context.Context, key Key, rec SignalRecord) (Revision, error)

	// Update writes rec's status fields and ExpiresAt in place of the record
	// at rev, returning the new revision. ErrConflict when rev is not
	// current, ErrNotFound when the key is absent or expired. The
	// publication fields, ID and CreatedAt are never rewritten: whatever rec
	// carries in them is ignored.
	Update(ctx context.Context, key Key, rev Revision, rec SignalRecord) (Revision, error)

	// Delete removes a record at once. Not an error when the key is absent.
	// Terminal records remove themselves through ExpiresAt, so this is for
	// the source-gone case only; ReleaseSource removes a whole source.
	Delete(ctx context.Context, key Key) error

	// List returns every non-expired record matching one declared index.
	// Order is unspecified; callers order by SignalArrivesBefore.
	// ErrUnknownIndex for an index outside DeclaredSignalIndexes.
	List(ctx context.Context, index SignalIndex, key SignalListKey) ([]SignalRecord, error)

	// Watch delivers an at-least-once, resumable feed with Store.Watch's
	// semantics: strictly after resume, an empty Revision starting from now,
	// the channel closing when ctx is done or the backend fails.
	Watch(ctx context.Context, resume Revision) (<-chan SignalEvent, error)

	// GetCooldown returns the cooldown window recorded for a source: one
	// entry per initiationId last admitted. A source with nothing recorded,
	// and a source the backend does not know, read as an empty window, never
	// as an error.
	GetCooldown(ctx context.Context, source string) ([]CooldownEntry, error)

	// PutCooldown replaces a source's whole window with entries, last write
	// wins. It takes no revision: the leader is the only writer and holds a
	// per-source lock around each write, and a lost write costs a duplicate
	// investigation after a restart, never a stuck signal, so the caller logs
	// a failure and carries on. ErrNotFound when the backend knows the source
	// to be gone.
	PutCooldown(ctx context.Context, source string, entries []CooldownEntry) error

	// RecordReceived counts one routed signal on a source: Total grows by one
	// and Last becomes at. Concurrent calls from the consumer pool all count:
	// the backend serializes them (a revisioned write retried on a lost race)
	// and never loses an increment. ErrNotFound when the backend knows the
	// source to be gone. The counters decide no route.
	RecordReceived(ctx context.Context, source string, at time.Time) error

	// GetReceived reads a source's counters. A source that is gone or has
	// counted nothing reads as the zero value, not an error.
	GetReceived(ctx context.Context, source string) (Received, error)

	// ReleaseSource removes the signals of a deleted source in any phase and
	// clears its cooldown window. Idempotent. It is the manager's own act on
	// a deleted source: no behaviour depends on an owner reference being
	// honoured (the same rule as a conversation's workloads).
	//
	// sourceID is the departed source's identity as its signals recorded it
	// (SignalRecord.SourceID), "" when the caller does not know it. A name
	// can be reused by a successor, so with an identity a signal recorded
	// under a DIFFERENT identity is the successor's and is kept, and so is the
	// successor's window where the backend can tell one holds the name. With
	// "" every signal of the name goes and the window clears.
	ReleaseSource(ctx context.Context, source, sourceID string) error
}
