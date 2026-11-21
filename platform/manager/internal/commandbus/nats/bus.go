// Package nats is the production command bus backend. It carries no state of
// its own beyond NATS: messages travel in one JetStream stream, a task's state
// is a record in a key-value bucket, and the delivery itself, order per key,
// redelivery and acknowledgement, is the broker's own.
//
// A message type is delivered one of three ways, by configuration:
//
//   - a regular queue: competing consumers on one shared durable consumer;
//   - a sequential convoy: one convoy queue per key, one message of it in
//     flight at a time, in order, while other keys run in parallel;
//   - a hand-over queue (a declared DeliveryTTL): served from the task
//     records to an adapter that acknowledges and reports over HTTP.
package nats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Option tunes a Bus.
type Option func(*busConfig)

type busConfig struct {
	replicas           int
	leaseDuration      time.Duration
	inventory          []commandbus.Declaration
	sweepInterval      time.Duration
	deadLetterMaxAge   time.Duration
	deadLetterMaxCount int
	routing            Routing
	concurrency        int
	convoyIdle         time.Duration
}

const (
	defaultLeaseDuration = 5 * time.Minute
	// DefaultSweepInterval is how often the sweep looks for settled records to
	// drop and idle convoy queues to remove.
	DefaultSweepInterval = 30 * time.Second
	// DefaultDeadLetterMaxAge and DefaultDeadLetterMaxCount bound the dead
	// letters a bus keeps.
	DefaultDeadLetterMaxAge   = 7 * 24 * time.Hour
	DefaultDeadLetterMaxCount = 10000
	defaultConvoyIdle         = 10 * time.Minute
)

