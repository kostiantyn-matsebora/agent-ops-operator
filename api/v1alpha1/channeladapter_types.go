package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	// Port the image's own HTTP surface listens on (implementations that are
	// PUSHED to rather than polling — e.g. a channel adapter receiving updates
	// forwarded by an ingest router). When set, the reconciler owns a Service
	// agentops-adapter-<name> targeting it and injects LISTEN_ADDR, so enabling
	// the adapter is a complete appliance and the chart ships no connectivity.
	// Unset = no inbound surface. Identical semantics to SignalAdapter.port.
	// +optional
	Port *int32 `json:"port,omitempty"`
	// KubernetesAccess declares that this implementation talks to the
	// Kubernetes API (e.g. a console rendering agentops CRs). When true the
	// reconciler mounts the SA token and injects POD_NAMESPACE — and grants
	// NOTHING: permissions are bound externally (chart or user) against the
	// deterministic SA name agentops-adapter-<name>. Identical semantics to
	// SignalAdapter.kubernetesAccess.
	// +optional
	KubernetesAccess *bool `json:"kubernetesAccess,omitempty"`
	// Singleton runs the workload as replicas 1 + strategy Recreate so no
	// rollout ever runs two instances side by side (required for pull-based
	// transports like Telegram getUpdates).
	// +kubebuilder:default=true
	// +optional
	Singleton *bool `json:"singleton,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// ConfigSchema is a JSON Schema (draft 2020-12) describing spec.config on
	// the Channels/SignalSources this adapter serves. OPTIONAL — declaring
	// nothing behaves exactly as before. This is interface metadata, not
	// configuration: it holds no config values, connectivity, or credentials,
	// so the CR stays pure implementation. Because it lives on the spec it is
	// readable by any cluster client (kubectl, docs tooling) the moment the CR
	// is applied — no registration step, and the adapter binary plays no part.
	// Authoring rule: bump the schema in the same diff as `image`.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ConfigSchema *runtime.RawExtension `json:"configSchema,omitempty"`
	// CredentialKeys documents the Secret keys the implementation expects in a
	// served CR's credentialsSecretRef. Documentation ONLY — the manager reads
	// no Secrets, so it can never verify these.
	// +optional
	CredentialKeys []CredentialKeyDoc `json:"credentialKeys,omitempty"`
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
