package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// Feed implements commandbus.Bus. Sequence is the queue log's own: every
// entry yields one event, so a resumed feed is gap-free by the log's own
// numbering. The consumer is a raw ordered consumer over the stream — the same
// reason the previous design's was, a mirror consumer cannot be resumed at an
// arbitrary point — and resuming at sequence N delivers entries from N+1.
//
// A resume of zero starts from now; a later Sequence replays what the stream
// still holds, since the log's own retention is the only thing that removes an
// entry.
func (b *Bus) Feed(ctx context.Context, resume commandbus.Sequence) (<-chan commandbus.TaskEvent, error) {
	start := uint64(resume) + 1
	if resume == 0 {
		info, err := b.logInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("commandbus/nats: feed: %w", err)
		}
		start = info.State.LastSeq + 1
	}
	consumer, err := b.js.OrderedConsumer(ctx, b.mirror.logName, jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy, OptStartSeq: start,
	})
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: open ordered consumer: %w", err)
	}
	msgs, err := consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: feed: open message iterator: %w", err)
	}
	out := make(chan commandbus.TaskEvent)
	go drainFeed(ctx, msgs, out)
	return out, nil
}

func drainFeed(ctx context.Context, msgs jetstream.MessagesContext, out chan<- commandbus.TaskEvent) {
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
		var e entrylog.Entry
		if json.Unmarshal(msg.Data(), &e) != nil || e.UID == "" {
			continue // an entry this process never wrote — dropped, not crashed
		}
		select {
		case out <- e.Event(meta.Sequence.Stream):
		case <-ctx.Done():
			return
		}
	}
}
