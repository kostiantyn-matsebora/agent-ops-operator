package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
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
	// ModeHandover is a queue read by an adapter over HTTP: the message is
	// stored under the key's subject and served from the task records, with no
	// consumer.
	ModeHandover
)

// Metadata the subscriber stamps on a message it delivers, and the bus reads.
const (
	metaUID      = "uid"
	metaReply    = "reply"
	metaAttempt  = "attempt"
	metaNakDelay = "nak-delay"
	metaTerm     = "term"
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
		if _, err := p.PublishSeq(topic, m); err != nil {
			return err
		}
	}
	return nil
}

// PublishSeq publishes one message and returns the sequence the stream gave
// it, which is its place in line.
func (p *Publisher) PublishSeq(topic string, m *message.Message) (uint64, error) {
	ctx := m.Context()
	subject := p.cfg.regularSubject(topic)
	switch p.cfg.routing(topic) {
	case ModeConvoy, ModeHandover:
		key := m.Metadata.Get(KeyMetadata)
		if key == "" {
			return 0, fmt.Errorf("commandbus/nats: topic %q needs the %q metadata", topic, KeyMetadata)
		}
		if p.cfg.routing(topic) == ModeConvoy {
			cv, err := p.cfg.convoys.Ensure(ctx, commandbus.Queue(topic), commandbus.Key(key), time.Now())
			if err != nil {
				return 0, err
			}
			subject = cv.Subject
		} else {
			subject = p.cfg.convoys.subjectFor(commandbus.Queue(topic), commandbus.Key(key))
		}
	}
	msg := nats.NewMsg(subject)
	msg.Data = m.Payload
	msg.Header.Set(msgIDHeader, m.UUID)
	for k, v := range m.Metadata {
		msg.Header.Set(metaPrefix+k, v)
	}
	ack, err := p.cfg.js.PublishMsg(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("commandbus/nats: publish to %q: %w", topic, err)
	}
	return ack.Sequence, nil
}

func (p *Publisher) Close() error { return nil }

// outbox is the channel a subscription delivers on. A send holds a read lock and
// the close takes the write lock, so a delivery that is still in flight when
// the subscription ends can never send on a closed channel.
type outbox struct {
	mu     sync.RWMutex
	closed bool
	ch     chan *message.Message
}

func newOutbox() *outbox { return &outbox{ch: make(chan *message.Message)} }

// send delivers m, or gives up when ctx ends or the outbox is closed.
func (o *outbox) send(ctx context.Context, m *message.Message) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.closed {
		return false
	}
	select {
	case o.ch <- m:
		return true
	case <-ctx.Done():
		return false
	}
}

func (o *outbox) close() {
	o.mu.Lock()
	if !o.closed {
		o.closed = true
		close(o.ch)
	}
	o.mu.Unlock()
}

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
	// renew keeps a held message alive with an in-progress signal. The bus turns
	// it off and leaves renewal to whoever holds the claim, so an abandoned claim
	// lapses.
	renew bool
	// attachHook, set by a test, runs before a convoy queue is attached and may
	// fail it.
	attachHook func(Convoy) error

	mu     sync.Mutex
	stops  []func()
	closed bool
}

func newSubscriber(cfg pubsubConfig, concurrency int) *Subscriber {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Subscriber{cfg: cfg, ackWait: defaultAckWait, nackDelay: time.Second, concurrency: concurrency, maxDeliver: 5, renew: true}
}

func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	box := newOutbox()
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
	closeOut := func() { wg.Wait(); box.close() }
	if s.cfg.routing(topic) == ModeConvoy {
		events, err := s.cfg.convoys.Watch(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.followConvoys(ctx, topic, events, box, &wg)
		}()
		go closeOut()
		return box.ch, nil
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
	if _, err := s.consume(ctx, cons, box, &wg); err != nil {
		cancel()
		return nil, err
	}
	go closeOut()
	return box.ch, nil
}

// convoyResync is how often the registry is read again, a safety net for an
// event the watch missed.
const convoyResync = 30 * time.Second

// attachRetry is how long a convoy queue that could not be attached waits
// before the next try.
const attachRetry = time.Second

// attached is a convoy queue this subscriber consumes.
type attached struct {
	stop context.CancelFunc
}

