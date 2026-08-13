// Transport-neutral inbound routing shared by every channel type (external
// adapters via POST /channel/inbound, built-ins via their own surfaces):
//
//	message inside a known thread  -> reply input on that conversation
//	message in an unknown thread   -> dropped (nothing to continue)
//
// CHANNELS DO NOT ORIGINATE. A Conversation is born only from a signal routed
// against a SignalSource some Ready Pipeline claims, so who answers is
// DECLARED by that claim rather than inferred. Chat is no exception: a message
// on a channel's general surface reaches the manager as a chat signal from a
// chat source (see internal/httpapi/signals.go), through the same claim check,
// cooldown, grouping and observability as an alert or a cron job.
//
// That is why there is no adoptThread, no bare-text branch, and no
// PipelineForChannel here any more. Each was a way for a channel to start a
// conversation with nothing having claimed it for the purpose — which forced
// "whichever Ready pipeline was created first" to stand in for an answer, a
// tiebreak this package refuses to make anywhere else.
//
// Conversations are bound to a SET of channels (the originating Pipeline's
// channels): every bound channel mirrors the whole conversation — acks fan out
// to all of them and a user message on one channel is relayed to the siblings
// as attributed text. Transport-specific concerns (update parsing, offsets,
// approver filtering) stay in the adapter. Outbound flows back as send ops on
// the OpQueue.
package chat

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/addressing"
)

// InboundMessage is one user message, already stripped of transport framing.
type InboundMessage struct {
	// ThreadID pins the message to a conversation thread; nil = the channel's
	// general surface.
	ThreadID *string
	Text     string
	// Sender is an optional transport-side identity, used only for attribution
	// when relaying to sibling channels.
	Sender string
}

// Router turns inbound messages into Conversations and inputs.
type Router struct {
	Client    client.Client
	Reader    client.Reader
	Namespace string
	Ops       *OpQueue
	// Activity records the inbound hop. Nil is inert.
	Activity *activity.Log
}

// HandleMessage routes one inbound message arriving on a channel. Reply-only:
// a message continues the conversation bound to its thread, or it is dropped.
//
// ThreadID is required by POST /channel/inbound — a nil one cannot reach here
// through the contract. An UNKNOWN thread is dropped rather than adopted:
// adoption was origination wearing a different name.
func (r *Router) HandleMessage(ctx context.Context, ch *agentopsv1alpha1.Channel, msg InboundMessage) error {
	text := strings.TrimSpace(msg.Text)
	if text == "" || msg.ThreadID == nil {
		return nil
	}
	conv := r.convByThread(ctx, ch, *msg.ThreadID)
	if conv == nil {
		return nil // no conversation in this thread — nothing to continue
	}
	// /close ends the conversation. Intercepted BEFORE the text becomes a reply
	// input: handing it to the agent would both dispatch a work unit for a
	// command and leave the conversation open.
	if isCloseCommand(text) {
		return r.CloseConversation(ctx, conv)
	}
	// A CLOSED conversation is inert, so an input appended here would sit
	// forever and never dispatch — a silent black hole on a surface where
	// somebody is waiting for an answer. Say so instead, and create nothing:
	// the same shape every command whose whole result is a reply already takes.
	//
	// Deliberately NOT an implicit reopen: reopening re-materialises threads on
	// every bound channel, which is a decision, not something a stray "thanks"
	// in an archived topic should trigger.
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		r.Ops.EnqueueMessage(ctx, ch, msg.ThreadID, Notice(
			"This conversation is closed, so nothing here reaches the agent. "+
				"Reopen it from the console to continue with its history."))
		return nil
	}
	busy := conv.Status.Inflight != nil || len(conv.Spec.Inputs) > 0
	inputID := newInputID()
	if err := r.appendInput(ctx, conv, agentopsv1alpha1.InputItem{
		ID: inputID, Type: agentopsv1alpha1.InputReply, Payload: text, ReceivedAt: metav1.Now(),
		// Channel origin: the person is looking at the surface they typed on, so
		// this input is never posted back as a card. Siblings see it through
		// relayToSiblings instead.
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: ch.Name},
	}); err != nil {
		return err
	}
	pipeline := ""
	if p := PipelineForConversation(ctx, r.Reader, r.Namespace, conv); p != nil {
		pipeline = p.Name
	}
	// From the CHANNEL, not its adapter: a reply travels surface -> conversation,
	// and naming the channel is what lets a graph light the wiring edge it
	// actually crossed. The adapter's own hop is its op completion.
	r.Activity.Emit(activity.Event{
		Kind:         activity.KindChannelInbound,
		From:         activity.Node(activity.NodeChannel, ch.Name),
		To:           activity.Node(activity.NodeConversation, conv.Name),
		Conversation: conv.Name, Pipeline: pipeline, InputID: inputID,
		Detail: "reply on " + ch.Name,
	})
	ack := "🔧 On it…"
	if busy {
		ack = "⏳ Noted — I'll pick this up as soon as the current step finishes."
	}
	r.relayToSiblings(ctx, conv, ch.Name, msg.Sender, text)
	r.FanOutSend(ctx, conv, Notice(ack))
	return nil
}

