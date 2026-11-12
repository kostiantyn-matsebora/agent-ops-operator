package nats

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// The bucket names of the closed inventory (package doc), before Scope's
// own prefix.
const (
	bucketTasks = "tasks"
	bucketIDs   = "ids"
	bucketDead  = "dead"
)

// bucketHistory mirrors internal/state/nats's own bucketHistory and the
// same reasoning: Feed's raw ordered consumer (feed.go) replays a key's
// earlier revisions to close a gap across a disconnect, and a bucket's
// History is enforced as MaxMsgsPerSubject on its backing stream — History
// 1 would purge a busy task's own earlier transitions the instant a newer
// one lands, not merely stop serving them (gotchas.md: "A KV bucket with
// History: 1 keeps nothing to replay after a purge").
const bucketHistory = 64

// Option configures Open.
type Option func(*busConfig)

type busConfig struct {
	replicas           int
	leaseDuration      time.Duration
	inventory          []commandbus.Declaration
	sweepInterval      time.Duration
	ledgerRetention    time.Duration
	deadLetterMaxAge   time.Duration
	deadLetterMaxCount int
	clock              *clock
}

// defaultLeaseDuration matches commandbus/memory's own default (and
// internal/chat's OpQueue.ReclaimAfter): no design value is named for
// commandbus specifically, and reusing the existing number is a smaller
// surprise than inventing a second one.
const defaultLeaseDuration = 5 * time.Minute

// DefaultSweepInterval is how often the background sweep (sweep.go) looks
// for a lapsed lease, an expired TTL, a released park, or retention to
// enforce.
const DefaultSweepInterval = 30 * time.Second

// DefaultLedgerRetention is how long a settled task's "tasks" record is
// kept once it has stopped suppressing its CommandID (commandbus.Task.
// Suppresses) — the ledger is declared bounded and lossy telemetry
// (invariants.md), and this is the bound.
const DefaultLedgerRetention = time.Hour

// DefaultDeadLetterMaxAge and DefaultDeadLetterMaxCount bound the "dead"
// bucket independently of the ledger's own retention — a dead-letter
// record is worth keeping far longer than a succeeded task's.
const (
	DefaultDeadLetterMaxAge   = 7 * 24 * time.Hour
	DefaultDeadLetterMaxCount = 10000
)

