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

// OriginKind says HOW an input reached the manager. Two values, and there are
// only two doors: a signal through a claimed SignalSource, or a channel the user
// is already looking at. (`POST /task` was a third once; it is gone.)
// +kubebuilder:validation:Enum=signal;channel
type OriginKind string

const (
	// OriginSignal: the input arrived through /signal/inbound on a
	// SignalSource. Name is that source.
	OriginSignal OriginKind = "signal"
	// OriginChannel: the input was typed into a channel — a command or a thread
	// reply. Name is that channel.
	OriginChannel OriginKind = "channel"
)

// InputOrigin records where one input came from. MATERIALIZED state, written at
// creation and never set by hand.
//
// It exists because provenance was previously an accident: `JobName` recorded
// the source for `kind: job` and dropped it for every other kind, so a
// conversation could not say what woke it. It replaces that field.
//
// The conversation's PIPELINE is deliberately NOT here. Conversations carry no
// pipelineRef; the route is inferred from the materialized bindings
// (chat.PipelineForConversation) and left blank when ambiguous. What is stored
// is only what cannot be derived afterwards.
type InputOrigin struct {
	Kind OriginKind `json:"kind"`
	// Name is the SignalSource or Channel the input came from.
	// +optional
	Name string `json:"name,omitempty"`
	// SignalKind is the originating signal's lane (alert | job | task | chat)
	// for `signal` origins, empty otherwise. Carried because CHAT is the one
	// signal a bound channel must NOT be shown: the person typed it, so posting
	// it back is an echo — and origin kind alone cannot tell it from an alert.
	// +optional
	SignalKind string `json:"signalKind,omitempty"`
}

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
	// Origin is where this input came from. Absent on inputs created before
	// provenance existed — and an absent origin means NOT POSTED to bound
	// channels, so upgrading cannot fill open threads with history.
	// +optional
	Origin *InputOrigin `json:"origin,omitempty"`
	// +optional
	ReceivedAt metav1.Time `json:"receivedAt,omitempty"`
}

// PostToChannels reports whether this input should be posted to the
// conversation's bound channels as a card.
//
// The rule is "post what a human has not already seen", read off the recorded
// origin rather than by enumerating input types — so a new input type inherits
// correct behavior instead of defaulting to whichever branch someone forgot:
//
//	signal  -> yes, except kind: chat (the person typed it; siblings get a relay)
//	channel -> no  (the originating surface already showed it)
//	absent  -> no  (predates provenance; the event is long delivered)
func (i *InputItem) PostToChannels() bool {
	if i.Origin == nil || i.Origin.Kind != OriginSignal {
		return false
	}
	return i.Origin.SignalKind != "chat"
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
	// PipelineRef names the Pipeline that ORIGINATED this conversation.
	//
	// PROVENANCE, NEVER WIRING. It is written once at creation and read for
	// exactly two things: scoping conversation REUSE, and ATTRIBUTION in
	// displays. Nothing resolves a profile, a channel set or a capability
	// through it — those come from the materialized fields beside it, which is
	// what keeps editing or deleting the Pipeline from re-wiring a conversation
	// already running. Resolving anything through this ref would undo that.
	//
	// It exists because a source is SHAREABLE: two Pipelines listing one source
	// both open a conversation per signal, and those conversations carry the
	// same signature. Without the ref, the second Pipeline's next signal would
	// be absorbed by the first Pipeline's conversation and run under the wrong
	// profile with the wrong tools. It also replaces attribution-by-inference,
	// which went blank exactly when two Pipelines wired identically.
	//
	// Absent on conversations predating the field. Nothing backfills it —
	// inference is what it replaces — so an empty ref is read conservatively
	// (see routeSignalGroup: reusable only while ONE Ready Pipeline serves the
	// source).
	// +optional
	PipelineRef *ObjectRef `json:"pipelineRef,omitempty"`
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
// +kubebuilder:validation:Enum=Pending;Idle;Queued;Working;Closed
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
	// ConversationClosed: INERT BUT INTACT. Closing stopped being deletion —
	// the object, its recorded runs, its context handle and its volume state
	// all survive, which is what makes reopening mean anything.
	//
	// Exhaustively, a Closed conversation has: no runtime pod, no MCP
	// ConfigMap, no dispatch and no work units; no capacity consumed and no
	// place in the pending backlog; no membership in conversation REUSE, so a
	// matching signature opens a NEW conversation; and no place in any
	// pipeline. Everything else — spec, materialized refs, runtimeContextID,
	// runs — is untouched on purpose.
	//
	// Deletion is a SECOND verb with its own flag and its own clock, measured
	// from ClosedAt. Nothing here deletes.
	ConversationClosed ConversationPhase = "Closed"
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
	// DeliveryTracked marks a run recorded by a manager that tracks delivery. It
	// is what tells an UNDELIVERED run from a PRE-UPGRADE one — both look like "a
	// completed run with no delivery markers", and both completed before the
	// current process started, so no timestamp can separate them.
	//
	// Absent (an older manager wrote the run): reconciliation records it
	// delivered without sending, so upgrading never re-posts history.
	// Set with no markers: the reply is genuinely owed and is re-enqueued.
	// +optional
	DeliveryTracked bool `json:"deliveryTracked,omitempty"`
	// Delivered names the bound channels whose thread has already received this
	// run's reply. It is what makes the reply DERIVABLE: reconciliation enqueues
	// a send for every bound thread missing from this list, so a manager restart
	// between `/work/done` and the adapter claiming the op re-derives the answer
	// instead of losing it.
	//
	// Per-THREAD rather than a per-run boolean on purpose: a fan-out to three
	// channels can be interrupted after one succeeds, and a boolean would either
	// re-post to the delivered thread or abandon the other two.
	// +optional
	Delivered []string `json:"delivered,omitempty"`
}

