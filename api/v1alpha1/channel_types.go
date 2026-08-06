package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeliveryMode selects how agent replies reach the channel.
// +kubebuilder:validation:Enum=result;agent
type DeliveryMode string

const (
	// DeliveryResult: the agent's printed answer is the deliverable — captured
	// via POST /work/done and surfaced by the channel implementation.
	DeliveryResult DeliveryMode = "result"
	// DeliveryAgent: the agent posts to the chat surface itself, following the
	// channel's AgentInstructions.
	DeliveryAgent DeliveryMode = "agent"
)

// DeliverySpec is the type-agnostic hint for how dispatched prompts should
// instruct the agent to deliver its answer.
type DeliverySpec struct {
	// +kubebuilder:default=result
	// +optional
	Mode DeliveryMode `json:"mode,omitempty"`
	// AgentInstructions is injected verbatim as the prompt's delivery section
	// when Mode is "agent" (e.g. Bot API curl steps).
	// +optional
	AgentInstructions string `json:"agentInstructions,omitempty"`
}

// ChannelSpec configures one chat surface: type-agnostic metadata plus an
// opaque per-type config that only the channel implementation interprets.
type ChannelSpec struct {
	// Type names the channel implementation serving this channel (e.g.
	// "telegram"). The operator never interprets it beyond routing.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type is immutable"
	Type string `json:"type"`
	// NOTE: the channel carries NO wiring — bare messages are answered by the
	// profile of the oldest Ready Pipeline referencing this channel; channels
	// in no pipeline answer bare messages with guidance only.
	// +optional
	Delivery *DeliverySpec `json:"delivery,omitempty"`
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
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
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
