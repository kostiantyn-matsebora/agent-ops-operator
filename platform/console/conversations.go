package main

import (
	"encoding/json"
	"strconv"
	"time"
)

// The live-runs model: Conversations projected into what the UI renders.
//
// Everything here comes from CR status, which is the durable record. The
// channel transcript (transcript.go) is a live overlay on top of it and is
// deliberately allowed to be lost — runs[] survives a console restart, the
// wire does not.

// ThreadBinding pins one bound channel to its thread id, and carries how far
// that channel has read it. The watermark is per THREAD, so a conversation read
// on Telegram is still unread here.
type ThreadBinding struct {
	Channel  string `json:"channel"`
	ThreadID string `json:"threadId"`
	// Readers is the per-IDENTITY overlay; ReadAt below stays the channel-wide
	// mark and is what a reader with no entry inherits.
	Readers []ReaderMark `json:"readers,omitempty"`
	// ReadAt is the manager-written watermark; ReadTracked marks a binding
	// created after read reporting existed. A binding WITHOUT it predates the
	// mechanism and is treated as READ — otherwise the first upgrade presents
	// every conversation in the namespace as new.
	ReadAt      string `json:"readAt,omitempty"`
	ReadTracked bool   `json:"readTracked,omitempty"`
}

// ReaderMark is one identity's watermark, keyed by an opaque salted hash.
type ReaderMark struct {
	Key    string `json:"key"`
	ReadAt string `json:"readAt,omitempty"`
}

// watermark resolves how far a reader has seen this thread: their own mark, and
// the channel-wide one when they have no entry — which is equally the answer
// for a newcomer and for someone the LRU evicted.
func (t ThreadBinding) watermark(reader string) string {
	if reader != "" {
		for _, r := range t.Readers {
			if r.Key == reader {
				return r.ReadAt
			}
		}
	}
	return t.ReadAt
}

