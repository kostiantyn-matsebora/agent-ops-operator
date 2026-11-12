package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// GetTask implements commandbus.Bus: the task folded from its own entries, so
// a lapsed lease or a passed TTL is already in the state returned, with no
// write behind it. A task the queue log no longer holds is still read from the
// dead log if it died.
func (b *Bus) GetTask(ctx context.Context, taskUID string) (commandbus.Task, error) {
	if err := b.sync(ctx); err != nil {
		return commandbus.Task{}, fmt.Errorf("commandbus/nats: get task: %w", err)
	}
	if t, ok := b.mirror.log.Task(taskUID, b.clock.now()); ok {
		return t, nil
	}
	if t, ok := b.mirror.deadTask(taskUID); ok {
		return t, nil
	}
	return commandbus.Task{}, commandbus.ErrNotFound
}

// Settle implements commandbus.Bus: a succeeded entry appended on the key's
// event subject, conditional on that subject's last sequence. A token that is
// not the live lease appends nothing and is ErrLeaseLost; so is a completion
// that loses the race to a competing entry, since the fresh view then shows
// the lease gone.
func (b *Bus) Settle(ctx context.Context, taskUID, leaseToken string) error {
	for attempt := 0; attempt < appendAttempts; attempt++ {
		if err := b.sync(ctx); err != nil {
			return fmt.Errorf("commandbus/nats: settle %s: %w", taskUID, err)
		}
		ap, err := b.mirror.log.PlanSettle(taskUID, leaseToken, b.clock.now())
		if err != nil {
			return err
		}
		seq, err := b.append(ctx, ap)
		if err == nil {
			if b.settledGrace <= 0 {
				b.removeConsumed(ctx, seq)
			}
			return nil
		}
		if !isWrongLastSeq(err) {
			return fmt.Errorf("commandbus/nats: settle %s: %w", taskUID, err)
		}
		if err := pause(ctx, raceBackoff(attempt)); err != nil {
			return fmt.Errorf("commandbus/nats: settle %s: %w", taskUID, err)
		}
	}
	return fmt.Errorf("commandbus/nats: settle %s: still losing the race after %d attempts", taskUID, appendAttempts)
}

// Fail implements commandbus.Bus: a failed entry, appended as Settle appends
// its own. A task that dies is also copied to the dead log.
func (b *Bus) Fail(ctx context.Context, taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	for attempt := 0; attempt < appendAttempts; attempt++ {
		if err := b.sync(ctx); err != nil {
			return fmt.Errorf("commandbus/nats: fail %s: %w", taskUID, err)
		}
		f, err := b.mirror.log.PlanFail(taskUID, leaseToken, code, detail, b.clock.now())
		if err != nil {
			return err
		}
		_, err = b.append(ctx, f.Append)
		if err == nil {
			if f.Task.State == commandbus.StateDead {
				b.archiveDead(ctx, f.Task)
			}
			return nil
		}
		if !isWrongLastSeq(err) {
			return fmt.Errorf("commandbus/nats: fail %s: %w", taskUID, err)
		}
		if err := pause(ctx, raceBackoff(attempt)); err != nil {
			return fmt.Errorf("commandbus/nats: fail %s: %w", taskUID, err)
		}
	}
	return fmt.Errorf("commandbus/nats: fail %s: still losing the race after %d attempts", taskUID, appendAttempts)
}

// archiveDead appends t to the dead log the instant it dies, so DeadLetters
// reaches past the queue log's own retention. Best-effort: the task is already
// dead in the queue log, and the sweep appends the copy of any dead task the
// dead log lacks, so a crash between the two appends heals.
func (b *Bus) archiveDead(ctx context.Context, t commandbus.Task) {
	data, err := json.Marshal(t)
	if err != nil {
		return
	}
	_, _ = b.js.Publish(ctx, b.subj.forDead(t.Queue, t.UID), data, jetstream.WithMsgID(t.UID+".dead"))
}

// removeConsumed removes the entries of every task consumed at least the grace
// ago from the queue log: the one delete the bus makes, and only ever of a task
// that succeeded (never of an unsettled entry). A removal is never a rewrite.
// Best-effort — an entry that survives is applied again by any later replay,
// which consumes the task again and removes it then. fence is a sequence the
// mirror must have applied first, so the task being removed is already gone from
// it.
func (b *Bus) removeConsumed(ctx context.Context, fence uint64) {
	if err := b.mirror.waitCaughtUpTo(ctx, fence); err != nil {
		return
	}
	seqs := b.mirror.log.TakeConsumed(b.clock.now(), b.settledGrace)
	if len(seqs) == 0 {
		return
	}
	for _, seq := range seqs {
		_ = b.logs.DeleteMsg(ctx, seq)
	}
}
