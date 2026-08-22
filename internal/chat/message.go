package chat

import (
	"strings"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// THE MANAGER COMPOSES MEANING; ADAPTERS COMPOSE PRESENTATION.
//
// An outbound op carries a TYPED message, never a rendered string. The manager
// decides WHAT is being said — this is an event, this is the agent's answer,
// this is somebody else's message mirrored here — and the adapter serving the
// surface decides how it looks: which markup, how long a message may be, what
// to do when it is longer, whether labels become a table or a line of k=v.
//
// The rule this replaces was a leak wearing a neutral name. `router.go` used to
// open with "transport-neutral" and then emit `<b>Agents</b>` and `&lt;` —
// Telegram's dialect, composed centrally by the component that knows the least
// about where the message is going, which every other surface then had to
// un-parse.
//
// Two consequences worth stating, because both were bugs before:
//   - Length and escaping belong to the adapter. Telegram caps messages at 4096
//     and breaks on an unescaped `<`; nothing else does. A manager-side fix
//     would be one transport's limits imposed on all of them.
//   - The same semantic message may look different on two surfaces. That is the
//     point of fan-out carrying meaning rather than markup.

// ContractVersion is the outbound message contract adapters must declare on
// GET /channel/ops. Version 1 was the string-valued op (`text`, `title`); an
// adapter still speaking it would receive messages with no `text` field and
// post empty strings, so the endpoint refuses it rather than serving nothing.
// Same posture as the retired `?type=` parameter: fail loudly, name the
// replacement.
const ContractVersion = "2"

// MessageKind names what the manager is saying. Four kinds cover everything it
// ever says; an adapter that does not recognise one should render the body
// rather than drop the message.
type MessageKind string

const (
	// MsgSignal is the event that opened or advanced a conversation, as it
	// arrived — the alert, the job tick, the posted task. Carries the payload
	// inline so an adapter can attach it, quote it, or fold it away.
	MsgSignal MessageKind = "signal"
	// MsgAnswer is agent output, reported through /work/done.
	MsgAnswer MessageKind = "answer"
	// MsgRelay is a user message from one channel mirrored onto its siblings,
	// with the attribution kept structured so the adapter formats it.
	MsgRelay MessageKind = "relay"
	// MsgNotice is everything the manager says on its own behalf: acks,
	// guidance, listings, refusals.
	MsgNotice MessageKind = "notice"
)

// NoticeLevel is the severity of a notice. Two-valued on purpose — errors that
// need durable recording surface as Conversation conditions, not as chat.
type NoticeLevel string

const (
	// NoticeInfo is an ack, a listing, a confirmation.
	NoticeInfo NoticeLevel = "info"
	// NoticeWarn is a refusal or a problem the reader must act on.
	NoticeWarn NoticeLevel = "warn"
)

// Choice is one action a message OFFERS. It is a structured field like Labels,
// not prose: the manager states WHICH actions are on offer, and says nothing
// about how they are presented, whether the transport has controls for them, or
// how many it can show at once.
//
// An adapter with selectable controls renders them as controls; one without
// renders the same list as text and is fully conformant. What it must not do is
// DROP them — they are the reader's only account of what is on offer.
type Choice struct {
	// Label names the action in the reader's terms.
	Label string `json:"label"`
	// Command is the addressed text the choice stands for, so a transport with
	// no controls can print something the reader can type.
	Command string `json:"command"`
}

// RunStatus values carried on an answer, so an adapter can style a failure
// differently from a result without parsing the body.
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Message is one semantic outbound message. Fields are populated per Kind; the
// zero value of an unused field is omitted on the wire.
//
// PROSE FIELDS ARE MARKDOWN, in one deliberately small subset:
//
//	**bold**   *italic*   `inline code`   ```fenced code```   [text](url)
//
// Anything outside that subset is UNDEFINED — an adapter may render it, escape
// it, or strip it, and no caller may depend on which. The subset is small
// because the alternative to naming it is every adapter inventing its own, which
// is the leak this contract exists to close, one dialect later.
//
// Structured fields stay typed rather than being folded into the prose, so an
// adapter can render `labels` as a table, a chip row, or nothing at all.
type Message struct {
	Kind MessageKind `json:"kind"`

	// Body is the prose, in the markdown subset above. Every kind has one.
	Body string `json:"body,omitempty"`

	// ---- signal ----

	// Pipeline is the route that originated the conversation, INFERRED from its
	// materialized bindings and EMPTY when that inference is ambiguous. A
	// conversation records no pipelineRef, so an empty value here means "not
	// determinable", never "none" — render it as absent rather than guessing.
	Pipeline string `json:"pipeline,omitempty"`
	// Source is the SignalSource the input came from, read off the input's
	// recorded origin.
	Source string `json:"source,omitempty"`
	// Title is the conversation title, for adapters that head a card with it.
	Title string `json:"title,omitempty"`
	// Labels are the signal's grouping labels, structured so each surface
	// decides how much of them to show.
	Labels map[string]string `json:"labels,omitempty"`

	// InputRef names the ConversationInput holding the full payload, so an
	// adapter can cite where the event lives for anyone with cluster access.
	// Named for the INPUT, not the source: `source` on this same message means
	// the SignalSource, and two fields a letter apart meaning different objects
	// is the kind of naming that produces one bug per adapter.
	InputRef string `json:"inputRef,omitempty"`

	// ---- any kind ----

	// Choices are the actions this message offers. Optional on every kind.
	Choices []Choice `json:"choices,omitempty"`

	// ExpectsReply marks a message that ASKS THE READER FOR SOMETHING, and
	// cannot proceed until they answer.
	//
	// It exists because a transport's own command menu SENDS on tap: a person
	// picking `/k8s-ops` from Telegram's list posts it bare, with no room to
	// type the task. Answering "usage: /k8s-ops <task>" makes the menu useless
	// for the thing it exists to start.
	//
	// The manager states that an answer is wanted. What a surface DOES about it
	// is the surface's business — Telegram opens the reply box on the reader's
	// behalf, a console focuses its composer, and an adapter that does neither
	// is still conformant because the prose says what to send.
	//
	// It pairs with InReplyTo: the reply arrives linked to this message, and
	// this message is linked to the one that prompted it, which is how the
	// answer finds its way back to the command without anything being stored.
	ExpectsReply bool `json:"expectsReply,omitempty"`

	// InReplyTo is the transport's OWN handle for the message this one answers,
	// supplied by the surface the original arrived on.
	//
	// OPAQUE. The manager stores and returns it unaltered and never parses,
	// validates, compares or constructs one — the same treatment threadId and
	// previousThreadId already get. That a message answers another is MEANING;
	// what the handle looks like is the transport's business.
	//
	// It is what lets an adapter offer an action on somebody's OWN WORDS
	// without holding state to remember them: the transport already links the
	// two messages, so a selection can carry the original forward with nothing
	// retained between the offer and the choice.
	InReplyTo string `json:"inReplyTo,omitempty"`

	// ---- relay ----

	// Origin is the channel the message was typed on.
	Origin string `json:"origin,omitempty"`
	// Sender is the transport-side identity, when the adapter supplied one.
	Sender string `json:"sender,omitempty"`

	// ---- answer ----

	// Status is the run outcome ("succeeded" / a failure string).
	Status string `json:"status,omitempty"`

	// ---- notice ----

	// Level is the notice severity.
	Level NoticeLevel `json:"level,omitempty"`
}

// TopicDescriptor is what ensure-topic carries instead of a baked title. The
// adapter names the thread from its own template and enforces its own limits —
// Telegram forum topics cap at 128 characters and take no markup, a web chat has
// neither constraint.
//
// The descriptor and the first card in the thread carry the SAME data, so an
// adapter can choose not to repeat itself: the alert name in the topic, the
// labels in the card.
type TopicDescriptor struct {
	Conversation string `json:"conversation"`
	// Pipeline is inferred and may be empty — see Message.Pipeline.
	Pipeline string            `json:"pipeline,omitempty"`
	Source   string            `json:"source,omitempty"`
	Title    string            `json:"title,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	// Kind is the originating signal kind (alert | job | task | chat), so an
	// adapter can prefix or icon a topic by lane.
	Kind string `json:"kind,omitempty"`
	// PreviousThreadID is a HINT, set only when a closed conversation is
	// reopened: the thread this conversation used before its topics were
	// archived.
	//
	// The adapter decides what it means. One whose transport can un-archive
	// returns this same id and the conversation continues where it left off;
	// one whose transport has no such notion ignores it and returns a new id,
	// which is already correct — that asymmetry is why this is a hint on an
	// existing op rather than a `reopen-topic` kind every adapter would have to
	// implement, most of them as a second name for ensure-topic.
	//
	// Whether a transport can un-archive is TRANSPORT KNOWLEDGE, and the
	// manager holds none — the same rule that keeps parse_mode and message
	// length limits out of internal/.
	PreviousThreadID string `json:"previousThreadId,omitempty"`
}

// SignalMessage builds the card for an input that reached the manager as a
// signal.
func SignalMessage(pipeline, source, title, inputRef string, labels map[string]string, body string) Message {
	return Message{
		Kind: MsgSignal, Pipeline: pipeline, Source: source, Title: title,
		Labels: labels, InputRef: inputRef, Body: body,
	}
}

// AnswerMessage builds agent output with its run outcome.
func AnswerMessage(body, status string) Message {
	return Message{Kind: MsgAnswer, Body: body, Status: status}
}

// RunReplyMessage builds what a completed run says on a bound thread, FROM THE
// RECORDED RUN ALONE. That is the point: `/work/done` and the reconciler
// backstop must compose the same message from the same facts, or a re-derived
// reply would differ from the one it replaces.
//
// A run that produced nothing is a FAILURE to report, not an answer to render —
// it goes out as a notice so an adapter styles it as one rather than presenting
// an empty agent reply.
func RunReplyMessage(run *agentopsv1alpha1.RunStatus) Message {
	body := strings.TrimSpace(run.Result)
	switch {
	case run.Status != "succeeded" && body != "":
		// A FAILED run that said why. Its result is the explanation — "this
		// conversation cannot be continued", a refusal, a diagnosis — and
		// discarding it in favour of "run failed" is exactly the inarticulate
		// failure that made answering-without-context look like the lesser evil.
		// The reader gets the reason; the level still says something went wrong.
		return Warn(body)
	case run.Status != "succeeded":
		return Warn("❌ run failed (" + run.Status + ")")
	case body == "":
		return Warn("❌ run finished without output")
	default:
		return AnswerMessage(body, run.Status)
	}
}

// RelayMessage builds a user message mirrored onto a sibling channel.
func RelayMessage(origin, sender, body string) Message {
	return Message{Kind: MsgRelay, Origin: origin, Sender: sender, Body: body}
}

// Notice builds an informational message from the manager.
func Notice(body string) Message {
	return Message{Kind: MsgNotice, Level: NoticeInfo, Body: body}
}

// Warn builds a warning from the manager — a refusal, or something the reader
// has to act on.
func Warn(body string) Message {
	return Message{Kind: MsgNotice, Level: NoticeWarn, Body: body}
}
