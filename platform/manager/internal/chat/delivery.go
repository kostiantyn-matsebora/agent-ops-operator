package chat

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// DELIVERING A MESSAGE TO THE SURFACES THAT HAVE NOT SHOWN IT.
//
// ONE rule, decided PER DESTINATION: an input is delivered to every bound
// channel except the surface it was typed on, because that surface showed it as
// it was typed. "Already seen" is a fact about a SURFACE, never about a
// message. The rule this replaced asked the question once, off the input's
// origin KIND, and so withheld a person's words from every channel that had
// never shown them — the console's own composer message among them.
//
// It carries what used to be two paths. An event that woke the agent goes out
// as a `signal` card; somebody's words go out as a `relay`, with origin and
// sender kept structured so each surface marks them its own way. Relaying used
// to happen inline in the router, in memory, to SIBLING channels only — which
// lost the message on a restart and could not deliver it to the surface it came
// from even when that surface renders nothing it is not sent.
//
// Called from TWO places and implemented in ONE, exactly as a run's reply is:
// the router calls it the moment it appends an input, so a thread reads in the
// order things happened, and the reconciler calls it on every pass, so a
// delivery survives the process that took it on. The op id is stable per
// conversation×input×channel, which is what lets both call it without anybody
// seeing a message twice.

// DeliverInputs enqueues every bound channel's outstanding input deliveries.
//
// Safe to call on every reconcile and on every inbound message:
//
//   - the op id is stable per conversation×input×channel, so a re-enqueue
//     dedups against both the pending map and the completed window;
//   - the decision is read off the input's recorded origin, so pre-provenance
//     inputs deliver nothing at all;
//   - whether the origin surface displayed it is read off the serving
//     ChannelAdapter, the only component that knows;
//   - a channel with no thread binding yet is skipped, and picked up on the
//     reconcile the binding triggers — enqueuing earlier would drop the message.
func DeliverInputs(ctx context.Context, reader client.Reader, ops *OpQueue, conv *agentopsv1alpha1.Conversation) {
	if ops == nil || len(conv.Spec.ChannelRefs) == 0 {
		return
	}
	ns := conv.Namespace
	pipeline := ""
	resolved := false
	echoes := map[string]bool{} // surface -> does its transport show its own?
	for i := range conv.Spec.Inputs {
		item := &conv.Spec.Inputs[i]
		if item.Origin == nil { // predates provenance: delivered nowhere
			continue
		}
		body, inputRef, labels := item.Payload, "", map[string]string(nil)
		if ci := InputPayload(ctx, reader, ns, item); ci != nil {
			body, inputRef, labels = ci.Spec.Payload, ci.Name, ci.Spec.Labels
		}
		surface := item.OriginSurface(labels)
		surfaceEchoes := true
		if surface != "" {
			known, ok := echoes[surface]
			if !ok {
				known = SurfaceEchoes(ctx, reader, ns, surface)
				echoes[surface] = known
			}
			surfaceEchoes = known
		}
		var msg Message
		if item.TypedByAPerson() {
			msg = RelayMessage(surface, item.OriginSender(labels), body)
		} else {
			if !resolved { // one lookup per pass, and only when a card needs it
				if p := PipelineForConversation(ctx, reader, ns, conv); p != nil {
					pipeline = p.Name
				}
				resolved = true
			}
			msg = SignalMessage(pipeline, item.Origin.Name, conv.Spec.Title, inputRef, labels, body)
		}
		for _, ref := range conv.Spec.ChannelRefs {
			if !item.DeliverTo(ref.Name, surface, surfaceEchoes) {
				continue
			}
			tid := conv.ThreadFor(ref.Name)
			if tid == nil {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &ch); err != nil ||
				ch.Spec.Adapter == "" {
				continue
			}
			ops.EnqueueInputDelivery(ctx, &ch, conv.Name, item.ID, tid, msg)
		}
	}
}

// SurfaceEchoes reports whether a channel shows a person the message they just
// typed on it. TRANSPORT knowledge, so it is read from the serving
// ChannelAdapter and never guessed here.
//
// An unreadable channel or adapter answers TRUE — the conservative half: the
// worst that withholds is one message from one surface, where the other way
// round posts somebody their own words back at them.
func SurfaceEchoes(ctx context.Context, reader client.Reader, ns, channel string) bool {
	var ch agentopsv1alpha1.Channel
	if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: channel}, &ch); err != nil ||
		ch.Spec.Adapter == "" {
		return true
	}
	var ca agentopsv1alpha1.ChannelAdapter
	if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: ch.Spec.Adapter}, &ca); err != nil {
		return true
	}
	return ca.Spec.Echoes()
}

// InputPayload reads an input's out-of-line ConversationInput, or nil when the
// payload is inline or the object is gone (pruned once processed). Exported
// because the topic descriptor reads the same object the same way, and two
// spellings of one read is how they drift.
func InputPayload(ctx context.Context, reader client.Reader, ns string,
	item *agentopsv1alpha1.InputItem) *agentopsv1alpha1.ConversationInput {

	if item.PayloadRef == nil {
		return nil
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: item.PayloadRef.Name}, &ci); err != nil {
		return nil
	}
	return &ci
}