// Run is one completed agent run from status.runs[].
type Run struct {
	RunID      string `json:"runId"`
	JobKind    string `json:"jobKind,omitempty"`
	Status     string `json:"status"`
	ExitCode   *int32 `json:"exitCode,omitempty"`
	Result     string `json:"result,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	// Inputs are the messages this run consumed — the DURABLE half of a
	// conversation's questions, kept where the run keeps its answer. Absent on
	// runs recorded before the manager kept them, which is why a thread from
	// before this change still reads as answers alone.
	Inputs []RecordedInput `json:"inputs,omitempty"`
	// Turns and ToolCalls are this run's own model/tool call diagnostics,
	// DERIVED by attachRunCalls from the activity feed's `model.call` /
	// `tool.call` hops for this RunID — never read off the Conversation. The
	// manager's own work contract is explicit that it writes neither to the
	// Conversation (docs/contracts.md, "What the run did: turns[] and
	// toolCalls[]"): that telemetry is recorded as activity hops only. Both
	// are absent on a run whose runtime reported neither, or one outside the
	// activity window.
	Turns     []RunTurn     `json:"turns,omitempty"`
	ToolCalls []RunToolCall `json:"toolCalls,omitempty"`
}

// RunTurn is one model call a run made, read off a `model.call` activity hop.
// Field names match the work contract's own `turns[]` exactly
// (docs/contracts.md).
type RunTurn struct {
	Model           string `json:"model,omitempty"`
	TokensIn        *int64 `json:"tokensIn,omitempty"`
	TokensOut       *int64 `json:"tokensOut,omitempty"`
	CacheReadTokens *int64 `json:"cacheReadTokens,omitempty"`
	StopReason      string `json:"stopReason,omitempty"`
}

// RunToolCall is one tool call a run made, read off a `tool.call` activity
// hop. Field names match the work contract's own `toolCalls[]` exactly
// (docs/contracts.md).
type RunToolCall struct {
	Tool        string `json:"tool,omitempty"`
	Server      string `json:"server,omitempty"`
	DurationMs  *int64 `json:"durationMs,omitempty"`
	ResultBytes *int64 `json:"resultBytes,omitempty"`
}

// attachRunCalls groups one conversation's `model.call` / `tool.call`
// activity hops by RunID and layers them onto the matching Run — the one
// place this telemetry reaches the browser as a PER-RUN diagnostic rather
// than loose hops on the activity stream. It reads the SAME events array the
// detail view already fetches for its own "events" field, never a second
// activity read.
//
// A run with no matching hops (a runtime that reported neither, or a run
// older than the activity window) keeps both fields absent — exactly the
// shape the work contract itself allows.
func attachRunCalls(runs []Run, events []ActivityEvent) []Run {
	if len(runs) == 0 || len(events) == 0 {
		return runs
	}
	turns := map[string][]RunTurn{}
	toolCalls := map[string][]RunToolCall{}
	for _, e := range events {
		if e.RunID == "" {
			continue
		}
		switch e.Kind {
		case "model.call":
			turns[e.RunID] = append(turns[e.RunID], RunTurn{
				Model: e.Data["model"], StopReason: e.Data["stopReason"],
				TokensIn: reportedCount(e.Data, "tokensIn"), TokensOut: reportedCount(e.Data, "tokensOut"),
				CacheReadTokens: reportedCount(e.Data, "cacheReadTokens"),
			})
		case "tool.call":
			toolCalls[e.RunID] = append(toolCalls[e.RunID], RunToolCall{
				Tool: e.Data["tool"], Server: e.Data["server"],
				ResultBytes: reportedCount(e.Data, "resultBytes"), DurationMs: reportedLatency(e.LatencyMs),
			})
		}
	}
	if len(turns) == 0 && len(toolCalls) == 0 {
		return runs
	}
	out := make([]Run, len(runs))
	for i, r := range runs {
		r.Turns = turns[r.RunID]
		r.ToolCalls = toolCalls[r.RunID]
		out[i] = r
	}
	return out
}

// reportedCount reads a bounded telemetry count out of an activity hop's
// string data map, or nil when the key is absent or not a clean
// non-negative integer. The manager OMITS a fact it cannot determine rather
// than reporting it as zero (docs/contracts.md: "A fact the runtime cannot
// determine is OMITTED, never sent as zero"), and a value that fails to
// parse across this boundary is treated the same way rather than trusted.
func reportedCount(data map[string]string, key string) *int64 {
	v, ok := data[key]
	if !ok {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// reportedLatency mirrors Edges' own treatment of an activity hop's
// LatencyMs: the wire's `omitempty` already makes zero and "not reported"
// indistinguishable, so zero reads as absent here too, consistently.
func reportedLatency(ms int64) *int64 {
	if ms <= 0 {
		return nil
	}
	return &ms
}

// RecordedInput is one message a run consumed, as the Conversation records it.
type RecordedInput struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
	// Truncated: Text is the beginning of a larger payload, not the whole of it.
	Truncated bool `json:"truncated,omitempty"`
	// Surface is the channel the message was typed on, empty when no surface
	// displayed it — an alert, a job tick, a posted task.
	Surface string `json:"surface,omitempty"`
	Sender  string `json:"sender,omitempty"`
	// Origin is the input's OriginKind ("signal" | "channel" | "member"), the
	// field name matching the manager's own API exactly
	// (api/v1alpha1.RecordedInput). "member" is the one value this console
	// acts on: a task `invoke` handed down, or a member's result routed back
	// up (coordination-loop) — both coordination-internal, never a human-facing
	// card. A genuine surfaceless signal (an alert, a job tick) also records
	// Surface as "", so Surface alone cannot tell the two apart — this is the
	// field that can. Empty on an input recorded before this field existed.
	Origin     string `json:"origin,omitempty"`
	ReceivedAt string `json:"receivedAt,omitempty"`
}

// originMember is recordedSignalCard's sibling fact: an input whose Origin is
// this is coordination plumbing between a conversation and its own causedBy
// member — already shown in full by the invoke card console-conversation-tree
// builds straight from that member's own transcript (ThreadPane.tsx,
// MemberInvocationExchange). Rendering it AGAIN here, as a generic signal
// card, duplicated a member's result on the page it was invoked from (item
// #15 QA, bug 2) — matches api/v1alpha1.OriginMember exactly.
const originMember = "member"

// Inflight is the unit currently dispatched to a runtime.
type Inflight struct {
	RunID        string `json:"runId"`
	DispatchedAt string `json:"dispatchedAt,omitempty"`
}

// refBinding is a materialized capability binding as the Conversation recorded
// it. Present on the CONVERSATION, not looked up from the pipeline: refs are
// snapshotted, so this is what the run actually had.
type refBinding struct {
	Mode string `json:"mode,omitempty"`
	Refs []Ref  `json:"refs,omitempty"`
}

// refs lists the bound names in order; nil-safe so call sites stay flat.
func (b *refBinding) refs() []string {
	if b == nil {
		return nil
	}
	out := make([]string, 0, len(b.Refs))
	for _, r := range b.Refs {
		out = append(out, r.Name)
	}
	return out
}

// Provenance is the console's read of Conversation.spec.causedBy — the
// immediate parent, one hop, never the tree's ultimate root. A member's own
// causedBy may itself be set, so a client walks it one hop at a time to reach
// the uncaused root.
type Provenance struct {
	Parent string `json:"parent"`
	Entry  string `json:"entry"`
}

// ConversationBudget is the console's read of Conversation.status.budget — a
// Coordinator-rooted conversation's own resource ceiling, PER LEVEL: nesting
// never pools a budget across levels.
type ConversationBudget struct {
	MaxAgents     int32  `json:"maxAgents,omitempty"`
	MaxTurns      int32  `json:"maxTurns,omitempty"`
	Deadline      string `json:"deadline,omitempty"`
	AgentsInvoked int32  `json:"agentsInvoked,omitempty"`
	Turns         int32  `json:"turns,omitempty"`
}

// convView is the console's read of a Conversation.
type convView struct {
	Spec struct {
		ChannelRefs []Ref `json:"channelRefs,omitempty"`
		ProfileRef  Ref   `json:"profileRef"`
		PipelineRef *Ref  `json:"pipelineRef,omitempty"`
		// CoordinatorRef: this conversation is itself a Coordinator's ROOT.
		// CausedBy: this conversation was itself INVOKED as a member. The two
		// are independent — a nested Coordinator's root carries both.
		CoordinatorRef *Ref        `json:"coordinatorRef,omitempty"`
		CausedBy       *Provenance `json:"causedBy,omitempty"`
		// Signal is what STARTED the conversation — the source and the labels
		// the adapter sent. Provenance, and the first question anybody asks of
		// an alert. Grouped on the CR because they are facts about one thing.
		Signal *struct {
			SourceRef *Ref              `json:"sourceRef,omitempty"`
			Labels    map[string]string `json:"labels,omitempty"`
		} `json:"signal,omitempty"`
		Title string `json:"title,omitempty"`
		// OriginReader names whoever STARTED this conversation, in the opaque
		// key space of the channel they started it from — what "Mine" reads,
		// since the person who typed the request is the one case a watermark
		// rule would otherwise get wrong (api/v1alpha1's own comment on the
		// field).
		OriginReader *originReaderRef `json:"originReader,omitempty"`
		Toolsets   *refBinding `json:"toolsets,omitempty"`
		MCPConfigs *refBinding `json:"mcpConfigs,omitempty"`
		Inputs     []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"inputs,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase            string          `json:"phase,omitempty"`
		Threads          []ThreadBinding `json:"threads,omitempty"`
		RuntimeContextID string          `json:"runtimeContextId,omitempty"`
		SessionID        string          `json:"sessionId,omitempty"`
		RuntimePod       string          `json:"runtimePod,omitempty"`
		Inflight         *Inflight       `json:"inflight,omitempty"`
		Runs             []Run           `json:"runs,omitempty"`
		LastActivity     string          `json:"lastActivity,omitempty"`
		// Budget/EscalatedAt/CloseReason/Brief are set only on a conversation
		// that is itself a Coordinator's root (or, for CloseReason, any closed
		// one — an ordinary /close carries none).
		Budget      *ConversationBudget `json:"budget,omitempty"`
		EscalatedAt string              `json:"escalatedAt,omitempty"`
		CloseReason string              `json:"closeReason,omitempty"`
		Brief       string              `json:"brief,omitempty"`
		Conditions  []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason,omitempty"`
			Message string `json:"message,omitempty"`
		} `json:"conditions,omitempty"`
	} `json:"status"`
}