// CloseCommand is the name /close parses to — a "pipeline" that ends a
// conversation rather than starting one.
const CloseCommand = "close"

// isCloseCommand recognises /close and its bot-suffixed form (/close@SomeBot).
// Deliberately strict about trailing text: "/close the incident once you have
// filed it" is an instruction for the agent, not a command for the manager.
func isCloseCommand(text string) bool {
	cmd, ok := addressing.Parse(text)
	return ok && cmd.Profile == CloseCommand && cmd.Agent == "" && cmd.Rest == ""
}

// CloseConversation says goodbye on every bound thread and closes the
// Conversation — /close, the console batch and the manager's timer all arrive
// here, which is what keeps the farewell, the teardown and the capacity release
// from drifting between originators.
//
// No authorization check: no surface in this system authorizes individual
// senders, and inventing one here would be the only such check in it.
func (r *Router) CloseConversation(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	text := "👋 Conversation closed. This thread is archived — reply here and I " +
		"will start a fresh conversation, or reopen this one from the console to " +
		"continue with its history."
	if conv.Status.Inflight != nil {
		// Honored mid-run on purpose: /close is most wanted for an agent that
		// has gone off the rails, and refusing then would make it useless.
		text = "👋 Conversation closed while a run was in progress — " +
			conv.Status.Inflight.RunID + " was abandoned. This thread is archived, " +
			"and the conversation can be reopened from the console."
	}
	return r.closeConversation(ctx, conv, text)
}

// AutoCloseConversation is the timer's entry point. It differs from a manual
// close in WORDS ONLY — a conversation ended by a timer needs the farewell more
// than one ended by hand, and it has to name the window so the person reading it
// can find the setting responsible.
func (r *Router) AutoCloseConversation(ctx context.Context, conv *agentopsv1alpha1.Conversation, idleFor time.Duration) error {
	return r.closeConversation(ctx, conv, fmt.Sprintf(
		"👋 Closed automatically after %s with no activity. This thread is "+
			"archived and nothing was lost — the conversation and its answers are "+
			"still readable, and it can be reopened from the console to continue "+
			"with its history.", idleFor.Round(time.Minute)))
}

// closeConversation is the ONE implementation. Its final step is a STATUS
// WRITE, not a delete: a closed conversation is inert but intact, so its
// recorded runs, its context handle and its volume state all survive and it can
// be reopened. Deletion is a second verb with its own flag and its own clock,
// measured from the ClosedAt stamped here.
//
// The teardown that used to ride on deletion — runtime pod, MCP ConfigMap,
// close-topic ops, capacity — is the reconciler's, driven off the phase. Doing
// it here as well would be a second implementation of it.
func (r *Router) closeConversation(ctx context.Context, conv *agentopsv1alpha1.Conversation, farewell string) error {
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		return nil // already closed: no second farewell
	}
	r.FanOutSend(ctx, conv, Notice(farewell))
	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &now
	return client.IgnoreNotFound(r.Client.Status().Patch(ctx, conv, patch))
}

