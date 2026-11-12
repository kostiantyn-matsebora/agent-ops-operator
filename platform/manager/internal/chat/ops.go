package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// OpKind is an outbound channel operation kind.
type OpKind string

const (
	// OpEnsureTopic asks the channel implementation to create a thread for a
	// conversation; completed with the thread id.
	OpEnsureTopic OpKind = "ensure-topic"
	// OpSend asks the channel implementation to post a message.
	OpSend OpKind = "send"
	// OpCloseTopic asks the channel implementation to archive/close a
	// conversation's thread; completed with an empty body.
	//
	// Enqueued when a conversation reaches phase Closed, and recorded in
	// status.threadsArchived[] — which is what makes it re-derivable, since the
	// object survives its close. An adapter that ignores the kind leaves an
	// open topic behind, which a person can close by hand.
	OpCloseTopic OpKind = "close-topic"
	// OpDeleteConversation reports that a Conversation is gone for good;
	// completed with an empty body. Carries the thread id and a notice.
	//
	// It REPORTS A FACT rather than instructing an adapter about a thread,
	// which is why it is named for the conversation: what ending means for a
	// thread is the adapter's decision — post a tombstone, archive, rename,
	// delete. Telegram cannot even post into an archived topic without
	// un-archiving first, and that is precisely the knowledge the manager does
	// not have.
	//
	// It REPLACES close-topic on the deletion path. A conversation being
	// deleted gets one or the other, never both, so no adapter has to work out
	// whether a pair means one ending or two.
	//
	// Unlike close-topic this is NOT derivable: the CR it belongs to is
	// disappearing, so there is nowhere to record what is owed. The deleting
	// Conversation holds a finalizer while it is outstanding, and failure is
	// now retried by the bus (design D-5's "out.<type> delete-conversation"
	// row: 3 attempts, never parked — see Complete's own doc comment) rather
	// than terminal on the first attempt as it once was; the finalizer's own
	// grace period, not this op's dedup state, is what eventually gives up.
	OpDeleteConversation OpKind = "delete-conversation"
)

// ConditionTopicReady reports the outcome of the latest ensure-topic op.
const ConditionTopicReady = "TopicReady"

// Op is one outbound operation addressed to a channel implementation.
// Delivery is at-least-once; consumers must tolerate duplicates by ID.
//
// The payload is STRUCTURED, never rendered: `Message` says what is being
// communicated and `Topic` describes the thread to create. The adapter turns
// either into whatever its surface speaks. See message.go for why.
type Op struct {
	ID           string `json:"id"`
	Channel      string `json:"channel"`
	Conversation string `json:"conversation,omitempty"`
	// ConversationID is the owning conversation's own spec.conversationId —
	// the sender-minted correlation id, carried BESIDE Conversation (the
	// object's name) rather than in place of it (channel-adapter-contract
	// OPS-9). Additive and opaque: a conformant adapter ignores it exactly as
	// it ignores any field it does not recognise. Empty on a conversation
	// that predates the field (legacy — never synthesized) and on an op with
	// no owning conversation at all (a general-surface notice).
	ConversationID string  `json:"conversationId,omitempty"`
	Kind           OpKind  `json:"kind"`
	ThreadID       *string `json:"threadId,omitempty"` // send target; nil = general

	// Topic describes the thread to create (ensure-topic only).
	Topic *TopicDescriptor `json:"topic,omitempty"`
	// Message is the semantic message to post (send only).
	Message *Message `json:"message,omitempty"`

	channelType string
	enqueuedAt  time.Time
	claimedAt   time.Time // zero = queued
	// stable marks an op whose id is DERIVED from CR state, so reconciliation
	// re-enqueues it on every pass — a stable op is submitted to the bus as
	// commandbus.KindOutStable (or KindOutDeleteConversation), which is what
	// makes a repeat enqueue dedup server-side while it is still pending,
	// retrying, parked or settled within the window (design D-4). Ephemeral
	// sends (acks, notices) carry a fresh id each time, are submitted as
	// commandbus.KindOutEphemeral, and are never suppressed — saying the same
	// thing twice is correct when a user asked twice.
	stable bool
}

// Age reports how long an op has been outstanding since it was enqueued.
func (o *Op) Age() time.Duration { return time.Since(o.enqueuedAt) }

// ClaimedAt is the moment an adapter took the op (zero while still queued).
func (o *Op) ClaimedAt() time.Time { return o.claimedAt }

// ChannelType is the adapter this op is addressed to.
func (o *Op) ChannelType() string { return o.channelType }