func newBusConfig(opts []Option) busConfig {
	cfg := busConfig{
		leaseDuration:      defaultLeaseDuration,
		inventory:          commandbus.DefaultInventory(),
		sweepInterval:      DefaultSweepInterval,
		ledgerRetention:    DefaultLedgerRetention,
		deadLetterMaxAge:   DefaultDeadLetterMaxAge,
		deadLetterMaxCount: DefaultDeadLetterMaxCount,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithReplicas overrides the replica count every bucket is created with.
// Zero (the default) means Conn's own configured replicas.
func WithReplicas(n int) Option { return func(c *busConfig) { c.replicas = n } }

// WithLeaseDuration overrides how long a Claim's lease runs before the
// backend treats it as lapsed (design D-2: bus-wide, never per queue).
func WithLeaseDuration(d time.Duration) Option { return func(c *busConfig) { c.leaseDuration = d } }

// WithInventory overrides the queue declarations Submit, Fail and Stats
// resolve against.
func WithInventory(inv []commandbus.Declaration) Option {
	return func(c *busConfig) { c.inventory = inv }
}

// WithSweepInterval overrides the background sweep's period.
func WithSweepInterval(d time.Duration) Option { return func(c *busConfig) { c.sweepInterval = d } }

// WithLedgerRetention overrides how long a settled, no-longer-suppressing
// task's "tasks" record survives before the sweep purges it.
func WithLedgerRetention(d time.Duration) Option {
	return func(c *busConfig) { c.ledgerRetention = d }
}

// WithDeadLetterRetention overrides the "dead" bucket's own age and count
// caps. Zero maxAge or maxCount disables that half of the cap.
func WithDeadLetterRetention(maxAge time.Duration, maxCount int) Option {
	return func(c *busConfig) { c.deadLetterMaxAge, c.deadLetterMaxCount = maxAge, maxCount }
}

// withClock injects a clock a test's Backing can Advance (claim.go,
// reap.go). Unexported: production has no business fast-forwarding its own
// bus, only commandbus/conformance's Backing contract needs it, and that is
// satisfied from inside this package's own tests (package nats, not
// nats_test), never from outside it.
func withClock(c *clock) Option { return func(cfg *busConfig) { cfg.clock = c } }

// Bus is the nats backend's commandbus.Bus: three JetStream KV buckets
// (package doc) bootstrapped under one scope, plus the in-memory mirror
// (mirror.go) Claim and Stats read instead of the server.
type Bus struct {
	js          jetstream.JetStream
	tasks       jetstream.KeyValue
	tasksBucket string // the scoped "tasks" bucket name, read only by Feed's subject prefix (feed.go)
	tasksStream string // "KV_<scoped tasks bucket>", read by tasksLatestSeq (stats.go) and Feed (feed.go)
	ids         jetstream.KeyValue
	dead        jetstream.KeyValue

	inventory     []commandbus.Declaration
	leaseDuration time.Duration
	ledgerRetention,
	deadLetterMaxAge time.Duration
	deadLetterMaxCount int
	sweepInterval      time.Duration

	clock  *clock
	mirror *taskMirror

	// selfSeq is the highest "tasks" revision this *Bus value has itself
	// written (bumped by casWriteTask and createTask). Claim fences on it
	// (claim.go) before trusting the mirror for the in-flight-cap and
	// per-key ordering checks: the mirror's watch is asynchronous even
	// against this same process's own prior writes, and two Claim calls
	// made back to back on one Bus value — the per-invocation cap test's
	// own shape — must see the first's effect on the second, not wait out
	// a stale view the way a genuinely cross-replica race may (design D-2
	// only promises that a stranger's stale view delays a claim, never
	// that this backend may ignore its own most recent write).
	selfSeq atomic.Uint64

	closed chan struct{}
}

// bumpSelfSeq records rev as having been written by this Bus value, if it
// is the highest one seen yet.
func (b *Bus) bumpSelfSeq(rev uint64) {
	for {
		cur := b.selfSeq.Load()
		if rev <= cur || b.selfSeq.CompareAndSwap(cur, rev) {
			return
		}
	}
}

var _ commandbus.Bus = (*Bus)(nil)

// Open bootstraps the three buckets under scope (empty means unscoped, a
// single-tenant install) and starts the tasks mirror, blocking until its
// first catch-up completes or ctx ends — so a Claim or Submit made right
// after Open returns never races an empty, still-filling cache.
func Open(ctx context.Context, conn *Conn, scope string, opts ...Option) (*Bus, error) {
	cfg := newBusConfig(opts)
	replicas := cfg.replicas
	if replicas <= 0 {
		replicas = conn.cfg.replicas()
	}

	tasksBucket := bucketName(scope, bucketTasks)
	tasksKV, err := bootstrapKV(ctx, conn.js, tasksBucket, replicas)
	if err != nil {
		return nil, err
	}
	idsKV, err := bootstrapKV(ctx, conn.js, bucketName(scope, bucketIDs), replicas)
	if err != nil {
		return nil, err
	}
	deadKV, err := bootstrapKV(ctx, conn.js, bucketName(scope, bucketDead), replicas)
	if err != nil {
		return nil, err
	}

	c := cfg.clock
	if c == nil {
		c = newClock(time.Now)
	}

	b := &Bus{
		js: conn.js, tasks: tasksKV, tasksBucket: tasksBucket, tasksStream: "KV_" + tasksBucket, ids: idsKV, dead: deadKV,
		inventory: cfg.inventory, leaseDuration: cfg.leaseDuration,
		ledgerRetention:  cfg.ledgerRetention,
		deadLetterMaxAge: cfg.deadLetterMaxAge, deadLetterMaxCount: cfg.deadLetterMaxCount,
		sweepInterval: cfg.sweepInterval,
		clock:         c,
		closed:        make(chan struct{}),
	}
	b.mirror = newTaskMirror(tasksKV, c.notify)
	if err := b.mirror.waitReady(ctx); err != nil {
		b.mirror.stop()
		return nil, fmt.Errorf("commandbus/nats: open: %w", err)
	}
	return b, nil
}

// Close ends the background mirror watch. Not part of commandbus.Bus —
// production holds one Bus for the manager's whole life — but a test
// opening many isolated Bus values over one scope calls it to avoid leaking
// a goroutine and a subscription per scenario.
func (b *Bus) Close() {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	b.mirror.stop()
}

// advanceClock fast-forwards b's clock by d (claim.go, reap.go), waking
// every blocked Claim at once rather than leaving it to the next poll.
// Unexported for the reason withClock is: only this package's own tests
// call it, through the conformance.Backing this package's test file
// builds.
func (b *Bus) advanceClock(d time.Duration) { b.clock.advance(d) }

// leaseDur reports the lease duration every Claim on b grants.
func (b *Bus) leaseDur() time.Duration { return b.leaseDuration }

// bucketName prefixes base with scope, exactly as internal/state/nats's own
// bucketName does — the two packages share the idea (an install may point
// both at one server, design D-1) without sharing the code.
func bucketName(scope, base string) string {
	if scope == "" {
		return base
	}
	return scope + "_" + base
}

// bootstrapKV creates or opens a bucket, with direct gets turned off: the
// client library creates every KV bucket with them on, and a direct get may
// be served by any replica, including one that has not applied the write
// the mirror or a CAS was just told about (gotchas.md's AllowDirect entry;
// internal/state/nats's own leaderReadsOnly is the same fix for the same
// reason, restated here rather than imported — design D-1 keeps the two
// backends independent).
func bootstrapKV(ctx context.Context, js jetstream.JetStream, name string, replicas int) (jetstream.KeyValue, error) {
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: name, Replicas: replicas, History: bucketHistory})
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: bootstrap bucket %q: %w", name, err)
	}
	if err := turnOffDirectGets(ctx, js, "KV_"+name); err != nil {
		return nil, fmt.Errorf("commandbus/nats: bucket %q direct reads: %w", name, err)
	}
	if kv, err = js.KeyValue(ctx, name); err != nil { // reopen so the handle reflects the stream as it is now
		return nil, fmt.Errorf("commandbus/nats: open bucket %q: %w", name, err)
	}
	return kv, nil
}

