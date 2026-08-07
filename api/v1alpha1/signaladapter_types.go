package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalAdapterSpec declares a signal-type IMPLEMENTATION — nothing more.
// The CR's NAME is the routing key: SignalSources whose spec.type equals it
// are served by this adapter (one adapter per implementation, by
// construction). No configuration lives here: per-source settings are on the
// served SignalSources (config, credentialsSecretRef — projected into the
// pod by the reconciler, kubelet-resolved, never read through the API).
type SignalAdapterSpec struct {
	// Image implementing the signal adapter contract.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Port the image's own HTTP surface listens on (webhook-receiving
	// implementations). When set, the reconciler owns a Service
	// agentops-signal-<name> targeting it and injects LISTEN_ADDR — enabling
	// the adapter is a complete appliance. Unset = no inbound surface (e.g.
	// cron).
	// +optional
	Port *int32 `json:"port,omitempty"`
	// KubernetesAccess declares that this implementation talks to the
	// Kubernetes API (e.g. to register itself with a sender). When true the
	// reconciler mounts the SA token and injects POD_NAMESPACE — and grants
	// NOTHING: permissions are bound externally (chart or user) against the
	// deterministic SA name agentops-signal-<name>.
	// +optional
	KubernetesAccess *bool `json:"kubernetesAccess,omitempty"`
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
	// Conditions: Deployed (workload rendered), Ready (workload available).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ServedSources counts SignalSources naming this adapter in spec.type.
	// +optional
	ServedSources int32 `json:"servedSources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.status.servedSources`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalAdapter plugs a signal implementation into the operator: apply one
// naming an image and every SignalSource whose spec.type equals THIS CR's
// NAME is served by it — no operator or chart change. The reconciler owns the
// adapter Deployment (zero-RBAC ServiceAccount, no SA token automount), the
// Service when port is declared, and injects the manager URL, a per-adapter
// derived auth token, and each served source's projected credentials.
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
