package main

import "encoding/json"

// The live-runs model: Conversations projected into what the UI renders.
//
// Everything here comes from CR status, which is the durable record. The
// channel transcript (transcript.go) is a live overlay on top of it and is
// deliberately allowed to be lost — runs[] survives a console restart, the
// wire does not.

// ThreadBinding pins one bound channel to its thread id.
type ThreadBinding struct {
	Channel  string `json:"channel"`
	ThreadID string `json:"threadId"`
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

// convView is the console's read of a Conversation.
type convView struct {
	Spec struct {
		ChannelRefs []Ref  `json:"channelRefs,omitempty"`
		ProfileRef  Ref    `json:"profileRef"`
		Title       string `json:"title,omitempty"`
		Inputs      []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"inputs,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase        string          `json:"phase,omitempty"`
		Threads      []ThreadBinding `json:"threads,omitempty"`
		SessionID    string          `json:"sessionId,omitempty"`
		RuntimePod   string          `json:"runtimePod,omitempty"`
		Inflight     *Inflight       `json:"inflight,omitempty"`
		Runs         []Run           `json:"runs,omitempty"`
		LastActivity string          `json:"lastActivity,omitempty"`
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
}

// summarize projects one Conversation for the browser. consoleChannel is the
// name of the Channel this console serves; a conversation is JOINED only when
// that channel has a thread binding — a binding is what gives the send box a
// destination.
func summarize(obj *Object, pipelines []*Object, consoleChannel string) ConversationSummary {
	v := conversationView(obj)
	s := ConversationSummary{
		Name: obj.Metadata.Name, UID: obj.Metadata.UID, Title: v.Spec.Title,
		Profile: v.Spec.ProfileRef.Name, Pipeline: AttributePipeline(obj, pipelines),
		Phase: v.Status.Phase, Inflight: v.Status.Inflight, Runs: v.Status.Runs,
		Threads: v.Status.Threads, RuntimePod: v.Status.RuntimePod,
		LastActivity: v.Status.LastActivity, Created: obj.Metadata.CreationTimestamp,
		Queued: len(v.Spec.Inputs),
	}
	if consoleChannel != "" {
		for _, t := range v.Status.Threads {
			if t.Channel == consoleChannel {
				s.Joined = true
				s.ConsoleThread = t.ThreadID
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
