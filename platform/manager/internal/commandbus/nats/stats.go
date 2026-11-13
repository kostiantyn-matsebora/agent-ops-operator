package nats

import (
	"context"
	"fmt"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Stats implements commandbus.Bus over the mirror, fenced on the server's last
// sequence so a write that landed a moment before the call is counted.
func (b *Bus) Stats(ctx context.Context, queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	if queue == "" {
		return commandbus.QueueStats{}, fmt.Errorf("%w: Stats needs a queue", commandbus.ErrInvalidRequest)
	}
	if err := b.sync(ctx); err != nil {
		return commandbus.QueueStats{}, fmt.Errorf("commandbus/nats: stats: %w", err)
	}
	return b.mirror.log.Stats(queue, kind, b.clock.now())
}
