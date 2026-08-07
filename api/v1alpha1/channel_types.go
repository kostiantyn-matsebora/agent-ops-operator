package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ChannelSpec configures one chat surface: type-agnostic metadata plus an
// opaque per-type config that only the channel implementation interprets.
//
// A Channel describes WHERE output goes, never HOW it is sent: delivery is the
// operator's job (it hands agent output to the serving adapter), so no agent
// ever learns a transport and no runtime holds a surface's credentials.
type ChannelSpec struct {
	// Adapter names the ChannelAdapter serving this surface — a REFERENCE, not
	// an attribute: the named adapter's implementation is what defines and
	// validates Config's schema. The operator never interprets it beyond
	// routing. (Sibling of Config by design, as Kubernetes pairs a selector
	// with its implementation-owned config: StorageClass.provisioner +
	// parameters, IngressClass.controller + parameters.)
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.adapter is immutable"
	Adapter string `json:"adapter"`
	// NOTE: the channel carries NO wiring — bare messages are answered by the
	// profile of the oldest Ready Pipeline referencing this channel; channels
	// in no pipeline answer bare messages with guidance only.
	// CredentialsSecretRef names the Secret holding this surface's transport
	// credentials (e.g. a bot token) — credentials are per-surface usage, never
	// per-implementation. The operator only writes the NAME into the serving
	// adapter's pod spec (kubelet-resolved envFrom projection); nothing reads
	// the Secret's values through the API.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// Config carries whatever the channel type needs; schema-less by design.
	// Validated by the serving adapter, never by the operator.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config *runtime.RawExtension `json:"config,omitempty"`
}

// ChannelStatus reports connectivity (conditions are owned by the serving
// channel implementation, via the adapter contract).
type ChannelStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Adapter",type=string,JSONPath=`.spec.adapter`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Channel is a chat surface served by a channel-type implementation (built-in
// or an external adapter).
type Channel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChannelSpec   `json:"spec,omitempty"`
	Status ChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChannelList contains a list of Channel.
type ChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Channel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Channel{}, &ChannelList{})
}
