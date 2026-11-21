package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Mode is how a message type is delivered. It is configuration, so a new type
// needs a routing entry and no change to the bus.
type Mode int

const (
	// ModeRegular is a plain queue read by competing consumers.
	ModeRegular Mode = iota
	// ModeConvoy is the sequential convoy: one convoy queue per key, one message
	// of it in flight at a time, in order. Other keys run in parallel.
	ModeConvoy
)

// Routing says how the messages of a topic (a queue) are delivered.
type Routing func(topic string) Mode

// KeyMetadata is the Watermill metadata entry naming a message's convoy key.
const KeyMetadata = "key"

const (
	msgIDHeader    = "Nats-Msg-Id"
	metaPrefix     = "wm-"
	defaultAckWait = 30 * time.Second
)

// pubsubConfig is what the publisher and subscriber share.
type pubsubConfig struct {
	js      jetstream.JetStream
	stream  string
	subj    subjects
	convoys *convoyController
	routing Routing
}

func (c pubsubConfig) regularSubject(topic string) string {
	return c.subj.prefix + ".r." + escapeToken(topic)
}

func regularConsumerName(topic string) string {
	sum := sha256.Sum256([]byte("r." + topic))
	return "rq-" + hex.EncodeToString(sum[:12])
}

// EnsureStream creates the one work-queue stream that holds every regular and
// convoy queue of the scope, if it does not exist. A message stays until a
// consumer acknowledges it, so work published before any consumer exists is
// kept.
func ensurePubSubStream(ctx context.Context, js jetstream.JetStream, name string, subj subjects, replicas int, dedup time.Duration) error {
	if _, err := js.Stream(ctx, name); err == nil {
		return nil
	} else if !errors.Is(err, jetstream.ErrStreamNotFound) {
		return fmt.Errorf("commandbus/nats: read stream %q: %w", name, err)
	}
	cfg := jetstream.StreamConfig{
		Name: name, Subjects: []string{subj.prefix + ".q.>", subj.prefix + ".r.>"},
		Retention: jetstream.WorkQueuePolicy, Duplicates: dedup, Storage: jetstream.FileStorage,
	}
	if replicas > 0 {
		cfg.Replicas = replicas
	}
	if _, err := js.CreateStream(ctx, cfg); err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("commandbus/nats: create stream %q: %w", name, err)
	}
	return nil
}

// Publisher is a Watermill publisher over JetStream. A regular topic is one
// publish to the queue's subject. A convoy topic first makes sure the key's
// convoy queue exists, then publishes to it. The call returns once the server
// has stored the message, and a repeated message id is suppressed by the
// server.
type Publisher struct{ cfg pubsubConfig }

func newPublisher(cfg pubsubConfig) *Publisher { return &Publisher{cfg: cfg} }

func (p *Publisher) Publish(topic string, messages ...*message.Message) error {
	for _, m := range messages {
		ctx := m.Context()
		subject := p.cfg.regularSubject(topic)
		if p.cfg.routing(topic) == ModeConvoy {
			key := m.Metadata.Get(KeyMetadata)
			if key == "" {
				return fmt.Errorf("commandbus/nats: convoy topic %q needs the %q metadata", topic, KeyMetadata)
			}
			cv, err := p.cfg.convoys.Ensure(ctx, commandbus.Queue(topic), commandbus.Key(key), time.Now())
			if err != nil {
				return err
			}
			subject = cv.Subject
		}
		msg := nats.NewMsg(subject)
		msg.Data = m.Payload
		msg.Header.Set(msgIDHeader, m.UUID)
		for k, v := range m.Metadata {
			msg.Header.Set(metaPrefix+k, v)
		}
		if _, err := p.cfg.js.PublishMsg(ctx, msg); err != nil {
			return fmt.Errorf("commandbus/nats: publish to %q: %w", topic, err)
		}
	}
	return nil
}

func (p *Publisher) Close() error { return nil }

// Subscriber is a Watermill subscriber over JetStream. A regular topic is one
// shared durable consumer. A convoy topic attaches to the shared durable
// consumer of every convoy queue of the topic, those that exist and those
// created later, and drops a consumer when its queue is removed.
type Subscriber struct {
	cfg     pubsubConfig
	ackWait time.Duration
	// nackDelay is how long a nacked message waits before it is delivered again.
	nackDelay time.Duration
	// concurrency is how many unacknowledged messages a regular topic may have
	// in flight across every subscriber of it.
	concurrency int
	maxDeliver  int

	mu     sync.Mutex
	stops  []func()
	closed bool
}

