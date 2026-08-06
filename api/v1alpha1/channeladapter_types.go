package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChannelAdapterSpec declares a channel-type implementation: the container
// image serving the adapter contract for one spec.type. Pure implementation —
// credentials never live here; they are declared per surface on
// Channel.credentialsSecretRef and projected into the adapter pod by the
// reconciler (kubelet-resolved, never read through the API).
type ChannelAdapterSpec struct {
	// Type is the channel type this adapter serves (the /channel/ops routing
	// key). Exactly one active adapter per type.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type is immutable"
	Type string `json:"type"`
	// Image implementing the adapter contract for this type.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Env: non-secret tuning only (e.g. LOG_LEVEL). Entries referencing
	// Secrets are rejected — credentials belong on Channel.credentialsSecretRef.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Singleton runs the workload as replicas 1 + strategy Recreate so no
	// rollout ever runs two instances side by side (required for pull-based
	// transports like Telegram getUpdates).
	// +kubebuilder:default=true
	// +optional
	Singleton *bool `json:"singleton,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ChannelAdapterStatus reports workload and serving state.
type ChannelAdapterStatus struct {
	// Conditions: Deployed (workload rendered), Ready (workload available),
	// TypeConflict (another adapter already serves the type).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ServedChannels counts Channels of this adapter's type.
	// +optional
	ServedChannels int32 `json:"servedChannels,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Channels",type=integer,JSONPath=`.status.servedChannels`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChannelAdapter plugs a channel type into the operator: apply one naming an
// image and every Channel with the matching spec.type is served by it — no
// operator or chart change. The reconciler owns the adapter Deployment
// (zero-RBAC ServiceAccount, no SA token automount) and injects the manager
// URL, a per-adapter derived auth token, and each served Channel's projected
// credentials.
type ChannelAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChannelAdapterSpec   `json:"spec,omitempty"`
	Status ChannelAdapterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChannelAdapterList contains a list of ChannelAdapter.
type ChannelAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChannelAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChannelAdapter{}, &ChannelAdapterList{})
}
