package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalSpec is everything about one signal arrival that a routing attempt —
// the inline first try at ingest, or a later retry in the reconciler — needs
// to act, with no other in-memory state (design D-3: "no in-memory routing
// state, a new leader resumes from the CRs alone").
//
// It is deliberately shaped like NormalizedSignal (internal/httpapi) plus the
// bookkeeping a durable record adds: a signal already carries everything a
// conversation create/append needs, so nothing here is invented, only
// persisted.
type SignalSpec struct {
	// SourceRef names the SignalSource this signal arrived on (same
	// namespace). The owning SignalSource also owns this object by
	// OwnerReference — set at creation by whatever persists the Signal — so a
	// deleted source takes its signals from any phase (design D-6). SourceRef
	// is carried here too, rather than read off the owner reference alone, so
	// a lister or index can filter by source without resolving it.
	SourceRef ObjectRef `json:"sourceRef"`
	// Adapter is the serving SignalAdapter's name, copied from
	// SourceRef's own spec.adapter at creation — a display/filter
	// convenience, so a reader says what kind of signal this is without a
	// live lookup of the (possibly since-renamed) source.
	// +kubebuilder:validation:MinLength=1
	Adapter string `json:"adapter"`
	// InitiationID is the sender-minted correlation id — see
	// NormalizedSignal.InitiationID. A fresh arrival whose id matches a
	// still-open (non-terminal) Signal of the same source is ABSORBED into
	// it: Payload appended, Occurrences bumped, no second CR (design D-5). A
	// repeat matching only a TERMINAL Signal creates a fresh CR instead —
	// reopening a terminal record would make the audit trail mutable (design
	// D-5a).
	//
	// MATERIALIZED at creation, never rewritten.
	// +kubebuilder:validation:MinLength=1
	InitiationID string `json:"initiationId"`
	// Kind selects the input lane: alert (default), job, task or chat — the
	// same closed vocabulary httpapi.NormalizedSignal.Kind and
	// InputOrigin.SignalKind already carry as a plain string, unenforced by
	// either's own CRD schema. Written once, from the occurrence that
	// created this Signal — a later absorb carries its own payload in but
	// never changes the lane.
	// +optional
	Kind string `json:"kind,omitempty"`
	// Title overrides the conversation title a routed claimant opens with.
	// Written once, like Kind.
	// +optional
	Title string `json:"title,omitempty"`
	// Severity is this signal's own urgency rating, from the occurrence that
	// created this Signal. Absent means unrated. Written once, like Kind —
	// an absorbed occurrence disagreeing with it is not retroactively
	// applied, the same rule SignalProvenance.Severity already follows on a
	// Conversation.
	// +optional
	Severity Severity `json:"severity,omitempty"`
	// Labels are attribution only — rendered on signal cards, feeding no
	// routing decision (the manager routes purely by InitiationID equality).
	// From the occurrence that created this Signal; written once, like Kind.
	// Bounded at MaxSignalLabels, the same cap SignalProvenance.Labels uses.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Reader is the opaque key of the person who sent this, for a chat
	// signal only — see NormalizedSignal.Reader. Written once, like Kind.
	// +optional
	Reader string `json:"reader,omitempty"`
	// Payload is this signal's content, handed to the agent. An absorbed
	// occurrence APPENDS its own payload here (the same join
	// httpapi.combineJoined already applies within one batch, now applied
	// across occurrences over this Signal's open lifetime) rather than
	// creating a second CR.
	//
	// Left unbounded like ConversationInputSpec.Payload: this is the bulky
	// half of a signal, out-of-line from the lighter-weight objects (status
	// subresources, Conversations) that reference it. The per-source live
	// cap plus absorb-by-id (design D-6) are what keep its growth bounded in
	// practice, not a field-level limit.
	// +optional
	Payload string `json:"payload,omitempty"`
	// Occurrences counts how many signal arrivals have landed on this CR —
	// 1 for the arrival that created it, bumped by every later absorb
	// (design D-5). Never decremented.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Occurrences int32 `json:"occurrences,omitempty"`
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

// ClaimantState is one claimant's own routing workflow (design D-4):
// Routing is non-terminal, the other three are terminal. A retry (the
// reconciler, internal/controller) touches only claimants still Routing.
// +kubebuilder:validation:Enum=Routing;Routed;Failed;Skipped
type ClaimantState string

const (
	// ClaimantRouting: not yet terminal — either never attempted, or
	// attempted and owed a retry.
	ClaimantRouting ClaimantState = "Routing"
	// ClaimantRouted: this claimant opened or appended to a conversation for
	// this signal. Idempotent by equality reuse (conversationId ==
	// initiationId) — a retry that finds a claimant already Routed only
	// confirms it, never re-routes.
	ClaimantRouted ClaimantState = "Routed"
	// ClaimantFailed: this claimant's retry budget is exhausted
	// (SIGNAL_RETRY_DEADLINE) with no successful route, or the per-source
	// live cap evicted it post-fan-out.
	ClaimantFailed ClaimantState = "Failed"
	// ClaimantSkipped: this claimant declined the signal outright rather
	// than failing to route it — e.g. the claimant itself refused capacity
	// for a reason distinct from the retry-exhaustion path that produces
	// Failed.
	ClaimantSkipped ClaimantState = "Skipped"
)

// SignalClaimant is one claimant's own routing workflow for a Signal — the
// state machine Phase derives from. The set is snapshotted ONCE, at the
// signal's first routing attempt: the same principle that pins a
// Conversation's refs at creation, so a later rewire of who serves the
// source never changes an in-flight signal's own fan-out (design D-4).
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
// Status.Claimants — never authored independently (design D-4). See
// RollupPhase.
// +kubebuilder:validation:Enum=Pending;Routed;PartiallyRouted;Dropped;Failed
type SignalPhase string

const (
	// SignalPending: no claimant has routed this signal yet, and nothing has
	// dropped it — either no routing attempt has been made, or every
	// claimant snapshotted so far is still Routing.
	SignalPending SignalPhase = "Pending"
	// SignalRouted: every claimant is terminal, and at least one Routed.
	// Retry exhaustion with at least one routed claimant still rolls up here
	// — the exhausted claimant is Failed, visibly, per claimant (design
	// D-5a).
	SignalRouted SignalPhase = "Routed"
	// SignalPartiallyRouted: at least one claimant has Routed, and at least
	// one is still Routing. Always exits once the retry deadline flips every
	// remaining claimant terminal (design D-4).
	SignalPartiallyRouted SignalPhase = "PartiallyRouted"
	// SignalDropped: this signal never reached a claimant snapshot at all —
	// unclaimed, cooldown or overflow dropped it BEFORE fan-out (design
	// D-5a: "Dropped stays strictly pre-fan-out"), or a chat signal was
	// declined at a full backlog (capacity). See SignalDropReason.
	SignalDropped SignalPhase = "Dropped"
	// SignalFailed: every claimant is terminal and none Routed — reserved
	// for a signal no claimant routed (design D-5a). Surfaced as loudly as
	// the unwired drop.
	SignalFailed SignalPhase = "Failed"
)

// SignalDropReason explains a Dropped phase (design D-5a/D-6/D-8). Set only
// alongside SignalDropped, which — unlike every other phase — precedes any
// claimant snapshot.
// +kubebuilder:validation:Enum=unclaimed;cooldown;overflow;capacity
type SignalDropReason string

const (
	// SignalDropUnclaimed: no Ready Pipeline or Coordinator serves the
	// source.
	SignalDropUnclaimed SignalDropReason = "unclaimed"
	// SignalDropCooldown: suppressed inside the source's cooldown window.
	SignalDropCooldown SignalDropReason = "cooldown"
	// SignalDropOverflow: evicted — the oldest live Signal on this source —
	// to hold the per-source live cap (design D-6).
	SignalDropOverflow SignalDropReason = "overflow"
	// SignalDropCapacity: a chat signal declined because the pending
	// conversation backlog was full (design D-8). Machine kinds never use
	// this reason — they wait as Pending instead.
	SignalDropCapacity SignalDropReason = "capacity"
)

// SignalStatus is the observed, derived state of a Signal.
type SignalStatus struct {
	// Phase is the roll-up over Claimants. See RollupPhase.
	// +optional
	Phase SignalPhase `json:"phase,omitempty"`
	// Reason explains a Dropped phase. Absent for every other phase.
	// +optional
	Reason SignalDropReason `json:"reason,omitempty"`
	// Claimants is the per-claimant routing workflow, snapshotted once at
	// first routing attempt. Empty while Pending (no attempt yet) or Dropped
	// (no claimant ever served the source).
	// +optional
	Claimants []SignalClaimant `json:"claimants,omitempty"`
	// FirstAttemptAt stamps the moment Claimants was snapshotted — the
	// origin the retry deadline (SIGNAL_RETRY_DEADLINE) measures from.
	// Absent while Pending or Dropped.
	// +optional
	FirstAttemptAt *metav1.Time `json:"firstAttemptAt,omitempty"`
}

// RollupPhase derives a signal's phase from its claimants alone — the pure
// function Status.Phase is always written FROM, never independently (design
// D-4), and the one implementation both the inline first routing attempt and
// the reconciler's retries share so they can never disagree about what a
// given claimant set means.
//
// It does NOT decide SignalDropped: a drop happens strictly before any
// claimant exists (design D-5a), so the caller sets SignalDropped (with its
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
// +kubebuilder:printcolumn:name="Occurrences",type=integer,JSONPath=`.spec.occurrences`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Signal is the durable, persist-then-ack record of one signal arrival (and
// every occurrence absorbed into it) and its per-claimant routing workflow.
// Owned by the SignalSource named in spec.sourceRef — set as this object's
// OwnerReference at creation by whatever persists it — so a deleted source
// takes its signals from any phase.
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