// DeliveredTo reports whether this run's reply already reached a channel.
func (r *RunStatus) DeliveredTo(channel string) bool {
	for _, c := range r.Delivered {
		if c == channel {
			return true
		}
	}
	return false
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
	// RuntimeContextID is the RUNTIME's opaque handle for this conversation's
	// accumulated context — every message, tool call and model response it has
	// built up. The manager stores it and hands it back on the next work unit;
	// it never interprets it and never assumes where the context lives (session
	// files on a volume, a thread id at a vendor API, rows in a database are all
	// valid, and none of them are distinguishable from here).
	//
	// Named for what agent-ops means, not for one backend's word: "session" is
	// claude-code's noun, and a vendor's noun in this API would teach every later
	// reader that the operator knows what is inside the handle.
	//
	// LATEST-WINS. Every completed run's reported handle replaces this one. It
	// was write-once, which was unsound: a run may legitimately end in a
	// different context than it was asked to continue, and keeping the first
	// handle then named something that no longer existed — so every later
	// message repeated the same failed continuation and one recoverable loss
	// became permanent.
	// +optional
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	// SessionID is the former name of RuntimeContextID.
	//
	// DEPRECATED, and retained for exactly one release so the rename cannot do
	// the harm this field exists to prevent: it is still DECODED, so a
	// conversation written by an older manager is adopted rather than losing its
	// handle at the moment of upgrade. Readers must prefer RuntimeContextID and
	// fall back to this; writers must only ever set RuntimeContextID.
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
	// ClosedAt stamps the transition into phase Closed, and is the ORIGIN of the
	// delete clock — the only thing that reads it.
	//
	// A dedicated timestamp rather than the Closed condition's
	// lastTransitionTime: a condition's transition time is rewritten by any
	// reason change on the same condition, so a clock built on it can be reset
	// by an unrelated status update. This is written once, at the transition,
	// and CLEARED by a reopen — which is what stops the delete clock.
	// +optional
	ClosedAt *metav1.Time `json:"closedAt,omitempty"`
	// ThreadsArchived names the bound channels whose thread has already been
	// archived by a completed close-topic op.
	//
	// This is what retires "close-topic is the ONE op not derivable from CR
	// state". It was the exception only because it was enqueued while the
	// object was disappearing, leaving nothing to record against. Closing no
	// longer deletes, so the object survives — and a Closed conversation whose
	// thread is missing from this list is an archive still owed, re-derivable
	// on the next reconcile exactly as runs[].delivered[] makes a reply
	// derivable. Per-THREAD for the same reason: a fan-out interrupted after
	// one channel must not re-archive that one or abandon the rest.
	// +optional
	ThreadsArchived []string `json:"threadsArchived,omitempty"`
	// Reopens counts how many times this conversation has been reopened.
	//
	// It is not decoration: ensure-topic op ids are STABLE per
	// conversation×channel so reconciliation can re-derive them, which means a
	// reopen's request to re-establish a thread would otherwise dedup against
	// the original topic creation and never reach the adapter. The count makes
	// each reopen's op distinct while keeping every one of them derivable.
	// +optional
	Reopens int32 `json:"reopens,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ThreadArchived reports whether this conversation's thread on a channel has
// already been archived.
func (s *ConversationStatus) ThreadArchived(channel string) bool {
	for _, c := range s.ThreadsArchived {
		if c == channel {
			return true
		}
	}
	return false
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

// ContextID returns the runtime's context handle for this conversation, or "".
//
// THE ONE PLACE the deprecated spelling is read. Prefer the current field, fall
// back to the old one so a conversation written before the rename keeps its
// handle across the upgrade instead of restarting its context — which is the
// exact failure this whole area exists to prevent, and would have been
// self-inflicted by a rename that simply moved the field.
//
// Callers use this rather than touching either field, so removing the fallback
// after one release is a one-line change here.
func (c *Conversation) ContextID() string {
	if c.Status.RuntimeContextID != "" {
		return c.Status.RuntimeContextID
	}
	return c.Status.SessionID
}

// SetContextID records the runtime's handle, writing ONLY the current field and
// retiring the deprecated one so an adopted conversation stops carrying both.
func (c *Conversation) SetContextID(id string) {
	c.Status.RuntimeContextID = id
	c.Status.SessionID = ""
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
