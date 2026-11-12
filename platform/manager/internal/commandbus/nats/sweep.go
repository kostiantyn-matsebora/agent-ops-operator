package nats

import (
	"context"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Run drives the background sweep until ctx ends or the Bus is closed. The
// sweep appends the two entries a reader cannot derive on its own, removes the
// entries of consumed tasks once their grace has passed (the only removal the
// bus makes), and lets the in-memory mirror forget what the logs' own limits
// have already dropped.
func (b *Bus) Run(ctx context.Context) {
	ticker := time.NewTicker(b.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.closed:
			return
		case <-ticker.C:
			b.sweepOnce(ctx)
		}
	}
}

func (b *Bus) sweepOnce(ctx context.Context) {
	now := b.clock.now()
	b.closeLapsedTasks(ctx, now)
	b.healDeadLog(ctx, now)
	b.mirror.forgetDead(func(t commandbus.Task) bool { return b.deadKept(t, now) })
	b.removeConsumed(ctx, 0)
	b.mirror.log.Prune(now, now.Add(-b.logMaxAge))
}

// closeLapsedTasks appends the dead entry for every task whose lease lapsed
// with its attempts spent. The fold already reads it as dead; the entry makes
// it a fact of the log and reaches the dead log. A lost race means a
// competing entry landed; the next sweep sees whatever is true then.
func (b *Bus) closeLapsedTasks(ctx context.Context, now time.Time) {
	for _, f := range b.mirror.log.PlanLapsedDeaths(now) {
		if _, err := b.append(ctx, f.Append); err == nil {
			b.archiveDead(ctx, f.Task)
		}
	}
}

// healDeadLog copies a dead task the dead log lacks — a crash between Fail's
// two appends — into it.
func (b *Bus) healDeadLog(ctx context.Context, now time.Time) {
	for _, t := range b.mirror.log.DeadTasks("", now) {
		if !b.mirror.hasDead(t.UID) {
			b.archiveDead(ctx, t)
		}
	}
}

// deadKept reports whether the dead log's own age limit still holds t.
func (b *Bus) deadKept(t commandbus.Task, now time.Time) bool {
	return b.deadLetterMaxAge <= 0 || now.Sub(t.DeadAt) <= b.deadLetterMaxAge
}

// withDeadLimits keeps what the dead log's limits would hold of dead (oldest
// death first): within the age cap, and the newest deadLetterMaxCount.
func (b *Bus) withinDeadLimits(dead []commandbus.Task, now time.Time) []commandbus.Task {
	kept := dead[:0:0]
	for _, t := range dead {
		if b.deadKept(t, now) {
			kept = append(kept, t)
		}
	}
	if n := b.deadLetterMaxCount; n > 0 && len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return kept
}
