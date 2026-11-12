package nats

import (
	"context"
	"fmt"
	"sort"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Running implements commandbus.Bus, reading taskMirror behind the same
// fence Stats uses so a claim another replica made a moment ago is seen. A
// lease past its Until is left out: the next Claim reaps and re-offers it.
func (b *Bus) Running(ctx context.Context, queue commandbus.Queue) ([]commandbus.Task, error) {
	target, err := b.tasksLatestSeq(ctx)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: running: %w", err)
	}
	if err := b.mirror.waitCaughtUpTo(ctx, target); err != nil {
		return nil, fmt.Errorf("commandbus/nats: running: %w", err)
	}
	now := b.clock.now()
	var out []commandbus.Task
	for _, e := range b.mirror.snapshot() {
		t := e.Task
		if t.Queue == queue && t.State == commandbus.StateRunning && t.Lease != nil && t.Lease.Until.After(now) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}