// originReaderRef is spec.originReader as the console reads it.
type originReaderRef struct {
	Channel string `json:"channel"`
	Key     string `json:"key"`
}

// conversationView parses a cached Conversation object.
func conversationView(obj *Object) convView {
	var v convView
	if len(obj.Spec) > 0 {
		_ = json.Unmarshal(obj.Spec, &v.Spec)
	}
	if len(obj.Status) > 0 {
		_ = json.Unmarshal(obj.Status, &v.Status)
	}
	return v
}

// ConversationSummary is one row of the conversations view.
type ConversationSummary struct {
	Name    string `json:"name"`
	UID     string `json:"uid,omitempty"`
	Title   string `json:"title,omitempty"`
	Profile string `json:"profile,omitempty"`
	// Pipeline is "" when attribution is not derivable (see AttributePipeline).
	Pipeline string `json:"pipeline,omitempty"`
	// Source is the SignalSource that opened this conversation, read straight
	// off the CR. Empty on one a channel started, and on any created before the
	// manager recorded it — render as absent, never guessed.
	Source string `json:"source,omitempty"`
	// SignalLabels are the originating signal's grouping labels, kept on the CR
	// so a REBUILT card carries them. They used to live only on the
	// ConversationInput, which is deleted with the queue entry — so a card lost
	// its label table the moment the console restarted.
	SignalLabels map[string]string `json:"signalLabels,omitempty"`
	Phase        string            `json:"phase,omitempty"`
	Inflight     *Inflight         `json:"inflight,omitempty"`
	// Runs is populated in the DETAIL view only; list rows carry RunCount
	// instead, because a result is a whole agent message and thousands of them
	// do not belong in a listing.
	Runs         []Run           `json:"runs,omitempty"`
	RunCount     int             `json:"runCount"`
	Threads      []ThreadBinding `json:"threads,omitempty"`
	RuntimePod   string          `json:"runtimePod,omitempty"`
	LastActivity string          `json:"lastActivity,omitempty"`
	Created      string          `json:"created,omitempty"`
	Queued       int             `json:"queued"`
	// Joined: the console channel holds a thread binding on this conversation,
	// so the transcript is live and a message can be sent. Observed
	// conversations (joined=false) are read-only views over CR status.
	Joined bool `json:"joined"`
	// ConsoleThread is the thread id to post replies against when joined.
	ConsoleThread string `json:"consoleThread,omitempty"`
	// Deleting: the Conversation has a deletionTimestamp and is held by its
	// close-topics finalizer while its threads are archived.
	//
	// It was called `closing`, from when /close DELETED the conversation. Those
	// are now two verbs — /close sets a PHASE and the object survives, delete
	// is a second verb that only accepts an already-Closed one — so the old
	// name told an operator watching a DELETE that something was "closing",
	// which is the other operation entirely.
	//
	// Without it the list looks untouched after a delete, and the operator
	// concludes it failed and deletes again.
	Deleting bool `json:"deleting"`

	// Errored: the most recent run did not succeed. A filter facet, so "show me
	// what went wrong" is one click rather than a scan.
	Errored bool `json:"errored"`
	// Blocked explains a conversation that is not running and is not merely
	// queued — its runtime pod could not start.
	//
	// This is the console half of the 2026-08-20 outage. Five conversations sat
	// with unstartable pods for fifteen hours and the list showed nothing but a
	// phase, so the operator could see that work had stopped and not why. The
	// REASON comes from the kubelet, through the RuntimeStarted condition.
	Blocked *BlockedReason `json:"blocked,omitempty"`
	// AgeSeconds is time since last activity (creation when it never ran) —
	// server-computed so sorting and the age filter agree with each other.
	AgeSeconds float64 `json:"ageSeconds"`
	// Toolsets/MCPConfigs are the bindings this conversation MATERIALIZED.
	Toolsets   []string `json:"toolsets,omitempty"`
	MCPConfigs []string `json:"mcpConfigs,omitempty"`

	// Unread: UnreadCount is above zero. An observed conversation — one with
	// no console thread — is never unread: the console holds no watermark on
	// it and has no standing to call it new.
	Unread bool `json:"unread"`
	// UnreadCount is the number of counted-kind messages (signal, agent,
	// relay — see countedKinds) on the CONSOLE's own thread, after the
	// reader's watermark. An ack, a notice, a console user's own words and a
	// run event are never counted, whatever the watermark says.
	UnreadCount int `json:"unreadCount"`
	// LastMessage is the most recent counted-kind message on the console
	// thread, read or not — the row's snippet, and never an ack: an ack is
	// PRESENCE, shown as the pulsing dot below, not a line of transcript.
	LastMessage *LastMessage `json:"lastMessage,omitempty"`
	// Presence: a run is inflight right now. Drawn apart from Unread (design
	// decision "Presence is drawn apart from unread") — a conversation can be
	// both, either or neither.
	Presence bool `json:"presence"`
	// Mine: the requesting reader is who STARTED this conversation (its
	// OriginReader, in this console's own channel's key space). The inbox's
	// "Mine" scope. Always false with no reader resolved — there is no
	// identity to compare against.
	Mine bool `json:"mine"`
	// ReadAt is the console thread's watermark, so the browser can report a
	// read only when it would actually advance.
	ReadAt string `json:"readAt,omitempty"`

	// Coordinator is "" when this conversation is not a Coordinator's root
	// (see AttributeCoordinator). Independent of Pipeline: a conversation
	// carries exactly one originating wiring object.
	Coordinator string `json:"coordinator,omitempty"`
	// CausedBy is set when this conversation was itself INVOKED as a member —
	// the immediate parent, one hop. The client walks it to the uncaused root
	// for the incident view; there is no server-side tree endpoint (design
	// D-G).
	CausedBy *Provenance `json:"causedBy,omitempty"`
	// Budget is set only on a conversation that is itself a Coordinator's
	// root.
	Budget *ConversationBudget `json:"budget,omitempty"`
	// EscalatedAt stamps when `escalate` opened this conversation's own
	// thread. Empty on a member, which bubbles instead of escalating.
	EscalatedAt string `json:"escalatedAt,omitempty"`
	// CloseReason is why this conversation closed. Set by an ordinary /close
	// only when the caller gave one; always set by budget-exceeded and by a
	// member's own close. An UN-escalated root closing with a reason is what
	// task 5.3 marks distinctly from an escalated one that opened a thread.
	CloseReason string `json:"closeReason,omitempty"`
	// Brief is one or two sentences of what this conversation is about,
	// written by the agent — shown wherever a list would otherwise show only
	// a name (design D-I).
	Brief string `json:"brief,omitempty"`

	// lastMessageAt is the newest counted message's own timestamp — never
	// serialized, since the row needs only LastMessage's kind/sender/text.
	// Kept server-side for the two computations that need the TIME rather
	// than the text: the read report on open (design D-D) and mark unread's
	// rewind target (design D-E), so neither re-derives it from the merged
	// transcript a second time.
	lastMessageAt string
}