// ReopenConversation brings a closed conversation back to Idle.
//
// The materialized refs are left EXACTLY as they are. That is the whole design:
// refs are snapshots and their CONTENT is re-read at every use, so a reopen that
// "re-resolved" wiring would do the one thing the snapshot rule forbids — let a
// Pipeline edit re-wire a conversation that already exists. A reopened
// conversation is the SAME conversation with the same profile and the same
// capabilities, or it is a new conversation wearing an old name.
//
// Continuity is restored where it was PROMISED: under contextStorage: volume the
// workspace and the context handle are both still there, so the agent resumes;
// under none it answers fresh and says so, exactly as a resume already does.
//
// Threads are re-established by the reconciler's ordinary ensure-topic pass,
// which carries the archived thread id as a hint (see TopicDescriptor).
func (r *Router) ReopenConversation(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	if conv.Status.Phase != agentopsv1alpha1.ConversationClosed {
		return fmt.Errorf("conversation %s is not closed", conv.Name)
	}
	if err := r.validateRefs(ctx, conv); err != nil {
		return err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Phase = agentopsv1alpha1.ConversationIdle
	conv.Status.ClosedAt = nil
	conv.Status.Reopens++
	now := metav1.Now()
	conv.Status.LastActivity = &now
	return r.Client.Status().Patch(ctx, conv, patch)
}

// validateRefs fails a reopen that would produce a conversation which cannot
// dispatch, NAMING the missing object. Never partially reopen and never
// silently drop a binding: a conversation reopened without its profile is one
// that looks alive and answers nothing.
func (r *Router) validateRefs(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	if conv.Spec.ProfileRef.Name == "" {
		return fmt.Errorf("conversation %s names no profile", conv.Name)
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("cannot reopen %s: AgentProfile %q no longer exists", conv.Name, conv.Spec.ProfileRef.Name)
		}
		return err
	}
	for _, ref := range conv.Spec.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: ref.Name}, &ch); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("cannot reopen %s: Channel %q no longer exists", conv.Name, ref.Name)
			}
			return err
		}
	}
	return nil
}

// boundChannels resolves the channel set a new conversation binds to: the
// originating Pipeline's channels, with the originating channel guaranteed
// included (it is where the user is looking).
func (r *Router) boundChannels(origin *agentopsv1alpha1.Pipeline, ch *agentopsv1alpha1.Channel) []agentopsv1alpha1.ObjectRef {
	if origin == nil {
		return []agentopsv1alpha1.ObjectRef{{Name: ch.Name}}
	}
	refs := append([]agentopsv1alpha1.ObjectRef{}, origin.Spec.ChannelRefs...)
	for _, ref := range refs {
		if ref.Name == ch.Name {
			return refs
		}
	}
	return append(refs, agentopsv1alpha1.ObjectRef{Name: ch.Name})
}