// followConvoys keeps this subscriber attached to every convoy queue of a
// topic. An attach that fails is retried, a consume loop that ends without
// being asked to is attached again, and the registry is read again now and
// then, so a queue is never left unconsumed because of one failed moment, such
// as a leader change.
func (s *Subscriber) followConvoys(ctx context.Context, topic string, events <-chan ConvoyEvent, out *outbox, wg *sync.WaitGroup) {
	live := map[string]*attached{}
	want := map[string]Convoy{}
	lost := make(chan string, 64)
	defer func() {
		for _, a := range live {
			a.stop()
		}
	}()
	try := func() {
		for rk, cv := range want {
			if _, ok := live[rk]; ok {
				continue
			}
			cctx, stop := context.WithCancel(ctx)
			closed, err := s.attachConvoy(cctx, cv, out, wg)
			if err != nil {
				stop()
				continue
			}
			live[rk] = &attached{stop: stop}
			go func() {
				select {
				case <-closed:
					if cctx.Err() == nil {
						select {
						case lost <- rk:
						case <-ctx.Done():
						}
					}
				case <-cctx.Done():
				}
			}()
		}
	}
	retry := time.NewTicker(attachRetry)
	defer retry.Stop()
	resync := time.NewTicker(convoyResync)
	defer resync.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Deleted {
				if a, ok := live[ev.Convoy.Subject]; ok { // a deletion names the registry key
					a.stop()
					delete(live, ev.Convoy.Subject)
				}
				delete(want, ev.Convoy.Subject)
				continue
			}
			if string(ev.Convoy.Queue) != topic {
				continue
			}
			want[registryKey(ev.Convoy.Queue, ev.Convoy.Key)] = ev.Convoy
			try()
		case rk := <-lost:
			if a, ok := live[rk]; ok {
				a.stop()
				delete(live, rk)
			}
			try()
		case <-retry.C:
			try()
		case <-resync.C:
			if all, err := s.cfg.convoys.List(ctx); err == nil {
				for _, cv := range all {
					if string(cv.Queue) == topic {
						want[registryKey(cv.Queue, cv.Key)] = cv
					}
				}
				try()
			}
		}
	}
}

// attachConvoy attaches to the shared consumer of one convoy queue, creating
// it if needed, with one message in flight.
func (s *Subscriber) attachConvoy(ctx context.Context, cv Convoy, out *outbox, wg *sync.WaitGroup) (<-chan struct{}, error) {
	if s.attachHook != nil {
		if err := s.attachHook(cv); err != nil {
			return nil, err
		}
	}
	actx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cons, err := s.cfg.js.CreateOrUpdateConsumer(actx, s.cfg.stream, jetstream.ConsumerConfig{
		Durable: cv.Consumer, FilterSubject: cv.Subject, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: s.ackWait, MaxDeliver: s.maxDeliver,
		MaxAckPending: 1, // the sequential convoy: one message of the key in flight
	})
	if err != nil {
		return nil, err
	}
	return s.consume(ctx, cons, out, wg)
}

// consume feeds a consumer's messages to out until ctx ends. Each message is
// kept alive with an in-progress signal while the handler holds it, and is
// acked, delayed or left to its ack-wait by the handler's answer.
func (s *Subscriber) consume(ctx context.Context, cons jetstream.Consumer, out *outbox, wg *sync.WaitGroup) (<-chan struct{}, error) {
	cc, err := cons.Consume(func(js jetstream.Msg) {
		m := message.NewMessage(js.Headers().Get(msgIDHeader), js.Data())
		for k, v := range js.Headers() {
			if len(k) > len(metaPrefix) && k[:len(metaPrefix)] == metaPrefix && len(v) > 0 {
				m.Metadata.Set(k[len(metaPrefix):], v[0])
			}
		}
		m.Metadata.Set(metaReply, js.Reply())
		if md, err := js.Metadata(); err == nil {
			m.Metadata.Set(metaAttempt, strconv.FormatUint(md.NumDelivered, 10))
		}
		m.SetContext(ctx)
		if !out.send(ctx, m) {
			return
		}
		go s.settle(ctx, js, m)
	}, jetstream.PullMaxMessages(1))
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: consume: %w", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		cc.Stop()
	}()
	return cc.Closed(), nil
}

// settle waits for the handler's answer, renewing the ack-wait while it runs.
// A nack carries its own delay, or asks that the message never come back.
func (s *Subscriber) settle(ctx context.Context, js jetstream.Msg, m *message.Message) {
	tick := time.NewTicker(s.ackWait / 3)
	defer tick.Stop()
	for {
		select {
		case <-m.Acked():
			_ = js.Ack()
			return
		case <-m.Nacked():
			if m.Metadata.Get(metaTerm) != "" {
				_ = js.Term()
				return
			}
			delay := s.nackDelay
			if d, err := time.ParseDuration(m.Metadata.Get(metaNakDelay)); err == nil {
				delay = d
			}
			_ = js.NakWithDelay(delay)
			return
		case <-tick.C:
			if s.renew {
				_ = js.InProgress()
			}
		case <-ctx.Done():
			return // unanswered: the server redelivers it after its ack-wait
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
