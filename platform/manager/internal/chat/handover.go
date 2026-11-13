package chat

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The adapter-facing delivery contract (async-command-bus design D-17): GET,
// ACK, then DONE or FAILED.
//
//   - Read lists the next messages of an adapter's channel queues and changes
//     nothing. A message read by an adapter that dies before acknowledging is
//     simply offered again.
//   - Acknowledge is the hand-over. From it the message is the adapter's and the
//     manager never sends it again; its delivery TTL starts.
//   - Complete is the adapter's report: done, or failed (an error). A message
//     with no report at its TTL is failed by the bus. Failed is final.
//
// An adapter in ReceiptAdapters (the console) is the simple case: its
// acknowledgement is only its receipt of the message, so the manager performs the
// delivery side effects and consumes the message at the acknowledgement, and the
// adapter reports nothing.

// receiptThreadPrefix is the shape of a receipt adapter's thread id. The manager
// assigns it when it enqueues ensure-topic, so no result travels back.
const receiptThreadPrefix = "console-"

// ReceiptThreadID is the thread id of conv on a receipt adapter's channel:
// deterministic from the conversation's UID, so the same conversation always maps
// to the same thread, across restarts and reopens.
func ReceiptThreadID(conv *state.ConversationRecord) string { return receiptThreadPrefix + conv.ID }

// IsReceipt reports whether adapter's acknowledgement is only its receipt.
func (q *OpQueue) IsReceipt(adapter string) bool { return slices.Contains(q.ReceiptAdapters, adapter) }

var (
	// ErrOpGone is an acknowledgement of a message the queue does not hold:
	// already consumed, expired out of the log, or never enqueued here. A
	// duplicate acknowledgement of a finished message reads as this and is
	// harmless.
	ErrOpGone = errors.New("no such message on the adapter's queues")
	// ErrOpNotOffered is an acknowledgement of a message that is held but not
	// being offered: behind an unfinished message of its conversation, or
	// finished.
	ErrOpNotOffered = errors.New("the message is not being offered")
)

// Read lists up to limit of the next messages of adapter's channel queues,
// oldest first, and changes nothing. It never blocks: handleChannelOps owns the
// long-poll wait and the stream its poll.
func (q *OpQueue) Read(ctx context.Context, adapter string, limit int) []*Op {
	var all []*Op
	for _, channel := range q.channelsOf(adapter) {
		tasks, err := q.Bus.Peek(ctx, commandbus.OutQueue(channel), limit)
		if err != nil {
			log.FromContext(ctx).Error(err, "read channel ops", "adapter", adapter, "channel", channel)
			continue
		}
		for _, task := range tasks {
			op, err := decodeOp(string(task.CommandID), task.Payload)
			if err != nil {
				// A payload this package itself wrote should always decode.
				log.FromContext(ctx).Error(err, "decode a channel op; skipped", "task", task.UID, "channel", channel)
				continue
			}
			op.channelType = adapter
			op.enqueuedAt = task.SubmittedAt
			q.noteChannel(channel, adapter)
			all = append(all, op)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].enqueuedAt.Before(all[j].enqueuedAt) })
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Receipt is what an acknowledgement answers.
type Receipt struct {
	// TTL is how long the adapter has to report: the message's delivery TTL.
	TTL time.Duration
	// Consumed reports that the manager has already consumed the message (a
	// receipt adapter): there is nothing further to report.
	Consumed bool
}

// Acknowledge hands the message id to adapter. It is idempotent: a message
// already acknowledged and within its TTL answers the same Receipt, so a retried
// acknowledgement, or one landing on another replica, hands over nothing twice.
func (q *OpQueue) Acknowledge(ctx context.Context, adapter, id string) (Receipt, error) {
	task, channel, err := q.acknowledgeOnABusQueue(ctx, adapter, id)
	if err != nil {
		return Receipt{}, err
	}
	op, err := decodeOp(id, task.Payload)
	if err != nil {
		return Receipt{}, fmt.Errorf("decode acknowledged op %s: %w", id, err)
	}
	op.channelType = adapter
	op.enqueuedAt = task.SubmittedAt
	op.claimedAt = task.UpdatedAt
	q.noteChannel(channel, adapter)
	entry := claimEntry{uid: task.UID, token: leaseTokenOf(task), attempt: task.Attempt, maxAttempts: task.MaxAttempts, op: op}
	q.mu.Lock()
	q.init()
	q.claimed[id] = entry
	q.mu.Unlock()
	q.markFromState(id, commandbus.StateRunning)
	receipt := Receipt{TTL: time.Until(task.Lease.Until)}
	if q.IsReceipt(adapter) {
		q.consume(ctx, entry)
		receipt.Consumed = true
	}
	return receipt, nil
}

// acknowledgeOnABusQueue finds the channel queue of adapter that holds id and
// hands the message over there.
func (q *OpQueue) acknowledgeOnABusQueue(ctx context.Context, adapter, id string) (commandbus.Task, string, error) {
	notOffered := false
	for _, channel := range q.channelsOf(adapter) {
		task, err := q.Bus.Acknowledge(ctx, commandbus.OutQueue(channel), commandbus.CommandID(id), channelOpsConsumerID)
		switch {
		case err == nil && task.Lease != nil:
			return task, channel, nil
		case errors.Is(err, commandbus.ErrNotFound):
		case errors.Is(err, commandbus.ErrNotOffered):
			notOffered = true
		case err != nil:
			return commandbus.Task{}, "", fmt.Errorf("acknowledge %s on %s: %w", id, channel, err)
		}
	}
	if notOffered {
		return commandbus.Task{}, "", ErrOpNotOffered
	}
	return commandbus.Task{}, "", ErrOpGone
}

// consume performs the delivery of an acknowledged message for a receipt
// adapter: the activity hop, the side effects an acknowledgement triggers (run
// reply delivered, thread bound or archived), then removal from the queue. A
// side effect that fails fails the message, which is final: reconciliation
// re-derives what it still owes.
func (q *OpQueue) consume(ctx context.Context, entry claimEntry) {
	op := entry.op
	q.mu.Lock()
	delete(q.claimed, op.ID)
	q.mu.Unlock()

	res := OpResult{}
	if op.Kind == OpEnsureTopic && op.ThreadID != nil {
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
		if ferr := q.Bus.Fail(ctx, entry.uid, entry.token, commandbus.CodeInternal, err.Error()); ferr != nil {
			log.FromContext(ctx).Error(ferr, "record a failed channel op", "op", op.ID)
			return
		}
		q.markFromState(op.ID, commandbus.StateDead)
		return
	}
	if err := q.Bus.Settle(ctx, entry.uid, entry.token); err != nil {
		log.FromContext(ctx).Error(err, "consume a channel op", "op", op.ID, "task", entry.uid)
		return
	}
	q.markFromState(op.ID, commandbus.StateSucceeded)
}
