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
	// for `signal` origins, empty otherwise. It says whether a PERSON typed
	// this input — which decides how it is rendered on the surfaces that did
	// not show it (somebody's words, or the event that woke the agent). It does
	// NOT decide whether it is delivered: that is per destination, read off the
	// origin SURFACE.
	// +optional
	SignalKind string `json:"signalKind,omitempty"`
	// Sender is the transport-side identity that typed this input, when the
	// serving adapter supplied one. Attribution, never authority: nothing is
	// resolved through it and no permission reads it.
	//
	// It is recorded because a person's message is now delivered to every OTHER
	// bound surface, and reconciliation composes that delivery from the
	// conversation alone. An in-memory sender would leave the same message
	// attributed on the fast path and anonymous when re-derived after a
	// restart — the same class of bug the delivery markers on runs fixed.
	// A chat signal carries the same fact in its labels
	// (LabelChatSender); this is where the CHANNEL lane keeps it.
	// +optional
	Sender string `json:"sender,omitempty"`
}

// SignalKindChat is the chat lane's name on InputOrigin.SignalKind — a person
// typing on a surface, as opposed to an alert, a job tick or a posted task.
const SignalKindChat = "chat"

// Reserved labels a chat signal carries into a conversation. Declared here
// because the DELIVERY rule reads the channel one: it names the SURFACE a
// message was typed on, and that surface is the one destination the message is
// not delivered to. /signal/inbound refuses a chat signal without it.
const (
	LabelChatChannel = "agentops.dev/channel"
	LabelChatSender  = "agentops.dev/sender"
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
	// Agent is DEPRECATED and no longer written. It carried the per-message
	// agent override of the retired `/<pipeline>:<agent>` addressing form,
	// which let whoever typed it select an agent definition the WIRING never
	// declared. A Pipeline names one profile and a profile names one agent, so
	// the agent is already fully determined by the wiring.
	//
	// Dispatch still READS it for one release, so inputs already queued when
	// the manager restarts dispatch to the agent they were parsed with. Same
	// posture as the retired `sessionId` dual-read; removing the field is a
	// later change.
	//
	// Deprecated: nothing sets this. Do not add a writer.
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

// OriginSurface names the CHANNEL this input was typed on, or "" when no
// surface displayed it (an alert, a job tick, a task a machine posted).
//
// ONE resolution for both lanes, because two would drift: a channel origin
// names its channel, and a chat signal carries the surface in its labels. The
// labels come from the input's ConversationInput, so the caller passes them in
// — nothing here reads the API.
func (i *InputItem) OriginSurface(labels map[string]string) string {
	if i.Origin == nil {
		return ""
	}
	switch i.Origin.Kind {
	case OriginChannel:
		return i.Origin.Name
	case OriginSignal:
		return labels[LabelChatChannel]
	}
	return ""
}

// OriginSender is the identity that typed this input, from whichever lane
// carried it, or "" when nobody is named.
func (i *InputItem) OriginSender(labels map[string]string) string {
	if i.Origin != nil && i.Origin.Sender != "" {
		return i.Origin.Sender
	}
	return labels[LabelChatSender]
}

// TypedByAPerson reports whether somebody wrote this input, as opposed to an
// event that woke the agent. It decides how a delivery is RENDERED — somebody's
// words carry their attribution, an event is a card — and nothing else.
func (i *InputItem) TypedByAPerson() bool {
	if i.Origin == nil {
		return false
	}
	return i.Origin.Kind == OriginChannel || i.Origin.SignalKind == SignalKindChat
}

// DeliverTo answers the ONE delivery question, per DESTINATION: does this bound
// channel still owe its readers this message?
//
// "Already seen" is a fact about a SURFACE, never about a message. A message
// typed on surface A was displayed by A as it was typed, and is new to every
// other bound channel. The rule this replaced decided once, from the origin
// KIND, and so withheld a person's words from channels that had never shown
// them — the console's own composer message among them.
//
// Whether the origin surface displayed it is a fact about its TRANSPORT, not
// about the message: a chat app echoes what you typed, a viewer that renders
// only what it is sent does not. The caller supplies that as originEchoes,
// read off the serving ChannelAdapter.
//
// An input with NO origin is delivered nowhere. It predates provenance, cannot
// be told from an alert, and its event was delivered long ago — so an upgrade
// cannot fill every open thread with history.
func (i *InputItem) DeliverTo(channel, originSurface string, originEchoes bool) bool {
	if i.Origin == nil {
		return false
	}
	return !(channel == originSurface && originEchoes)
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
	// OriginReader records WHO started this conversation, so their own read
	// watermark can be stamped the moment their thread is created — the person
	// who typed the request has by definition seen it, and presenting it back
	// to them as unread before any answer exists is the one case the watermark
	// rule gets plainly wrong.
	//
	// Written once at creation and read EXACTLY ONCE, when the binding on its
	// channel is established. It is provenance in the same sense as
	// PipelineRef: nothing resolves anything through it, and it grants nothing.
	//
	// The key is OPAQUE — the originating surface's own salted hash, in that
	// surface's key space, which is why the channel is recorded beside it. No
	// identity is stored, here or anywhere else on this object.
	// +optional
	OriginReader *OriginReader `json:"originReader,omitempty"`
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
	// Inputs are the messages this run consumed, kept where the run keeps its
	// answer — so a conversation records the questions as well as the answers
	// and its whole timeline reads off status in order.
	//
	// The queue is a QUEUE: spec.inputs[] is pruned once dispatch has consumed
	// an entry, which is what stops answered work running twice, and it took
	// the only copy of what a person said with it. This is the copy that stays.
	//
	// Written in the SAME status write that marks the inputs processed, and
	// therefore strictly before the reconciler prunes them: a record written
	// afterwards would be lost by a crash in between, permanently.
	//
	// Absent on runs written before this existed. A viewer renders what it has
	// and invents nothing.
	// +optional
	Inputs []RecordedInput `json:"inputs,omitempty"`
}

// MaxRecordedInputText bounds how much of one message is inlined into a
// conversation's record. THE cap, stated once: nothing configures it, because
// an installation that could opt out of it would be an installation that could
// grow a Conversation past what etcd will store.
//
// Large payloads already live out of line in a ConversationInput for exactly
// that reason, and copying them into status would undo it.
const MaxRecordedInputText = 2000

// RecordedInput is one message a run consumed: what was said, when, and where
// it entered the system — everything a reader needs to render it without
// inference.
type RecordedInput struct {
	// ID is the input's id, the same one runs[].inputIds names.
	ID string `json:"id"`
	// +optional
	Type InputType `json:"type,omitempty"`
	// Text is the message, inlined up to MaxRecordedInputText.
	// +optional
	Text string `json:"text,omitempty"`
	// Truncated says Text is NOT the whole message — it was longer than the cap
	// and what is kept here is its beginning. A reader must present it as a
	// fragment rather than as what somebody said.
	// +optional
	Truncated bool `json:"truncated,omitempty"`
	// PayloadRef names the ConversationInput the full text was read from, when
	// there was one. A CITATION of where the message lived, not a live pointer:
	// that object is deleted with the queue entry, which is why the beginning of
	// the text is kept here rather than only referenced.
	// +optional
	PayloadRef *ObjectRef `json:"payloadRef,omitempty"`
	// Surface is the channel this message was typed on, empty when no surface
	// displayed it (an alert, a job tick, a posted task).
	// +optional
	Surface string `json:"surface,omitempty"`
	// Sender is who typed it, when a sender was named. Attribution only.
	// +optional
	Sender string `json:"sender,omitempty"`
	// +optional
	ReceivedAt *metav1.Time `json:"receivedAt,omitempty"`
}

// Record renders one queued input as the record entry the run keeps, applying
// the inline cap in the ONE place it is applied.
//
// text is the resolved message — the inline payload, or the referenced
// ConversationInput's — and labels are that object's, so surface and sender are
// resolved by the same helpers the delivery rule uses.
func (i *InputItem) Record(text string, labels map[string]string) RecordedInput {
	rec := RecordedInput{
		ID: i.ID, Type: i.Type, PayloadRef: i.PayloadRef,
		Surface: i.OriginSurface(labels), Sender: i.OriginSender(labels),
	}
	if !i.ReceivedAt.IsZero() {
		at := i.ReceivedAt
		rec.ReceivedAt = &at
	}
	// Rune-safe: a byte cut would halve a multi-byte character, and chat input
	// is exactly where non-ASCII shows up.
	if runes := []rune(text); len(runes) > MaxRecordedInputText {
		rec.Text, rec.Truncated = string(runes[:MaxRecordedInputText]), true
	} else {
		rec.Text = text
	}
	return rec
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

// OriginReader names the reader who started a conversation, in the key space of
// the channel they started it from. Keys are per-channel by construction — each
// surface salts its own — so the channel is not decoration: stamping this key
// on another channel's binding would record a reader nothing can match.
type OriginReader struct {
	Channel string `json:"channel"`
	Key     string `json:"key"`
}

// ThreadBinding pins one bound channel to its conversation thread.
type ThreadBinding struct {
	// Channel name (same namespace).
	Channel string `json:"channel"`
	// ThreadID — an opaque string in the channel type's own id space (e.g. a
	// Telegram forum topic id in decimal, a Slack ts).
	ThreadID string `json:"threadId"`
	// ReadAt is how far this thread has been SEEN — reported by the adapter that
	// serves the channel, never inferred. MONOTONIC: it only moves forward, and
	// the manager clamps it to its own clock, so neither a stale client nor a
	// skewed one can un-read a thread or mark future activity read.
	//
	// Per THREAD, therefore per CHANNEL: a conversation bound to Telegram and the
	// console has two audiences reading it in two places, and one shared mark
	// would let a Telegram reader clear the console's.
	// +optional
	ReadAt *metav1.Time `json:"readAt,omitempty"`
	// ReadTracked marks a binding created by a manager that tracks reads. It is
	// what tells a NEVER-READ binding from a PRE-UPGRADE one — both look like "a
	// binding with no readAt", and no timestamp can separate them, exactly as
	// with status.runs[].deliveryTracked.
	//
	// Absent (an older manager bound the thread): treated as READ, so upgrading
	// never presents the whole namespace as new.
	// Set with no ReadAt: genuinely unseen, and unread.
	// +optional
	ReadTracked bool `json:"readTracked,omitempty"`
	// Readers is the PER-IDENTITY watermark, for transports that can tell one
	// reader from another. ReadAt above stays the CHANNEL-WIDE mark and is not
	// replaced by this: a Telegram topic is read or it is not, and there is
	// nobody to attribute that to — so an adapter reporting no reader keeps
	// reporting only the channel-wide mark and stays fully conformant.
	//
	// BOUNDED at MaxReadersPerThread, oldest watermark evicted first, because
	// this list grows with readers × conversations. Eviction is not a loss: an
	// evicted reader falls back to the channel-wide mark, exactly as a reader
	// who has never reported does.
	// +optional
	Readers []ReaderMark `json:"readers,omitempty"`
}

// MaxReadersPerThread bounds the per-identity overlay on one binding.
const MaxReadersPerThread = 50

// ReaderMark is one identity's watermark on a thread.
type ReaderMark struct {
	// Key identifies the reader and is OPAQUE to the manager — a salted hash
	// the reporting adapter computed. The manager never derives it, never
	// interprets it, and stores no identity it was derived from, so a
	// conversation records THAT someone read it without recording WHO. Same
	// contract as ThreadID and RuntimeContextID.
	Key string `json:"key"`
	// +optional
	ReadAt *metav1.Time `json:"readAt,omitempty"`
}

// Watermark returns how far a reader has seen this thread: their own mark when
// they have one, and the CHANNEL-WIDE mark otherwise.
//
// The fallback is the whole reason eviction is safe. A reader who has never
// reported and one whose entry was evicted are indistinguishable here, and both
// inherit where the channel as a whole got to — so a teammate joining today is
// not handed a namespace-sized backlog they can act on none of, which is the
// ReadTracked backfill argument one level down.
func (t *ThreadBinding) Watermark(reader string) *metav1.Time {
	if reader != "" {
		for i := range t.Readers {
			if t.Readers[i].Key == reader {
				return t.Readers[i].ReadAt
			}
		}
	}
	return t.ReadAt
}

// Unread reports whether this thread has activity newer than its watermark,
// given the conversation's last activity. It is the one implementation of the
// rule — the manager, the console and the tests all read it from here.
//
// A binding without ReadTracked predates the mechanism and is READ. A tracked
// binding with no watermark is bound and never seen, so it is UNREAD. Otherwise
// activity strictly after the watermark is unread.
//
// reader is the OPAQUE key of the identity asking; empty asks the channel-wide
// question ("has this thread been seen at all"), which is the only question a
// transport with no reader identity can answer.
func (t *ThreadBinding) Unread(lastActivity *metav1.Time, reader string) bool {
	if !t.ReadTracked {
		return false
	}
	at := t.Watermark(reader)
	if at == nil {
		return true
	}
	if lastActivity == nil {
		return false
	}
	return lastActivity.Time.After(at.Time)
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
	// RuntimeStartFailures counts CONSECUTIVE failures to bring a runtime pod
	// up, and is reset to zero the moment one reaches Running.
	//
	// It exists to back off. A pod that cannot start for an environmental
	// reason — a volume that will not attach is the case this was built for —
	// fails again immediately if it is recreated immediately, and the resulting
	// hot loop buys nothing while filling the event stream. The count is what
	// makes the interval grow.
	//
	// Kept on status rather than in memory because the decision must survive a
	// manager restart: a process that forgets is a process that hot-loops from
	// zero every rollout, exactly when a storage incident is most likely.
	// ContextCheckpoint records the most recent SUCCESSFUL durable copy of this
	// conversation's context.
	//
	// Durable state rather than telemetry, and the distinction is load-bearing.
	// The activity log is bounded and lossy by design, but whether a
	// conversation has a usable context after a crash decides whether it can
	// continue at all — so it cannot depend on a record that may have been
	// evicted from a ring buffer.
	//
	// Written ONLY when a checkpoint actually transferred data. A skipped
	// checkpoint writes nothing: recording every skip would patch every
	// conversation on every interval forever, which is precisely the write
	// amplification that suppressed signals already avoid by writing cooldown
	// only on admission.
	// +optional
	ContextCheckpoint *ContextCheckpoint `json:"contextCheckpoint,omitempty"`
	// +optional
	RuntimeStartFailures int32 `json:"runtimeStartFailures,omitempty"`
	// LastRuntimeStartFailure stamps the most recent reap of a pod that never
	// started. Together with RuntimeStartFailures it is the whole of the
	// backoff state — the delay is derived from the two, never stored, so
	// nothing can disagree about when the next attempt is due.
	// +optional
	LastRuntimeStartFailure *metav1.Time `json:"lastRuntimeStartFailure,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Thread returns this conversation's binding on a channel, or nil when the
// channel holds no thread here.
func (s *ConversationStatus) Thread(channel string) *ThreadBinding {
	for i := range s.Threads {
		if s.Threads[i].Channel == channel {
			return &s.Threads[i]
		}
	}
	return nil
}

// UnreadFor reports whether a channel's thread on this conversation has activity
// the given reader has not been reported read up to. A channel with NO binding
// is never unread: with no thread there is no watermark and no claim to make.
//
// An empty reader asks the channel-wide question.
func (s *ConversationStatus) UnreadFor(channel, reader string) bool {
	t := s.Thread(channel)
	if t == nil {
		return false
	}
	return t.Unread(s.LastActivity, reader)
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

// ContextCheckpoint is one durable copy of a conversation's context.
type ContextCheckpoint struct {
	// At is when the copy completed.
	At metav1.Time `json:"at"`
	// Generation names the copy on the volume, so an operator recovering by
	// hand knows which directory to look in and a restore can fall back to an
	// earlier one.
	// +optional
	Generation string `json:"generation,omitempty"`
	// Quiesced reports whether this copy was taken at a WORK BOUNDARY, with
	// nothing inflight, or during a run.
	//
	// A mid-run copy is still worth taking — a long run is exactly what a crash
	// would otherwise lose in full — but it may contain a partially written
	// file. Labelling it is what lets a restore, and a person, tell a
	// known-consistent copy from a best-effort one instead of guessing.
	Quiesced bool `json:"quiesced"`
	// Bytes transferred by this checkpoint. Zero is meaningful: it means the
	// copy ran and found nothing changed.
	// +optional
	Bytes int64 `json:"bytes,omitempty"`
}