func turnOffDirectGets(ctx context.Context, js jetstream.JetStream, stream string) error {
	s, err := js.Stream(ctx, stream)
	if err != nil {
		return err
	}
	cfg := s.CachedInfo().Config
	if !cfg.AllowDirect {
		return nil
	}
	cfg.AllowDirect = false
	_, err = js.UpdateStream(ctx, cfg)
	return err
}

// clock is the virtual time every Claim/Submit/Fail/reap decision reads
// (mirroring commandbus/memory's own store.now/advance): base keeps moving
// in real time on its own, offset only ever grows, via advance — so a
// test's one jump forward stays fast-forwarded, rather than snapping back
// the moment real time next advances. See package memory's own doc on
// Backing.Advance for why that is what "fast forward" has to mean here.
type clock struct {
	base   func() time.Time
	offset atomic.Int64

	mu sync.Mutex
	ch chan struct{}
}

func newClock(base func() time.Time) *clock {
	return &clock{base: base, ch: make(chan struct{})}
}

func (c *clock) now() time.Time { return c.base().Add(time.Duration(c.offset.Load())) }

func (c *clock) advance(d time.Duration) {
	c.offset.Add(int64(d))
	c.notify()
}

// notify wakes everyone waiting on wait(): every blocked Claim poll, so a
// clock jump or a fresh mirror update is seen at once.
func (c *clock) notify() {
	c.mu.Lock()
	close(c.ch)
	c.ch = make(chan struct{})
	c.mu.Unlock()
}

func (c *clock) wait() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ch
}
