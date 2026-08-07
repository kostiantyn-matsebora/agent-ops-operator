package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChannelAdapterSpec declares a channel-type IMPLEMENTATION — nothing more.
// The CR's NAME is the routing key: Channels whose spec.type equals it are
// served by this adapter (one adapter per implementation, by construction).
// No configuration lives here: per-surface settings are on the served
// Channels (config, credentialsSecretRef — projected into the pod by the
// reconciler, kubelet-resolved, never read through the API).
type ChannelAdapterSpec struct {
	// Image implementing the adapter contract.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
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
	// Conditions: Deployed (workload rendered), Ready (workload available).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ServedChannels counts Channels naming this adapter in spec.type.
	// +optional
	ServedChannels int32 `json:"servedChannels,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Channels",type=integer,JSONPath=`.status.servedChannels`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChannelAdapter plugs a channel implementation into the operator: apply one
// naming an image and every Channel whose spec.type equals THIS CR's NAME is
// served by it — no operator or chart change. The reconciler owns the adapter
// Deployment (zero-RBAC ServiceAccount, no SA token automount) and injects
// the manager URL, a per-adapter derived auth token, and each served
// Channel's projected credentials.
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