// HandleCommand answers chat input that addresses a pipeline. Called from the
// CHAT SIGNAL path, which is where origination lives — commands that only
// produce a response emit a send op and create no Conversation.
func (r *Router) HandleCommand(ctx context.Context, ch *agentopsv1alpha1.Channel, cmd addressing.Command) error {
	if cmd.Profile == "agents" || cmd.Profile == "help" || cmd.Profile == "start" {
		// List PIPELINES: they are what a message addresses, and what carries
		// the capabilities the resulting conversation will have. Listing
		// profiles would name things a user cannot actually address.
		var pipelines agentopsv1alpha1.PipelineList
		if err := r.Reader.List(ctx, &pipelines, client.InNamespace(r.Namespace)); err != nil {
			return err
		}
		// Each entry carries its answering PROFILE, matching what a surface with
		// input assistance offers in its typeahead. Two agents on one surface is
		// now an ordinary configuration — a source is shareable — and a bare
		// list of names is not enough to choose between them.
		lines := make([]string, 0, len(pipelines.Items))
		for i := range pipelines.Items {
			p := &pipelines.Items[i]
			if !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
				continue
			}
			line := "• `/" + p.Name + "`"
			if profile := p.Spec.ProfileRef.Name; profile != "" {
				line += " — " + profile
			}
			lines = append(lines, line)
		}
		sort.Strings(lines)
		r.Ops.EnqueueMessage(ctx, ch, nil, Notice("🤖 **Agents**\n"+strings.Join(lines, "\n")+
			"\n\nUsage: `/<agent> <task>` — each call gets its own topic. "+
			"`/<agent>:<role>` picks a role inside the agent's repo."))
		return nil
	}
	if cmd.Profile == CloseCommand {
		// /close reaches HandleCommand only from a general surface, where there
		// is no conversation to end. Answer with usage rather than "unknown
		// agent": typing it here is an obvious mistake, not a typo'd pipeline.
		r.Ops.EnqueueMessage(ctx, ch, nil, Warn("⚠️ `/close` ends a conversation — send it inside that "+
			"conversation's own thread. Nothing was closed."))
		return nil
	}
	// A command addresses a PIPELINE: it originates the conversation, so it
	// supplies the profile AND the capabilities. Addressing a profile would
	// name something with no wiring, and therefore nothing to grant.
	var pipe agentopsv1alpha1.Pipeline
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: cmd.Profile}, &pipe); err != nil {
		if apierrors.IsNotFound(err) {
			r.Ops.EnqueueMessage(ctx, ch, nil, Warn(fmt.Sprintf("⚠️ Unknown agent **%s** — see `/agents`.", cmd.Profile)))
			return nil
		}
		return err
	}
	if cmd.Rest == "" {
		r.Ops.EnqueueMessage(ctx, ch, nil, Warn(fmt.Sprintf("⚠️ Usage: `/%s <task>`", cmd.Profile)))
		return nil
	}
	_, err := r.CreateTaskConversation(ctx, ch, pipe.Spec.ProfileRef.Name, cmd.Agent, cmd.Rest, &pipe)
	return err
}

// CreateTaskConversation starts a task conversation originating on a channel,
// bound to the origin Pipeline's channel set. The origin also snapshots its
// tooling bindings onto the conversation — capabilities come from the wiring
// that originated it, never from the profile.
func (r *Router) CreateTaskConversation(ctx context.Context, ch *agentopsv1alpha1.Channel, profile, agentOverride, task string,
	origin *agentopsv1alpha1.Pipeline) (*agentopsv1alpha1.Conversation, error) {
	title := "🛠 " + strings.Join(strings.Fields(task), " ")
	if agentOverride != "" || profile != "" {
		title = "🤖 " + profile + ": " + strings.Join(strings.Fields(task), " ")
	}
	if len(title) > 60 {
		title = title[:60]
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = r.Namespace
	conv.GenerateName = "task-"
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ChannelRefs: r.boundChannels(origin, ch),
		ProfileRef:  agentopsv1alpha1.ObjectRef{Name: profile},
		Title:       title,
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask,
			Payload: task, Agent: agentOverride, ReceivedAt: metav1.Now(),
			// A command is typed on a surface, so it is a channel origin and
			// posts no card — the user already sees what they sent.
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: ch.Name},
		}},
	}
	if origin != nil {
		conv.Spec.Toolsets = origin.Spec.Toolsets.DeepCopy()
		conv.Spec.MCPConfigs = origin.Spec.MCPConfigs.DeepCopy()
		// Provenance, written once at creation like the bindings above it and
		// read for the same reasons they are not: attribution and reuse
		// scoping, never to resolve wiring. An addressed command is the one
		// origination that names its pipeline outright, so this ref is exact
		// rather than inferred.
		conv.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: origin.Name}
	}
	if err := r.Client.Create(ctx, conv); err != nil {
		return conv, err
	}
	originName, originKind := ch.Name, activity.NodeChannel
	pipeline := ""
	if origin != nil {
		originName, originKind, pipeline = origin.Name, activity.NodePipeline, origin.Name
	}
	r.Activity.Emit(activity.Event{
		Kind:         activity.KindConversationCreated,
		From:         activity.Node(originKind, originName),
		To:           activity.Node(activity.NodeConversation, conv.Name),
		Conversation: conv.Name, Pipeline: pipeline,
		InputID: conv.Spec.Inputs[0].ID, Detail: title,
	})
	return conv, nil
}

