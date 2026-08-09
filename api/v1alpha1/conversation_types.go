package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InputType classifies a work unit.
// +kubebuilder:validation:Enum=task;alert;reply;recurrence;job
type InputType string

const (
	InputTask       InputType = "task"
	InputAlert      InputType = "alert"
	InputReply      InputType = "reply"
	InputRecurrence InputType = "recurrence"
	InputJob        InputType = "job"
)

// InputItem is one queued work unit. Payload is inline OR referenced via
// PayloadRef (a ConversationInput object) for large payloads.
type InputItem struct {
	// +kubebuilder:validation:MinLength=1
	ID   string    `json:"id"`
	Type InputType `json:"type"`
	// +optional
	Payload string `json:"payload,omitempty"`
	// +optional
	PayloadRef *ObjectRef `json:"payloadRef,omitempty"`
	// Agent override for task inputs (`/<profile>:<agent>` addressing).
	// +optional
	Agent string `json:"agent,omitempty"`
	// JobName for job inputs.
	// +optional
	JobName string `json:"jobName,omitempty"`
	// +optional
	ReceivedAt metav1.Time `json:"receivedAt,omitempty"`
}

// ConversationSpec pins a conversation to its chat surfaces and an agent
// profile, and carries its queue of work units (append-only; pruned once
// processed).
type ConversationSpec struct {
	// ChannelRefs — every listed channel mirrors the whole conversation (own
	// thread per channel, replies and acks fanned out). Empty = chat-less
	// (HTTP-only / shadow).
	// +optional
	ChannelRefs []ObjectRef `json:"channelRefs,omitempty"`
	ProfileRef  ObjectRef   `json:"profileRef"`
	// +optional
	Title string `json:"title,omitempty"`
	// Signature groups same/similar problems into one conversation
	// (e.g. alertgroup/alertname/namespace, job:<name>).
	// +optional
	Signature string `json:"signature,omitempty"`
	// Toolsets / MCPConfigs are the originating Pipeline's tooling bindings,
	// snapshotted at creation like ChannelRefs and ProfileRef: materialized
	// per-conversation state, NOT wiring — nothing sets them by hand, and
	// re-wiring the pipeline affects only new conversations. Only the refs are
	// snapshotted; the referenced CRs' CONTENT is re-read at every use, so
	// editing a toolset or config reaches running conversations. Every
	// origination has a Pipeline behind it — a signal of any kind through a
	// claimed source, or a chat command naming one — and a conversation whose
	// Pipeline declared no bindings carries none, because nothing else supplies
	// them: profiles carry no capabilities at all.
	// +optional
	Toolsets *ToolsetBinding `json:"toolsets,omitempty"`
	// +optional
	MCPConfigs *ToolingBinding `json:"mcpConfigs,omitempty"`
	// +optional
	Inputs []InputItem `json:"inputs,omitempty"`
}

// ConversationPhase is the coarse conversation state.
// +kubebuilder:validation:Enum=Pending;Idle;Queued;Working
type ConversationPhase string

const (
	// ConversationPending: created, awaiting a capacity slot; nothing
	// provisioned. No runtime pod, no chat topic, no MCP ConfigMap — the
	// conversation holds its inputs and its wiring snapshot and nothing else.
	// Distinct from Queued, which means ADMITTED with work waiting behind the
	// serial-per-conversation rule.
	ConversationPending ConversationPhase = "Pending"
	ConversationIdle    ConversationPhase = "Idle"
	ConversationQueued  ConversationPhase = "Queued"
	ConversationWorking ConversationPhase = "Working"
)

// RunStatus records one completed agent run.
type RunStatus struct {
	RunID   string `json:"runId"`
	JobKind string `json:"jobKind,omitempty"`
	Status  string `json:"status"`
	// +optional
	ExitCode *int32 `json:"exitCode,omitempty"`
	// +optional
	Result string `json:"result,omitempty"`
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// +optional
	InputIDs []string `json:"inputIds,omitempty"`
}

// InflightRun tracks the unit currently dispatched to the worker.
type InflightRun struct {
	RunID string `json:"runId"`
	// +optional
	InputIDs []string `json:"inputIds,omitempty"`
	// +optional
	DispatchedAt metav1.Time `json:"dispatchedAt,omitempty"`
}

// ThreadBinding pins one bound channel to its conversation thread.
type ThreadBinding struct {
	// Channel name (same namespace).
	Channel string `json:"channel"`
	// ThreadID — an opaque string in the channel type's own id space (e.g. a
	// Telegram forum topic id in decimal, a Slack ts).
	ThreadID string `json:"threadId"`
}

// ConversationStatus is the observed state.
type ConversationStatus struct {
	// +optional
	Phase ConversationPhase `json:"phase,omitempty"`
	// Threads: one binding per bound channel whose topic has been created.
	// +optional
	Threads []ThreadBinding `json:"threads,omitempty"`
	// Agent session id (resume handle).
	// +optional
	SessionID string `json:"sessionId,omitempty"`
	// +optional
	RuntimePod string `json:"runtimePod,omitempty"`
	// +optional
	Inflight *InflightRun `json:"inflight,omitempty"`
	// Last runs, newest last (bounded).
	// +optional
	Runs []RunStatus `json:"runs,omitempty"`
	// IDs of processed inputs (bounded; used to prune spec.inputs).
	// +optional
	ProcessedInputIDs []string `json:"processedInputIds,omitempty"`
	// +optional
	LastActivity *metav1.Time `json:"lastActivity,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=conv
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
// +kubebuilder:printcolumn:name="Thread",type=string,JSONPath=`.status.threads[0].threadId`
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.status.runtimePod`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Conversation is one incident/task conversation: chat threads (one per bound
// channel, all mirroring the same exchange), an agent session, and a queue of
// work units executed strictly serially.
type Conversation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConversationSpec   `json:"spec,omitempty"`
	Status ConversationStatus `json:"status,omitempty"`
}

// ThreadFor returns the thread id bound for a channel, or nil.
func (c *Conversation) ThreadFor(channel string) *string {
	for i := range c.Status.Threads {
		if c.Status.Threads[i].Channel == channel {
			return &c.Status.Threads[i].ThreadID
		}
	}
	return nil
}

// BoundTo reports whether the conversation references a channel.
func (c *Conversation) BoundTo(channel string) bool {
	for i := range c.Spec.ChannelRefs {
		if c.Spec.ChannelRefs[i].Name == channel {
			return true
		}
	}
	return false
}

// +kubebuilder:object:root=true

// ConversationList contains a list of Conversation.
type ConversationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Conversation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Conversation{}, &ConversationList{})
}
