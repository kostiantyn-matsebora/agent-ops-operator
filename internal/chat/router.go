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
// to all of them, and a user message is delivered to every bound channel except
// the surface that displayed it as it was typed. That delivery is the
// reconciler's, composed from the conversation's own inputs, so it survives a
// restart of this process. Transport-specific concerns (update parsing, offsets,
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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

// InboundMessage is one user message, already stripped of transport framing.
type InboundMessage struct {
	// ThreadID pins the message to a conversation thread; nil = the channel's
	// general surface.
	ThreadID *string
	Text     string
	// Sender is an optional transport-side identity, recorded on the input for
	// attribution when the message is delivered to the other bound surfaces.
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
	// Runtime is the bootstrap runtime config — the same value httpapi.Server
	// holds — used ONLY to answer whether a released conversation keeps its
	// context. The router builds no pods.
	Runtime runtimepod.Config
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
	// /exit releases the runtime and keeps the conversation. Intercepted for the
	// same reason /close is — the command must not become an input — but placed
	// AFTER the closed check on purpose: a closed conversation's pod is already
	// gone, so answering "nothing to release" would be technically true and
	// useless. Everything typed into a closed thread gets one answer.
	if isExitCommand(text) {
		return r.ReleaseRuntime(ctx, conv)
	}
	busy := conv.Status.Inflight != nil || len(conv.Spec.Inputs) > 0
	inputID := newInputID()
	if err := r.appendInput(ctx, conv, agentopsv1alpha1.InputItem{
		ID: inputID, Type: agentopsv1alpha1.InputReply, Payload: text, ReceivedAt: metav1.Now(),
		// Channel origin, with the sender kept: this input is delivered to every
		// OTHER bound surface as an attributed relay, and reconciliation composes
		// that from the conversation alone. Sending it from here instead would be
		// a second delivery path — in memory, lost on a restart, and unable to
		// reach a surface that renders nothing it is not sent.
		Origin: &agentopsv1alpha1.InputOrigin{
			Kind: agentopsv1alpha1.OriginChannel, Name: ch.Name, Sender: msg.Sender,
		},
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
	// FAST PATH for delivery, before the ack: the reconciler re-derives this on
	// its next pass and the op ids are stable, so calling it here costs nothing
	// and buys the ORDER a thread reads in — somebody's message, then the ack
	// that answers it. Composed by the one implementation both callers share.
	if fresh, err := r.conversation(ctx, conv.Name); err == nil {
		DeliverInputs(ctx, r.Reader, r.Ops, fresh)
	}
	ack := "🔧 On it…"
	if busy {
		ack = "⏳ Noted — I'll pick this up as soon as the current step finishes."
	}
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

// ExitCommand is the name /exit parses to — a "pipeline" that releases a
// conversation's runtime rather than starting one.
//
// One word from /close and a world away from it: /exit frees the pod and leaves
// the conversation open, /close ends the conversation and archives its thread.
// That is why both are named in the /agents listing and in every reply either
// one produces.
const ExitCommand = "exit"

// isExitCommand recognises /exit and its bot-suffixed form, with the same
// strictness as /close: "/exit the maintenance window once the rollout drains"
// is an instruction for the agent.
func isExitCommand(text string) bool {
	cmd, ok := addressing.Parse(text)
	return ok && cmd.Profile == ExitCommand && cmd.Agent == "" && cmd.Rest == ""
}

// ReleaseRuntime deletes a conversation's runtime pod and NOTHING else — the
// conversation, its threads, its inputs, its run history and its context handle
// all survive, and the next input admits it again with a fresh pod.
//
// This is eviction, reached by hand. Automatic eviction only runs when something
// is WAITING for capacity; with nothing waiting, an idle pod holds its slot, its
// checkout and whatever its runtime keeps resident until the idle TTL expires —
// and the installs that raise that TTL (large repositories, local models worth
// keeping warm) are exactly the ones where the wait is longest.
//
// The pod is deleted directly rather than flagged for the reconciler. Immediacy
// is the whole feature, and a field whose only purpose is to defer the thing
// being asked for would be state standing in for an action.
func (r *Router) ReleaseRuntime(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	// The SAME predicate that makes a pod evictable, called rather than
	// restated — see dispatch.NeedsWorker. The two refusals below differ because
	// the reasons differ: one is dangerous, the other merely pointless.
	if conv.Status.Inflight != nil {
		// Refused, not honored. Deleting a pod mid-run is doubly harmful: the
		// replacement is created immediately (Inflight makes NeedsWorker true) but
		// /work hands it nothing, so it idles out its TTL — the LONG one — and is
		// reaped as Succeeded, which clears Inflight, which makes the input
		// pending again and RE-RUNS work that may already have acted. /close
		// already owns abandonment, and owns it safely.
		r.FanOutSend(ctx, conv, Warn(fmt.Sprintf(
			"⚠️ Not released — run `%s` is still in progress, and releasing the runtime "+
				"now would stall it and then repeat it. Wait for it to finish, or use "+
				"`/close` to abandon the run and end the conversation.",
			conv.Status.Inflight.RunID)))
		return nil
	}
	if dispatch.NeedsWorker(conv) {
		r.FanOutSend(ctx, conv, Warn(
			"⚠️ Not released — there is still queued work, so the runtime would be "+
				"recreated immediately and nothing would be freed. Try again once the "+
				"agent has caught up."))
		return nil
	}
	var pod corev1.Pod
	name := types.NamespacedName{Namespace: conv.Namespace, Name: runtimepod.PodName(conv.Name)}
	if err := r.Client.Get(ctx, name, &pod); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// The desired state already holds. Not an error, and worth saying: the
		// difference between "released it" and "there was nothing running" is the
		// difference between a command that worked and one that was ignored.
		r.FanOutSend(ctx, conv, Notice(
			"💤 Nothing to release — this conversation has no runtime running. "+
				"It stays open; the next message starts one."))
		return nil
	}
	if err := client.IgnoreNotFound(r.Client.Delete(ctx, &pod)); err != nil {
		return err
	}
	r.FanOutSend(ctx, conv, Notice(r.releaseText(ctx, conv)))
	return nil
}

// releaseText says what the release COST. Continuity is not a guess: the
// manager already computes whether this conversation's runtime can carry context
// across a pod loss, and the dispatch path uses the same answer to decide
// whether to hand back a context handle at all.
//
// Where it cannot, the warning is not announcing a new loss — the idle TTL would
// have caused exactly the same one — but a release somebody CHOSE should state
// its price while they are choosing it, rather than leaving it to be discovered
// as a fault later.
func (r *Router) releaseText(ctx context.Context, conv *agentopsv1alpha1.Conversation) string {
	const kept = "♻️ Runtime released — the slot is free. This conversation and its thread " +
		"stay open, and it keeps its context: the next message picks up where this left off."
	const fresh = "♻️ Runtime released — the slot is free. This conversation and its thread " +
		"stay open, but this runtime does not keep context between runs, so the next " +
		"message starts fresh."
	var profile agentopsv1alpha1.AgentProfile
	if err := r.Reader.Get(ctx, types.NamespacedName{
		Namespace: conv.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
		// Degrade to the neutral wording rather than failing: the pod is already
		// gone, and refusing to report a completed release because a Get failed
		// would be the worst of both outcomes.
		return neutralRelease
	}
	resolved, err := runtimepod.ResolveFor(ctx, r.Reader, conv.Namespace, &profile, r.Runtime)
	if err != nil {
		return neutralRelease
	}
	if resolved.ContinuityPossible() {
		return kept
	}
	return fresh
}

const neutralRelease = "♻️ Runtime released — the slot is free. This conversation and its " +
	"thread stay open; send a message to continue."

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
	// A STABLE op id, so a close whose status write fails and is retried says
	// goodbye once rather than once per attempt. The farewell still goes FIRST:
	// a thread that simply stops is indistinguishable from a fault, so a lost
	// farewell is worse than a suppressed duplicate.
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueFarewell(ctx, ch, conv, tid, Notice(farewell))
	})
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

// FanOutReopenNotice tells every bound thread the conversation is live again.
//
// Enqueued by the reconciler AFTER topics are ensured, not by the reopen verb
// itself: a reopened thread may still be archived on its transport, and ops are
// FIFO per adapter, so letting ensure-topic go first is what makes this land in
// a thread that can receive it.
func (r *Router) FanOutReopenNotice(ctx context.Context, conv *agentopsv1alpha1.Conversation) {
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueReopenNotice(ctx, ch, conv, tid, Notice(
			"↩️ Conversation reopened — this thread is live again, with everything above it intact. "+
				"Reply here to continue."))
	})
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
//
// sender is the transport identity that typed it, when the adapter named one.
// It is carried because an addressed command is a MESSAGE like any other: it is
// delivered to every surface that did not display it, and one with no sender
// arrives there anonymous.
func (r *Router) HandleCommand(ctx context.Context, ch *agentopsv1alpha1.Channel, cmd addressing.Command, sender string) error {
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
		// Both thread commands are named TOGETHER, with the difference spelled
		// out. Two commands one word apart, one of which archives a thread, are
		// exactly the pair nobody should have to guess between.
		r.Ops.EnqueueMessage(ctx, ch, nil, Notice("🤖 **Agents**\n"+strings.Join(lines, "\n")+
			"\n\nUsage: `/<agent> <task>` — each call gets its own topic. "+
			"`/<agent>:<role>` picks a role inside the agent's repo.\n"+
			"Inside a conversation's own thread: `/exit` releases its runtime and keeps "+
			"the conversation, `/close` ends the conversation and archives the thread."))
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
	if cmd.Profile == ExitCommand {
		// Same shape as /close's: there is no conversation on a general surface,
		// so there is no runtime to release. Usage, not "unknown agent".
		r.Ops.EnqueueMessage(ctx, ch, nil, Warn("⚠️ `/exit` releases a conversation's runtime — send it "+
			"inside that conversation's own thread. Nothing was released."))
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
	_, err := r.CreateTaskConversation(ctx, ch, pipe.Spec.ProfileRef.Name, cmd.Agent, cmd.Rest, sender, &pipe)
	return err
}

// CreateTaskConversation starts a task conversation originating on a channel,
// bound to the origin Pipeline's channel set. The origin also snapshots its
// tooling bindings onto the conversation — capabilities come from the wiring
// that originated it, never from the profile.
func (r *Router) CreateTaskConversation(ctx context.Context, ch *agentopsv1alpha1.Channel, profile, agentOverride, task, sender string,
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
			// A command is typed on a surface, so its origin is that CHANNEL —
			// which is the one destination it is not delivered to, and the
			// sender is who it is attributed to everywhere else.
			Origin: &agentopsv1alpha1.InputOrigin{
				Kind: agentopsv1alpha1.OriginChannel, Name: ch.Name, Sender: sender,
			},
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

// conversation re-reads a conversation, for the paths that need the object as
// it stands AFTER their own write.
func (r *Router) conversation(ctx context.Context, name string) (*agentopsv1alpha1.Conversation, error) {
	var conv agentopsv1alpha1.Conversation
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: name}, &conv); err != nil {
		return nil, err
	}
	return &conv, nil
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
