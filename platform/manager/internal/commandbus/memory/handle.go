package memory

import (
	"context"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

func (h *handle) Submit(_ context.Context, req commandbus.SubmitRequest) (commandbus.Task, error) {
	return h.s.submit(req)
}

func (h *handle) Claim(ctx context.Context, queue commandbus.Queue, consumerID string) (commandbus.Task, error) {
	return h.s.claim(ctx, queue, consumerID)
}

func (h *handle) Settle(_ context.Context, taskUID, leaseToken string) error {
	return h.s.settle(taskUID, leaseToken)
}

func (h *handle) Fail(_ context.Context, taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	return h.s.fail(taskUID, leaseToken, code, detail)
}

func (h *handle) GetTask(_ context.Context, taskUID string) (commandbus.Task, error) {
	return h.s.getTask(taskUID)
}

func (h *handle) DeadLetters(_ context.Context, queue commandbus.Queue) ([]commandbus.Task, error) {
	return h.s.deadLetters(queue)
}

func (h *handle) Running(_ context.Context, queues []commandbus.Queue, after commandbus.CommandID) ([]commandbus.Task, error) {
	return h.s.running(queues, after), nil
}

func (h *handle) Feed(ctx context.Context, resume commandbus.Sequence) (<-chan commandbus.TaskEvent, error) {
	return h.s.feed(ctx, resume), nil
}

func (h *handle) Stats(_ context.Context, queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	return h.s.stats(queue, kind)
}

func (h *handle) Publish(_ context.Context, feed commandbus.Queue, id commandbus.CommandID, payload []byte) (commandbus.FeedEntry, bool, error) {
	return h.s.publish(feed, id, payload)
}

func (h *handle) Follow(ctx context.Context, feed commandbus.Queue, after commandbus.CommandID) (<-chan commandbus.FeedEntry, error) {
	return h.s.follow(ctx, feed, after)
}
