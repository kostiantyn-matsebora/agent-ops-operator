// Transport-neutral inbound routing shared by every channel type (external
// adapters via POST /channel/inbound, built-ins via their own surfaces):
//
//	message inside a known thread  -> reply input on that conversation
//	/agents | /help                -> AgentProfile listing
//	/<profile>[:<agent>] <task>    -> new task conversation (own thread)
//	plain text, no thread          -> default-profile conversation
//	message in an unknown thread   -> adopted as a conversation in that thread
//
// Conversations are bound to a SET of channels (the originating channel's
// Ready Pipeline's channels, or just the originating channel): every bound
// channel mirrors the whole conversation — acks fan out to all of them and a
// user message on one channel is relayed to the siblings as attributed text.
// Transport-specific concerns (update parsing, offsets, approver filtering)
// stay in the adapter. Outbound flows back as send ops on the OpQueue.
package chat

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// HandleMessage routes one inbound message arriving on a channel.
func (r *Router) HandleMessage(ctx context.Context, ch *agentopsv1alpha1.Channel, msg InboundMessage) error {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}
	if msg.ThreadID != nil {
		if conv := r.convByThread(ctx, ch, *msg.ThreadID); conv != nil {
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
		// unknown thread: adopt as a conversation pinned to it
		return r.adoptThread(ctx, ch, *msg.ThreadID, text)
	}

	// general surface
	if cmd, ok := addressing.Parse(text); ok {
		return r.handleCommand(ctx, ch, cmd)
	}
	if p := r.defaultPipeline(ctx, ch); p != nil {
		_, err := r.CreateTaskConversation(ctx, ch, p.Spec.ProfileRef.Name, "", text, p)
		return err
	}
	r.Ops.EnqueueSend(ctx, ch, nil, "⚠️ No default profile on this channel — use /&lt;profile&gt; &lt;task&gt; (see /agents).")
	return nil
}

// defaultPipeline resolves the wiring for bare messages — pipeline-only: the
// oldest Ready Pipeline referencing this channel supplies both the default
// profile and the tooling bindings; channels in no pipeline have no default
// (nil → guidance message).
func (r *Router) defaultPipeline(ctx context.Context, ch *agentopsv1alpha1.Channel) *agentopsv1alpha1.Pipeline {
	return PipelineForChannel(ctx, r.Client, r.Namespace, ch.Name)
}

// boundChannels resolves the channel set a new conversation originating on ch
// binds to: the channel's Ready Pipeline's channels (origin guaranteed
// included), or just the origin.
func (r *Router) boundChannels(ctx context.Context, ch *agentopsv1alpha1.Channel) []agentopsv1alpha1.ObjectRef {
	if p := PipelineForChannel(ctx, r.Client, r.Namespace, ch.Name); p != nil {
		refs := append([]agentopsv1alpha1.ObjectRef{}, p.Spec.ChannelRefs...)
		found := false
		for _, ref := range refs {
			if ref.Name == ch.Name {
				found = true
			}
		}
		if !found {
			refs = append(refs, agentopsv1alpha1.ObjectRef{Name: ch.Name})
		}
		return refs
	}
	return []agentopsv1alpha1.ObjectRef{{Name: ch.Name}}
}

func (r *Router) handleCommand(ctx context.Context, ch *agentopsv1alpha1.Channel, cmd addressing.Command) error {
	if cmd.Profile == "agents" || cmd.Profile == "help" || cmd.Profile == "start" {
		var profiles agentopsv1alpha1.AgentProfileList
		if err := r.Reader.List(ctx, &profiles, client.InNamespace(r.Namespace)); err != nil {
			return err
		}
		names := make([]string, 0, len(profiles.Items))
		for _, pr := range profiles.Items {
			names = append(names, "/"+pr.Name)
		}
		sort.Strings(names)
		r.Ops.EnqueueSend(ctx, ch, nil, "🤖 <b>Agents</b>: "+strings.Join(names, "  ")+
			"\nUsage: /&lt;agent&gt; &lt;task&gt; — each call gets its own topic. /&lt;agent&gt;:&lt;role&gt; picks a role inside the agent's repo.")
		return nil
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: cmd.Profile}, &profile); err != nil {
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
	// Explicitly addressed profile: the channel's pipeline supplies the mirrored
	// channel set but NOT its capabilities — the named profile is not that
	// pipeline's, so its toolsets would grant tools meant for a different agent.
	// The named profile's own baseline supplies them instead.
	_, err := r.CreateTaskConversation(ctx, ch, cmd.Profile, cmd.Agent, cmd.Rest, nil)
	return err
}

// CreateTaskConversation starts a task conversation originating on a channel,
// bound to the channel's resolved set (pipeline channels or just the origin).
// A non-nil origin pipeline also snapshots its tooling bindings onto the
// conversation; nil leaves the conversation on the profile's own tooling.
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
		ChannelRefs: r.boundChannels(ctx, ch),
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

// adoptThread: user wrote in a thread we don't know — bind a new conversation
// to it. Created without channelRefs first so the controller can't race a
// topic creation on the origin; the origin's thread binding lands via status
// patch, then the full channel set is attached (sibling topics are ensured by
// the controller from there).
func (r *Router) adoptThread(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID, text string) error {
	p := r.defaultPipeline(ctx, ch)
	if p == nil {
		r.Ops.EnqueueSend(ctx, ch, &threadID, "⚠️ No default profile configured — I can't adopt this topic.")
		return nil
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = r.Namespace
	conv.GenerateName = "adopted-"
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef: p.Spec.ProfileRef,
		Toolsets:   p.Spec.Toolsets.DeepCopy(),
		MCPConfigs: p.Spec.MCPConfigs.DeepCopy(),
		Title:      "🛠 " + strings.Join(strings.Fields(text), " "),
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask, Payload: text, ReceivedAt: metav1.Now(),
		}},
	}
	if err := r.Client.Create(ctx, conv); err != nil {
		return err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: ch.Name, ThreadID: threadID}}
	if err := r.Client.Status().Patch(ctx, conv, patch); err != nil {
		return err
	}
	spec := client.MergeFrom(conv.DeepCopy())
	conv.Spec.ChannelRefs = r.boundChannels(ctx, ch)
	if err := r.Client.Patch(ctx, conv, spec); err != nil {
		return err
	}
	r.Ops.EnqueueSend(ctx, ch, &threadID, "🆕 New conversation adopted — working on it…")
	return nil
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
