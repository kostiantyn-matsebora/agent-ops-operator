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
	busy := conv.Status.Inflight != nil || len(conv.Spec.Inputs) > 0
	if err := r.appendInput(ctx, conv, agentopsv1alpha1.InputItem{
		ID: newInputID(), Type: agentopsv1alpha1.InputReply, Payload: text, ReceivedAt: metav1.Now(),
	}); err != nil {
		return err
	}
	ack := "🔧 On it…"
	if busy {
		ack = "⏳ Noted — I'll pick this up as soon as the current step finishes."
	}
	r.relayToSiblings(ctx, conv, ch.Name, msg.Sender, text)
	r.FanOutSend(ctx, conv, ack)
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

// CloseConversation says goodbye on every bound thread and deletes the
// Conversation. Deletion does the rest through machinery that already exists:
// ownerRefs GC the runtime pod and the MCP ConfigMap, the close-topics
// finalizer archives the threads, and input pruning cleans the payload objects.
//
// No authorization check: no surface in this system authorizes individual
// senders, and inventing one here would be the only such check in it.
func (r *Router) CloseConversation(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	text := "👋 Conversation closed. This thread is archived."
	if conv.Status.Inflight != nil {
		// Honored mid-run on purpose: /close is most wanted for an agent that
		// has gone off the rails, and refusing then would make it useless.
		text = "👋 Conversation closed while a run was in progress — " +
			conv.Status.Inflight.RunID + " was abandoned. This thread is archived."
	}
	r.FanOutSend(ctx, conv, text)
	return client.IgnoreNotFound(r.Client.Delete(ctx, conv))
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
		names := make([]string, 0, len(pipelines.Items))
		for i := range pipelines.Items {
			if apimeta.IsStatusConditionTrue(pipelines.Items[i].Status.Conditions, "Ready") {
				names = append(names, "/"+pipelines.Items[i].Name)
			}
		}
		sort.Strings(names)
		r.Ops.EnqueueSend(ctx, ch, nil, "🤖 <b>Agents</b>: "+strings.Join(names, "  ")+
			"\nUsage: /&lt;agent&gt; &lt;task&gt; — each call gets its own topic. /&lt;agent&gt;:&lt;role&gt; picks a role inside the agent's repo.")
		return nil
	}
	if cmd.Profile == CloseCommand {
		// /close reaches HandleCommand only from a general surface, where there
		// is no conversation to end. Answer with usage rather than "unknown
		// agent": typing it here is an obvious mistake, not a typo'd pipeline.
		r.Ops.EnqueueSend(ctx, ch, nil, "⚠️ /close ends a conversation — send it inside that "+
			"conversation's own thread. Nothing was closed.")
		return nil
	}
	// A command addresses a PIPELINE: it originates the conversation, so it
	// supplies the profile AND the capabilities. Addressing a profile would
	// name something with no wiring, and therefore nothing to grant.
	var pipe agentopsv1alpha1.Pipeline
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: cmd.Profile}, &pipe); err != nil {
		if apierrors.IsNotFound(err) {
			r.Ops.EnqueueSend(ctx, ch, nil, fmt.Sprintf("⚠️ Unknown agent <b>%s</b> — see /agents.", cmd.Profile))
			return nil
		}
		return err
	}
	if cmd.Rest == "" {
		r.Ops.EnqueueSend(ctx, ch, nil, fmt.Sprintf("⚠️ Usage: /%s &lt;task&gt;", cmd.Profile))
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
		}},
	}
	if origin != nil {
		conv.Spec.Toolsets = origin.Spec.Toolsets.DeepCopy()
		conv.Spec.MCPConfigs = origin.Spec.MCPConfigs.DeepCopy()
	}
	return conv, r.Client.Create(ctx, conv)
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

// FanOutSend posts a message to every bound channel of a conversation, each in
// its own thread (channels without a binding yet are skipped — their topic
// catches up via reconciliation).
func (r *Router) FanOutSend(ctx context.Context, conv *agentopsv1alpha1.Conversation, text string) {
	for _, ref := range conv.Spec.ChannelRefs {
		tid := conv.ThreadFor(ref.Name)
		if tid == nil {
			continue
		}
		var bound agentopsv1alpha1.Channel
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref.Name}, &bound); err != nil {
			continue
		}
		r.Ops.EnqueueSend(ctx, &bound, tid, text)
	}
}

// relayToSiblings mirrors a user message onto the conversation's other
// channels as attributed text ("channels fully repeat the conversation").
// Channel implementations must never re-ingest their own outbound posts.
func (r *Router) relayToSiblings(ctx context.Context, conv *agentopsv1alpha1.Conversation, origin, sender, text string) {
	who := origin
	if sender != "" {
		who = origin + "/" + sender
	}
	relay := "💬 <b>" + who + "</b>: " + text
	for _, ref := range conv.Spec.ChannelRefs {
		if ref.Name == origin {
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
		r.Ops.EnqueueSend(ctx, &bound, tid, relay)
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