// readReportTime is what reporting a conversation read sends: the newest
// counted message's own time, or the conversation's activity time where that
// is later (design D-D). Never a locally generated "now" — both halves are
// read off the conversation's own state.
//
// Falls back to sortKey() when there is no counted message yet to read a
// time from — the conversation predates a message, or carries none on this
// console's thread at all — which is exactly what reporting read against
// this conversation did before counted messages existed.
func (s ConversationSummary) readReportTime() string {
	newest, ok := parseAt(s.lastMessageAt)
	if !ok {
		return s.sortKey()
	}
	if act, ok := parseAt(s.LastActivity); ok && act.After(newest) {
		return s.LastActivity
	}
	return s.lastMessageAt
}

// RewindTarget is what "mark unread" (design D-E) asks the manager to set the
// reader's own watermark to: just before the newest counted message, so that
// message counts again and nothing earlier does.
//
// Absent when there is no counted message to rewind to — mark unread has
// nothing to offer a conversation nobody has to read yet.
func (s ConversationSummary) RewindTarget() (string, bool) {
	newest, ok := parseAt(s.lastMessageAt)
	if !ok {
		return "", false
	}
	return newest.Add(-time.Nanosecond).Format(time.RFC3339Nano), true
}

// summarize projects one Conversation for the browser. consoleChannel is the
// name of the Channel this console serves; a conversation is JOINED only when
// that channel has a thread binding — a binding is what gives the send box a
// destination.
// BlockedReason is why a conversation's runtime could not start.
type BlockedReason struct {
	// Reason is the bounded classifier — VolumeUnavailable, Unschedulable,
	// ImageUnavailable, StorageUnavailable, NotStarted.
	Reason string `json:"reason"`
	// Detail is the kubelet's own words, shown on hover and in the detail view.
	// Free text: it is what turns "not running" into something actionable.
	Detail string `json:"detail,omitempty"`
	// Storage marks the subset an operator should read as "your volume is
	// broken" rather than "this pod had a bad day".
	Storage bool `json:"storage"`
}

