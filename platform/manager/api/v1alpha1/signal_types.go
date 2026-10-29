package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalNamePrefix prefixes every Signal's name, deterministic or generated.
const SignalNamePrefix = "sig-"

// signalNameHexLen is how many hex characters of the digest name a Signal: 128
// bits, far past any collision a per-source message stream can produce.
const signalNameHexLen = 32

// SignalNameFor is the name of the Signal carrying a signalId: sig-<first 32
// hex of sha256(source NUL signalId)>. The source is in the hash so two
// sources never collide on one id, and the NUL cannot occur in a Kubernetes
// name, so (source, id) pairs never alias. Publication creates under this name
// and treats AlreadyExists as the duplicate — the API server's create is the
// single arbiter, with no lookup and no list.
func SignalNameFor(source, signalID string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + signalID))
	return SignalNamePrefix + hex.EncodeToString(sum[:])[:signalNameHexLen]
}

// SignalSpec is everything about ONE signal message that the consumer needs to
// route it, written by PUBLICATION only and never rewritten (design D-2): a
// Signal has one payload and one arrival, and a repeat of an initiationId is
// its own Signal — the Conversation is the accumulator, not this object.
//
// It is deliberately shaped like NormalizedSignal (internal/httpapi) plus the
// bookkeeping a durable record adds, so nothing here is invented, only
// persisted.
type SignalSpec struct {
	// SourceRef names the SignalSource this signal arrived on (same
	// namespace). The owning SignalSource also owns this object by
	// OwnerReference — set at creation — so a deleted source takes its
	// signals from any phase. SourceRef is carried here too so a lister or
	// index can filter by source without resolving the owner.
	SourceRef ObjectRef `json:"sourceRef"`
	// Adapter is the serving SignalAdapter's name, copied from the source's
	// spec.adapter at creation — a display copy, so a reader says what kind of
	// signal this is without a live lookup of the (possibly since-renamed)
	// source.
	// +kubebuilder:validation:MinLength=1
	Adapter string `json:"adapter"`
	// InitiationID is the sender-minted correlation id, shared across the
	// messages of one conversation on purpose. The consumer's key for finding
	// the conversation (equality reuse); publication never reads it.
	// +kubebuilder:validation:MinLength=1
	InitiationID string `json:"initiationId"`
	// SignalID is the sender's id for this one MESSAGE, stable across every
	// re-send of it (design D-8). When present it fixes the Signal's name —
	// sig-<first 32 hex of sha256(source NUL signalId)> — so publication is
	// idempotent per (source, signalId) without a lookup. Opaque: compared for
	// equality, never parsed. Absent: a generated name and no deduplication.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +optional
	SignalID string `json:"signalId,omitempty"`
	// Kind selects the input lane: alert (absent), job, task or chat — the
	// same closed vocabulary httpapi.NormalizedSignal.Kind and
	// InputOrigin.SignalKind already carry as a plain string.
	// +optional
	Kind string `json:"kind,omitempty"`
	// Title overrides the conversation title a routed claimant opens with.
	// +optional
	Title string `json:"title,omitempty"`
	// Severity is this signal's own urgency rating. Absent means unrated.
	// +optional
	Severity Severity `json:"severity,omitempty"`
	// Labels are attribution only — rendered on signal cards, feeding no
	// routing decision (the manager routes purely by InitiationID equality),
	// except the reserved chat labels naming the originating surface. Bounded
	// at MaxSignalLabels, the same cap SignalProvenance.Labels uses.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Reader is the opaque key of the person who sent this, for a chat
	// signal only — see NormalizedSignal.Reader.
	// +optional
	Reader string `json:"reader,omitempty"`
	// Payload is this ONE message's content, handed to the agent. Left
	// unbounded like ConversationInputSpec.Payload: the bulky half of a
	// signal, out-of-line from the lighter-weight objects that reference it.
	// An addressed chat target (/<pipeline> <task>) stays in it verbatim — the
	// consumer parses it, publication never does.
	// +optional
	Payload string `json:"payload,omitempty"`
}

// ClaimantKind says whether a Signal's claimant is a Pipeline or a
// Coordinator — mirrors internal/chat.ClaimantKind, duplicated here because
// this package is self-contained (structure.md) and carries no dependency on
// internal/chat.
// +kubebuilder:validation:Enum=Pipeline;Coordinator
type ClaimantKind string

