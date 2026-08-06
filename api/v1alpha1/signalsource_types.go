package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// SourceAlertmanager is the built-in, in-process signal type (the manager
// hosts its webhook endpoint). Every other type is served by an external
// signal adapter via the /signal/* contract.
const SourceAlertmanager = "alertmanagerWebhook"

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
type SignalSourceSpec struct {
	// Type names the signal implementation serving this source (e.g.
	// "alertmanagerWebhook" built-in, "cron", "pagerduty", …). The operator
	// never interprets it beyond routing.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type is immutable"
	Type string `json:"type"`
	// +optional
	ChannelRef *ObjectRef `json:"channelRef,omitempty"`
	ProfileRef ObjectRef  `json:"profileRef"`
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

// SignalSourceStatus reports ingest liveness.
type SignalSourceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	LastReceived *metav1.Time `json:"lastReceived,omitempty"`
	// +optional
	ReceivedTotal int64 `json:"receivedTotal,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sigsrc
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
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
