package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalSourceType enumerates ingest kinds.
// +kubebuilder:validation:Enum=alertmanagerWebhook;cron;k8sEvents
type SignalSourceType string

const (
	SourceAlertmanager SignalSourceType = "alertmanagerWebhook"
	SourceCron         SignalSourceType = "cron"
	SourceK8sEvents    SignalSourceType = "k8sEvents"
)

// GroupingSpec controls signature-based conversation grouping and dedupe.
type GroupingSpec struct {
	// SignatureLabels compose the signature (alert labels / event fields).
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

// CronSpec fires a conversation input on a schedule.
type CronSpec struct {
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`
	// Input text handed to the profile (job lane).
	// +optional
	Input string `json:"input,omitempty"`
}

// EventsSpec watches Kubernetes Events as signals.
type EventsSpec struct {
	// Reasons allowlist (e.g. CrashLoopBackOff, OOMKilled, FailedScheduling).
	// +optional
	Reasons []string `json:"reasons,omitempty"`
	// Namespaces allowlist; empty = all.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`
}

// SignalSourceSpec maps a signal stream to conversations with a profile.
type SignalSourceSpec struct {
	Type SignalSourceType `json:"type"`
	// +optional
	ChannelRef *ObjectRef `json:"channelRef,omitempty"`
	ProfileRef ObjectRef  `json:"profileRef"`
	// +optional
	Grouping GroupingSpec `json:"grouping,omitempty"`
	// +optional
	Cron *CronSpec `json:"cron,omitempty"`
	// +optional
	Events *EventsSpec `json:"events,omitempty"`
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

// SignalSource is an ingest lane (Alertmanager webhook, cron, k8s Events).
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
