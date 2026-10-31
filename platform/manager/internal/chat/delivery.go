package chat

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Env is what a chat helper reads through: the cluster's custom resources via
// Reader, conversations and their input payloads via Store, all in one
// Namespace (a Store is bound to its namespace at construction, but the
// Reader's calls name one).
type Env struct {
	Reader    client.Reader
	Store     state.Store
	Namespace string
}

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
//
// A COORDINATOR ROOT'S CHANNELS ARE BOUND AT CREATION, NOT AT ESCALATION
// (coordinator-unconditional-channels, superseding design D-D's escalation
// fencing): every channel in `conv.ChannelRefs` already has a thread from
// the conversation's own start, so there is no "freshly bound channel" moment
// for a backlog to flood. Every input is delivered to it in the ordinary
// per-destination way, `escalate` included — it posts a notice into a thread
// that was already open, not the thread's opening post. Nothing here is fenced
// on `status.escalatedAt` any more.
func DeliverInputs(ctx context.Context, env Env, ops *OpQueue, conv *state.ConversationRecord) {
	if ops == nil || len(conv.ChannelRefs) == 0 {
		return
	}
	reader, ns := env.Reader, env.Namespace
	pipeline := ""
	resolved := false
	echoes := map[string]bool{} // surface -> does its transport show its own?
	for i := range conv.Inputs {
		item := &conv.Inputs[i]
		if item.Origin == nil { // predates provenance: delivered nowhere
			continue
		}
		body, inputRef, labels, severity := item.Payload, "", map[string]string(nil), agentopsv1alpha1.Severity("")
		if blob, ok := InputPayload(ctx, env.Store, item); ok {
			body, inputRef, labels, severity = blob.Payload, string(item.PayloadRef), blob.Labels, agentopsv1alpha1.Severity(blob.Severity)
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
			msg = SignalMessage(pipeline, item.Origin.Name, conv.Title, inputRef, labels, body, agentopsv1alpha1.OriginKind(item.Origin.Kind)).WithSeverity(severity)
		}
		for _, ref := range conv.ChannelRefs {
			if !item.DeliverTo(ref, surface, surfaceEchoes) {
				continue
			}
			tid := conv.ThreadFor(ref)
			if tid == nil {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref}, &ch); err != nil ||
				ch.Spec.Adapter == "" {
				continue
			}
			ops.EnqueueInputDelivery(ctx, &ch, conv, item.ID, tid, msg)
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

// InputPayload reads an input's out-of-line payload blob, or reports false when
// the payload is inline or the blob is gone (pruned once processed). Exported
// because the topic descriptor reads the same blob the same way, and two
// spellings of one read is how they drift.
func InputPayload(ctx context.Context, store state.Store, item *state.InputItem) (state.Blob, bool) {
	if item.PayloadRef == "" {
		return state.Blob{}, false
	}
	blob, err := store.GetBlob(ctx, item.PayloadRef)
	if err != nil {
		return state.Blob{}, false
	}
	return blob, true
}
