package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentCapabilitySpec is WHAT AN AGENT MAY DO AND UNDER WHOSE IDENTITY — the six
// fields `PipelineSpec` used to carry alone, extracted so a Pipeline and a
// Coordinator can share ONE capability rather than each restating it.
//
// It carries no subscription (no signal sources, no channels) and no
// coordination fields (no `agents[]`, no limits): those are what a Pipeline or
// a Coordinator adds around it. An AgentCapability nothing wires is inert —
// unwired is the ordinary state of one that exists only to be REFERENCED.
type AgentCapabilitySpec struct {
	// ProfileRef: the agent this capability answers as. Optional on the
	// struct because a Pipeline embedding this inline may instead name a
	// `capabilityRef` for the whole capability — CEL cannot express "one of"
	// across an embedded struct, so the two-refs-or-neither check is a Ready
	// condition on the EMBEDDING kind, not admission here.
	// +optional
	ProfileRef *ObjectRef `json:"profileRef,omitempty"`
	// RuntimeRef selects the AgentRuntime executing this capability's
	// conversations. Absent, the AgentRuntime named "default" — the one the
	// parent chart renders — then the manager's bootstrap configuration.
	//
	// IT REPLACES `AgentProfile.spec.runtimeRef`, which is deprecated. An
	// AgentRuntime carries the ServiceAccount an agent runs as, so selecting
	// one is selecting the agent's power in the cluster — and that is a
	// capability decision, made beside the tools and servers the same
	// capability grants, not an attribute of the prompts an agent is written
	// with.
	//
	// The CONVERSATION snapshots the resolved name at creation, so editing
	// this field re-wires only conversations created afterwards. The
	// referenced CR's CONTENT — image, idle TTL, volumes — is re-read at
	// every pod build, so fixing a runtime heals conversations already
	// running.
	// +optional
	RuntimeRef *ObjectRef `json:"runtimeRef,omitempty"`
	// ServiceAccountName is the identity the runtime executes under,
	// OVERRIDING the AgentRuntime's own `serviceAccountName`. Absent, the
	// runtime's — which the chart still defaults to `agentops-runtime`.
	//
	// NAMING IS NOT CREATING. No reconciler creates a ServiceAccount, and
	// nothing here validates that one exists or that its RBAC is sufficient:
	// who may create an account and what it is bound to stays an EXTERNAL
	// grant, the same posture adapters already have. A name nothing backs
	// fails at pod admission, naming the account.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Toolsets binds MCPToolset CRs contributing to the allowlist of this
	// capability's conversations, plus the mode composing them with what the
	// AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces).
	// +optional
	Toolsets *ToolsetBinding `json:"toolsets,omitempty"`
	// MCPConfigs binds MCPConfig CRs supplying this capability's MCP servers,
	// overlaid per server key in ref order (later wins). No mode: an agent
	// definition declares no servers, so there is nothing to compose against.
	// +optional
	MCPConfigs *ToolingBinding `json:"mcpConfigs,omitempty"`
	// Persistence declares WHERE this capability's conversations keep their
	// state — the CONTEXT volume and the WORKSPACE volume, independently.
	//
	// PRECEDENCE, and no other order:
	//
	//	<embedding kind>.persistence.<volume> -> the chart's release default -> ephemeral
	//
	// The CONVERSATION snapshots the RESOLVED claim at creation, so editing
	// this field re-wires only conversations created afterwards. Nothing
	// reads a Pipeline or Coordinator at pod-build time.
	// +optional
	Persistence *PipelinePersistence `json:"persistence,omitempty"`
}

// ProfileName returns the referenced profile's name, or empty when this
// capability names none — a nil-safe convenience for display and comparison
// call sites that need a string, never a substitute for
// dispatch.ResolveCapability where the capability may live behind a
// `capabilityRef` instead of being inlined.
func (s AgentCapabilitySpec) ProfileName() string {
	if s.ProfileRef == nil {
		return ""
	}
	return s.ProfileRef.Name
}

// AgentCapabilityStatus reports whether every reference this capability names
// resolves.
type AgentCapabilityStatus struct {
	// Conditions: Ready (every named reference resolves).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentCapability is a NAMED CAPABILITY — a profile, a runtime, an identity,
// tools and MCP servers, and where its conversations persist — declared once
// and REFERENCED from a Pipeline's or a Coordinator's `capabilityRef`, as an
// alternative to inlining the same six fields on each.
//
// It carries no wiring of its own: no signal sources, no channels. An
// AgentCapability nothing references is inert, exactly as an unwired Channel
// or SignalSource is — that is the ordinary state of one meant to be shared.
type AgentCapability struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentCapabilitySpec   `json:"spec,omitempty"`
	Status AgentCapabilityStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentCapabilityList contains a list of AgentCapability.
type AgentCapabilityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentCapability `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentCapability{}, &AgentCapabilityList{})
}
