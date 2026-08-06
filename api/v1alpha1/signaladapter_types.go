package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalAdapterSpec declares a signal-type implementation: the container image
// serving the /signal/* contract for one spec.type. Pure implementation —
// credentials never live here; they are declared per source on
// SignalSource.credentialsSecretRef and projected into the adapter pod by the
// reconciler (kubelet-resolved, never read through the API).
type SignalAdapterSpec struct {
	// Type is the signal type this adapter serves. Exactly one active adapter
	// per type.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type is immutable"
	Type string `json:"type"`
	// Image implementing the signal adapter contract for this type.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Env: non-secret tuning only (e.g. LOG_LEVEL). Entries referencing
	// Secrets are rejected — credentials belong on
	// SignalSource.credentialsSecretRef.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Singleton runs the workload as replicas 1 + strategy Recreate so no
	// rollout ever runs two instances side by side (pollers and schedulers
	// must not double-fire).
	// +kubebuilder:default=true
	// +optional
	Singleton *bool `json:"singleton,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SignalAdapterStatus reports workload and serving state.
type SignalAdapterStatus struct {
	// Conditions: Deployed (workload rendered), Ready (workload available),
	// TypeConflict (another adapter already serves the type).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ServedSources counts SignalSources of this adapter's type.
	// +optional
	ServedSources int32 `json:"servedSources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.status.servedSources`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalAdapter plugs a signal type into the operator: apply one naming an
// image and every SignalSource with the matching spec.type is served by it —
// no operator or chart change. The reconciler owns the adapter Deployment
// (zero-RBAC ServiceAccount, no SA token automount) and injects the manager
// URL, a per-adapter derived auth token, and each served source's projected
// credentials.
type SignalAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalAdapterSpec   `json:"spec,omitempty"`
	Status SignalAdapterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SignalAdapterList contains a list of SignalAdapter.
type SignalAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SignalAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SignalAdapter{}, &SignalAdapterList{})
}
