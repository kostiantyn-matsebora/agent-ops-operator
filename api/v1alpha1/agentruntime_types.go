package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HomeVolume selects durable agent state storage for runtime pods.
type HomeVolume struct {
	// PVCRef mounts an existing (usually RWX) PVC at /data/home.
	// +optional
	PVCRef *ObjectRef `json:"pvcRef,omitempty"`
	// EmptyDir (default when no pvcRef): session state dies with the pod.
	// +optional
	EmptyDir bool `json:"emptyDir,omitempty"`
}

// WorkspaceVolume selects storage for the repository checkout at
// /data/workspace. Shaped identically to HomeVolume, and deliberately a
// SEPARATE claim: the two hold different kinds of state and are enabled
// independently (sessions by default, checkouts on request).
//
// Each conversation gets its own subdirectory within the claim, so concurrent
// runtime pods never share a working tree. The mount path does not move —
// claude-code keys sessions by cwd.
type WorkspaceVolume struct {
	// PVCRef mounts an existing (RWX when conversations run concurrently) PVC,
	// one subdirectory per conversation, at /data/workspace.
	// +optional
	PVCRef *ObjectRef `json:"pvcRef,omitempty"`
	// EmptyDir (default when no pvcRef): the checkout dies with the pod, which
	// costs a re-clone and nothing else.
	// +optional
	EmptyDir bool `json:"emptyDir,omitempty"`
}

// ContextStorage declares WHERE a runtime keeps a conversation's accumulated
// context, which is what decides whether continuity is possible in a given
// deployment.
//
// The RUNTIME declares it rather than the chart inferring it: the chart would
// have to know which images need a volume, and a runtime that keeps context at a
// vendor API needs none and must not be told otherwise.
// +kubebuilder:validation:Enum=volume;external;none
type ContextStorage string

const (
	// ContextOnVolume: context lives on the runtime's home volume, so continuity
	// requires that volume to outlive the pod. claude-code's session files are
	// this case.
	ContextOnVolume ContextStorage = "volume"
	// ContextExternal: context lives somewhere the operator does not provide — a
	// vendor API, a database. Continuity does not depend on any volume here.
	ContextExternal ContextStorage = "external"
	// ContextNone: the runtime cannot continue anything. Every run starts fresh,
	// and saying so is what stops it looking identical to one that lost context.
	ContextNone ContextStorage = "none"
)

// AgentRuntimeSpec defines HOW agents execute: the worker image implementing
// the operator's work contract, and its pod-level defaults. Adopters bring
// their own agent backend (claude-code, aider, custom) by supplying an image
// that:
//
//  1. long-polls  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//  2. executes the returned unit (promptText or promptFile+promptVars against
//     the checked-out repository), streaming progress to STDOUT (pod logs)
//  3. reports    POST $CONTROL_URL/work/done {convo,runId,status,sessionId,result}
//  4. exits 0 after RUNTIME_IDLE_TTL_M minutes without work
type AgentRuntimeSpec struct {
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Command/Args override the image entrypoint.
	// +optional
	Command []string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// Env: extra environment for every worker of this runtime.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// ServiceAccountName is this runtime's security identity: its RBAC defines
	// exactly what agents executing on this runtime may do in the cluster.
	// Give each runtime with a different trust level its OWN ServiceAccount —
	// runtimes sharing an SA share powers. Falls back to the operator's default
	// runtime SA when empty.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// IdleTTLMinutes before an idle worker exits (respawned on demand).
	// +kubebuilder:default=10
	// +optional
	IdleTTLMinutes int32 `json:"idleTtlMinutes,omitempty"`
	// Home volume for durable agent session state.
	// +optional
	Home *HomeVolume `json:"home,omitempty"`
	// Workspace volume for the repository checkout. Absent = ephemeral, which
	// is always correct — persisting it preserves uncommitted work across a pod
	// restart and skips the re-clone.
	// +optional
	Workspace *WorkspaceVolume `json:"workspace,omitempty"`
	// ContextStorage declares where this runtime keeps a conversation's context,
	// so the manager can tell whether continuity is possible here BEFORE
	// promising it. A runtime keeping context on its home volume, in a
	// deployment that provides none, can never continue anything — and saying
	// that up front is what stops every follow-up failing for a reason the
	// operator already chose.
	// +kubebuilder:default=volume
	// +optional
	ContextStorage ContextStorage `json:"contextStorage,omitempty"`
	// Resources default for runtime pods (AgentProfile.resources overrides).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentRuntimeStatus reports validation state.
type AgentRuntimeStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wrt
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="TTL",type=integer,JSONPath=`.spec.idleTtlMinutes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentRuntime defines an executable agent backend (worker image + pod defaults).
// AgentProfiles select one via spec.runtimeRef; the CR named "default" is the
// namespace fallback.
type AgentRuntime struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentRuntimeSpec   `json:"spec,omitempty"`
	Status AgentRuntimeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentRuntimeList contains a list of AgentRuntime.
type AgentRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentRuntime `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentRuntime{}, &AgentRuntimeList{})
}
