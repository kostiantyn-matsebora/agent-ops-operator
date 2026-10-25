package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AttributeFilterOperator is the matching test an AttributeFilterRequirement
// applies — the same four operators a Kubernetes matchExpressions entry
// supports. An owned type rather than metav1.LabelSelectorRequirement's own
// Operator field, so the CRD schema carries a real enum: the upstream field
// has no +kubebuilder:validation:Enum marker, and embedding it verbatim would
// accept any string, silently ignoring one it cannot evaluate.
// +kubebuilder:validation:Enum=In;NotIn;Exists;DoesNotExist
type AttributeFilterOperator string

const (
	AttributeFilterIn           AttributeFilterOperator = "In"
	AttributeFilterNotIn        AttributeFilterOperator = "NotIn"
	AttributeFilterExists       AttributeFilterOperator = "Exists"
	AttributeFilterDoesNotExist AttributeFilterOperator = "DoesNotExist"
)

// AttributeFilterRequirement is one matchExpressions-shaped test against a
// signal's normalized labels. Values is required for In/NotIn and must stay
// empty for Exists/DoesNotExist — that arity rule is NOT enforced by the CRD
// schema (only Operator's enum is), so a malformed requirement is instead
// evaluated as never-satisfied at ingest (fail closed, never open).
type AttributeFilterRequirement struct {
	// Key is the attribute label name to test.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
	// Operator selects the test. See AttributeFilterOperator.
	Operator AttributeFilterOperator `json:"operator"`
	// Values is the comparison set for In/NotIn, and must be empty for
	// Exists/DoesNotExist.
	// +optional
	Values []string `json:"values,omitempty"`
}

// SignalSourceSpec maps a signal stream to conversations with a profile:
// type-agnostic routing metadata plus an opaque per-type config that only the
// serving signal implementation interprets.
//
// The source carries NO wiring — which profile answers and which channels
// mirror is declared exclusively on a Pipeline that claims this source.
// Unclaimed sources drop signals (Wired=False condition).
type SignalSourceSpec struct {
	// Adapter names the SignalAdapter serving this source — a REFERENCE, not an
	// attribute: the named adapter's implementation defines and validates
	// Config's schema. Sibling of Config by design (see Channel.Adapter).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.adapter is immutable"
	Adapter string `json:"adapter"`
	// CooldownHours: suppress a repeat of the same initiationId within this
	// window — a re-send, safe to drop under at-least-once delivery. A repeat
	// outside the window is a re-notification and routes by id equality
	// instead. The one surviving knob, promoted out of the removed Grouping
	// stanza — the source carries no grouping policy any more, only this
	// durability setting. Chat/task sources keep an effective 0 (one-shot, no
	// suppression) via their own origination path rather than this field.
	// +kubebuilder:default=6
	// +optional
	CooldownHours int32 `json:"cooldownHours,omitempty"`
	// CredentialsSecretRef names the Secret holding this source's transport
	// credentials (e.g. an API key). The operator only writes the NAME into
	// the serving adapter's pod spec (kubelet-resolved envFrom projection);
	// nothing reads the Secret's values through the API.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// Config carries whatever the signal type needs; schema-less by design.
	// Validated by the serving adapter, never by the operator.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config *runtime.RawExtension `json:"config,omitempty"`
	// AttributeFilters narrows which signals this source admits, evaluated
	// against the normalized signal's labels with Kubernetes label-selector
	// semantics. A signal must satisfy EVERY requirement to be admitted.
	// Empty or unset admits everything — identical to today's behavior.
	// Evaluated before cooldown and before routing: a filtered signal spends
	// no cooldown state and never reaches a serving pipeline.
	// +optional
	AttributeFilters []AttributeFilterRequirement `json:"attributeFilters,omitempty"`
	// LogFiltered governs whether a signal dropped by AttributeFilters is
	// still recorded as a signal.dropped activity event — unset means true.
	// Set false to silence a known-noisy filter. Has no effect on any other
	// drop reason, which always logs.
	// +optional
	LogFiltered *bool `json:"logFiltered,omitempty"`
}

// CooldownEntry records when an initiationId was last admitted, so
// suppression survives a manager restart. MATERIALIZED state — written by
// ingest, never hand-set.
type CooldownEntry struct {
	// InitiationID identifies the signal being suppressed — the sender-minted
	// correlation id cooldown now keys on, replacing the retired fingerprint.
	InitiationID string `json:"initiationId"`
	// At is when this id was last admitted; the window runs from here.
	At metav1.Time `json:"at"`
}

// SignalSourceStatus reports ingest liveness.
type SignalSourceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	LastReceived *metav1.Time `json:"lastReceived,omitempty"`
	// +optional
	ReceivedTotal int64 `json:"receivedTotal,omitempty"`
	// Cooldown records initiationId suppression for THIS source. The manager
	// keeps an in-memory map as the hot path, but this is the record: it is
	// loaded on first use per source after a restart, so a restart mid-incident
	// no longer re-opens conversations for signals inside an active window.
	//
	// Bounded and pruned past the window. Only a FRESH id writes here — and a
	// fresh id already causes a conversation create or patch — so the
	// suppressed high-volume case costs nothing.
	// +optional
	Cooldown []CooldownEntry `json:"cooldown,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sigsrc
// +kubebuilder:printcolumn:name="Adapter",type=string,JSONPath=`.spec.adapter`
// +kubebuilder:printcolumn:name="Wired",type=string,JSONPath=`.status.conditions[?(@.type=="Wired")].status`
// +kubebuilder:printcolumn:name="Served",type=string,JSONPath=`.status.conditions[?(@.type=="Served")].status`
// +kubebuilder:printcolumn:name="Received",type=integer,JSONPath=`.status.receivedTotal`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalSource is an ingest lane served by a signal-type implementation
// (built-in Alertmanager webhook or an external signal adapter).
type SignalSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalSourceSpec   `json:"spec,omitempty"`
	Status SignalSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SignalSourceList contains a list of SignalSource.
type SignalSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SignalSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SignalSource{}, &SignalSourceList{})
}
