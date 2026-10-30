package signalroute

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// The notices a chat signal dropped before fan-out is owed. A person who just
// typed into a chat is waiting; an alert dropping silently is a condition for
// the operator to find, but a question is not. They go through the existing
// in-memory channel-operation queue and are fire-and-forget by design: a lost
// notice loses no fact, because the Signal records the Dropped reason.

func unwiredNotice(source string) chat.Message {
	return chat.Warn(fmt.Sprintf(
		"⚠️ Nothing here is wired to answer. No Ready Pipeline or Coordinator serves the chat source **%s**, "+
			"so this message was dropped. Add it to one's sources to give it one.", source))
}

func unknownTargetNotice(name string) chat.Message {
	return chat.Warn(fmt.Sprintf(
		"⚠️ Unknown pipeline or coordinator **%s** — see `/%s`.", name, chat.ListCommand))
}

func capacityNotice() chat.Message {
	return chat.Warn("⚠️ At capacity — too many conversations are already " +
		"waiting for a runtime slot, so this message was dropped. Try again once the backlog clears.")
}

// ambiguousNotice renders the refusal a bare message gets on a surface several
// pipelines serve.
//
// It is the TEACHING MOMENT, not an error string: somebody who has never heard
// of the listing command finds out the addressed form exists at the one moment
// they need it, so it names every server with the profile answering for it and
// shows the form ready to copy. It also OFFERS each Pipeline as a choice: the
// person has already typed their task, so a surface with controls can send THAT
// text to the Pipeline they pick rather than making them type it again.
func ambiguousNotice(servers []chat.Claimant) chat.Message {
	var b strings.Builder
	fmt.Fprintf(&b, "🤔 **%d pipelines/coordinators serve this chat, so I don't know who you meant.**\n\n", len(servers))
	b.WriteString("Address one of them:\n")
	choices := make([]chat.Choice, 0, len(servers))
	for i := range servers {
		fmt.Fprintf(&b, "• `/%s <task>` — %s\n", servers[i].GetName(), servers[i].InlineCapability().ProfileName())
		choices = append(choices, chat.Choice{
			Label:   servers[i].GetName(),
			Command: "/" + servers[i].GetName(),
		})
	}
	fmt.Fprintf(&b, "\nYour message was not sent to anyone. `/%s` shows this list any time.", chat.ListCommand)
	msg := chat.Warn(b.String())
	msg.Choices = choices
	return msg
}

// channelOf resolves the Channel a chat signal came from. Nil when the signal
// names none or the Channel does not exist.
func channelOf(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal) *agentopsv1alpha1.Channel {
	name := sig.Spec.Labels[agentopsv1alpha1.LabelChatChannel]
	if name == "" {
		return nil
	}
	var ch agentopsv1alpha1.Channel
	if err := deps.Reader.Get(ctx, types.NamespacedName{Namespace: deps.Namespace, Name: name}, &ch); err != nil {
		return nil
	}
	return &ch
}

// tell posts one notice on the surface a chat signal came from, linked to the
// message that provoked it when the adapter supplied a handle for one.
func tell(ctx context.Context, deps Deps, ch *agentopsv1alpha1.Channel, sig *agentopsv1alpha1.Signal, msg chat.Message) {
	if deps.Ops == nil || ch == nil {
		return
	}
	msg.InReplyTo = sig.Spec.Labels[agentopsv1alpha1.LabelChatMessage]
	deps.Ops.EnqueueMessage(ctx, ch, nil, nil, msg)
}