const (
	ClaimantKindPipeline    ClaimantKind = "Pipeline"
	ClaimantKindCoordinator ClaimantKind = "Coordinator"
)

// ClaimantRef names one Pipeline or Coordinator claiming a Signal's
// SignalSource. Kind is explicit, exactly as AdapterRef's own comment
// explains: the two wiring kinds share a namespace and may share a name, so
// the name alone would not identify one.
type ClaimantRef struct {
	// Kind of the claiming object.
	Kind ClaimantKind `json:"kind"`
	// Name of the claiming object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ClaimantState is one claimant's own routing workflow (design D-6):
// Routing is non-terminal, the other three are terminal. A later pass of the
// consumer touches only claimants still Routing.
// +kubebuilder:validation:Enum=Routing;Routed;Failed;Skipped
type ClaimantState string

const (
	// ClaimantRouting: not yet terminal — either never attempted, or
	// attempted and owed a retry.
	ClaimantRouting ClaimantState = "Routing"
	// ClaimantRouted: this claimant opened or appended to a conversation for
	// this signal. Idempotent by equality reuse (conversationId ==
	// initiationId) and by the input named for the Signal and claimant — a
	// repeated pass never re-routes it.
	ClaimantRouted ClaimantState = "Routed"
	// ClaimantFailed: this claimant's retry budget is exhausted
	// (SIGNAL_RETRY_DEADLINE) with no successful route, or the per-source
	// cap evicted it post-fan-out. The reason is recorded verbatim.
	ClaimantFailed ClaimantState = "Failed"
	// ClaimantSkipped: routing was deliberately not attempted. In this change
	// exactly one condition produces it — the snapshotted claimant no longer
	// exists when a pass would route it.
	ClaimantSkipped ClaimantState = "Skipped"
)

// SignalClaimant is one claimant's own routing workflow for a Signal — the
// state machine Phase derives from. The set is snapshotted ONCE, at the
// signal's first processing: the same principle that pins a Conversation's
// refs at creation, so a later rewire of who serves the source never changes
// an in-flight signal's own fan-out (design D-6).
type SignalClaimant struct {
	// Ref names the claiming Pipeline or Coordinator.
	Ref ClaimantRef `json:"ref"`
	// State is this claimant's own routing workflow.
	State ClaimantState `json:"state"`
	// ConversationRef names the conversation this claimant routed the
	// signal onto, once State is Routed.
	// +optional
	ConversationRef *ObjectRef `json:"conversationRef,omitempty"`
	// Reason explains a Failed or Skipped state. Empty for Routing/Routed.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// SignalPhase is the signal's own coarse state, a derived ROLL-UP over
// Status.Claimants — never authored independently (design D-6). See
// RollupPhase.
// +kubebuilder:validation:Enum=Pending;Routed;PartiallyRouted;Dropped;Failed
type SignalPhase string

const (
	// SignalPending: not yet consumed, or every snapshotted claimant is still
	// Routing. An absent phase reads as Pending.
	SignalPending SignalPhase = "Pending"
	// SignalRouted: every claimant is terminal, and at least one Routed.
	// Retry exhaustion with at least one routed claimant still rolls up here
	// — the exhausted claimant is Failed, visibly, per claimant.
	SignalRouted SignalPhase = "Routed"
	// SignalPartiallyRouted: at least one claimant has Routed, and at least
	// one is still Routing. Always exits once the retry deadline flips every
	// remaining claimant terminal (design D-6).
	SignalPartiallyRouted SignalPhase = "PartiallyRouted"
	// SignalDropped: terminal BEFORE fan-out — this signal never reached a
	// claimant snapshot ("Dropped stays strictly pre-fan-out"). See
	// SignalDropReason.
	SignalDropped SignalPhase = "Dropped"
	// SignalFailed: every claimant is terminal and none Routed. Surfaced as
	// loudly as the unwired drop.
	SignalFailed SignalPhase = "Failed"
)

// SignalDropReason explains a Dropped phase. Set only alongside
// SignalDropped, which — unlike every other phase — precedes any claimant
// snapshot.
// +kubebuilder:validation:Enum=unclaimed;ambiguous;unaddressable;cooldown;capacity;overflow;exhausted
type SignalDropReason string

const (
	// SignalDropUnclaimed: no Ready Pipeline or Coordinator serves the
	// source (or an addressed target does not exist).
	SignalDropUnclaimed SignalDropReason = "unclaimed"
	// SignalDropAmbiguous: an unaddressed chat signal on a source several
	// Ready Pipelines or Coordinators serve.
	SignalDropAmbiguous SignalDropReason = "ambiguous"
	// SignalDropUnaddressable: a chat signal whose originating Channel does
	// not exist — there is nowhere to answer.
	SignalDropUnaddressable SignalDropReason = "unaddressable"
	// SignalDropCooldown: suppressed inside the source's cooldown window.
	SignalDropCooldown SignalDropReason = "cooldown"
	// SignalDropCapacity: a chat signal declined because the pending
	// conversation backlog was full. Machine kinds never use this reason —
	// their claimants wait, Routing, instead.
	SignalDropCapacity SignalDropReason = "capacity"
	// SignalDropOverflow: evicted — the oldest non-terminal Signal on this
	// source — to hold the per-source cap, before it fanned out.
	SignalDropOverflow SignalDropReason = "overflow"
	// SignalDropExhausted: the retry deadline passed before fan-out.
	SignalDropExhausted SignalDropReason = "exhausted"
)

// SignalStatus is written by the CONSUMER and HOUSEKEEPING only.
type SignalStatus struct {
	// Phase is the roll-up over Claimants. See RollupPhase. Absent reads as
	// Pending.
	// +optional
	Phase SignalPhase `json:"phase,omitempty"`
	// Reason explains a Dropped phase. Absent for every other phase.
	// +optional
	Reason SignalDropReason `json:"reason,omitempty"`
	// Attempts counts the processing attempts started, monotonic.
	// +optional
	Attempts int32 `json:"attempts,omitempty"`
	// FirstAttemptAt stamps the first processing pass — the origin the retry
	// deadline (SIGNAL_RETRY_DEADLINE) measures from.
	// +optional
	FirstAttemptAt *metav1.Time `json:"firstAttemptAt,omitempty"`
	// CompletedAt is set by the write that first makes the phase terminal and
	// never moved by a later one — the origin every TTL measures from.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// ConsumedBy is the leader's identity, the same string as the leader
	// Lease's holderIdentity, set by the write that makes the phase terminal.
	// +optional
	ConsumedBy string `json:"consumedBy,omitempty"`
	// ConsumedAt is when that identity processed it, set by the same write.
	// +optional
	ConsumedAt *metav1.Time `json:"consumedAt,omitempty"`
	// Claimants is the per-claimant routing workflow, snapshotted once at the
	// first processing that reaches fan-out. Empty while Pending or Dropped.
	// +optional
	Claimants []SignalClaimant `json:"claimants,omitempty"`
}

// RollupPhase derives a signal's phase from its claimants alone — the pure
// function Status.Phase is always written FROM, never independently (design
// D-6).
//
// It does NOT decide SignalDropped: a drop happens strictly before any
// claimant exists, so the caller sets SignalDropped (with its
// SignalDropReason) directly instead of calling this function at all.
func RollupPhase(claimants []SignalClaimant) SignalPhase {
	if len(claimants) == 0 {
		return SignalPending
	}
	var anyRouting, anyRouted bool
	for i := range claimants {
		switch claimants[i].State {
		case ClaimantRouting:
			anyRouting = true
		case ClaimantRouted:
			anyRouted = true
		}
	}
	switch {
	case anyRouting && anyRouted:
		return SignalPartiallyRouted
	case anyRouting:
		return SignalPending
	case anyRouted:
		return SignalRouted
	default:
		return SignalFailed
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sig
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceRef.name`
// +kubebuilder:printcolumn:name="InitiationID",type=string,JSONPath=`.spec.initiationId`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Attempts",type=integer,JSONPath=`.status.attempts`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Signal is the durable message of one admitted signal: published by
// /signal/inbound, consumed (routed) by the leader's consumer pool, bounded by
// the leader's housekeeping. Owned by the SignalSource named in
// spec.sourceRef — set as this object's OwnerReference at creation — so a
// deleted source takes its signals from any phase.
type Signal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalSpec   `json:"spec,omitempty"`
	Status SignalStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SignalList contains a list of Signal.
type SignalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Signal `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Signal{}, &SignalList{})
}
