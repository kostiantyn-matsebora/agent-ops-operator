package nats

import (
	"context"
	"fmt"
	"sort"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// DeadLetters implements commandbus.Bus: every dead task the queue log still
// holds, plus every one only the dead log still holds, oldest death first,
// within the dead log's own age and count limits. The dead log is what lets
// this reach past the queue log's own retention.
func (b *Bus) DeadLetters(ctx context.Context, queue commandbus.Queue) ([]commandbus.Task, error) {
	if err := b.sync(ctx); err != nil {
		return nil, fmt.Errorf("commandbus/nats: dead letters: %w", err)
	}
	now := b.clock.now()
	out := b.mirror.log.DeadTasks(queue, now)
	seen := make(map[string]bool, len(out))
	for _, t := range out {
		seen[t.UID] = true
	}
	for _, t := range b.mirror.deadSnapshots() {
		if !seen[t.UID] && (queue == "" || t.Queue == queue) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DeadAt.Before(out[j].DeadAt) })
	return b.withinDeadLimits(out, now), nil
}
