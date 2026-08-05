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

// ConversationSpec pins a conversation to a channel topic and an agent profile,
// and carries its queue of work units (append-only; pruned once processed).
type ConversationSpec struct {
	// ChannelRef — omit for chat-less conversations (HTTP-only / shadow).
	// +optional
	ChannelRef *ObjectRef `json:"channelRef,omitempty"`
	ProfileRef ObjectRef  `json:"profileRef"`
	// +optional
	Title string `json:"title,omitempty"`
	// Signature groups same/similar problems into one conversation
	// (e.g. alertgroup/alertname/namespace, job:<name>).
	// +optional
	Signature string `json:"signature,omitempty"`
	// +optional
	Inputs []InputItem `json:"inputs,omitempty"`
}

// ConversationPhase is the coarse conversation state.
// +kubebuilder:validation:Enum=Idle;Queued;Working
type ConversationPhase string

const (
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

// ConversationStatus is the observed state.
type ConversationStatus struct {
	// +optional
	Phase ConversationPhase `json:"phase,omitempty"`
	// Chat thread id (e.g. Telegram forum topic).
	// +optional
	ThreadID *int64 `json:"threadId,omitempty"`
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
// +kubebuilder:printcolumn:name="Thread",type=integer,JSONPath=`.status.threadId`
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.status.runtimePod`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Conversation is one incident/task conversation: a chat topic, an agent
// session, and a queue of work units executed strictly serially.
type Conversation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConversationSpec   `json:"spec,omitempty"`
	Status ConversationStatus `json:"status,omitempty"`
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