// OpResult completes an operation.
type OpResult struct {
	ThreadID string `json:"threadId,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ReclaimAfter is the lease every adapter is told to expect on GET
// /channel/ops (server.go's claimedOp.ReclaimAfterSeconds) — a dead
// adapter's claim is returned to the queue after this long. Answering it is
// now the command bus's own job (design D-2's lease, D-10: "this becomes
// unnecessary once the bus's own lease-lapse-and-redelivery does it server-
// side") rather than OpQueue scanning for stale claims itself; the constant
// stays, unchanged, purely as the adapter-facing hint server.go already
// sends — commandbus/memory's own defaultLeaseDuration is kept equal to it
// on purpose, and a production nats bus is configured the same way (task
// 8's chart wiring).
const ReclaimAfter = 5 * time.Minute

var sendSeq atomic.Int64

// sendEpoch makes a fire-and-forget send id unique ACROSS manager restarts,
// not merely within one process. Adapters dedup by op id, as the contract
// tells them to, so a counter that restarted at 1 handed a fresh notice the
// id of one already acknowledged — and the adapter, correctly, refused to
// repeat it. Found by the conformance suite's ack-once rule meeting the
// system pack's manager restart.
var sendEpoch = strconv.FormatInt(time.Now().UnixNano(), 36)

// settledWindowSize bounds Settled's local mirror, matching the old in-
// memory queue's own "recent" ring — a settled id older than the last 512
// is simply no longer answerable as "settled", exactly as before.
const settledWindowSize = 512

// outKinds is every commandbus.Kind the "out" family declares (design D-5) —
// Stats() asks the bus about each in turn, since the bus has no single call
// answering "every kind of this queue at once".
var outKinds = []commandbus.Kind{commandbus.KindOutStable, commandbus.KindOutEphemeral, commandbus.KindOutTopic, commandbus.KindOutDeleteConversation}

// OpQueue is the channel-adapter-facing outbound surface: Enqueue* submits
// one task per op onto the command bus's "out.<channelType>" queue family
// (design D-5), Claim/Complete are GET /channel/ops and POST /channel/ops/
// {id}/done's thin, direct wrappers over Bus.Claim/Settle/Fail, and
// Pending/Settled/Stats read a local mirror rather than the bus itself
// (design D-10) — the exact public surface this type carried before the
// command bus existed, kept byte-for-byte so the router, the reconciler and
// httpapi's handlers need no change at all.
//
// Every op's id is its commandbus.CommandID verbatim: the published, stable
// shapes in CloseTopicOpID/DeleteConversationOpID/InputOpID/RunReplyOpID
// (channel-adapter-contract ADA-15) are what a producer derives and a
// consumer parses back, same as always — the bus never sees or interprets
// them beyond comparing for equality.
//
// `delete-conversation` is the one exception to every other op being
// DERIVED from CR state — see OpDeleteConversation's own doc comment — and
// is why its queue declares a short TTL and no park cool-off (design D-5):
// the finalizer's own grace period, not this op's retry count, is what
// gives up.
type OpQueue struct {
	// Conversations holds the conversations the queue marks delivery, archiving
	// and threads on as ops complete. Nil marks nothing.
	Conversations state.Collection[state.ConversationRecord]
	// Runs holds the conversations' append-only run history, for the run-reply
	// delivery marker (markDelivered) and its backstop.
	Runs     state.Appended[state.ConversationRecord, state.Run]
	Registry *Registry
	// Activity records enqueue and completion hops. Nil is inert.
	Activity *activity.Log

	// Bus is every Enqueue*/Claim/Complete call's own backend (async-command-
	// bus task 6). Required: a nil Bus fails the first real call it reaches,
	// exactly as every other command-bus-backed surface in this module does
	// (internal/httpapi's own accept handlers) — cmd/manager wires the same
	// Bus value here that every other producer and consumer in the process
	// shares.
	Bus commandbus.Bus
	// ChannelsFor lists the names of every Channel whose spec.adapter names the
	// adapter. Optional: a replica that restarted knows no channel queue until
	// the feed or an enqueue shows it one, and this closes that window from the
	// Kubernetes cache. Nil leaves Claim to what it has learned.
	ChannelsFor func(ctx context.Context, adapter string) []string
	// Concurrency sizes the in-process consumer.Pool Run builds per
	// Registry-registered channel queue (design D-10's "in-process providers
	// become a consumer"). Defaults to 1 when zero or negative.
	Concurrency int
	// ReceiptAdapters names the adapters whose acknowledgement is their receipt
	// of the message from the manager, and nothing more (the console, which
	// pushes on to browsers fire-and-forget): the manager consumes the message
	// at the acknowledgement. Every other adapter (Telegram) reports done or
	// failed after delivering to its surface, within the delivery TTL that
	// starts at its acknowledgement (design D-17).
	ReceiptAdapters []string

	mu sync.Mutex
	// claimed indexes a currently-claimed op's PUBLISHED id (the id an
	// external adapter sees on GET /channel/ops and echoes back to POST
	// /channel/ops/{id}/done) onto the bus task it came from: the lease
	// token Complete must present to Settle/Fail, and the attempt/
	// maxAttempts pair Complete uses to tell a retry from a dead end without
	// a second round trip to the bus (resultingStateAfterFail).
	claimed map[string]claimEntry
	// pending/settled are the watch-fed local mirror design D-10 asks
	// Pending/Settled to read instead of the bus — updated synchronously by
	// every Submit/Claim/Settle/Fail THIS value makes (markFromState), and
	// kept current for transitions made elsewhere (another replica, the
	// bus's own reaping) by watchMirror's Feed consumption once Run is
	// running. Reconcile-hot-path cost is exactly what this trades away
	// (design.md's own Risks).
	//
	// pending clears on ANY final state (Succeeded, Dead, Expired). settled
	// is narrower — SUCCEEDED only, never Dead — and doubles as enqueue's
	// own local short-circuit: see markFromState's and enqueue's doc
	// comments for why a Dead task must never be mistaken for a settled one
	// here.
	pending     map[string]bool
	settled     map[string]bool
	settledRing []string
	// channelAdapter maps every out.<channel> queue this process has seen to
	// the adapter that serves it (Channel.spec.adapter), learned from its own
	// enqueues and from watchMirror's Feed consumption. Claim(adapter) drains
	// every channel queue of that adapter and Stats() reports per adapter, and
	// the bus has no "list every queue in use" call, so this is the one place
	// the channel->adapter fact lives (design D-10: one queue per Channel).
	channelAdapter map[string]string
	// poolWake nudges Run to start a consumer for a newly learned channel.
	poolWake chan struct{}
}

// claimEntry is one externally-claimed op's bus-side credentials, keyed by
// its published id (see OpQueue.claimed).
type claimEntry struct {
	uid         string
	token       string
	attempt     int
	maxAttempts int
	op          *Op
}

func (q *OpQueue) init() {
	if q.claimed == nil {
		q.claimed = map[string]claimEntry{}
		q.pending = map[string]bool{}
		q.settled = map[string]bool{}
		q.channelAdapter = map[string]string{}
		q.poolWake = make(chan struct{}, 1)
	}
}

// EnqueueEnsureTopic queues topic creation for a conversation on ONE of its
// bound channels. The id is stable per conversation×channel so
// reconcile-driven re-enqueues dedup while one is pending and after
// completion.
// The descriptor carries the same facts as the first card in the thread, so an
// adapter can choose not to repeat itself — the alert name in the topic, the
// labels in the card — and can enforce its own naming limits (Telegram forum
// topics cap at 128 characters) instead of receiving a title already cut to a
// length the manager guessed at.
func (q *OpQueue) EnqueueEnsureTopic(ctx context.Context, ch *agentopsv1alpha1.Channel, conv *state.ConversationRecord, topic TopicDescriptor) {
	topic.Conversation = conv.Name
	if topic.Title == "" {
		topic.Title = conv.Title
	}
	if topic.Title == "" {
		topic.Title = conv.Name
	}
	// ':' keeps the id a single URL path segment for /channel/ops/{id}/done.
	// The reopen counter keeps the id STABLE (so reconciliation re-derives it)
	// while making each reopen's request distinct — without it a reopen's
	// ensure-topic dedups against the original topic creation and never reaches
	// the adapter.
	//
	// UID, not just name: AllocateName reuses a name once its holder is
	// deleted, and the bus's own dedup window outlives any one Conversation. A
	// freshly created object sharing a deleted one's exact name would
	// otherwise derive the SAME id as that deleted conversation's already-
	// completed ensure-topic, silently dropping the new one's own enqueue —
	// measured live: `chat-echo-console` recreated across two sequential
	// smoke tests never got a thread id, because the manager process (and its
	// in-memory dedup window) outlived the first one.
	id := "topic:" + conv.Name + ":" + conv.ID + ":" + ch.Name
	if conv.Reopens > 0 {
		id += ":r" + strconv.Itoa(int(conv.Reopens))
	}
	op := &Op{
		ID: id, Channel: ch.Name, Conversation: conv.Name, ConversationID: conv.ConversationID,
		Kind: OpEnsureTopic, Topic: &topic, channelType: ch.Spec.Adapter, stable: true,
	}
	if q.IsReceipt(ch.Spec.Adapter) {
		// A feed adapter is told its thread, never asked for one: nothing comes
		// back from a stream.
		if conv.ID == "" {
			log.FromContext(ctx).Error(errors.New("conversation has no UID"), "enqueue ensure-topic on a receipt channel", "conversation", conv.Name, "channel", ch.Name)
			return
		}
		tid := ReceiptThreadID(conv)
		op.ThreadID = &tid
	}
	q.enqueue(ctx, op)
}

// EnqueueCloseTopic queues archiving of one conversation thread. The id is
// stable per conversation×channel, so the reconciler re-enqueuing it on every
// pass while the finalizer holds dedups instead of piling up.
func (q *OpQueue) EnqueueCloseTopic(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID string, conv *state.ConversationRecord) {
	tid := threadID
	q.enqueue(ctx, &Op{
		ID: CloseTopicOpID(conv.Name, ch.Name), Channel: ch.Name, Conversation: conv.Name, ConversationID: conv.ConversationID,
		Kind: OpCloseTopic, ThreadID: &tid, channelType: ch.Spec.Adapter, stable: true,
	})
}

// CloseTopicOpID is the stable id of a conversation's close-topic op on one
// channel. Exported because the finalizer asks whether it is still outstanding.
func CloseTopicOpID(conversation, channel string) string {
	return "close:" + conversation + ":" + channel
}

// EnqueueDeleteConversation tells one bound thread that its conversation is
// gone for good.
//
// The MESSAGE is composed here and rendered by the adapter, like every other
// message: the manager states what happened, and escaping, splitting and
// placement belong to whoever knows the transport. An adapter is free to ignore
// the text and express the ending structurally instead.
func (q *OpQueue) EnqueueDeleteConversation(ctx context.Context, ch *agentopsv1alpha1.Channel, threadID string, conv *state.ConversationRecord) {
	tid := threadID
	msg := Notice("🗑 This conversation has been deleted, along with everything it recorded. " +
		"The thread stays here to read. A new message starts a NEW conversation — it will not continue this one.")
	q.enqueue(ctx, &Op{
		ID: DeleteConversationOpID(conv.Name, ch.Name), Channel: ch.Name, Conversation: conv.Name, ConversationID: conv.ConversationID,
		Kind: OpDeleteConversation, ThreadID: &tid, Message: &msg,
		channelType: ch.Spec.Adapter, stable: true,
	})
}

// DeleteConversationOpID is the stable id of a conversation's
// delete-conversation op on one channel. The finalizer asks whether it is still
// outstanding.
func DeleteConversationOpID(conversation, channel string) string {
	return "gone:" + conversation + ":" + channel
}

// ParseCloseTopicOpID splits a close-topic op id back into its parts. Channel
// names cannot contain ":" (they are Kubernetes object names), so the split is
// unambiguous.
func ParseCloseTopicOpID(id string) (conversation, channel string, ok bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] != "close" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// Settled reports whether an op id has already completed within the local
// mirror's retained window (design D-10) — never the bus itself. Used by the
// close finalizer to tell "archived" from "never enqueued".
func (q *OpQueue) Settled(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.init()
	return q.settled[id]
}

// EnqueueMessage queues one semantic message. Fire-and-forget: the id is fresh
// each time, so sends are never suppressed by dedup.
//
// Callers pass a Message built by one of the kind constructors in message.go
// (Notice, Warn, AnswerMessage, RelayMessage, SignalMessage) — there is no
// string-valued form on purpose, because that is what let transport markup back
// into the manager.
//
// conv is nilable: a general-surface notice (command usage, an unwired-source
// warning) belongs to no conversation, so both Conversation and
// ConversationID stay empty on its op rather than being guessed.
func (q *OpQueue) EnqueueMessage(ctx context.Context, ch *agentopsv1alpha1.Channel, conv *state.ConversationRecord, threadID *string, msg Message) {
	name, conversationID := "", ""
	if conv != nil {
		name, conversationID = conv.Name, conv.ConversationID
	}
	q.enqueueMessage(ctx, "send:"+sendEpoch+":"+strconv.FormatInt(sendSeq.Add(1), 36), ch, name, conversationID, threadID, msg, false)
}

// EnqueueFarewell posts a conversation's closing notice with a STABLE id, so a
// close that is attempted more than once says goodbye exactly once.
//
// This is not hypothetical tidiness. The farewell is posted before the status
// write — deliberately, because a conversation whose thread simply stops is
// indistinguishable from a fault, so a lost farewell is worse than a repeated
// one — and a close whose status write FAILS is retried. A live smoke against a
// cluster whose CRD did not yet allow phase `Closed` put four farewells in one
// thread over seven hours of retries. An ephemeral id makes every retry a new
// message; a stable one makes them the same message.
//
// Keyed on the reopen count as well, for the same reason ensure-topic is: a
// conversation closed, reopened and closed again is owed a SECOND farewell, and
// a genuinely stable id would suppress it.
func (q *OpQueue) EnqueueFarewell(ctx context.Context, ch *agentopsv1alpha1.Channel,
	conv *state.ConversationRecord, threadID *string, msg Message) {
	id := fmt.Sprintf("farewell:%s:%s:%d", conv.Name, ch.Name, conv.Reopens)
	q.enqueueMessage(ctx, id, ch, conv.Name, conv.ConversationID, threadID, msg, true)
}

// EnqueueReopenNotice announces that a conversation is live again, on every
// bound thread.
//
// The MANAGER says this, not each adapter. A closing thread already gets a
// farewell from here, and a reopen is the same kind of fact — so synthesizing
// it adapter-side would put it on whichever surface happened to implement it
// and leave the others with a thread that silently starts working again. It is
// also the rule: the manager composes meaning, adapters compose presentation.
//
// Stable per reopen, so reconciliation may re-derive it on every pass and it
// still lands once — and a SECOND reopen gets its own.
func (q *OpQueue) EnqueueReopenNotice(ctx context.Context, ch *agentopsv1alpha1.Channel,
	conv *state.ConversationRecord, threadID *string, msg Message) {
	id := fmt.Sprintf("reopened:%s:%s:%d", conv.Name, ch.Name, conv.Reopens)
	q.enqueueMessage(ctx, id, ch, conv.Name, conv.ConversationID, threadID, msg, true)
}

// EnqueueEscalationMessage posts the digest an `escalate` call supplied as the
// opening message of a newly-bound escalation thread (design D-D).
//
// The ensure-topic enqueue and the topic actually existing (an adapter
// completing it with a thread id) are two different moments, so this is a
// SEPARATE op rather than data riding on ensure-topic — the reconciler calls
// it once a bound channel has a thread, exactly as it does for the reopen
// notice. Stable per conversation×channel×reopen count, so re-deriving it on
// every reconcile pass posts the digest once.
func (q *OpQueue) EnqueueEscalationMessage(ctx context.Context, ch *agentopsv1alpha1.Channel,
	conv *state.ConversationRecord, threadID *string, msg Message) {
	id := fmt.Sprintf("escalate:%s:%s:%d", conv.Name, ch.Name, conv.Reopens)
	q.enqueueMessage(ctx, id, ch, conv.Name, conv.ConversationID, threadID, msg, true)
}

// EnqueueInputDelivery queues ONE input's delivery to ONE channel — the event
// card for something that woke the agent, or somebody's words relayed from the
// surface they were typed on. One queue call for both, because they are one
// delivery decided by one rule.
//
// Unlike an ordinary send its id is STABLE per conversation×input×channel, so
// the reconciler re-deriving it on every pass dedups against both the pending
// map and the recent window — exactly as ensure-topic already does. Without
// that, a conversation would repost its alert, and everything anyone said, on
// every reconcile.
func (q *OpQueue) EnqueueInputDelivery(ctx context.Context, ch *agentopsv1alpha1.Channel,
	conv *state.ConversationRecord, inputID string, threadID *string, msg Message) {

	q.enqueueMessage(ctx, InputOpID(conv.Name, inputID, ch.Name), ch, conv.Name, conv.ConversationID, threadID, msg, true)
}

// InputOpID is the stable id of one input's delivery on one channel.
func InputOpID(conversation, inputID, channel string) string {
	return "input:" + conversation + ":" + inputID + ":" + channel
}

// EnqueueRunReply queues one run's answer on one channel. Its id is STABLE per
// conversation×channel×run, which is what makes the reply derivable: `/work/done`
// enqueues it on the fast path and reconciliation re-enqueues it for any bound
// thread still lacking a delivery marker, and the two dedup against each other
// through the bus's own pending/settled window.
//
// Ack and notice sends stay on the counter-based id: saying the same thing twice
// is correct when a user asked twice, and neither derives from CR state.
func (q *OpQueue) EnqueueRunReply(ctx context.Context, ch *agentopsv1alpha1.Channel,
	conv *state.ConversationRecord, runID string, threadID *string, msg Message) {

	q.enqueueMessage(ctx, RunReplyOpID(conv.Name, ch.Name, runID), ch, conv.Name, conv.ConversationID, threadID, msg, true)
}

// RunReplyOpID is the stable id of one run's reply on one channel. Exported
// because completion has to map the op back to the run it delivers.
func RunReplyOpID(conversation, channel, runID string) string {
	return "send:" + conversation + ":" + channel + ":" + runID
}

// ParseRunReplyOpID splits a reply op id back into its parts, reporting false
// for any other id. Completion uses it to mark the run/channel pair delivered
// without the op having to carry redundant fields.
func ParseRunReplyOpID(id string) (conversation, channel, runID string, ok bool) {
	parts := strings.Split(id, ":")
	// Conversation and channel are Kubernetes object names, so they cannot
	// contain ':'; a run id could in principle, so the REMAINDER is the run.
	if len(parts) < 4 || parts[0] != "send" {
		return "", "", "", false
	}
	return parts[1], parts[2], strings.Join(parts[3:], ":"), true
}

func (q *OpQueue) enqueueMessage(ctx context.Context, id string, ch *agentopsv1alpha1.Channel,
	conversation, conversationID string, threadID *string, msg Message, stable bool) {

	q.enqueue(ctx, &Op{
		ID: id, Channel: ch.Name, Conversation: conversation, ConversationID: conversationID,
		Kind: OpSend, ThreadID: threadID, Message: &msg, channelType: ch.Spec.Adapter, stable: stable,
	})
}

// outKindFor resolves which of the "out" family's three commandbus.Kind
// values op's queue declaration lives under (design D-5): a conversation
// disappearing for good is always KindOutDeleteConversation regardless of
// op.stable (today's only such op happens to be stable too — see
// OpDeleteConversation); every other stable op (ensure-topic, run reply,
// input card, close-topic, and — ground truth from op.stable, NOT this
// package's own English-language table in the handback report — the
// farewell/reopen/escalation notices, which the reconciler re-derives on
// every pass exactly like ensure-topic) is KindOutStable; a fresh-id-every-
// call op (only EnqueueMessage) is KindOutEphemeral.
func outKindFor(op *Op) commandbus.Kind {
	if op.Kind == OpDeleteConversation {
		return commandbus.KindOutDeleteConversation
	}
	if op.Kind == OpEnsureTopic || op.Kind == OpCloseTopic {
		return commandbus.KindOutTopic
	}
	if op.stable {
		return commandbus.KindOutStable
	}
	return commandbus.KindOutEphemeral
}

// generalKey is the ordering key of an op bound to no conversation (a
// general-surface notice). Bus keys must be non-empty, and a Kubernetes
// object name cannot start with an underscore, so it never collides with a
// conversation.
const generalKey = "_general"

// outKey derives the ordering key design D-5 declares for every "out.<channel>"
// queue: the conversation. The channel is the queue itself, so a key names
// only what is left to order within it.
func outKey(conversation string) commandbus.Key {
	if conversation == "" {
		return generalKey
	}
	return commandbus.Key(conversation)
}

// conversationFromKey recovers outKey's conversation for Stats() — the bus
// treats Key as opaque, but this package minted it, so only this package may
// read it back.
func conversationFromKey(key commandbus.Key) string {
	if key == generalKey {
		return ""
	}
	return string(key)
}

// queue is the out.<channel> queue this op travels on.
func (o *Op) queue() commandbus.Queue { return commandbus.OutQueue(o.Channel) }

// opWire is the "out" family's one wire shape — never a separate package
// the way "conv"'s convtask is, because both producer (this file's own
// Enqueue* methods) and consumer (Claim, and Run's in-process handler) live
// in this one package. It omits Op's unexported bookkeeping fields (none of
// which encoding/json would carry anyway) and Op.ID itself, which is always
// the task's own CommandID — carrying it twice would let the two disagree.
type opWire struct {
	Channel        string           `json:"channel"`
	Conversation   string           `json:"conversation,omitempty"`
	ConversationID string           `json:"conversationId,omitempty"`
	Kind           OpKind           `json:"kind"`
	ThreadID       *string          `json:"threadId,omitempty"`
	Topic          *TopicDescriptor `json:"topic,omitempty"`
	Message        *Message         `json:"message,omitempty"`
	ChannelType    string           `json:"channelType"`
}

// encode is Submit's own Payload — never fails (a struct of strings/a
// pointer/a Message), so callers compose it inline.
func (op *Op) encode() []byte {
	b, _ := json.Marshal(opWire{
		Channel: op.Channel, Conversation: op.Conversation, ConversationID: op.ConversationID,
		Kind: op.Kind, ThreadID: op.ThreadID, Topic: op.Topic, Message: op.Message,
		ChannelType: op.channelType,
	})
	return b
}

// decodeOp is encode's inverse, Claim's and the in-process handler's own
// entry point: id is the task's CommandID, which the wire form never
// carries (see opWire's own doc comment).
func decodeOp(id string, raw []byte) (*Op, error) {
	var w opWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("decode channel op %s: %w", id, err)
	}
	return &Op{
		ID: id, Channel: w.Channel, Conversation: w.Conversation, ConversationID: w.ConversationID,
		Kind: w.Kind, ThreadID: w.ThreadID, Topic: w.Topic, Message: w.Message,
		channelType: w.ChannelType,
	}, nil
}

// enqueue submits op onto its own out.<channel> queue (design D-5),
// updates the local mirror from the task the bus handed back, and emits the
// "enqueued" activity hop exactly once per actually-new submission —
// never for one the bus suppressed as a dedup (design D-4), matching the
// old in-memory queue's own "a dedup that silently suppresses the op emits
// nothing" rule (the console renders an enqueue as "sent, unconfirmed"
// until completion, and a suppressed one is neither).
//
// A LOCAL success-forever check runs BEFORE the bus is ever asked (see the
// mirror's own doc comment on OpQueue.settled): design D-5 declares
// out.deleteConversation's own DedupWindow as 0 ("delete of a conversation
// that is already gone succeeds" — a deliberate, conformance-tested
// choice, commandbus/inventory.go), which means the BUS alone would offer
// NO suppression at all for a resubmit made the instant after success —
// exactly what finalizeClose's own "enqueue every pass, then check
// Pending" (conversation_controller.go) does. Measured by this change's
// own test suite: without this, a conversation whose adapter acked
// delete-conversation at once never looked settled for even one
// reconcile pass, because the very next enqueue manufactured a brand new,
// unsettled task before anyone could observe the old one's success. The
// old in-memory queue's own "recent" map had exactly this property (an id
// stayed in it until the 512-entry ring evicted it, success or dead
// alike) — restoring it, but SUCCESS ONLY: a Dead task must never be
// hidden behind a local cache the design went to some trouble to make the
// BUS own instead (the 2026-08-13 incident, Complete's own doc comment).
func (q *OpQueue) enqueue(ctx context.Context, op *Op) {
	q.mu.Lock()
	alreadySucceeded := q.settled[op.ID]
	q.mu.Unlock()
	if alreadySucceeded {
		return
	}

	before := time.Now()
	task, err := q.Bus.Submit(ctx, commandbus.SubmitRequest{
		Queue:     op.queue(),
		Kind:      outKindFor(op),
		CommandID: commandbus.CommandID(op.ID),
		Key:       outKey(op.Conversation),
		Payload:   op.encode(),
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "enqueue channel op", "op", op.ID, "kind", op.Kind, "channel", op.Channel)
		return
	}
	q.noteChannel(op.Channel, op.channelType)
	q.markFromState(op.ID, task.State)
	if task.SubmittedAt.Before(before) {
		return // deduped by the bus -- see this method's own doc comment
	}

	// Enqueued is INTENT, not delivery. The console renders this edge as "sent,
	// unconfirmed" until a channel.op.completed arrives.
	q.Activity.Emit(activity.Event{
		Kind:         activity.KindChannelOpEnqueued,
		From:         opOriginNode(op),
		To:           activity.Node(activity.NodeChannel, op.Channel),
		Conversation: op.Conversation, OpID: op.ID, Code: string(op.Kind), Detail: op.Channel,
	})
}

// noteChannel records which adapter serves a channel's queue and, when the
// channel is new, wakes Run so an in-process provider gets its consumer.
func (q *OpQueue) noteChannel(channel, adapter string) {
	if channel == "" || adapter == "" {
		return
	}
	q.mu.Lock()
	q.init()
	_, known := q.channelAdapter[channel]
	q.channelAdapter[channel] = adapter
	wake := q.poolWake
	q.mu.Unlock()
	if !known {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (q *OpQueue) knowsChannel(channel string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.init()
	_, ok := q.channelAdapter[channel]
	return ok
}

// channelsOf lists the channels serving adapter, sorted, from what this
// process has learned plus ChannelsFor's answer.
func (q *OpQueue) channelsOf(adapter string) []string {
	q.mu.Lock()
	q.init()
	set := map[string]bool{}
	for ch, a := range q.channelAdapter {
		if a == adapter {
			set[ch] = true
		}
	}
	q.mu.Unlock()
	if q.ChannelsFor != nil {
		for _, ch := range q.ChannelsFor(context.Background(), adapter) {
			set[ch] = true
		}
	}
	out := make([]string, 0, len(set))
	for ch := range set {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}

// markFromState applies one task's current state to the local pending/
// settled mirror (design D-10) — the one choke point every Submit, Claim,
// Settle/Fail and Feed-fed transition in this file goes through.
// markFromState applies one task's current state to the local pending/
// settled mirror (design D-10). pending tracks EVERY non-final state
// (Queued, Running, Retrying) — cleared on Succeeded, Dead or Expired
// alike. settled is narrower, Succeeded ONLY: a Dead task must never look
// settled here, or a failed stable op's own recovery (reconciliation re-
// deriving it) would be hidden behind this local cache exactly as
// Complete's own 2026-08-13 incident describes — see enqueue's own doc
// comment for the one place settled feeds back into a decision.
func (q *OpQueue) markFromState(id string, state commandbus.TaskState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.init()
	switch state {
	case commandbus.StateSucceeded:
		delete(q.pending, id)
		q.markSettledLocked(id)
	case commandbus.StateDead, commandbus.StateExpired:
		delete(q.pending, id)
	default: // StateQueued, StateRunning, StateRetrying
		q.pending[id] = true
	}
}

// markSettledLocked records id as settled, with the SAME bounded ring
// eviction the old in-memory queue's own "recent" map used. Called with mu
// held.
func (q *OpQueue) markSettledLocked(id string) {
	if q.settled[id] {
		return
	}
	q.settled[id] = true
	q.settledRing = append(q.settledRing, id)
	if len(q.settledRing) > settledWindowSize {
		delete(q.settled, q.settledRing[0])
		q.settledRing = q.settledRing[1:]
	}
}

// resultingStateAfterFail infers a task's state immediately after a Fail
// call from information already in hand — Bus.Fail's own documented
// contract (commandbus/commandbus.go): "exhausting the queue's declared
// attempts, or a non-retryable code on any attempt, moves the task to Dead
// instead of back to Retrying." A GetTask round trip to learn this is
// exactly the server read Pending/Settled's own mirror exists to avoid.
func resultingStateAfterFail(code commandbus.ErrorCode, attempt, maxAttempts int) commandbus.TaskState {
	if code.Retryable() && attempt < maxAttempts {
		return commandbus.StateRetrying
	}
	return commandbus.StateDead
}

// adopt records a Running task's claim in this process, as claimFrom does for
// one it claimed itself. task.Lease must be non-nil (Bus.Running guarantees it).
func (q *OpQueue) adopt(task commandbus.Task, adapter string) (*Op, bool) {
	op, err := decodeOp(string(task.CommandID), task.Payload)
	if err != nil {
		log.FromContext(context.Background()).Error(err, "decode running channel op", "task", task.UID)
		return nil, false
	}
	op.channelType = adapter
	op.enqueuedAt = task.SubmittedAt
	op.claimedAt = task.UpdatedAt
	q.mu.Lock()
	q.init()
	q.claimed[op.ID] = claimEntry{uid: task.UID, token: leaseTokenOf(task), attempt: task.Attempt, maxAttempts: task.MaxAttempts, op: op}
	q.mu.Unlock()
	q.markFromState(op.ID, task.State)
	cp := *op
	return &cp, true
}

// adoptRunning finds the Running task of command id on any channel queue this
// process knows and adopts its claim, so an acknowledgement that reaches a
// replica other than the one that made the claim still settles it rather than
// leaving the op to lapse and be delivered again. False when nothing runs
// under that id (a duplicate acknowledgement, a lapsed lease).
func (q *OpQueue) adoptRunning(ctx context.Context, id string) (claimEntry, bool) {
	q.mu.Lock()
	q.init()
	channels := make(map[string]string, len(q.channelAdapter))
	for ch, adapter := range q.channelAdapter {
		channels[ch] = adapter
	}
	q.mu.Unlock()
	for channel, adapter := range channels {
		tasks, err := q.Bus.Running(ctx, []commandbus.Queue{commandbus.OutQueue(channel)}, "")
		if err != nil {
			continue
		}
		for _, task := range tasks {
			if string(task.CommandID) != id {
				continue
			}
			if _, ok := q.adopt(task, adapter); !ok {
				return claimEntry{}, false
			}
			q.mu.Lock()
			entry, ok := q.claimed[id]
			q.mu.Unlock()
			return entry, ok
		}
	}
	return claimEntry{}, false
}

// channelOpsConsumerID names every acknowledgement this surface makes, for a
// human reading a Lease's Holder back (/state/tasks) — never consulted by Settle
// or Fail.
const channelOpsConsumerID = "channel-ops"

func leaseTokenOf(t commandbus.Task) string {
	if t.Lease == nil {
		return ""
	}
	return t.Lease.Token
}

// Complete finishes an op. Unknown or already-completed ids are tolerated
// (at-least-once delivery makes duplicate completions legal) — handled here
// by claimed no longer holding an entry for id, so there is nothing left to
// Settle or Fail against.
//
// THE HANDLERS BELOW STILL RUN SYNCHRONOUSLY BEFORE THIS RETURNS (design
// D-10): finishEnsureTopic, markDelivered/markUndelivered, markThreadArchived.
// The thread binding they write is what inbound messages resolve through.
//
// 2026-08-13 INCIDENT, AND WHY THIS NO LONGER RELEASES A DEDUP ENTRY BY
// HAND. The OLD queue had no retry or backoff of its own, so the only way a
// failed stable op ever got another chance was Complete deleting its own
// completed-window entry on ANY transport failure — Telegram rate-limited a
// 44-alert burst, 74 rejected sends released themselves and got re-derived
// at once, and 22 topics stayed empty for four and a half hours with the
// answers already sitting delivered-nowhere in status.runs[].result.
//
// The command bus's own per-queue attempts and backoff (design D-5: 8
// attempts, 5s→5m jittered, for out.stable) is what replaces that hack now:
// a failed attempt calls Fail with the matching ErrorCode and lets the BUS
// retry it with backoff, rather than forcing an immediate release the way
// the old code did. A handler that fails — the Kubernetes write itself,
// not the adapter's own call — still "releases the id, as today" (design
// D-10): Fail(CodeInternal) puts the SAME task back into Retrying (or Dead,
// once attempts are exhausted), which is this surface's modern equivalent
// of the old hand-rolled release.
func (q *OpQueue) Complete(ctx context.Context, id string, res OpResult) {
	q.mu.Lock()
	q.init()
	entry, ok := q.claimed[id]
	if ok {
		delete(q.claimed, id)
	}
	q.mu.Unlock()
	if !ok {
		// The claim may have been made on another replica (the stream and the
		// acknowledgement are separate connections a Service can spread). The
		// bus holds the lease, so read it back before calling this a duplicate.
		if entry, ok = q.adoptRunning(ctx, id); !ok {
			return
		}
		q.mu.Lock()
		delete(q.claimed, id)
		q.mu.Unlock()
	}
	op := entry.op

	// The manager's own view of delivery. An adapter that also POSTs /activity
	// reports the latency it measured on the wire; this one is queue-to-ack.
	done := activity.Event{
		Kind: activity.KindChannelOpCompleted,
		From: activity.Node(activity.NodeChannelAdapter, op.channelType),
		// The CHANNEL, not the manager: the adapter delivered to that surface,
		// which is both what happened and something the graph can draw. Naming
		// the manager made this hop unrenderable — the manager is not a wiring
		// node, so the served-by edge every reply travels never lit up.
		To:           activity.Node(activity.NodeChannel, op.Channel),
		Conversation: op.Conversation, OpID: op.ID, Code: string(op.Kind),
		Detail:    op.Channel,
		LatencyMs: time.Since(op.enqueuedAt).Milliseconds(),
	}
	if res.Error != "" {
		done.Status = activity.StatusError
		done.Detail = op.Channel + ": " + res.Error
	}
	q.Activity.Emit(done)

	handlerErr := q.runSideEffects(ctx, op, res)

	var code commandbus.ErrorCode
	switch {
	case res.Error != "":
		// An adapter's reported error maps to unavailable (design D-6), so a
		// rate-limited surface backs off rather than dead-lettering on its
		// first rejection.
		code = commandbus.CodeUnavailable
	case handlerErr != nil:
		// The adapter's own call succeeded; the manager's OWN write failed.
		// Not covered by the adapter-error mapping above.
		code = commandbus.CodeInternal
	}
	if code != "" {
		detail := res.Error
		if detail == "" && handlerErr != nil {
			detail = handlerErr.Error()
		}
		if err := q.Bus.Fail(ctx, entry.uid, entry.token, code, detail); err != nil {
			// The op stays leased and is redelivered when the lease lapses: say so.
			log.FromContext(ctx).Error(err, "record a failed channel op; it will be redelivered when its lease lapses", "op", id, "task", entry.uid)
			return
		}
		q.markFromState(id, resultingStateAfterFail(code, entry.attempt, entry.maxAttempts))
		return
	}
	if err := q.Bus.Settle(ctx, entry.uid, entry.token); err != nil {
		log.FromContext(ctx).Error(err, "acknowledge a channel op; it will be redelivered when its lease lapses", "op", id, "task", entry.uid)
		return
	}
	q.markFromState(id, commandbus.StateSucceeded)
}

// runSideEffects runs the one Kubernetes-state-mutating handler op.Kind
// calls for, logging and returning its error so Complete (and the
// in-process handler) can decide Settle vs Fail — never deciding that
// itself. A nil return means "nothing more to do", not "this op
// succeeded": OpSend's run-reply failure branch and OpCloseTopic/
// OpDeleteConversation's own failure branches run a side effect (or just a
// log line) and still return nil, because Complete's own res.Error check
// already decided Fail before this runs at all.
func (q *OpQueue) runSideEffects(ctx context.Context, op *Op, res OpResult) error {
	logger := log.FromContext(ctx)
	switch {
	case op.Kind == OpEnsureTopic:
		if err := q.finishEnsureTopic(ctx, op, res); err != nil {
			logger.Error(err, "complete ensure-topic", "conversation", op.Conversation)
			return err
		}
	case op.Kind == OpSend && res.Error == "" && isRunReply(op.ID):
		// Delivery becomes a FACT on the conversation. Marking on completion
		// rather than on enqueue is what makes the two paths safe: an op lost
		// before completion is re-derived by reconciliation, and one completed
		// before a restart is not.
		if err := q.markDelivered(ctx, op); err != nil {
			logger.Error(err, "mark run reply delivered", "conversation", op.Conversation, "op", op.ID)
			return err
		}
	case op.Kind == OpSend && res.Error != "" && isRunReply(op.ID):
		// Complete's own res.Error branch already decided to Fail this task —
		// this is purely the observable half: an owed reply is visible on the
		// object itself, not only in manager logs.
		if err := q.markUndelivered(ctx, op); err != nil {
			logger.Error(err, "mark run reply undelivered", "conversation", op.Conversation, "op", op.ID)
		}
		logger.Info("run reply send failed; will be retried or re-derived",
			"channel", op.Channel, "conversation", op.Conversation, "op", op.ID, "error", res.Error)
	case op.Kind == OpDeleteConversation:
		// The bus retries this per design D-5's own declared attempts; the
		// finalizer's grace, not this op's own state, is what eventually
		// gives up. There is no object left to mark against either way.
		if res.Error != "" {
			logger.Info("delete-conversation op failed; the thread was not told",
				"channel", op.Channel, "conversation", op.Conversation, "error", res.Error)
		}
	case op.Kind == OpCloseTopic:
		// Archiving became a FACT on the conversation when closing stopped
		// deleting. The object survives the close, so there is something to
		// record against.
		if res.Error == "" {
			if err := q.markThreadArchived(ctx, op); err != nil {
				logger.Error(err, "mark thread archived", "conversation", op.Conversation, "op", op.ID)
				return err
			}
			break
		}
		logger.Info("close-topic op failed; leaving the thread open",
			"channel", op.Channel, "conversation", op.Conversation, "error", res.Error)
	case res.Error != "":
		// NAME THE OP. The incident the doc comment above describes was
		// diagnosable only by correlating counts across two systems, because
		// this line said neither which op nor which kind had failed.
		logger.Info("channel op failed; will be retried or re-derived",
			"op", op.ID, "kind", string(op.Kind), "channel", op.Channel,
			"conversation", op.Conversation, "error", res.Error)
	}
	return nil
}

// isRunReply reports whether an op id is a stable run-reply id.
func isRunReply(id string) bool {
	_, _, _, ok := ParseRunReplyOpID(id)
	return ok
}

// markThreadArchived records that one bound thread has been archived, so
// reconciliation stops re-deriving its close-topic op. Mirrors markDelivered,
// including the conflict retry: a close races the next status write.
//
// A conversation on its way out is tolerated as NotFound — the finalizer path
// has no object to mark and does not need one.
func (q *OpQueue) markThreadArchived(ctx context.Context, op *Op) error {
	_, channel, ok := ParseCloseTopicOpID(op.ID)
	if !ok || q.Conversations == nil {
		return nil
	}
	_, err := state.Mutate(ctx, q.Conversations, state.Key(op.Conversation), func(rec *state.ConversationRecord) error {
		if rec.Phase != state.PhaseClosed && rec.ClosedAt == nil {
			// A close-topic replayed from a feed after the conversation was
			// reopened must not archive a live thread.
			return nil
		}
		if !rec.ThreadArchived(channel) {
			rec.ThreadsArchived = append(rec.ThreadsArchived, channel)
		}
		return nil
	})
	return state.IgnoreNotFound(err)
}

// markDelivered records that a run's reply reached one channel's thread, so
// reconciliation stops re-deriving it.
//
// TWO WRITES, IN THIS ORDER: the run's delivery marker (Runs.Grow, which only
// ever grows the run and so never conflicts with the next run's own append),
// then the thread's undelivered-reply note (a record write, retried on
// conflict). The first is the fact — it is what stops the re-derivation — and
// the second is the observable note beside it. A crash between them leaves a
// run recorded delivered with a note still naming it; SettleDeliveredNote,
// which both delivery passes run for every run they skip as delivered, clears it.
// The reverse order would clear the note on a reply nothing yet records as
// delivered, and a crash then would hide an owed reply.
//
// A plain merge patch on `status.runs` replaced the whole array, so two
// threads' completions milliseconds apart each wrote the array they had read
// and the last one won (the system pack's fan-out test found it: console plus
// Telegram bound to one conversation, `delivered: [tg-ops]` forever). Grow
// re-reads and applies its one effect, which is what makes the second write
// safe where the first was not.
func (q *OpQueue) markDelivered(ctx context.Context, op *Op) error {
	_, channel, runID, ok := ParseRunReplyOpID(op.ID)
	if !ok || q.Conversations == nil {
		// Same guard markThreadArchived carries: a queue with no store marks
		// nothing rather than panicking. The asymmetry was invisible because no
		// test had ever completed a run reply SUCCESSFULLY without one.
		return nil
	}
	key := state.Key(op.Conversation)
	runs, err := q.Runs.List(ctx, key)
	if err != nil {
		return state.IgnoreNotFound(err)
	}
	var run *state.Run
	for i := range runs {
		if runs[i].RunID == runID {
			run = &runs[i]
			break
		}
	}
	if run == nil {
		// The run aged out of the bounded list. Nothing is owed — a run that
		// is no longer recorded cannot be re-derived either.
		return nil
	}
	if run.DeliveredTo(channel) {
		return nil
	}
	if err := q.Runs.Grow(ctx, key, runID, state.Run{Delivered: []string{channel}}); err != nil {
		return state.IgnoreNotFound(err)
	}
	// Only the note naming THIS run is settled. A note naming an older run
	// is a reply still owed to the thread, and this one reaching it does not
	// pay that off.
	return clearUndeliveredNote(ctx, q.Conversations, key, channel, runID)
}

// clearUndeliveredNote empties the thread's undelivered-reply note when it
// names runID. Writes nothing when the thread is gone or the note names
// another run or none.
func clearUndeliveredNote(ctx context.Context, conversations state.Collection[state.ConversationRecord], key state.Key, channel, runID string) error {
	_, err := state.Mutate(ctx, conversations, key, func(rec *state.ConversationRecord) error {
		if b := rec.Thread(channel); b != nil && b.UndeliveredReply == runID {
			b.UndeliveredReply = ""
		}
		return nil
	})
	return state.IgnoreNotFound(err)
}

// markUndelivered records that a run's reply failed to reach one channel's
// thread, so an empty-looking chat thread is diagnosable from the Conversation
// object alone rather than only from manager logs (state-durability).
//
// Complete's own res.Error branch has already decided to Fail this task, so
// the bus (or reconciliation, once the task is finally dead) re-derives the
// send on its own; this is purely the observable half of that, and never the
// thing re-derivation depends on.
func (q *OpQueue) markUndelivered(ctx context.Context, op *Op) error {
	_, channel, runID, ok := ParseRunReplyOpID(op.ID)
	if !ok || q.Conversations == nil {
		return nil
	}
	_, err := state.Mutate(ctx, q.Conversations, state.Key(op.Conversation), func(rec *state.ConversationRecord) error {
		if binding := rec.Thread(channel); binding != nil {
			binding.UndeliveredReply = runID
		}
		return nil
	})
	if err = state.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("marking run %s undelivered on %s: %w", runID, channel, err)
	}
	return nil
}

// finishEnsureTopic records the thread an adapter created. Mutate re-reads and
// retries on a lost race, which is what keeps two channels' topics — completing
// within milliseconds of each other on a multi-channel conversation — from
// overwriting one another's binding, where a plain merge patch on
// `status.threads` let the second one win and the fan-out then had one thread
// forever.
func (q *OpQueue) finishEnsureTopic(ctx context.Context, op *Op, res OpResult) error {
	cond := state.Condition{Type: ConditionTopicReady, Status: state.ConditionTrue, Reason: "TopicCreated",
		Message: channelMsgPrefix + op.Channel}
	switch {
	case res.Error != "":
		cond.Status = state.ConditionFalse
		cond.Reason = "AdapterError"
		cond.Message = channelMsgPrefix + op.Channel + ": " + res.Error
	case res.ThreadID == "":
		cond.Status = state.ConditionFalse
		cond.Reason = "AdapterError"
		cond.Message = channelMsgPrefix + op.Channel + ": adapter completed ensure-topic without a thread id"
	}
	_, err := state.Mutate(ctx, q.Conversations, state.Key(op.Conversation), func(rec *state.ConversationRecord) error {
		recordThread(rec, op, res, cond)
		return nil
	})
	if err != nil {
		return state.IgnoreNotFound(err)
	}
	if cond.Status == state.ConditionFalse {
		return fmt.Errorf("ensure-topic for %s: %s", op.Conversation, cond.Message)
	}
	return nil
}

const channelMsgPrefix = "channel "

// recordThread applies one completed ensure-topic to a conversation record:
// the thread binding it created or re-established, the claim it releases, and
// the TopicReady condition.
func recordThread(rec *state.ConversationRecord, op *Op, res OpResult, cond state.Condition) {
	if cond.Status == state.ConditionTrue {
		bindThread(rec, op, res)
	}
	// async-command-bus design D-11 retires status.threads[].claim — nothing
	// in this process writes one any more (the BUS's own CommandID dedup
	// replaces the reconciler-level placeholder this cleared). Left in place
	// as a harmless no-op rather than removed: it still scrubs a value a
	// pre-upgrade replica wrote and never released, the one case a value can
	// still arrive here, and "a value found there is ignored" is true either
	// way — read or not, this op's own completion is what makes it stale.
	if b := rec.Thread(op.Channel); b != nil {
		b.Claim = nil
	}
	if cond.Status == state.ConditionTrue {
		// No longer archived: there is a live thread here again.
		kept := rec.ThreadsArchived[:0]
		for _, c := range rec.ThreadsArchived {
			if c != op.Channel {
				kept = append(kept, c)
			}
		}
		rec.ThreadsArchived = kept
	}
	rec.SetCondition(cond)
}

// bindThread records the thread a successful ensure-topic created or
// re-established.
func bindThread(rec *state.ConversationRecord, op *Op, res OpResult) {
	binding := rec.Thread(op.Channel)
	if binding != nil && binding.ThreadID != "" {
		// A REOPEN re-establishing an archived thread. Whatever came back is
		// the thread now: an adapter that honoured the hint returns the same id
		// and the conversation continues where it left off, one that could not
		// returns a fresh one. Both are recorded the same way, because both are
		// the truth about that transport.
		binding.ThreadID = res.ThreadID
		binding.Claim = nil
		return
	}
	// Brand new thread — including a binding that so far carries only a
	// leftover claim (pre-async-command-bus-D-11 bookkeeping this process
	// never writes any more) and never a real thread id. Treated exactly as
	// "no binding at all": such a placeholder is bookkeeping about dispatch,
	// not a prior thread.
	readers := originReaders(rec, op.Channel)
	if binding == nil {
		// ReadTracked for EVERY channel, whether or not its adapter reports
		// reads: the backfill rule has to stay one rule, or "no readAt" would
		// mean "pre-upgrade" on some channels and "never seen" on others.
		rec.Threads = append(rec.Threads, state.ThreadBinding{
			Channel: op.Channel, ThreadID: res.ThreadID, ReadTracked: true, Readers: readers,
		})
		return
	}
	binding.ThreadID = res.ThreadID
	binding.ReadTracked = true
	binding.Readers = readers
	binding.Claim = nil
}

// originReaders is the new thread's reader marks. The person who STARTED this
// conversation has seen it — they typed it. Stamping their own watermark here,
// at the one moment their thread comes into existence, is what stops a
// conversation being presented back to its author as unread before any answer
// could exist. Their key only, everyone else's view is untouched, which is
// what makes this safe to do at all.
func originReaders(rec *state.ConversationRecord, channel string) []state.ReaderMark {
	o := rec.OriginReader
	if o == nil || o.Key == "" || o.Channel != channel {
		return nil
	}
	// Opaque RFC3339Nano string, never a time — see ThreadBinding.ReadAt's comment.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return []state.ReaderMark{{Key: o.Key, ReadAt: &now}}
}

// Pending reports whether an op is queued, running or retrying (used by
// tests, and by the delete-conversation finalizer) — the local mirror, never
// the bus (design D-10).
func (q *OpQueue) Pending(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.init()
	return q.pending[id]
}

// opOriginNode names where an op came from. A conversation-bound op flows from
// its conversation; a bare notice (an unwired-source warning on a general
// surface) flows from the manager.
func opOriginNode(op *Op) *activity.NodeRef {
	if op.Conversation != "" {
		return activity.Node(activity.NodeConversation, op.Conversation)
	}
	return activity.Node(activity.NodeManager, activity.NodeManager)
}

// Run drives this process's half of the command bus's "out" family that
// Claim/Complete do not cover on their own (design D-10):
//
//   - the watch-fed mirror Pending/Settled/Stats read, kept current for
//     transitions this process did not itself make (another replica's
//     Settle/Fail, the bus's own lease-lapse or TTL reaping);
//   - one consumer.Pool per channel queue whose adapter is registered with the
//     Registry as an in-process implementation ("in-process providers become a
//     consumer consuming out.<channel>"). Channels appear at run time, so a
//     pool starts when its channel is first learned. Today's Registry carries
//     no production registration (platform/console is an EXTERNAL adapter), so
//     this is exercised only by whatever a future in-process type or a test
//     registers, but the mechanism is real.
//
// Registered once from cmd/manager as a manager.RunnableFunc, on EVERY
// replica — never gated on leader election, the same posture every other
// consumer.Pool in this process already has. Blocks until ctx ends. A nil
// Bus runs nothing.
func (q *OpQueue) Run(ctx context.Context) error {
	if q.Bus == nil {
		return nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		q.watchMirror(ctx)
	}()
	defer wg.Wait()
	if q.Registry == nil {
		return nil
	}

	q.mu.Lock()
	q.init()
	wake := q.poolWake
	q.mu.Unlock()
	started := map[string]bool{}
	for {
		if err := q.startInProcessPools(ctx, &wg, started); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-wake:
		}
	}
}

// startInProcessPools starts a consumer for every learned channel whose
// adapter has a registered in-process factory and that has none yet.
func (q *OpQueue) startInProcessPools(ctx context.Context, wg *sync.WaitGroup, started map[string]bool) error {
	concurrency := q.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	q.mu.Lock()
	snapshot := make(map[string]string, len(q.channelAdapter))
	for ch, adapter := range q.channelAdapter {
		snapshot[ch] = adapter
	}
	q.mu.Unlock()
	for channel, adapter := range snapshot {
		if started[channel] {
			continue
		}
		factory := q.Registry.Resolve(adapter)
		if factory == nil {
			continue
		}
		pool, err := consumer.New(consumer.Config{
			Bus: q.Bus, Queue: commandbus.OutQueue(channel),
			Handler:     q.inProcessHandler(adapter, factory),
			Concurrency: concurrency,
			ConsumerID:  "in-process-" + channel,
		})
		if err != nil {
			return fmt.Errorf("in-process consumer for channel %q (adapter %q): %w", channel, adapter, err)
		}
		started[channel] = true
		wg.Add(1)
		go func(p *consumer.Pool) {
			defer wg.Done()
			_ = p.Run(ctx)
		}(pool)
	}
	return nil
}

// watchMirror consumes Bus.Feed for the lifetime of ctx, applying every
// transition to the local pending/settled mirror and learning every
// "out.<channel>" queue it sees — the cross-replica half of the channel->
// adapter map (this process's own Enqueue/Claim calls are the single-process
// half, noteChannel). The feed names the queue, not its adapter, so the first
// event of an unknown channel reads the task once for the adapter its payload
// carries.
func (q *OpQueue) watchMirror(ctx context.Context) {
	events, err := q.Bus.Feed(ctx, 0)
	if err != nil {
		log.FromContext(ctx).Error(err, "open command bus feed for the out-queue mirror")
		return
	}
	for ev := range events {
		q.markFromState(string(ev.CommandID), ev.State)
		if channel, ok := strings.CutPrefix(string(ev.Queue), outFamilyQueuePrefix); ok && !q.knowsChannel(channel) {
			q.learnChannel(ctx, channel, ev.UID)
		}
	}
}

// learnChannel resolves a channel's adapter from one of its tasks' payloads.
func (q *OpQueue) learnChannel(ctx context.Context, channel, taskUID string) {
	task, err := q.Bus.GetTask(ctx, taskUID)
	if err != nil {
		return // the next event of this channel tries again
	}
	op, err := decodeOp(string(task.CommandID), task.Payload)
	if err != nil {
		return
	}
	q.noteChannel(channel, op.channelType)
}

// outFamilyQueuePrefix mirrors commandbus.OutQueue's own "out." prefix —
// restated rather than imported as a function call, since watchMirror needs
// the inverse (strip the prefix), which commandbus does not export.
const outFamilyQueuePrefix = "out."

// inProcessHandler adapts one Registry-registered Provider onto
// consumer.HandlerFunc: decode, execute exactly as an external adapter
// would from the SAME payload, then run the SAME side effects Complete runs
// for an externally-claimed op — an in-process provider is a second
// renderer, never a second completion path.
func (q *OpQueue) inProcessHandler(adapter string, factory ProviderFactory) consumer.HandlerFunc {
	return func(ctx context.Context, task commandbus.Task) error {
		op, err := decodeOp(string(task.CommandID), task.Payload)
		if err != nil {
			return consumer.WithCode(commandbus.CodeInvalidCommand, err)
		}
		op.channelType = adapter

		res := executeInProcess(ctx, factory, op)

		done := activity.Event{
			Kind:         activity.KindChannelOpCompleted,
			From:         activity.Node(activity.NodeChannelAdapter, op.channelType),
			To:           activity.Node(activity.NodeChannel, op.Channel),
			Conversation: op.Conversation, OpID: op.ID, Code: string(op.Kind), Detail: op.Channel,
		}
		if res.Error != "" {
			done.Status = activity.StatusError
			done.Detail = op.Channel + ": " + res.Error
		}
		q.Activity.Emit(done)

		if handlerErr := q.runSideEffects(ctx, op, res); handlerErr != nil {
			return consumer.WithCode(commandbus.CodeInternal, handlerErr)
		}
		if res.Error != "" {
			return consumer.WithCode(commandbus.CodeUnavailable, errors.New(res.Error))
		}
		return nil
	}
}

// executeInProcess runs one op against the in-process provider the factory
// builds for its channel, folding any failure into the result's Error.
func executeInProcess(ctx context.Context, factory ProviderFactory, op *Op) OpResult {
	res := OpResult{}
	provider, err := factory(ctx, op.Channel)
	if err == nil {
		switch op.Kind {
		case OpEnsureTopic:
			if op.Topic != nil {
				res.ThreadID, err = provider.EnsureTopic(ctx, *op.Topic)
			}
		case OpSend:
			if op.Message != nil {
				err = provider.Send(ctx, op.ThreadID, *op.Message)
			}
		case OpCloseTopic:
			if closer, ok := provider.(TopicCloser); ok && op.ThreadID != nil {
				err = closer.CloseTopic(ctx, *op.ThreadID)
			}
		}
	}
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

// QueueStat is one adapter's outstanding-op picture: how deep, how old, and
// WHICH op is the oldest of each kind. The identities live here rather than in
// a metric label, because a conversation or op id as a label grows series
// without bound — metrics answer "how deep, how old", this answers "which one".
type QueueStat struct {
	Adapter string `json:"adapter"`
	// Queued ops wait for an adapter to claim them: deep-and-old means nothing
	// is polling.
	Queued int `json:"queued"`
	// Claimed ops were taken and never completed: deep-and-old means an adapter
	// is wedged mid-delivery. The two failure modes look identical from outside,
	// which is exactly why they are counted apart.
	Claimed             int     `json:"claimed"`
	OldestQueuedOpID    string  `json:"oldestQueuedOpId,omitempty"`
	OldestQueuedAgeSecs float64 `json:"oldestQueuedAgeSeconds,omitempty"`
	OldestQueuedConv    string  `json:"oldestQueuedConversation,omitempty"`

	OldestClaimedOpID    string  `json:"oldestClaimedOpId,omitempty"`
	OldestClaimedAgeSecs float64 `json:"oldestClaimedAgeSeconds,omitempty"`
	OldestClaimedConv    string  `json:"oldestClaimedConversation,omitempty"`
}

// Stats snapshots the queue per adapter, aggregating over every channel queue
// the adapter serves and over the three commandbus.Kind values the "out"
// family declares (design D-5) — commandbus.Bus.Stats answers one (queue,
// kind) pair at a time, so this is three calls per channel queue. The console's
// queue view stays per adapter, whatever the queue's own granularity.
//
// channelAdapter (noteChannel/watchMirror) is this method's own answer to a
// real gap: commandbus.Bus has no "list every out.<channel> queue in use" call
// (D-5's inventory is closed over KIND, never over the open set of channels a
// deployment may ever route through). A channel this process has never
// enqueued onto or claimed from, and whose Feed events this process's
// watchMirror has not yet seen, is simply absent here, exactly as the old
// in-memory map was empty for an adapter it had never heard from.
func (q *OpQueue) Stats() []QueueStat {
	q.mu.Lock()
	byAdapter := map[string][]string{}
	for channel, adapter := range q.channelAdapter {
		byAdapter[adapter] = append(byAdapter[adapter], channel)
	}
	q.mu.Unlock()

	ctx := context.Background()
	out := make([]QueueStat, 0, len(byAdapter))
	for adapter, channels := range byAdapter {
		st := QueueStat{Adapter: adapter}
		for _, channel := range channels {
			q.addChannelStats(ctx, &st, channel)
		}
		if st.Queued == 0 && st.Claimed == 0 {
			continue // pruned -- nothing outstanding reads as absent, as it always has
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Adapter < out[j].Adapter })
	return out
}

// addChannelStats folds one channel queue's counts into its adapter's stat,
// keeping the oldest queued and the oldest claimed op across all of them.
func (q *OpQueue) addChannelStats(ctx context.Context, st *QueueStat, channel string) {
	queue := commandbus.OutQueue(channel)
	for _, kind := range outKinds {
		bs, err := q.Bus.Stats(ctx, queue, kind)
		if err != nil {
			log.FromContext(ctx).Error(err, "channel op stats", "channel", channel, "kind", kind)
			continue
		}
		st.Queued += bs.Pending + bs.Scheduled
		st.Claimed += bs.Running
		if bs.Pending > 0 && bs.OldestPendingAge.Seconds() >= st.OldestQueuedAgeSecs {
			st.OldestQueuedOpID = string(bs.OldestPendingID)
			st.OldestQueuedConv = conversationFromKey(bs.OldestPendingKey)
			st.OldestQueuedAgeSecs = bs.OldestPendingAge.Seconds()
		}
		if bs.Running > 0 && bs.OldestRunningAge.Seconds() >= st.OldestClaimedAgeSecs {
			st.OldestClaimedOpID = string(bs.OldestRunningID)
			st.OldestClaimedConv = conversationFromKey(bs.OldestRunningKey)
			st.OldestClaimedAgeSecs = bs.OldestRunningAge.Seconds()
		}
	}
}
