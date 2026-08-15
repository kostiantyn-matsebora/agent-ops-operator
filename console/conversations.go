package main

import (
	"encoding/json"
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

// unread reports whether this binding has activity newer than the reader's
// watermark. It mirrors ThreadBinding.Unread on the CRD type — the console
// reads the CR over HTTP and holds no Go dependency on the operator module.
func (t ThreadBinding) unread(lastActivity, reader string) bool {
	if !t.ReadTracked {
		return false
	}
	at := t.watermark(reader)
	if at == "" {
		return true
	}
	read, err := parseStamp(at)
	if err != nil {
		return false
	}
	act, err := parseStamp(lastActivity)
	if err != nil {
		return false
	}
	return act.After(read)
}

// parseStamp reads an API-server timestamp in either precision.
func parseStamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
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
}

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

// convView is the console's read of a Conversation.
type convView struct {
	Spec struct {
		ChannelRefs []Ref       `json:"channelRefs,omitempty"`
		ProfileRef  Ref         `json:"profileRef"`
		PipelineRef *Ref        `json:"pipelineRef,omitempty"`
		Title       string      `json:"title,omitempty"`
		Toolsets    *refBinding `json:"toolsets,omitempty"`
		MCPConfigs  *refBinding `json:"mcpConfigs,omitempty"`
		Inputs      []struct {
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
	} `json:"status"`
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
	Pipeline string    `json:"pipeline,omitempty"`
	Phase    string    `json:"phase,omitempty"`
	Inflight *Inflight `json:"inflight,omitempty"`
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
	// Closing: the Conversation is deleted and held by its close-topics
	// finalizer while the threads are archived. Without this the list looks
	// untouched after a close, the operator concludes it failed, and re-closes.
	Closing bool `json:"closing"`

	// Errored: the most recent run did not succeed. A filter facet, so "show me
	// what went wrong" is one click rather than a scan.
	Errored bool `json:"errored"`
	// AgeSeconds is time since last activity (creation when it never ran) —
	// server-computed so sorting and the age filter agree with each other.
	AgeSeconds float64 `json:"ageSeconds"`
	// Toolsets/MCPConfigs are the bindings this conversation MATERIALIZED.
	Toolsets   []string `json:"toolsets,omitempty"`
	MCPConfigs []string `json:"mcpConfigs,omitempty"`

	// Unread: the CONSOLE's own thread has activity newer than its watermark.
	// An observed conversation — one with no console thread — is never unread:
	// the console holds no watermark on it and has no standing to call it new.
	Unread bool `json:"unread"`
	// ReadAt is the console thread's watermark, so the browser can report a
	// read only when it would actually advance.
	ReadAt string `json:"readAt,omitempty"`
}

// summarize projects one Conversation for the browser. consoleChannel is the
// name of the Channel this console serves; a conversation is JOINED only when
// that channel has a thread binding — a binding is what gives the send box a
// destination.
func summarize(obj *Object, pipelines []*Object, consoleChannel, reader string) ConversationSummary {
	v := conversationView(obj)
	s := ConversationSummary{
		Name: obj.Metadata.Name, UID: obj.Metadata.UID, Title: v.Spec.Title,
		Profile: v.Spec.ProfileRef.Name, Pipeline: AttributePipeline(obj, pipelines),
		Phase: v.Status.Phase, Inflight: v.Status.Inflight, Runs: v.Status.Runs,
		Threads: v.Status.Threads, RuntimePod: v.Status.RuntimePod,
		LastActivity: v.Status.LastActivity, Created: obj.Metadata.CreationTimestamp,
		Queued:     len(v.Spec.Inputs),
		Toolsets:   v.Spec.Toolsets.refs(),
		MCPConfigs: v.Spec.MCPConfigs.refs(),
		Closing:    obj.Metadata.DeletionTimestamp != "",
	}
	// RunCount is set HERE, not only on the list path: the detail view carries
	// Runs too, and a summary that reported 0 runs beside a populated list was
	// exactly the kind of small lie a live payload makes obvious.
	s.RunCount = len(v.Status.Runs)
	if n := len(v.Status.Runs); n > 0 && v.Status.Runs[n-1].Status != "succeeded" {
		s.Errored = true
	}
	s.AgeSeconds = ageSeconds(time.Now(), s.sortKey())
	if consoleChannel != "" {
		for _, t := range v.Status.Threads {
			if t.Channel == consoleChannel {
				s.Joined = true
				s.ConsoleThread = t.ThreadID
				s.ReadAt = t.watermark(reader)
				// sortKey(), not LastActivity, so the unread mark and the
				// ordering can never disagree: unread rows are exactly a prefix
				// of the list.
				s.Unread = t.unread(s.sortKey(), reader)
			}
		}
	}
	return s
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
