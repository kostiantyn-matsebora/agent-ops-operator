package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineSpec declares the wiring between the pipeline elements: every
// referenced signal source's signals become conversations bound to ALL
// referenced channels with the pipeline's profile, and conversations started
// from any referenced channel are bound to all of them (full mirroring).
// Runtime selection stays profile.runtimeRef — the Pipeline binds no runtime,
// credentials, or config.
type PipelineSpec struct {
	// SignalSourceRefs: the sources feeding this pipeline. A source is
	// SHAREABLE exactly as a channel is — any number of pipelines may list
	// one, and a signal admitted there opens a conversation on EVERY Ready
	// pipeline listing it, each with its own profile and capabilities. Listing
	// a source means "I watch this", not "I own this".
	// +optional
	SignalSourceRefs []ObjectRef `json:"signalSourceRefs,omitempty"`
	// ChannelRefs: every conversation of this pipeline is mirrored on all of
	// these surfaces. Channels may appear in several pipelines.
	// +optional
	ChannelRefs []ObjectRef `json:"channelRefs,omitempty"`
	// ProfileRef: the agent answering the conversations this pipeline
	// originates — those from the signal sources it WATCHES, and those a chat
	// command addresses to it by name. Channels supply no default.
	ProfileRef ObjectRef `json:"profileRef"`
	// Toolsets binds MCPToolset CRs contributing to the allowlist of this
	// wiring's conversations, plus the mode composing them with what the
	// AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces).
	// +optional
	Toolsets *ToolsetBinding `json:"toolsets,omitempty"`
	// MCPConfigs binds MCPConfig CRs supplying this wiring's MCP servers,
	// overlaid per server key in ref order (later wins). No mode: an agent
	// definition declares no servers, so there is nothing to compose against.
	// +optional
	MCPConfigs *ToolingBinding `json:"mcpConfigs,omitempty"`
}

// PipelineStatus reports wiring validity.
type PipelineStatus struct {
	// Conditions: Ready (all references resolve). There is no SourceConflict
	// condition — sources are shareable, so listing one another pipeline also
	// lists is a valid configuration, not a conflict.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Pipeline binds signal sources, channels, and an agent profile into one
// declared flow: N sources fan into conversations mirrored across M channels.
type Pipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PipelineSpec   `json:"spec,omitempty"`
	Status PipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipeline.
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