// convByThread resolves a (channel, thread) pair to its conversation. Thread
// ids are opaque strings scoped per channel — no cross-channel collisions.
func (r *Router) convByThread(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID string) *agentopsv1alpha1.Conversation {
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	for i := range list.Items {
		c := &list.Items[i]
		bound := c.ThreadFor(ch.Name)
		if bound == nil || *bound != threadID {
			continue
		}
		if len(c.Spec.ChannelRefs) == 0 || c.BoundTo(ch.Name) {
			return c
		}
	}
	return nil
}

// FanOutSend posts ONE SEMANTIC MESSAGE to every bound channel of a
// conversation, each in its own thread (channels without a binding yet are
// skipped — their topic catches up via reconciliation).
//
// Each adapter renders it independently, so the same answer may look different
// on Telegram and on the web. That is the point of fanning out meaning rather
// than markup.
func (r *Router) FanOutSend(ctx context.Context, conv *agentopsv1alpha1.Conversation, msg Message) {
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueMessage(ctx, ch, tid, msg)
	})
}

// FanOutRunReply posts ONE RUN'S ANSWER to every bound thread that has not
// already received it. Unlike FanOutSend it is DERIVABLE: the op id is stable
// per conversation×channel×run and delivery is recorded on the run, so both the
// `/work/done` fast path and the reconciler backstop can call it and the second
// caller is a no-op.
//
// A thread already marked delivered is skipped before the queue is even asked —
// the queue's dedup window is bounded, and after a restart it is empty, so the
// CR is the only thing that can tell an undelivered reply from a delivered one.
func (r *Router) FanOutRunReply(ctx context.Context, conv *agentopsv1alpha1.Conversation, runID string, msg Message) {
	var run *agentopsv1alpha1.RunStatus
	for i := range conv.Status.Runs {
		if conv.Status.Runs[i].RunID == runID {
			run = &conv.Status.Runs[i]
			break
		}
	}
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		if run != nil && run.DeliveredTo(ch.Name) {
			return
		}
		r.Ops.EnqueueRunReply(ctx, ch, conv.Name, runID, tid, msg)
	})
}

// relayToSiblings mirrors a user message onto the conversation's other
// channels ("channels fully repeat the conversation"). The attribution stays
// STRUCTURED — origin and sender as fields, not composed into the body — so
// each surface decides how to mark somebody else's words.
// Channel implementations must never re-ingest their own outbound posts.
func (r *Router) relayToSiblings(ctx context.Context, conv *agentopsv1alpha1.Conversation, origin, sender, text string) {
	msg := RelayMessage(origin, sender, text)
	r.eachBoundThread(ctx, conv, origin, func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueMessage(ctx, ch, tid, msg)
	})
}

// eachBoundThread runs fn for every bound channel that already has a thread,
// optionally skipping one by name. Channels with no binding are skipped rather
// than queued: a send to a thread that does not exist has nowhere to land.
func (r *Router) eachBoundThread(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	skip string, fn func(ch *agentopsv1alpha1.Channel, threadID *string)) {

	for _, ref := range conv.Spec.ChannelRefs {
		if ref.Name == skip {
			continue
		}
		tid := conv.ThreadFor(ref.Name)
		if tid == nil {
			continue
		}
		var bound agentopsv1alpha1.Channel
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref.Name}, &bound); err != nil {
			continue
		}
		fn(&bound, tid)
	}
}

func (r *Router) appendInput(ctx context.Context, conv *agentopsv1alpha1.Conversation, item agentopsv1alpha1.InputItem) error {
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: conv.Name}, &fresh); err != nil {
			return err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, item)
		if err := r.Client.Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("conflict appending input to %s", conv.Name)
}

func newInputID() string {
	return "in-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