// LastMessage is the most recent counted-kind message on a conversation's
// console thread (see countedKinds) — the row's snippet, carried as a
// kind/sender/text triple rather than a flattened string so the UI can draw
// it the way it draws the thread itself.
type LastMessage struct {
	Kind   string `json:"kind"`
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text"`
}

// summarize projects one Conversation for the browser. transcripts is nilable
// so pure CR-derived fields stay testable with no live buffer at all — a row
// with no console thread never reads it either.
func summarize(obj *Object, pipelines, coordinators []*Object, consoleChannel, reader string, transcripts *Transcripts) ConversationSummary {
	v := conversationView(obj)
	s := ConversationSummary{
		Name: obj.Metadata.Name, UID: obj.Metadata.UID, Title: v.Spec.Title,
		Profile: v.Spec.ProfileRef.Name, Pipeline: AttributePipeline(obj, pipelines),
		Coordinator: AttributeCoordinator(obj, coordinators),
		Source:      signalSource(v), SignalLabels: signalLabels(v),
		Phase: v.Status.Phase, Inflight: v.Status.Inflight, Runs: v.Status.Runs,
		Threads: v.Status.Threads, RuntimePod: v.Status.RuntimePod,
		LastActivity: v.Status.LastActivity, Created: obj.Metadata.CreationTimestamp,
		Queued:      len(v.Spec.Inputs),
		Toolsets:    v.Spec.Toolsets.refs(),
		MCPConfigs:  v.Spec.MCPConfigs.refs(),
		Deleting:    obj.Metadata.DeletionTimestamp != "",
		CausedBy:    v.Spec.CausedBy,
		Budget:      v.Status.Budget,
		EscalatedAt: v.Status.EscalatedAt,
		CloseReason: v.Status.CloseReason,
		Brief:       v.Status.Brief,
		Presence:    v.Status.Inflight != nil,
		Mine: reader != "" && v.Spec.OriginReader != nil &&
			v.Spec.OriginReader.Channel == consoleChannel && v.Spec.OriginReader.Key == reader,
	}
	// RunCount is set HERE, not only on the list path: the detail view carries
	// Runs too, and a summary that reported 0 runs beside a populated list was
	// exactly the kind of small lie a live payload makes obvious.
	s.RunCount = len(v.Status.Runs)
	if n := len(v.Status.Runs); n > 0 && v.Status.Runs[n-1].Status != "succeeded" {
		s.Errored = true
	}
	s.Blocked = blockedReason(v)
	s.AgeSeconds = ageSeconds(time.Now(), s.sortKey())
	s.joinConsoleThread(v, consoleChannel, reader)
	// Unreadness is a property of the CONSOLE's own thread, and only of it: a
	// conversation this console merely observes carries no watermark and has
	// no standing to call anything new.
	if s.Joined {
		s.applyUnread(v, consoleChannel, transcripts)
	}
	return s
}

