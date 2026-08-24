package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineSpec declares the wiring between the pipeline elements: every
// referenced signal source's signals become conversations bound to ALL
// referenced channels with the pipeline's profile, and conversations started
// from any referenced channel are bound to all of them (full mirroring).
//
// It also selects WHAT EXECUTES those conversations and UNDER WHOSE IDENTITY —
// `runtimeRef` and `serviceAccountName`. Capabilities and execution identity
// are the same decision: one says which tools may be called, the other with
// whose credentials, and split across two objects no single object states an
// agent's power. The Pipeline still carries no credentials and no server or
// tool definitions.
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
	// RuntimeRef selects the AgentRuntime executing this wiring's
	// conversations. Absent, the AgentRuntime named "default" — the one the
	// parent chart renders — then the manager's bootstrap configuration.
	//
	// IT REPLACES `AgentProfile.spec.runtimeRef`, which is deprecated. An
	// AgentRuntime carries the ServiceAccount an agent runs as, so selecting
	// one is selecting the agent's power in the cluster — and that is a wiring
	// decision, made beside the tools and servers the same route grants, not an
	// attribute of the prompts an agent is written with.
	//
	// The CONVERSATION snapshots the resolved name at creation, so editing this
	// field re-wires only conversations created afterwards. The referenced CR's
	// CONTENT — image, idle TTL, volumes — is re-read at every pod build, so
	// fixing a runtime heals conversations already running.
	// +optional
	RuntimeRef *ObjectRef `json:"runtimeRef,omitempty"`
	// ServiceAccountName is the identity the runtime executes under,
	// OVERRIDING the AgentRuntime's own `serviceAccountName`. Absent, the
	// runtime's — which the chart still defaults to `agentops-runtime`.
	//
	// This is what makes one runtime image serve several trust levels: an
	// observing route and an acting route differ in their account, not in their
	// image, so the second no longer needs a cloned AgentRuntime to carry it.
	//
	// NAMING IS NOT CREATING. No reconciler creates a ServiceAccount, and
	// nothing here validates that one exists or that its RBAC is sufficient:
	// who may create an account and what it is bound to stays an EXTERNAL
	// grant, the same posture adapters already have. A name nothing backs fails
	// at pod admission, naming the account.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
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
	// Persistence declares WHERE this route's conversations keep their state —
	// the CONTEXT volume and the WORKSPACE volume, independently.
	//
	// It sits here for the reason `runtimeRef` and `serviceAccountName` do: a
	// runtime is an engine, and where a route persists is the route's decision.
	// An AgentRuntime declares neither volume, so two Pipelines sharing one
	// runtime keep their conversations on different volumes without cloning it.
	//
	// PRECEDENCE, and no other order:
	//
	//	pipeline.spec.persistence.<volume> -> the chart's release default -> ephemeral
	//
	// The CONVERSATION snapshots the RESOLVED claim at creation, so editing
	// this field re-wires only conversations created afterwards. Nothing reads
	// a Pipeline at pod-build time.
	// +optional
	Persistence *PipelinePersistence `json:"persistence,omitempty"`
}

// PersistenceBinding says where ONE of a route's two volumes comes from, and
// which of the two spellings is used decides who renders the claim.
//
// A pod can mount only a CLAIM, never a PersistentVolume — so naming a volume
// requires that something render the claim on it. That something is THE
// MANAGER when the binding is on a Pipeline, and the CHART when it is in
// values. This is the one place in this system where naming a resource creates
// it, and it is stated rather than smuggled.
//
// The claim the manager renders carries NO ownerRef on the Pipeline. Deleting
// a Pipeline must never delete the accumulated context of the conversations it
// started — storage is the one thing here whose loss cannot be repaired by
// reconciling again.
//
// +kubebuilder:validation:XValidation:rule="!(has(self.claimName) && has(self.volumeName))",message="claimName and volumeName are mutually exclusive: name a claim that already exists, or a PersistentVolume for the manager to render a claim on"
type PersistenceBinding struct {
	// ClaimName is a PersistentVolumeClaim that ALREADY EXISTS. Nothing is
	// created; conversations this route originates mount it.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	ClaimName string `json:"claimName,omitempty"`
	// VolumeName is a PersistentVolume the manager renders a claim on, bound to
	// that volume by name with an EXPLICIT empty storage class — which is what
	// disables dynamic provisioning. An absent storage class is filled in by
	// the cluster's default StorageClass, which provisions a second volume and
	// leaves the operator's untouched.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	VolumeName string `json:"volumeName,omitempty"`
	// Size requested by the claim the manager renders for VolumeName. Ignored
	// with ClaimName, where nothing is rendered. Empty requests 5Gi — a claim
	// binding to a pre-created volume gets that volume's capacity whatever it
	// asks for, so this is a floor rather than a size.
	// +optional
	Size string `json:"size,omitempty"`
	// AccessModes for that claim. Empty is ReadWriteMany, which is what
	// concurrent conversations on one volume need.
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// StorageClassName on that claim. EMPTY RENDERS AN EXPLICIT EMPTY STRING,
	// which is the only value that binds to a pre-created volume — and it is
	// the default here rather than in the chart because this field only ever
	// accompanies VolumeName, where anything else is a mistake. An absent field
	// is filled in by admission with the cluster's default StorageClass, which
	// provisions a second volume beside the one that was named.
	//
	// Set it only for a class whose volumes are pre-created and selected by
	// name, which some CSI drivers require.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	StorageClassName string `json:"storageClassName,omitempty"`
}

// PipelinePersistence declares where the conversations a route originates keep
// their state — INDEPENDENTLY for the two volumes, because they hold different
// kinds of state and are enabled on different terms.
//
// PERSISTENCE IS WIRING. It sits beside the tools this route grants, the
// channels it delivers to, the runtime it selects and the identity it executes
// under, for the reason all four are here: whoever is trusted to grant an agent
// tools and a cluster identity is more qualified to say where its context
// lives, not less.
//
// The chain is exactly:
//
//	pipeline.spec.persistence.<volume>  ->  the chart's release default  ->  ephemeral
//
// THE RUNTIME IS IN NO PART OF IT. It carries no volume at all; it declares
// only `contextStorage`, the SHAPE of its backend's storage.
//
// The CONVERSATION snapshots what this resolved to at creation, exactly as it
// snapshots the execution identity. Without that, editing a Pipeline changes
// which volume an INFLIGHT conversation's next pod mounts — a storage change
// applied to work that has already written to the old one.
type PipelinePersistence struct {
	// Context: a conversation's accumulated context, the thing
	// `runtimeContextId` is a handle into. Absent takes the release default.
	// +optional
	Context *PersistenceBinding `json:"context,omitempty"`
	// Workspace: the repository checkout, one subdirectory per conversation.
	// Absent takes the release default, which is ephemeral unless the install
	// turned workspace persistence on.
	// +optional
	Workspace *PersistenceBinding `json:"workspace,omitempty"`
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
