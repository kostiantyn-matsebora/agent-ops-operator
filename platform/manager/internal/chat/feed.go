package chat

import (
	"context"
	"errors"
	"slices"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The retained feed (async-command-bus design D-16). An adapter named in
// OpQueue.FeedAdapters (the console) is not asked to claim and acknowledge its
// ops: each op is appended once to a retained log, in the order it was
// produced, and the adapter reads that log over a stream by cursor, applying
// every event to its own state. A connection claims, leases and deletes
// nothing, so a dead or a second connection strands nothing.
//
// DELIVERED means the frame was written to a live stream. Delivered then runs
// the side effects an acknowledgement used to trigger, keyed by op id so a
// replay does not repeat them.

// consoleThreadPrefix is the shape of a feed adapter's thread id. The manager
// assigns it when it enqueues ensure-topic, so no result travels back.
const consoleThreadPrefix = "console-"

// FeedThreadID is the thread id of conv on a feed adapter's channel:
// deterministic from the conversation's UID, so the same conversation always
// maps to the same thread, across restarts and reopens.
func FeedThreadID(conv *state.ConversationRecord) string { return consoleThreadPrefix + conv.ID }

// IsFeed reports whether adapter reads its ops from a retained feed rather
// than claiming them.
func (q *OpQueue) IsFeed(adapter string) bool { return slices.Contains(q.FeedAdapters, adapter) }

// publishFeed appends op to its adapter's feed. A feed entry is durable the
// moment it is appended, so a delete-conversation, whose only effect is a
// tombstone line, never holds the finalizer waiting for a console to connect.
func (q *OpQueue) publishFeed(ctx context.Context, op *Op) {
	q.mu.Lock()
	alreadyDelivered := q.settled[op.ID]
	q.mu.Unlock()
	if alreadyDelivered {
		return
	}
	_, created, err := q.Bus.Publish(ctx, commandbus.FeedQueue(op.channelType), commandbus.CommandID(op.ID), op.encode())
	if err != nil {
		log.FromContext(ctx).Error(err, "publish channel op to the feed", "op", op.ID, "kind", op.Kind, "channel", op.Channel)
		return
	}
	q.noteChannel(op.Channel, op.channelType)
	if op.Kind == OpDeleteConversation {
		q.markFromState(op.ID, commandbus.StateSucceeded)
	} else {
		q.markFromState(op.ID, commandbus.StateQueued)
	}
	if !created {
		return
	}
	q.Activity.Emit(activity.Event{
		Kind:         activity.KindChannelOpEnqueued,
		From:         opOriginNode(op),
		To:           activity.Node(activity.NodeChannel, op.Channel),
		Conversation: op.Conversation, OpID: op.ID, Code: string(op.Kind), Detail: op.Channel,
	})
}

// OpenFeed follows adapter's feed after the op with command id after (empty or
// unknown: from the oldest retained). Ops arrive in log order, then as they are
// produced, until ctx ends and the channel closes.
func (q *OpQueue) OpenFeed(ctx context.Context, adapter, after string) (<-chan *Op, error) {
	entries, err := q.Bus.Follow(ctx, commandbus.FeedQueue(adapter), commandbus.CommandID(after))
	if err != nil {
		return nil, err
	}
	out := make(chan *Op)
	go func() {
		defer close(out)
		for e := range entries {
			op, err := decodeOp(string(e.ID), e.Payload)
			if err != nil {
				log.FromContext(ctx).Error(err, "decode a feed entry; skipped", "adapter", adapter)
				continue
			}
			op.channelType = adapter
			op.enqueuedAt = e.At
			select {
			case out <- op:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// errNoThreadAssigned is an ensure-topic that reached a feed with no thread id.
var errNoThreadAssigned = errors.New("ensure-topic carries no thread id the manager assigned")

// Delivered records that op was written to a live stream and runs the side
// effects an acknowledgement used to trigger (the run reply's delivered
// marker, the archive of a closed thread, the thread binding of an
// ensure-topic). It is idempotent per op id: a replay after a reconnect, or a
// second connection writing the same frame, runs them once. A side effect that
// fails is not recorded, so the next replay retries it.
func (q *OpQueue) Delivered(ctx context.Context, op *Op) {
	q.mu.Lock()
	q.init()
	if q.settled[op.ID] || q.delivering[op.ID] {
		q.mu.Unlock()
		return
	}
	q.delivering[op.ID] = true
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		delete(q.delivering, op.ID)
		q.mu.Unlock()
	}()

	res := OpResult{}
	if op.Kind == OpEnsureTopic {
		if op.ThreadID == nil || *op.ThreadID == "" {
			log.FromContext(ctx).Error(errNoThreadAssigned, "deliver ensure-topic", "op", op.ID, "conversation", op.Conversation)
			return
		}
		res.ThreadID = *op.ThreadID
	}
	q.Activity.Emit(activity.Event{
		Kind:         activity.KindChannelOpCompleted,
		From:         activity.Node(activity.NodeChannelAdapter, op.channelType),
		To:           activity.Node(activity.NodeChannel, op.Channel),
		Conversation: op.Conversation, OpID: op.ID, Code: string(op.Kind), Detail: op.Channel,
		LatencyMs: time.Since(op.enqueuedAt).Milliseconds(),
	})
	if err := q.runSideEffects(ctx, op, res); err != nil {
		return
	}
	q.markFromState(op.ID, commandbus.StateSucceeded)
}
