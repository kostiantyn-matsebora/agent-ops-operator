package nats

import (
	"context"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The KV wire header a delete or purge carries, the plain NATS KV
// convention (nats.go's own kv.go keeps these unexported, so this backend
// restates the literal values rather than reaching into another package's
// internals).
const (
	kvOperationHeader = "KV-Operation"
	kvOperationDelete = "DEL"
	kvOperationPurge  = "PURGE"
)

// kvEntry is one raw message off a bucket's backing stream, decoded just
// enough for Watch (collection.go) and WatchWithItems (appended.go) to build
// the contract's Event.
type kvEntry struct {
	Key      string
	Revision uint64
	Value    []byte
	IsDelete bool
}

// watchRaw opens an ordered, pull-based consumer directly on the bucket's
// backing stream (design D-2's own "ordered consumer") and drains it into a
// channel of kvEntry until ctx is done.
//
// This bypasses the KV Watch convenience (jetstream.KeyValue.WatchAll)
// deliberately: resuming it couples "start at sequence rev+1"
// (ResumeFromRevision) with "only the latest value per key" (the implicit
// DeliverLastPerSubject a non-UpdatesOnly KV watch always asks for), so a
// resumed feed silently collapses every intermediate write between the
// disconnect and the resume point down to whatever is current by the time
// it reconnects — caught by the conformance suite's own
// NoGapAcrossConsumerDisconnect scenario. An ordered consumer with
// DeliverByStartSequencePolicy alone replays every message in order, which
// is what "at-least-once, no gap" actually requires.
func watchRaw(ctx context.Context, js jetstream.JetStream, streamName, bucket string, resume state.Revision) (<-chan kvEntry, error) {
	cfg := jetstream.OrderedConsumerConfig{DeliverPolicy: jetstream.DeliverByStartSequencePolicy}
	if resume == "" {
		// "From now" is fixed HERE, as a sequence, and never as the server's
		// "new" policy. An ordered consumer reset before it has delivered
		// anything is recreated from its original config, and the server
		// resolves "new" again at that moment, so everything written in
		// between is skipped for good. Found live: a reconciler's feed reset
		// 30s after it opened and never heard of the conversations created
		// meanwhile.
		stream, err := js.Stream(ctx, streamName)
		if err != nil {
			return nil, fmt.Errorf("nats: stream info %q: %w", streamName, err)
		}
		cfg.OptStartSeq = stream.CachedInfo().State.LastSeq + 1
	} else {
		seq, err := parseRevision(resume)
		if err != nil {
			return nil, err
		}
		cfg.OptStartSeq = seq + 1
	}
	consumer, err := js.OrderedConsumer(ctx, streamName, cfg)
	if err != nil {
		return nil, fmt.Errorf("nats: open ordered consumer on %q: %w", streamName, err)
	}
	msgs, err := consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("nats: open message iterator on %q: %w", streamName, err)
	}
	out := make(chan kvEntry)
	go drainRaw(ctx, msgs, "$KV."+bucket+".", out)
	return out, nil
}

func drainRaw(ctx context.Context, msgs jetstream.MessagesContext, subjectPrefix string, out chan<- kvEntry) {
	defer close(out)
	go func() {
		<-ctx.Done()
		msgs.Stop()
	}()
	for {
		msg, err := msgs.Next()
		if err != nil {
			return // ctx cancelled or a terminal failure either way: no error value crosses the contract
		}
		meta, err := msg.Metadata()
		if err != nil {
			continue
		}
		key := strings.TrimPrefix(msg.Subject(), subjectPrefix)
		op := msg.Headers().Get(kvOperationHeader)
		entry := kvEntry{
			Key: key, Revision: meta.Sequence.Stream, Value: msg.Data(),
			IsDelete: op == kvOperationDelete || op == kvOperationPurge,
		}
		select {
		case out <- entry:
		case <-ctx.Done():
			return
		}
	}
}
