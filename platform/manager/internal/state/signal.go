package state

import "time"

// This file is the Signal half of the state vocabulary: the durable message of
// one admitted signal, the consumer's status on it, and the per-source cooldown
// window and receipt counters. The collections that hold them are fields of
// Stores (stores.go); nothing here is an operation.

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
	// deleted source's signals itself, in the controller's source-release helper.
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

// Received is the count of signals a source has routed and the instant of the
// latest, the reported counters a configuration surface shows. The record of
// the Receipts collection, keyed by the source's name.
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

// CooldownWindow is one source's whole cooldown window, the record of the
// Cooldowns collection keyed by the source's name. A replace writes all of it;
// a source with nothing recorded reads not found, which the router's helper
// reads as an empty window.
type CooldownWindow struct {
	Entries []CooldownEntry
}
