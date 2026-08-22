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
	// Icon is how this Pipeline is RECOGNISED in a list of them. Optional, and
	// purely how the name is presented.
	//
	// A REFERENCE, not an image. Four forms, and the manager tells them apart
	// not at all — it publishes the string and interprets it no further:
	//
	//	aops:kubernetes            the built-in set, shipped inside each surface
	//	mdi:kubernetes             a named icon from a public set
	//	https://example/logo.svg   your own, by URL
	//	🔎                         an emoji, drawable by anything
	//
	// WHAT A SURFACE CAN DRAW IS THE SURFACE'S BUSINESS. `aops:` and an emoji
	// work everywhere, because every adapter ships the first and every
	// transport can print the second. Telegram can draw neither a URL nor a
	// named set — a command menu takes no image — so it renders what it can and
	// omits the rest. Nothing fails over an icon.
	//
	// Prefer `aops:` for anything shipped: it needs no network, survives an
	// air-gapped install, and is the only form guaranteed on every surface.
	//
	// It is INTERFACE METADATA, not wiring, and it does not weaken the rule
	// that this CR carries the wiring exclusively: nothing routes on it, no
	// condition reads it, and removing it changes where not one signal goes.
	// Same category as `ChannelAdapter.spec.configSchema`.
	//
	// It lives HERE rather than on the profile because a Pipeline is what a
	// message addresses, so a Pipeline is what appears in a menu.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	Icon string `json:"icon,omitempty"`
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
