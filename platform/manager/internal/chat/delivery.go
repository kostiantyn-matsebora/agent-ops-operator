package chat

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Env is what a chat helper reads through: the cluster's custom resources via
// Reader, queued inputs' out-of-line payloads via Blobs, all in one Namespace
// (Blobs is bound to its namespace at construction, but the Reader's calls
// name one).
type Env struct {
	Reader    client.Reader
	Blobs     state.Blobs
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
	DeliverInputsInOrder(ctx, env, ops, conv, nil)
}

// DeliverInputsInOrder is DeliverInputs that calls flushBefore(t) ahead of
// each input received at t, so the caller can enqueue what happened earlier
// (a run's reply) first. A channel's ops are claimed in the order they were
// enqueued, so this is what keeps a thread reading in the order things
// happened: a run's answer before the hand-back its own turn caused.
func DeliverInputsInOrder(ctx context.Context, env Env, ops *OpQueue, conv *state.ConversationRecord, flushBefore func(time.Time)) {
	if ops == nil || len(conv.ChannelRefs) == 0 {
		return
	}
	p := &deliveryPass{env: env, ops: ops, conv: conv, echoes: map[string]bool{}}
	for i := range conv.Inputs {
		item := &conv.Inputs[i]
		if item.Origin == nil { // predates provenance: delivered nowhere
			continue
		}
		if flushBefore != nil {
			flushBefore(item.ReceivedAt)
		}
		p.deliver(ctx, item)
	}
}

// deliveryPass is the state one DeliverInputs call carries across inputs.
type deliveryPass struct {
	env      Env
	ops      *OpQueue
	conv     *state.ConversationRecord
	pipeline string
	resolved bool
	echoes   map[string]bool // surface -> does its transport show its own?
}

// deliver enqueues one input to every bound channel that should show it.
func (p *deliveryPass) deliver(ctx context.Context, item *state.InputItem) {
	body, inputRef, labels, severity := item.Payload, "", map[string]string(nil), agentopsv1alpha1.Severity("")
	if blob, ok := InputPayload(ctx, p.env.Blobs, item); ok {
		body, inputRef, labels, severity = blob.Payload, string(item.PayloadRef), blob.Labels, agentopsv1alpha1.Severity(blob.Severity)
	}
	surface := item.OriginSurface(labels)
	surfaceEchoes := p.surfaceEchoes(ctx, surface)
	var msg Message
	if item.TypedByAPerson() {
		msg = RelayMessage(surface, item.OriginSender(labels), body)
	} else {
		msg = SignalMessage(p.pipelineName(ctx), item.Origin.Name, p.conv.Title, inputRef, labels, body, agentopsv1alpha1.OriginKind(item.Origin.Kind)).WithSeverity(severity)
	}
	for _, ref := range p.conv.ChannelRefs {
		if !item.DeliverTo(ref, surface, surfaceEchoes) {
			continue
		}
		tid := p.conv.ThreadFor(ref)
		if tid == nil {
			continue
		}
		var ch agentopsv1alpha1.Channel
		if err := p.env.Reader.Get(ctx, types.NamespacedName{Namespace: p.env.Namespace, Name: ref}, &ch); err != nil ||
			ch.Spec.Adapter == "" {
			continue
		}
		p.ops.EnqueueInputDelivery(ctx, &ch, p.conv, item.ID, tid, msg)
	}
}

// surfaceEchoes answers once per surface per pass.
func (p *deliveryPass) surfaceEchoes(ctx context.Context, surface string) bool {
	if surface == "" {
		return true
	}
	known, ok := p.echoes[surface]
	if !ok {
		known = SurfaceEchoes(ctx, p.env.Reader, p.env.Namespace, surface)
		p.echoes[surface] = known
	}
	return known
}

// pipelineName is one lookup per pass, and only when a card needs it.
func (p *deliveryPass) pipelineName(ctx context.Context) string {
	if !p.resolved {
		if pl := PipelineForConversation(ctx, p.env.Reader, p.env.Namespace, p.conv); pl != nil {
			p.pipeline = pl.Name
		}
		p.resolved = true
	}
	return p.pipeline
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
func InputPayload(ctx context.Context, blobs state.Blobs, item *state.InputItem) (state.Blob, bool) {
	if item.PayloadRef == "" {
		return state.Blob{}, false
	}
	blob, err := blobs.Get(ctx, item.PayloadRef)
	if err != nil {
		return state.Blob{}, false
	}
	return blob, true
}
