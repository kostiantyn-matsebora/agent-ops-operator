package nats

import (
	"context"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Feed implements commandbus.Bus. Sequence maps 1:1 onto the "tasks"
// bucket's own backing-stream sequence: every Submit, Claim, Settle, Fail
// and reap writes exactly one message there, so consecutive transitions
// get consecutive numbers, with no other writer sharing that stream
// (design D-2: "the transport only stores and wakes" — here, the
// transport IS the ledger bucket's own watch). A ledger-retention purge
// (sweep.go) also consumes one of this counter's numbers with no matching
// TaskEvent, which can leave a numeric gap where nothing happened — never
// a dropped event, which is the property Sequence actually promises
// ("resuming strictly after X delivers every event after X").
//
// This opens its own ordered, pull-based consumer directly on the
// backing stream, exactly as internal/state/nats/rawwatch.go does and for
// the identical reason (restated here, not imported — design D-1):
// resuming jetstream.KeyValue.WatchAll couples "start at sequence rev+1"
// with "only the latest value per key", silently collapsing every
// intermediate write between a disconnect and the resume point
// (gotchas.md's own entry on exactly this).
func (b *Bus) Feed(ctx context.Context, resume commandbus.Sequence) (<-chan commandbus.TaskEvent, error) {
	raw, err := b.watchTasksRaw(ctx, resume)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: %w", err)
	}
	out := make(chan commandbus.TaskEvent)
	go drainTaskFeed(ctx, raw, out)
	return out, nil
}

// feedEntry is one raw message off the "tasks" stream.
type feedEntry struct {
	UID      string
	Sequence uint64
	Value    []byte
	IsDelete bool
}

func (b *Bus) watchTasksRaw(ctx context.Context, resume commandbus.Sequence) (<-chan feedEntry, error) {
	cfg := jetstream.OrderedConsumerConfig{DeliverPolicy: jetstream.DeliverByStartSequencePolicy}
	if resume == 0 {
		// "From now" is fixed HERE, as a sequence, never as the server's
		// own "new" policy — an ordered consumer reset before it has
		// delivered anything re-resolves "new" at THAT moment, skipping
		// everything written in between for good (the exact gotcha
		// internal/state/nats/rawwatch.go documents and works around).
		s, err := b.js.Stream(ctx, b.tasksStream)
		if err != nil {
			return nil, fmt.Errorf("stream info %q: %w", b.tasksStream, err)
		}
		cfg.OptStartSeq = s.CachedInfo().State.LastSeq + 1
	} else {
		cfg.OptStartSeq = uint64(resume) + 1
	}
	consumer, err := b.js.OrderedConsumer(ctx, b.tasksStream, cfg)
	if err != nil {
		return nil, fmt.Errorf("open ordered consumer on %q: %w", b.tasksStream, err)
	}
	msgs, err := consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("open message iterator on %q: %w", b.tasksStream, err)
	}
	out := make(chan feedEntry)
	go drainFeedRaw(ctx, msgs, "$KV."+b.tasksBucket+".", out)
	return out, nil
}

func drainFeedRaw(ctx context.Context, msgs jetstream.MessagesContext, subjectPrefix string, out chan<- feedEntry) {
	defer close(out)
	go func() {
		<-ctx.Done()
		msgs.Stop()
	}()
	for {
		msg, err := msgs.Next()
		if err != nil {
			return // ctx canceled or a terminal failure — no error value crosses the contract
		}
		meta, err := msg.Metadata()
		if err != nil {
			continue
		}
		op := msg.Headers().Get("KV-Operation")
		entry := feedEntry{
			UID: strings.TrimPrefix(msg.Subject(), subjectPrefix), Sequence: meta.Sequence.Stream,
			Value: msg.Data(), IsDelete: op == "DEL" || op == "PURGE",
		}
		select {
		case out <- entry:
		case <-ctx.Done():
			return
		}
	}
}

// drainTaskFeed translates raw stream entries into the contract's
// TaskEvent, dropping a delete/purge outright — the ledger-retention
// sweep's own cleanup, never a task transition a caller asked to see.
func drainTaskFeed(ctx context.Context, raw <-chan feedEntry, out chan<- commandbus.TaskEvent) {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-raw:
			if !ok {
				return
			}
			if entry.IsDelete {
				continue
			}
			t, err := decodeTask(entry.Value)
			if err != nil {
				continue // an entry this process never wrote — dropped, not crashed
			}
			ev := commandbus.TaskEvent{
				Sequence: commandbus.Sequence(entry.Sequence), UID: t.UID, Queue: t.Queue, Kind: t.Kind,
				Key: t.Key, CommandID: t.CommandID, InvocationID: t.InvocationID,
				State: t.State, Attempt: t.Attempt, Code: t.Code,
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}
