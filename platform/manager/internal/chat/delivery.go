package chat

import (
	"context"

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
	Reader client.Reader
	Blobs  state.Blobs
	// Conversations is where a settled undelivered-reply note is written back.
	// Nil settles none.
	Conversations state.Collection[state.ConversationRecord]
	Namespace     string
}

// deliveryPass is the state one FanOutThread call carries across inputs.
type deliveryPass struct {
	env      Env
	conv     *state.ConversationRecord
	pipeline string
	resolved bool
	echoes   map[string]bool // surface -> does its transport show its own?
}

// inputMessage composes one input's delivery: the message every destination
// shows, the surface the input was typed on, and whether that surface's
// transport shows its own messages back (so DeliverTo can withhold it there).
func (p *deliveryPass) inputMessage(ctx context.Context, item *state.InputItem) (msg Message, surface string, echoes bool) {
	body, inputRef, labels, severity := item.Payload, "", map[string]string(nil), agentopsv1alpha1.Severity("")
	if blob, ok := InputPayload(ctx, p.env.Blobs, item); ok {
		body, inputRef, labels, severity = blob.Payload, string(item.PayloadRef), blob.Labels, agentopsv1alpha1.Severity(blob.Severity)
	}
	surface = item.OriginSurface(labels)
	echoes = p.surfaceEchoes(ctx, surface)
	if item.TypedByAPerson() {
		return RelayMessage(surface, item.OriginSender(labels), body), surface, echoes
	}
	msg = SignalMessage(p.pipelineName(ctx), item.Origin.Name, p.conv.Title, inputRef, labels, body, agentopsv1alpha1.OriginKind(item.Origin.Kind)).WithSeverity(severity)
	return msg, surface, echoes
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