// blockedReason reads the RuntimeStarted=False condition, the last one wins.
func blockedReason(v convView) *BlockedReason {
	var blocked *BlockedReason
	for _, c := range v.Status.Conditions {
		if c.Type == "RuntimeStarted" && c.Status == "False" {
			blocked = &BlockedReason{
				Reason: c.Reason, Detail: c.Message,
				Storage: c.Reason == "VolumeUnavailable" || c.Reason == "StorageUnavailable",
			}
		}
	}
	return blocked
}

// joinConsoleThread records the console's own thread binding, when there is one.
func (s *ConversationSummary) joinConsoleThread(v convView, consoleChannel, reader string) {
	if consoleChannel == "" {
		return
	}
	for _, t := range v.Status.Threads {
		if t.Channel == consoleChannel {
			s.Joined = true
			s.ConsoleThread = t.ThreadID
			s.ReadAt = t.watermark(reader)
		}
	}
}

// applyUnread counts what the console thread holds past the reader's watermark.
func (s *ConversationSummary) applyUnread(v convView, consoleChannel string, transcripts *Transcripts) {
	var live []Message
	if transcripts != nil {
		live = transcripts.Thread(s.ConsoleThread)
	}
	merged := mergeTranscript(s.ConsoleThread, consoleChannel, live, v.Status.Runs, *s)
	count, newest, hasNewest := countUnread(merged, s.ReadAt)
	s.UnreadCount = count
	s.Unread = count > 0
	if hasNewest {
		s.LastMessage = &LastMessage{Kind: newest.Kind, Sender: newest.Sender, Text: newest.Text}
		s.lastMessageAt = newest.At
	}
}