func newSubscriber(cfg pubsubConfig, concurrency int) *Subscriber {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Subscriber{cfg: cfg, ackWait: defaultAckWait, nackDelay: time.Second, concurrency: concurrency, maxDeliver: 5}
}

func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	out := make(chan *message.Message)
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return nil, errors.New("commandbus/nats: subscriber closed")
	}
	s.stops = append(s.stops, cancel)
	s.mu.Unlock()

	var wg sync.WaitGroup
	closeOut := func() { wg.Wait(); close(out) }
	if s.cfg.routing(topic) == ModeConvoy {
		events, err := s.cfg.convoys.Watch(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.followConvoys(ctx, topic, events, out, &wg)
		}()
		go closeOut()
		return out, nil
	}
	cons, err := s.cfg.js.CreateOrUpdateConsumer(ctx, s.cfg.stream, jetstream.ConsumerConfig{
		Durable: regularConsumerName(topic), FilterSubject: s.cfg.regularSubject(topic),
		AckPolicy: jetstream.AckExplicitPolicy, AckWait: s.ackWait, MaxDeliver: s.maxDeliver,
		MaxAckPending: s.concurrency,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("commandbus/nats: attach regular queue %q: %w", topic, err)
	}
	if err := s.consume(ctx, cons, out, &wg); err != nil {
		cancel()
		return nil, err
	}
	go closeOut()
	return out, nil
}

// followConvoys attaches to the convoy queues of one topic as the registry
// reports them.
func (s *Subscriber) followConvoys(ctx context.Context, topic string, events <-chan ConvoyEvent, out chan<- *message.Message, wg *sync.WaitGroup) {
	attached := map[string]context.CancelFunc{}
	defer func() {
		for _, stop := range attached {
			stop()
		}
	}()
	for ev := range events {
		if ev.Deleted {
			if stop, ok := attached[ev.Convoy.Subject]; ok { // a deletion names the registry key
				stop()
				delete(attached, ev.Convoy.Subject)
			}
			continue
		}
		cv := ev.Convoy
		if string(cv.Queue) != topic {
			continue
		}
		rk := registryKey(cv.Queue, cv.Key)
		if _, ok := attached[rk]; ok {
			continue
		}
		cons, err := s.cfg.js.CreateOrUpdateConsumer(ctx, s.cfg.stream, jetstream.ConsumerConfig{
			Durable: cv.Consumer, FilterSubject: cv.Subject, AckPolicy: jetstream.AckExplicitPolicy,
			AckWait: s.ackWait, MaxDeliver: s.maxDeliver,
			MaxAckPending: 1, // the sequential convoy: one message of the key in flight
		})
		if err != nil {
			continue // the registry is watched again on the next event; the entry stays
		}
		cctx, stop := context.WithCancel(ctx)
		if err := s.consume(cctx, cons, out, wg); err != nil {
			stop()
			continue
		}
		attached[rk] = stop
	}
}

// consume feeds a consumer's messages to out until ctx ends. Each message is
// kept alive with an in-progress signal while the handler holds it, and is
// acked, delayed or left to its ack-wait by the handler's answer.
func (s *Subscriber) consume(ctx context.Context, cons jetstream.Consumer, out chan<- *message.Message, wg *sync.WaitGroup) error {
	cc, err := cons.Consume(func(js jetstream.Msg) {
		m := message.NewMessage(js.Headers().Get(msgIDHeader), js.Data())
		for k, v := range js.Headers() {
			if len(k) > len(metaPrefix) && k[:len(metaPrefix)] == metaPrefix && len(v) > 0 {
				m.Metadata.Set(k[len(metaPrefix):], v[0])
			}
		}
		m.SetContext(ctx)
		select {
		case out <- m:
		case <-ctx.Done():
			return
		}
		s.settle(ctx, js, m)
	})
	if err != nil {
		return fmt.Errorf("commandbus/nats: consume: %w", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		cc.Stop()
	}()
	return nil
}

// settle waits for the handler's answer, renewing the ack-wait while it runs.
func (s *Subscriber) settle(ctx context.Context, js jetstream.Msg, m *message.Message) {
	tick := time.NewTicker(s.ackWait / 3)
	defer tick.Stop()
	for {
		select {
		case <-m.Acked():
			_ = js.Ack()
			return
		case <-m.Nacked():
			_ = js.NakWithDelay(s.nackDelay)
			return
		case <-tick.C:
			_ = js.InProgress()
		case <-ctx.Done():
			return // unacked: the server redelivers it after its ack-wait
		}
	}
}

func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, stop := range s.stops {
		stop()
	}
	s.stops = nil
	return nil
}