func newBusConfig(opts []Option) busConfig {
	cfg := busConfig{
		leaseDuration: defaultLeaseDuration, inventory: commandbus.DefaultInventory(),
		sweepInterval: DefaultSweepInterval, deadLetterMaxAge: DefaultDeadLetterMaxAge,
		deadLetterMaxCount: DefaultDeadLetterMaxCount, concurrency: 4, convoyIdle: defaultConvoyIdle,
		routing: func(string) Mode { return ModeConvoy },
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithReplicas sets the replica count of every stream and bucket the bus creates.
func WithReplicas(n int) Option { return func(c *busConfig) { c.replicas = n } }

// WithLeaseDuration sets how long a delivered message may go unanswered before
// the broker hands it to another consumer.
func WithLeaseDuration(d time.Duration) Option { return func(c *busConfig) { c.leaseDuration = d } }

// WithInventory replaces the queue inventory.
func WithInventory(inv []commandbus.Declaration) Option {
	return func(c *busConfig) { c.inventory = inv }
}

// WithSweepInterval sets the sweep period.
func WithSweepInterval(d time.Duration) Option { return func(c *busConfig) { c.sweepInterval = d } }

// WithDeadLetterRetention bounds the dead letters kept.
func WithDeadLetterRetention(maxAge time.Duration, maxCount int) Option {
	return func(c *busConfig) { c.deadLetterMaxAge, c.deadLetterMaxCount = maxAge, maxCount }
}

// WithRouting says, per queue, whether it is a regular queue or a sequential
// convoy. A new message type is one more answer here, and no other change.
func WithRouting(r Routing) Option { return func(c *busConfig) { c.routing = r } }

// WithConsumerConcurrency sets how many messages of a regular queue may be in
// flight at once, across every replica.
func WithConsumerConcurrency(n int) Option { return func(c *busConfig) { c.concurrency = n } }

// WithConvoyIdle sets how long a convoy queue sits empty before it is removed.
func WithConvoyIdle(d time.Duration) Option { return func(c *busConfig) { c.convoyIdle = d } }

// Bus implements commandbus.Bus over NATS.
type Bus struct {
	// Retired names the streams of the earlier log-based bus this Open deleted.
	Retired []string

	js     jetstream.JetStream
	nc     *nats.Conn
	subj   subjects
	stream string
	cfg    busConfig

	tasks       jetstream.KeyValue
	tasksStream string
	convoys     *convoyController
	pub         *Publisher
	mirror      *taskMirror

	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	feeds map[commandbus.Queue]<-chan *message.Message
	held  map[string]*message.Message // lease token to the message this process holds
	subs  []*Subscriber
}

func bucketName(scope, base string) string {
	if scope == "" {
		return base
	}
	return scope + "_" + base
}

// Open creates, or finds, everything the bus needs and starts it. It reads
// before it writes, so an install that already has every stream and bucket
// needs no metadata change to start.
func Open(ctx context.Context, conn *Conn, scope string, opts ...Option) (*Bus, error) {
	cfg := newBusConfig(opts)
	replicas := cfg.replicas
	if replicas <= 0 {
		replicas = conn.cfg.Replicas
	}
	subj := newSubjects(scope)
	stream := bucketName(scope, "cb")
	if err := ensurePubSubStream(ctx, conn.js, stream, subj, replicas, time.Minute); err != nil {
		return nil, err
	}
	tasksBucket := bucketName(scope, "cbtasks")
	tasks, err := openBucket(ctx, conn.js, jetstream.KeyValueConfig{Bucket: tasksBucket, History: 8, Replicas: replicas, TTL: cfg.deadLetterMaxAge * 2})
	if err != nil {
		return nil, err
	}
	registry, err := openBucket(ctx, conn.js, jetstream.KeyValueConfig{Bucket: bucketName(scope, "cbconvoys"), History: 1, Replicas: replicas})
	if err != nil {
		return nil, err
	}
	bctx, cancel := context.WithCancel(context.Background())
	b := &Bus{
		js: conn.js, nc: conn.nc, subj: subj, stream: stream, cfg: cfg,
		tasks: tasks, tasksStream: "KV_" + tasksBucket,
		convoys: newConvoyController(registry, subj), ctx: bctx, cancel: cancel,
		feeds: map[commandbus.Queue]<-chan *message.Message{}, held: map[string]*message.Message{},
	}
	b.Retired = retireLegacyStreams(ctx, conn.js, scope)
	b.pub = newPublisher(b.pubsubConfig())
	b.mirror = newTaskMirror()
	go b.mirror.run(bctx, tasks)
	if err := b.mirror.waitReady(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("commandbus/nats: open: %w", err)
	}
	return b, nil
}

// legacyStreams are the streams the log-based bus created. No code reads them
// now, and what they hold was never handed to the backend that replaced it.
var legacyStreams = []string{"cmdbus_log", "cmdbus_dead"}

// retireLegacyStreams deletes each legacy stream that exists and returns the
// names it deleted. A failed delete is left for the next open.
func retireLegacyStreams(ctx context.Context, js jetstream.JetStream, scope string) []string {
	var gone []string
	for _, base := range legacyStreams {
		name := bucketName(scope, base)
		if err := js.DeleteStream(ctx, name); err == nil {
			gone = append(gone, name)
		}
	}
	return gone
}

// openBucket gets a bucket and creates it only if it is missing, then makes
// sure no replica answers a read from a view that lags a write.
func openBucket(ctx context.Context, js jetstream.JetStream, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	kv, err := js.KeyValue(ctx, cfg.Bucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: open bucket %q: %w", cfg.Bucket, err)
	}
	s, err := js.Stream(ctx, "KV_"+cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: open bucket %q: %w", cfg.Bucket, err)
	}
	if sc := s.CachedInfo().Config; sc.AllowDirect {
		sc.AllowDirect = false
		if _, err := js.UpdateStream(ctx, sc); err != nil {
			return nil, fmt.Errorf("commandbus/nats: bucket %q: turn direct reads off: %w", cfg.Bucket, err)
		}
		if kv, err = js.KeyValue(ctx, cfg.Bucket); err != nil {
			return nil, fmt.Errorf("commandbus/nats: reopen bucket %q: %w", cfg.Bucket, err)
		}
	}
	return kv, nil
}

func (b *Bus) pubsubConfig() pubsubConfig {
	return pubsubConfig{js: b.js, stream: b.stream, subj: b.subj, convoys: b.convoys, routing: b.route}
}

// route is the Routing the publisher uses: a hand-over queue is served from
// the records, everything else as configured.
func (b *Bus) route(topic string) Mode {
	if b.handover(commandbus.Queue(topic)) {
		return ModeHandover
	}
	return b.cfg.routing(topic)
}

// handover reports whether a queue is a hand-over queue (design D-17).
func (b *Bus) handover(q commandbus.Queue) bool {
	for _, d := range b.cfg.inventory {
		if d.Queue == familyOf(q) && d.Policy.DeliveryTTL > 0 {
			return true
		}
	}
	return false
}

func familyOf(q commandbus.Queue) commandbus.Queue {
	if strings.HasPrefix(string(q), "out.") {
		return commandbus.OutFamily
	}
	return q
}

func (b *Bus) decl(q commandbus.Queue, k commandbus.Kind) (commandbus.Declaration, error) {
	d, ok := commandbus.Resolve(b.cfg.inventory, q, k)
	if !ok {
		return commandbus.Declaration{}, fmt.Errorf("%w: queue=%q kind=%q", commandbus.ErrUnknownQueue, q, k)
	}
	return d, nil
}

// Close stops everything the bus runs. The connection stays the caller's.
func (b *Bus) Close() {
	b.cancel()
	b.mu.Lock()
	subs := b.subs
	b.subs = nil
	b.mu.Unlock()
	for _, s := range subs {
		_ = s.Close()
	}
}
