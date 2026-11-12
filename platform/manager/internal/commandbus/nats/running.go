package nats

import (
	"context"
	"fmt"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Running implements commandbus.Bus: the folded running tasks, in log order,
// after the entry the cursor names. It fences on the server's last sequence
// first, so a resume landing on a replica that did not make a claim still sees
// it.
func (b *Bus) Running(ctx context.Context, queues []commandbus.Queue, after commandbus.CommandID) ([]commandbus.Task, error) {
	if err := b.sync(ctx); err != nil {
		return nil, fmt.Errorf("commandbus/nats: running: %w", err)
	}
	return b.mirror.log.Running(queues, after, b.clock.now()), nil
}
