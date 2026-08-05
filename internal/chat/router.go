// Transport-neutral inbound routing shared by every channel type (external
// adapters via POST /channel/inbound, built-ins via their own surfaces):
//
//	message inside a known thread  -> reply input on that conversation
//	/agents | /help                -> AgentProfile listing
//	/<profile>[:<agent>] <task>    -> new task conversation (own thread)
//	plain text, no thread          -> channel's defaultProfileRef conversation
//	message in an unknown thread   -> adopted as a conversation in that thread
//
// Transport-specific concerns (update parsing, offsets, approver filtering)
// stay in the adapter. Acks flow back as send ops on the OpQueue.
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
}

// Router turns inbound messages into Conversations and inputs.
type Router struct {
	Client    client.Client
	Reader    client.Reader
	Namespace string
	Ops       *OpQueue
}

// HandleMessage routes one inbound message for a channel.
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
			r.Ops.EnqueueSend(ctx, ch, msg.ThreadID, ack)
			return nil
		}
		// unknown thread: adopt as a conversation pinned to it
		return r.adoptThread(ctx, ch, *msg.ThreadID, text)
	}

	// general surface
	if cmd, ok := addressing.Parse(text); ok {
		return r.handleCommand(ctx, ch, cmd)
	}
	if ch.Spec.DefaultProfileRef == nil {
		r.Ops.EnqueueSend(ctx, ch, nil, "⚠️ No default profile on this channel — use /&lt;profile&gt; &lt;task&gt; (see /agents).")
		return nil
	}
	_, err := r.CreateTaskConversation(ctx, ch, ch.Spec.DefaultProfileRef.Name, "", text)
	return err
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
	_, err := r.CreateTaskConversation(ctx, ch, cmd.Profile, cmd.Agent, cmd.Rest)
	return err
}

// CreateTaskConversation starts a task conversation on a channel.
func (r *Router) CreateTaskConversation(ctx context.Context, ch *agentopsv1alpha1.Channel, profile, agentOverride, task string) (*agentopsv1alpha1.Conversation, error) {
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
		ChannelRef: &agentopsv1alpha1.ObjectRef{Name: ch.Name},
		ProfileRef: agentopsv1alpha1.ObjectRef{Name: profile},
		Title:      title,
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask,
			Payload: task, Agent: agentOverride, ReceivedAt: metav1.Now(),
		}},
	}
	return conv, r.Client.Create(ctx, conv)
}

// adoptThread: user wrote in a thread we don't know — bind a new conversation
// to it. Created without channelRef first so the controller can't race a topic
// creation; the threadId lands via status patch, then the channel is attached.
func (r *Router) adoptThread(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID, text string) error {
	if ch.Spec.DefaultProfileRef == nil {
		r.Ops.EnqueueSend(ctx, ch, &threadID, "⚠️ No default profile configured — I can't adopt this topic.")
		return nil
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = r.Namespace
	conv.GenerateName = "adopted-"
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef: agentopsv1alpha1.ObjectRef{Name: ch.Spec.DefaultProfileRef.Name},
		Title:      "🛠 " + strings.Join(strings.Fields(text), " "),
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask, Payload: text, ReceivedAt: metav1.Now(),
		}},
	}
	if err := r.Client.Create(ctx, conv); err != nil {
		return err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.ThreadID = &threadID
	if err := r.Client.Status().Patch(ctx, conv, patch); err != nil {
		return err
	}
	spec := client.MergeFrom(conv.DeepCopy())
	conv.Spec.ChannelRef = &agentopsv1alpha1.ObjectRef{Name: ch.Name}
	if err := r.Client.Patch(ctx, conv, spec); err != nil {
		return err
	}
	r.Ops.EnqueueSend(ctx, ch, &threadID, "🆕 New conversation adopted — working on it…")
	return nil
}

// convByThread resolves a thread id to its conversation, scoped to the channel
// (a mid-adoption conversation may not carry its channelRef yet).
func (r *Router) convByThread(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID string) *agentopsv1alpha1.Conversation {
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	for i := range list.Items {
		c := &list.Items[i]
		if c.Status.ThreadID == nil || *c.Status.ThreadID != threadID {
			continue
		}
		if c.Spec.ChannelRef == nil || c.Spec.ChannelRef.Name == ch.Name {
			return c
		}
	}
	return nil
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
