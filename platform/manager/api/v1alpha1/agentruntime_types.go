package v1alpha1

import (
	"time"

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
	// Image implementing the work contract. Derive your own to add tooling:
	// what an agent may REACH is wiring, so an image never grants it.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Command/Args override the image entrypoint.
	// +optional
	Command []string `json:"command,omitempty"`
	// Command / Args override the image's entrypoint. Both empty runs it as
	// the image declares it.
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
	// NodeSelector placing runtime pods, applied with Tolerations and
	// Affinity below.
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
	// ContextSync moves the LIVE context off the durable volume and keeps a
	// snapshot on it instead. ABSENT means today's behaviour, unchanged: the
	// home volume is mounted directly and there is no sidecar.
	// +optional
	ContextSync *ContextSync `json:"contextSync,omitempty"`
	// EgressMediation interposes a proxy in the runtime pod that the agent's
	// traffic cannot route around, so the tool access its wiring granted is
	// enforced somewhere the agent does not control. ABSENT means today's pod
	// exactly: no proxy, no interception, no added containers.
	//
	// The RUNTIME declares it because enabling it changes what the pod may do
	// at startup, and a namespace under `restricted` Pod Security admission
	// cannot run it at all. That is an execution-substrate property, which is
	// what an AgentRuntime is for.
	// +optional
	EgressMediation *EgressMediation `json:"egressMediation,omitempty"`
	// Resources default for runtime pods (AgentProfile.resources overrides).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// EgressMediation declares that the agent container's egress is redirected
// through a proxy that enforces the conversation's bound tool access.
//
// Presence is the switch. There is no `enabled` field, following ContextSync:
// a stanza that exists to be declared is clearer than a stanza that exists to
// be read as false.
type EgressMediation struct {
	// Port the proxy listens on inside the pod, and the port the agent's
	// traffic is redirected to.
	//
	// Overridable only because a runtime image may already use the default.
	// Nothing outside the pod can reach it — the two containers share a network
	// namespace and no Service names it.
	// +kubebuilder:default=15001
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`
	// ExcludePorts are destination ports left unredirected.
	//
	// For destinations that must not pass through a userspace proxy at all —
	// not for tuning. Anything excluded here is reachable by the agent
	// UNMEDIATED, so the list is a hole in the boundary by construction and is
	// reported as one.
	// +optional
	ExcludePorts []int32 `json:"excludePorts,omitempty"`
	// Resources for the proxy container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// MediationPort returns the port the proxy listens on, defaulting when the
// stanza is present but the field was left unset (a CR applied before the
// default existed, or one built in Go rather than through the API server).
func (e *EgressMediation) MediationPort() int32 {
	if e == nil || e.Port == 0 {
		return DefaultEgressMediationPort
	}
	return e.Port
}

// DefaultEgressMediationPort is the proxy's default listen port. 15001 follows
// the convention service meshes use, so an operator reading a netstat in a
// runtime pod recognises what it is.
const DefaultEgressMediationPort int32 = 15001

// ContextSync declares how a runtime's context is kept durable when the live
// copy lives on pod-local storage.
//
// The RUNTIME declares it, for exactly the reason ContextStorage gives: the
// chart cannot know where a given agent backend keeps its context, and a wrong
// guess produces a configuration that looks like it works right up until a
// resume fails. Neither the chart nor the manager may infer these paths.
//
// The manager copies the declared tree without reading it. That is not the same
// as interpreting context — the handle stays opaque, exactly as it is today.
type ContextSync struct {
	// Paths are INCLUDE globs, relative to the runtime's HOME, naming what is
	// worth persisting. For the reference runtime that is
	// ".claude/projects/-data-workspace/**".
	//
	// An include list rather than an exclude list, deliberately: caches, tool
	// state and telemetry are then excluded BY CONSTRUCTION, instead of by a
	// list that has to chase every file a vendor decides to add. It is also the
	// difference between copying a few megabytes of transcripts and copying a
	// package cache over NFS every two minutes.
	// +kubebuilder:validation:MinItems=1
	Paths []string `json:"paths"`
	// Exclude drops churn from INSIDE the included paths — lock files, temp
	// files, anything rewritten constantly without being context. Without it
	// the change detector reports a change on nearly every cycle and the
	// skip-when-unchanged rule buys nothing.
	// +optional
	Exclude []string `json:"exclude,omitempty"`
	// Interval is how often the context is checkpointed while a pod is alive,
	// as a Go duration ("2m").
	//
	// "0" disables the timer and leaves only work-boundary checkpoints, which
	// is the right setting for a low-churn backend. The interval bounds what a
	// SIGKILL can lose: a crash, an OOM or a node reboot takes everything
	// written since the last checkpoint, and no design removes that — only
	// shortens it.
	// +kubebuilder:default="2m"
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
	// Retain is how many previous copies to keep.
	//
	// More than one because a checkpoint taken mid-run may hold a partially
	// written file. Keeping the previous generations means such a copy costs a
	// fallback rather than the context itself.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +optional
	Retain int32 `json:"retain,omitempty"`
}

// SyncInterval returns the configured checkpoint period, and whether periodic
// checkpointing is on at all. A nil ContextSync is off.
func (c *ContextSync) SyncInterval() (time.Duration, bool) {
	if c == nil || c.Interval == nil {
		return 0, false
	}
	return c.Interval.Duration, c.Interval.Duration > 0
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