// sortKey orders the listing newest-activity-first. lastActivity is the field
// that matters when thousands of conversations exist; creation timestamp is
// the fallback for ones that never ran.
func (s ConversationSummary) sortKey() string {
	if s.LastActivity != "" {
		return s.LastActivity
	}
	return s.Created
}

// UnjoinedPipelines lists Ready pipelines whose channels[] does not include the
// console channel — the ones a user must edit to watch conversations live.
// The console reports them; it never edits a Pipeline.
func UnjoinedPipelines(c *Cache, consoleChannel string) []string {
	var out []string
	for _, p := range c.List("pipelines") {
		spec := decodeSpec[pipelineSpec](p.Spec)
		joined := false
		for _, ref := range spec.ChannelRefs {
			if ref.Name == consoleChannel {
				joined = true
			}
		}
		if !joined {
			out = append(out, p.Metadata.Name)
		}
	}
	return out
}

// signalSource is the SignalSource that opened a conversation, or "" when it
// was started by a channel or predates the field.
func signalSource(v convView) string {
	if v.Spec.Signal == nil || v.Spec.Signal.SourceRef == nil {
		return ""
	}
	return v.Spec.Signal.SourceRef.Name
}

// signalLabels are the originating signal's labels, or nil.
func signalLabels(v convView) map[string]string {
	if v.Spec.Signal == nil {
		return nil
	}
	return v.Spec.Signal.Labels
}
