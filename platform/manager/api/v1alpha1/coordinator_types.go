package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CoordinatorAgentEntry names one agent a Coordinator's own conversation may
// INVOKE. It is never wiring for a conversation that already exists — the
// invoke is what creates one.
//
// Its capability comes from EXACTLY one of the two refs, mutually exclusive
// by CEL: `capabilityRef` for an ordinary member, `coordinatorRef` for
// NESTING — the invoked conversation is itself that Coordinator's root, and
// may in turn invoke its OWN `agents[]`. There is no separate "sub-Coordinator"
// kind; nesting is this same entry naming a Coordinator instead of an
// AgentCapability.
//
// +kubebuilder:validation:XValidation:rule="has(self.capabilityRef) != has(self.coordinatorRef)",message="exactly one of capabilityRef or coordinatorRef"
type CoordinatorAgentEntry struct {
	// Name is how the `invoke` verb addresses this entry — unique within the
	// Coordinator, never a Kubernetes identifier.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
	// CapabilityRef names the AgentCapability this entry invokes as an
	// ordinary member — one with no `agents[]` of its own.
	// +optional
	CapabilityRef *ObjectRef `json:"capabilityRef,omitempty"`
	// CoordinatorRef names another Coordinator this entry invokes AS A
	// MEMBER. The invoked conversation is that Coordinator's own root, so it
	// may invoke further — nesting, bounded only by a cycle in the
	// Coordinator graph (refused both statically, on this object's own
	// `Ready`, and again at `invoke` time against the live `causedBy` chain).
	// +optional
	CoordinatorRef *ObjectRef `json:"coordinatorRef,omitempty"`
	// Description tells the COORDINATING AGENT what this entry IS and can do —
	// its purpose and reach, what it cannot do, and what to hand it — so that a
	// person's instruction matches it as readily as a signal does. It is read
	// by the agent and parsed by nothing here. Four parts need room: the bound
	// was 512 bytes and the chart's own purpose-shaped texts did not fit.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Description string `json:"description"`
}

// CoordinatorLimits bounds ONE Coordinator's own conversation tree, evaluated
// per level: a member that is itself a Coordinator's root enforces its OWN
// limits, independent of its ancestor's — nesting never pools a budget across
// levels.
//
// Enforcement is a later phase of this change; this struct is the schema the
// budget edges (`invoke`, `handleWorkDone`, the reconciler's deadline requeue)
// will read. A zero field here means unset, not zero — the enforcing code
// picks the default, which is why none is declared as a CRD default yet: the
// design leaves the deadline default an open question.
type CoordinatorLimits struct {
	// MaxAgents: the ceiling on how many agents this Coordinator's own
	// conversation may invoke over its lifetime (`status.budget.agentsInvoked`).
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxAgents int32 `json:"maxAgents,omitempty"`
	// MaxTurns: the ceiling on `status.budget.turns`.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTurns int32 `json:"maxTurns,omitempty"`
	// Deadline: how long after creation this Coordinator's own conversation
	// may run before it is closed `budget-exceeded`.
	// +optional
	Deadline *metav1.Duration `json:"deadline,omitempty"`
}

// CoordinatorSpec declares a COORDINATING agent: its own capability, the
// sources it claims, the channels a conversation it opens binds, and the
// agents its own conversation may invoke.
//
// An UNCAUSED root this Coordinator creates binds `ChannelRefs`
// UNCONDITIONALLY at creation (coordinator-unconditional-channels) — the
// same moment a Pipeline's own `channelRefs` bind — so a human can reach any
// open coordinator conversation whether or not its agent ever calls
// `escalate`. It is also snapshotted onto the root as
// `spec.escalationChannelRefs`, kept as separate provenance of the
// Coordinator's own declared set. A member (one `invoke` created) still
// binds no channel at all.
//
// +kubebuilder:validation:XValidation:rule="!(has(self.capabilityRef) && (has(self.profileRef) || has(self.runtimeRef) || has(self.serviceAccountName) || has(self.toolsets) || has(self.mcpConfigs) || has(self.persistence)))",message="capabilityRef and the inline capability fields (profileRef, runtimeRef, serviceAccountName, toolsets, mcpConfigs, persistence) are mutually exclusive"
type CoordinatorSpec struct {
	// SignalSourceRefs: the sources this Coordinator claims, exactly as a
	// Pipeline's own — shareable, so a Pipeline and a Coordinator may claim
	// the same source without conflict.
	// +optional
	SignalSourceRefs []ObjectRef `json:"signalSourceRefs,omitempty"`
	// ChannelRefs are the surfaces a conversation this Coordinator opens
	// binds at creation, unconditionally — see the type doc comment.
	// +optional
	ChannelRefs []ObjectRef `json:"channelRefs,omitempty"`
	// AgentCapabilitySpec is the CAPABILITY for the COORDINATING agent
	// itself — the one that reads what started the conversation and decides
	// which of `agents[]` to invoke. Mutually exclusive with capabilityRef,
	// exactly as on a Pipeline.
	// +optional
	AgentCapabilitySpec `json:",inline"`
	// AgentRef (`capabilityRef` in JSON) names an AgentCapability holding the
	// coordinating agent's capability INSTEAD of inlining it above.
	// +optional
	AgentRef *ObjectRef `json:"capabilityRef,omitempty"`
	// Agents this Coordinator's own conversation may invoke.
	// +optional
	Agents []CoordinatorAgentEntry `json:"agents,omitempty"`
	// Limits bounds this Coordinator's own conversation tree.
	// +optional
	Limits *CoordinatorLimits `json:"limits,omitempty"`
}

// CoordinatorStatus reports whether this Coordinator's own capability and
// every `agents[]` entry resolves to something itself Ready.
type CoordinatorStatus struct {
	// Conditions: Ready — see CoordinatorSpec and the reconciler's own
	// documentation for exactly what it checks.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Coordinator declares a coordinating agent: a capability, the sources it
// claims, and a named set of agents its own conversation may invoke to fan a
// task out and read the results back. It reconciles like a Pipeline —
// wiring validation only, no ownerRef on anything it later causes.
//
// A Coordinator nobody applies changes nothing: it originates no
// conversation on its own, and deleting one cascades nothing — open roots it
// already created keep running on their own snapshot until their own budget
// or a person closes them.
type Coordinator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CoordinatorSpec   `json:"spec,omitempty"`
	Status CoordinatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CoordinatorList contains a list of Coordinator.
type CoordinatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Coordinator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Coordinator{}, &CoordinatorList{})
}

// CapabilityRef names the AgentCapability this Coordinator references INSTEAD
// of inlining a capability, or nil when it inlines one. Satisfies
// dispatch.CapabilityHolder.
func (c *Coordinator) CapabilityRef() *ObjectRef { return c.Spec.AgentRef }

// InlineCapability returns this Coordinator's own embedded capability fields.
// Meaningless when CapabilityRef is non-nil. Satisfies
// dispatch.CapabilityHolder.
func (c *Coordinator) InlineCapability() AgentCapabilitySpec { return c.Spec.AgentCapabilitySpec }
