package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GroupingSpec controls signature-based conversation grouping and dedupe.
// This is deliberately typed, manager-side policy — it applies to every source
// type, built-in or adapter-fed; adapters only normalize signals.
type GroupingSpec struct {
	// SignatureLabels compose the signature (signal label keys).
	// +optional
	SignatureLabels []string `json:"signatureLabels,omitempty"`
	// WindowDays: reuse an existing conversation with the same signature
	// updated within this window.
	// +kubebuilder:default=7
	// +optional
	WindowDays int32 `json:"windowDays,omitempty"`
	// CooldownHours: suppress identical fingerprints within this window.
	// +kubebuilder:default=6
	// +optional
	CooldownHours int32 `json:"cooldownHours,omitempty"`
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
	// Grouping decides which signals share one conversation, and how long a
	// repeat of the same fingerprint is suppressed.
	// +optional
	Grouping GroupingSpec `json:"grouping,omitempty"`
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
}

// CooldownEntry records when a fingerprint was last admitted, so suppression
// survives a manager restart. MATERIALIZED state — written by ingest, never
// hand-set.
type CooldownEntry struct {
	// Fingerprint identifies the signal being suppressed.
	Fingerprint string `json:"fingerprint"`
	// At is when the fingerprint was last admitted; the window runs from here.
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
	// Cooldown records fingerprint suppression for THIS source. The manager
	// keeps an in-memory map as the hot path, but this is the record: it is
	// loaded on first use per source after a restart, so a restart mid-incident
	// no longer re-opens conversations for signals inside an active window.
	//
	// Bounded and pruned past the window. Only a FRESH fingerprint writes here —
	// and a fresh fingerprint already causes a conversation create or patch — so
	// the suppressed high-volume case costs nothing.
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
