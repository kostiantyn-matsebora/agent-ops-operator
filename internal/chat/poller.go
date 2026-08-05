// Telegram polling loop: one leader-elected Runnable that serves every Channel
// with spec.telegram.pollingEnabled. Routes messages into Conversations:
//
//	message inside a known topic  -> reply input on that conversation
//	/agents | /help               -> AgentProfile listing
//	/<profile>[:<agent>] <task>   -> new task conversation (own topic)
//	plain text in General         -> channel's defaultProfileRef conversation
//	message in an unknown topic   -> adopted as a conversation in that topic
//
// The getUpdates offset persists as an annotation on the Channel. Only user
// ids in spec.telegram.approvers may talk to the bot when the list is set.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/addressing"
)

// OffsetAnnotation stores the getUpdates offset per Channel.
const OffsetAnnotation = "agentops.dev/telegram-offset"

// TokenReader resolves a Channel's bot token (manager-scoped secret read).
type TokenReader func(ctx context.Context, ch *agentopsv1alpha1.Channel) (string, error)

// Poller is a manager Runnable (leader-gated by default — exactly one poller).
type Poller struct {
	Client    client.Client
	Reader    client.Reader
	Namespace string
	Token     TokenReader
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		IsTopicMessage  bool  `json:"is_topic_message"`
		MessageThreadID int64 `json:"message_thread_id"`
	} `json:"message"`
}

