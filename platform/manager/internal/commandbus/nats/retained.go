package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// A retained feed (design D-16) is one JetStream stream per feed, under the
// subject cb.<scope>.feed.<feed>.<command id>. The stream's own MaxAge and
// MaxMsgs are the feed's retention, and nothing else removes an entry: a
// follower is an ordered consumer, which owns no server-side state a dead
// connection could strand.

// feedWire is an entry as stored.
type feedWire struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Payload []byte    `json:"payload,omitempty"`
}

// feeds caches the stream handle of every feed this Bus has opened.
type feeds struct {
	mu      sync.Mutex
	streams map[commandbus.Queue]jetstream.Stream
}

func (s subjects) feedBase(feed commandbus.Queue) string {
	return s.prefix + ".feed." + escapeToken(string(feed))
}

func (s subjects) forFeed(feed commandbus.Queue, id commandbus.CommandID) string {
	return s.feedBase(feed) + "." + escapeToken(string(id))
}

func (b *Bus) feedStreamName(feed commandbus.Queue) string {
	return bucketName(b.scope, "cmdbus_feed_"+strings.ReplaceAll(escapeToken(string(feed)), "=", "-"))
}

// feedStream opens, creating on first use, the stream of feed under its
// declared retention.
func (b *Bus) feedStream(ctx context.Context, feed commandbus.Queue) (jetstream.Stream, commandbus.FeedPolicy, error) {
	decl, ok := commandbus.ResolveFeed(b.feedInv, feed)
	if !ok {
		return nil, commandbus.FeedPolicy{}, fmt.Errorf("%w: feed=%q", commandbus.ErrUnknownQueue, feed)
	}
	b.feeds.mu.Lock()
	defer b.feeds.mu.Unlock()
	if st, ok := b.feeds.streams[feed]; ok {
		return st, decl.Policy, nil
	}
	replicas := b.replicas
	st, err := bootstrapStream(ctx, b.js, jetstream.StreamConfig{
		Name: b.feedStreamName(feed), Subjects: []string{b.subj.feedBase(feed) + ".>"}, Replicas: replicas,
		MaxAge: decl.Policy.MaxAge, MaxMsgs: limitOrUnset(int64(decl.Policy.MaxEntries)),
	})
	if err != nil {
		return nil, commandbus.FeedPolicy{}, err
	}
	if b.feeds.streams == nil {
		b.feeds.streams = map[commandbus.Queue]jetstream.Stream{}
	}
	b.feeds.streams[feed] = st
	return st, decl.Policy, nil
}

// Publish implements commandbus.Bus: one append, conditional on the id's own
// subject still ending where it was read, so of two replicas publishing one id
// exactly one appends.
func (b *Bus) Publish(ctx context.Context, feed commandbus.Queue, id commandbus.CommandID, payload []byte) (commandbus.FeedEntry, bool, error) {
	if id == "" || len(id) > commandbus.MaxFeedIDLen {
		return commandbus.FeedEntry{}, false, fmt.Errorf("%w: a feed entry needs a command id of 1 to %d bytes", commandbus.ErrInvalidRequest, commandbus.MaxFeedIDLen)
	}
	st, policy, err := b.feedStream(ctx, feed)
	if err != nil {
		return commandbus.FeedEntry{}, false, err
	}
	subject := b.subj.forFeed(feed, id)
	for attempt := 0; attempt < appendAttempts; attempt++ {
		now := b.clock.now()
		var expect uint64
		last, err := st.GetLastMsgForSubject(ctx, subject)
		switch {
		case err == nil:
			var w feedWire
			if json.Unmarshal(last.Data, &w) == nil && now.Sub(w.At) < policy.DedupWindow {
				return commandbus.FeedEntry{ID: id, At: w.At, Payload: w.Payload}, false, nil
			}
			expect = last.Sequence
		case !errors.Is(err, jetstream.ErrMsgNotFound):
			return commandbus.FeedEntry{}, false, fmt.Errorf("commandbus/nats: publish %q to %q: %w", id, feed, err)
		}
		data, _ := json.Marshal(feedWire{ID: string(id), At: now, Payload: payload})
		_, err = b.js.Publish(ctx, subject, data, jetstream.WithExpectLastSequencePerSubject(expect))
		if err == nil {
			return commandbus.FeedEntry{ID: id, At: now, Payload: payload}, true, nil
		}
		if !isWrongLastSeq(err) {
			return commandbus.FeedEntry{}, false, fmt.Errorf("commandbus/nats: publish %q to %q: %w", id, feed, err)
		}
		if err := pause(ctx, raceBackoff(attempt)); err != nil {
			return commandbus.FeedEntry{}, false, err
		}
	}
	return commandbus.FeedEntry{}, false, fmt.Errorf("commandbus/nats: publish %q to %q: still losing the race", id, feed)
}

// Follow implements commandbus.Bus with one ordered consumer for the life of
// ctx, started after the last entry of the cursor's id.
func (b *Bus) Follow(ctx context.Context, feed commandbus.Queue, after commandbus.CommandID) (<-chan commandbus.FeedEntry, error) {
	st, _, err := b.feedStream(ctx, feed)
	if err != nil {
		return nil, err
	}
	start := uint64(1)
	if after != "" {
		if m, err := st.GetLastMsgForSubject(ctx, b.subj.forFeed(feed, after)); err == nil {
			start = m.Sequence + 1
		} else if !errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil, fmt.Errorf("commandbus/nats: follow %q: find cursor: %w", feed, err)
		}
	}
	consumer, err := b.js.OrderedConsumer(ctx, b.feedStreamName(feed), jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy, OptStartSeq: start,
	})
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: follow %q: open ordered consumer: %w", feed, err)
	}
	msgs, err := consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: follow %q: open message iterator: %w", feed, err)
	}
	out := make(chan commandbus.FeedEntry)
	go func() {
		defer close(out)
		go func() {
			<-ctx.Done()
			msgs.Stop()
		}()
		for {
			msg, err := msgs.Next()
			if err != nil {
				return
			}
			var w feedWire
			if json.Unmarshal(msg.Data(), &w) != nil || w.ID == "" {
				continue
			}
			select {
			case out <- commandbus.FeedEntry{ID: commandbus.CommandID(w.ID), At: w.At, Payload: w.Payload}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