// Start implements manager.Runnable (leader-only: no NeedLeaderElection override).
func (p *Poller) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("telegram-poller")
	logger.Info("telegram poller started")
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var channels agentopsv1alpha1.ChannelList
		if err := p.Reader.List(ctx, &channels, client.InNamespace(p.Namespace)); err != nil {
			logger.Error(err, "list channels")
			sleepCtx(ctx, 5*time.Second)
			continue
		}
		polled := false
		for i := range channels.Items {
			ch := &channels.Items[i]
			if ch.Spec.Telegram == nil || !ch.Spec.Telegram.PollingEnabled {
				continue
			}
			polled = true
			if err := p.pollChannel(ctx, ch); err != nil && ctx.Err() == nil {
				logger.Error(err, "poll", "channel", ch.Name)
				sleepCtx(ctx, 5*time.Second)
			}
		}
		if !polled {
			sleepCtx(ctx, 10*time.Second) // nothing enabled — idle re-list
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (p *Poller) pollChannel(ctx context.Context, ch *agentopsv1alpha1.Channel) error {
	token, err := p.Token(ctx, ch)
	if err != nil {
		return err
	}
	tg := NewTelegram(token, ch.Spec.Telegram.ChatID)

	offset, _ := strconv.ParseInt(ch.Annotations[OffsetAnnotation], 10, 64)
	res, err := tg.API(ctx, "getUpdates", map[string]any{
		"timeout": 20, "offset": offset, "allowed_updates": []string{"message"},
	})
	if err != nil {
		return err
	}
	var updates []tgUpdate
	if err := json.Unmarshal(res, &updates); err != nil {
		return err
	}
	logger := log.FromContext(ctx)
	for _, upd := range updates {
		offset = upd.UpdateID + 1
		if err := p.saveOffset(ctx, ch, offset); err != nil {
			return err
		}
		m := upd.Message
		if m == nil || m.Chat == nil || strconv.FormatInt(m.Chat.ID, 10) != ch.Spec.Telegram.ChatID || m.Text == "" {
			continue
		}
		if len(ch.Spec.Telegram.Approvers) > 0 && (m.From == nil || !containsID(ch.Spec.Telegram.Approvers, m.From.ID)) {
			continue // not an approved user — ignore silently
		}
		var threadID *int64
		if m.IsTopicMessage {
			t := m.MessageThreadID
			threadID = &t
		}
		if err := p.handleMessage(ctx, ch, tg, threadID, strings.TrimSpace(m.Text)); err != nil {
			logger.Error(err, "handle message", "channel", ch.Name)
		}
	}
	return nil
}

func containsID(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func (p *Poller) saveOffset(ctx context.Context, ch *agentopsv1alpha1.Channel, offset int64) error {
	patch := client.MergeFrom(ch.DeepCopy())
	if ch.Annotations == nil {
		ch.Annotations = map[string]string{}
	}
	ch.Annotations[OffsetAnnotation] = strconv.FormatInt(offset, 10)
	return p.Client.Patch(ctx, ch, patch)
}

func (p *Poller) handleMessage(ctx context.Context, ch *agentopsv1alpha1.Channel, tg *Telegram, threadID *int64, text string) error {
	if threadID != nil {
		if conv := p.convByThread(ctx, *threadID); conv != nil {
			busy := conv.Status.Inflight != nil || len(conv.Spec.Inputs) > 0
			if err := p.appendInput(ctx, conv, agentopsv1alpha1.InputItem{
				ID: newInputID(), Type: agentopsv1alpha1.InputReply, Payload: text, ReceivedAt: metav1.Now(),
			}); err != nil {
				return err
			}
			ack := "🔧 On it…"
			if busy {
				ack = "⏳ Noted — I'll pick this up as soon as the current step finishes."
			}
			return tg.Send(ctx, threadID, ack)
		}
		// unknown topic: adopt as a conversation pinned to this thread
		return p.adoptTopic(ctx, ch, tg, *threadID, text)
	}

	// General topic
	if cmd, ok := addressing.Parse(text); ok {
		return p.handleCommand(ctx, ch, tg, cmd)
	}
	if ch.Spec.DefaultProfileRef == nil {
		return tg.Send(ctx, nil, "⚠️ No default profile on this channel — use /&lt;profile&gt; &lt;task&gt; (see /agents).")
	}
	_, err := p.createTaskConversation(ctx, ch, ch.Spec.DefaultProfileRef.Name, "", text)
	return err
}

func (p *Poller) handleCommand(ctx context.Context, ch *agentopsv1alpha1.Channel, tg *Telegram, cmd addressing.Command) error {
	if cmd.Profile == "agents" || cmd.Profile == "help" || cmd.Profile == "start" {
		var profiles agentopsv1alpha1.AgentProfileList
		if err := p.Reader.List(ctx, &profiles, client.InNamespace(p.Namespace)); err != nil {
			return err
		}
		names := make([]string, 0, len(profiles.Items))
		for _, pr := range profiles.Items {
			names = append(names, "/"+pr.Name)
		}
		sort.Strings(names)
		return tg.Send(ctx, nil, "🤖 <b>Agents</b>: "+strings.Join(names, "  ")+
			"\nUsage: /&lt;agent&gt; &lt;task&gt; — each call gets its own topic. /&lt;agent&gt;:&lt;role&gt; picks a role inside the agent's repo.")
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := p.Reader.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: cmd.Profile}, &profile); err != nil {
		if apierrors.IsNotFound(err) {
			return tg.Send(ctx, nil, fmt.Sprintf("⚠️ Unknown agent <b>%s</b> — see /agents.", cmd.Profile))
		}
		return err
	}
	if cmd.Rest == "" {
		return tg.Send(ctx, nil, fmt.Sprintf("⚠️ Usage: /%s &lt;task&gt;", cmd.Profile))
	}
	_, err := p.createTaskConversation(ctx, ch, cmd.Profile, cmd.Agent, cmd.Rest)
	return err
}

func (p *Poller) createTaskConversation(ctx context.Context, ch *agentopsv1alpha1.Channel, profile, agentOverride, task string) (*agentopsv1alpha1.Conversation, error) {
	title := "🛠 " + strings.Join(strings.Fields(task), " ")
	if agentOverride != "" || profile != "" {
		title = "🤖 " + profile + ": " + strings.Join(strings.Fields(task), " ")
	}
	if len(title) > 60 {
		title = title[:60]
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = p.Namespace
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
	return conv, p.Client.Create(ctx, conv)
}

// adoptTopic: user wrote in a topic we don't know — bind a new conversation to
// it. Created without channelRef first so the controller can't race a topic
// creation; the threadId lands via status patch, then the channel is attached.
func (p *Poller) adoptTopic(ctx context.Context, ch *agentopsv1alpha1.Channel, tg *Telegram, threadID int64, text string) error {
	if ch.Spec.DefaultProfileRef == nil {
		return tg.Send(ctx, &threadID, "⚠️ No default profile configured — I can't adopt this topic.")
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = p.Namespace
	conv.GenerateName = "adopted-"
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef: agentopsv1alpha1.ObjectRef{Name: ch.Spec.DefaultProfileRef.Name},
		Title:      "🛠 " + strings.Join(strings.Fields(text), " "),
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask, Payload: text, ReceivedAt: metav1.Now(),
		}},
	}
	if err := p.Client.Create(ctx, conv); err != nil {
		return err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.ThreadID = &threadID
	if err := p.Client.Status().Patch(ctx, conv, patch); err != nil {
		return err
	}
	spec := client.MergeFrom(conv.DeepCopy())
	conv.Spec.ChannelRef = &agentopsv1alpha1.ObjectRef{Name: ch.Name}
	if err := p.Client.Patch(ctx, conv, spec); err != nil {
		return err
	}
	return tg.Send(ctx, &threadID, "🆕 New conversation adopted — working on it…")
}

func (p *Poller) convByThread(ctx context.Context, threadID int64) *agentopsv1alpha1.Conversation {
	var list agentopsv1alpha1.ConversationList
	if err := p.Reader.List(ctx, &list, client.InNamespace(p.Namespace)); err != nil {
		return nil
	}
	for i := range list.Items {
		if t := list.Items[i].Status.ThreadID; t != nil && *t == threadID {
			return &list.Items[i]
		}
	}
	return nil
}

func (p *Poller) appendInput(ctx context.Context, conv *agentopsv1alpha1.Conversation, item agentopsv1alpha1.InputItem) error {
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := p.Reader.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: conv.Name}, &fresh); err != nil {
			return err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, item)
		if err := p.Client.Patch(ctx, &fresh, patch); err != nil {
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
