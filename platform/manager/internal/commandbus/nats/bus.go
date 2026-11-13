package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// The stream names of the closed inventory (package doc), before Scope's own
// prefix.
const (
	streamLog  = "cmdbus_log"
	streamDead = "cmdbus_dead"
)

// Option configures Open.
type Option func(*busConfig)

type busConfig struct {
	replicas           int
	leaseDuration      time.Duration
	inventory          []commandbus.Declaration
	sweepInterval      time.Duration
	logMaxAge          time.Duration
	logMaxMsgs         int64
	settledGrace       time.Duration
	deadLetterMaxAge   time.Duration
	deadLetterMaxCount int
	clock              *clock
}

// defaultLeaseDuration matches commandbus/memory's own default (and
// internal/chat's OpQueue.ReclaimAfter): no design value is named for
// commandbus specifically, and reusing the existing number is a smaller
// surprise than inventing a second one.
const defaultLeaseDuration = 5 * time.Minute

// DefaultSweepInterval is how often the background sweep (sweep.go) closes a
// lapsed task whose attempts are spent, heals the dead log, and lets the
// mirror forget what the log's own retention has dropped.
const DefaultSweepInterval = 30 * time.Second

// DefaultLogMaxAge is the queue log's retention: the stream drops an entry
// older than this, and nothing else removes one (design D-2). It is long
// enough that a conversation task queued through an outage is still there to
// run, and every task has settled long before it.
const DefaultLogMaxAge = 7 * 24 * time.Hour

// DefaultSettledGrace is how long the entries of a task that succeeded stay in
// the log before they are removed. Every replica's mirror drops the task the
// moment it applies the succeeded entry, so the grace changes no state; it only
// lets a replica still catching up, and a Feed consumer, read the entries of a
// task before they go, instead of finding a gap. Zero removes them inside the
// Settle that appended the succeeded entry.
const DefaultSettledGrace = 30 * time.Second

// DefaultDeadLetterMaxAge and DefaultDeadLetterMaxCount bound the dead log
// independently of the queue log's own retention — a dead-letter record is
// worth keeping as long as the queue's entries, and a count cap besides.
const (
	DefaultDeadLetterMaxAge   = 7 * 24 * time.Hour
	DefaultDeadLetterMaxCount = 10000
)

func newBusConfig(opts []Option) busConfig {
	cfg := busConfig{
		leaseDuration:      defaultLeaseDuration,
		inventory:          commandbus.DefaultInventory(),
		sweepInterval:      DefaultSweepInterval,
		logMaxAge:          DefaultLogMaxAge,
		settledGrace:       DefaultSettledGrace,
		deadLetterMaxAge:   DefaultDeadLetterMaxAge,
		deadLetterMaxCount: DefaultDeadLetterMaxCount,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithReplicas overrides the replica count every stream is created with.
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

// WithLogRetention overrides the queue log's own limits: the age past which
// the stream drops an entry, and an optional count cap (zero means none).
// These are the only way an entry leaves the log.
func WithLogRetention(maxAge time.Duration, maxMsgs int64) Option {
	return func(c *busConfig) { c.logMaxAge, c.logMaxMsgs = maxAge, maxMsgs }
}

// WithSettledGrace overrides how long a consumed task's entries stay in the log
// before they are removed. Zero removes them inside Settle itself.
func WithSettledGrace(d time.Duration) Option { return func(c *busConfig) { c.settledGrace = d } }

// WithDeadLetterRetention overrides the dead log's own age and count caps.
// Zero maxAge or maxCount disables that half of the cap.
func WithDeadLetterRetention(maxAge time.Duration, maxCount int) Option {
	return func(c *busConfig) { c.deadLetterMaxAge, c.deadLetterMaxCount = maxAge, maxCount }
}

// withClock injects a clock a test's Backing can Advance (claim.go). Unexported:
// production has no business fast-forwarding its own bus, only
// commandbus/conformance's Backing contract needs it, and that is satisfied
// from inside this package's own tests (package nats, not nats_test), never
// from outside it.
func withClock(c *clock) Option { return func(cfg *busConfig) { cfg.clock = c } }

// Bus is the nats backend's commandbus.Bus: two JetStream streams (package
// doc) bootstrapped under one scope, plus the in-memory mirror (mirror.go)
// Claim and Stats read instead of the server.
type Bus struct {
	js      jetstream.JetStream
	logName string
	// logs is the queue log stream handle, used only for calls that keep no
	// state on it (GetLastMsgForSubject, DeleteMsg): a handle caches its info
	// unguarded, so Info is always read through a fresh one (logInfo).
	logs jetstream.Stream
	subj subjects
	inv  []commandbus.Declaration
	dead string // the dead stream's name

	leaseDuration      time.Duration
	sweepInterval      time.Duration
	logMaxAge          time.Duration
	settledGrace       time.Duration
	deadLetterMaxAge   time.Duration
	deadLetterMaxCount int

	clock  *clock
	mirror *mirror

	// selfSeq is the highest log sequence this *Bus value has itself
	// appended. Claim fences on it (claim.go) before trusting the mirror for
	// the in-flight-cap and per-key ordering checks: the mirror's watch is
	// asynchronous even against this same process's own prior writes, and two
	// Claim calls made back to back on one Bus value must see the first's
	// effect on the second, not wait out a stale view the way a genuinely
	// cross-replica race may (design D-2 only promises that a stranger's
	// stale view delays a claim, never that this backend may ignore its own
	// most recent write).
	selfSeq atomic.Uint64

	closed chan struct{}
}

// bumpSelfSeq records seq as having been appended by this Bus value, if it is
// the highest one seen yet.
func (b *Bus) bumpSelfSeq(seq uint64) {
	for {
		cur := b.selfSeq.Load()
		if seq <= cur || b.selfSeq.CompareAndSwap(cur, seq) {
			return
		}
	}
}

var _ commandbus.Bus = (*Bus)(nil)

// Open bootstraps the two streams under scope (empty means unscoped, a
// single-tenant install) and starts the mirror, blocking until its first
// catch-up completes or ctx ends — so a Claim or Submit made right after Open
// returns never races an empty, still-filling cache.
func Open(ctx context.Context, conn *Conn, scope string, opts ...Option) (*Bus, error) {
	cfg := newBusConfig(opts)
	replicas := cfg.replicas
	if replicas <= 0 {
		replicas = conn.cfg.replicas()
	}
	subj := newSubjects(scope)

	logName := bucketName(scope, streamLog)
	logStream, err := bootstrapStream(ctx, conn.js, jetstream.StreamConfig{
		Name: logName, Subjects: []string{subj.logFilter()}, Replicas: replicas,
		MaxAge: cfg.logMaxAge, MaxMsgs: limitOrUnset(cfg.logMaxMsgs),
	})
	if err != nil {
		return nil, err
	}
	deadName := bucketName(scope, streamDead)
	if _, err := bootstrapStream(ctx, conn.js, jetstream.StreamConfig{
		Name: deadName, Subjects: []string{subj.deadFilter()}, Replicas: replicas,
		MaxAge: cfg.deadLetterMaxAge, MaxMsgs: limitOrUnset(int64(cfg.deadLetterMaxCount)),
	}); err != nil {
		return nil, err
	}

	c := cfg.clock
	if c == nil {
		c = newClock(time.Now)
	}
	b := &Bus{
		js: conn.js, logs: logStream, logName: logName, subj: subj, inv: cfg.inventory, dead: deadName,
		leaseDuration: cfg.leaseDuration, sweepInterval: cfg.sweepInterval, logMaxAge: cfg.logMaxAge, settledGrace: cfg.settledGrace,
		deadLetterMaxAge: cfg.deadLetterMaxAge, deadLetterMaxCount: cfg.deadLetterMaxCount,
		clock: c, closed: make(chan struct{}),
	}
	b.mirror = newMirror(conn.js, logName, deadName, entrylog.New(cfg.inventory), c.notify)
	if err := b.mirror.waitReady(ctx); err != nil {
		b.mirror.stop()
		return nil, fmt.Errorf("commandbus/nats: open: %w", err)
	}
	return b, nil
}

// limitOrUnset maps a zero cap to JetStream's "no limit".
func limitOrUnset(n int64) int64 {
	if n <= 0 {
		return -1
	}
	return n
}

// Close ends the background mirror watch. Not part of commandbus.Bus —
// production holds one Bus for the manager's whole life — but a test opening
// many isolated Bus values over one scope calls it to avoid leaking a goroutine
// and a subscription per scenario.
func (b *Bus) Close() {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	b.mirror.stop()
}

// advanceClock fast-forwards b's clock by d (claim.go), waking every blocked
// Claim at once rather than leaving it to the next poll. Unexported for the
// reason withClock is: only this package's own tests call it, through the
// conformance.Backing this package's test file builds.
func (b *Bus) advanceClock(d time.Duration) { b.clock.advance(d) }

// leaseDur reports the lease duration every Claim on b grants.
func (b *Bus) leaseDur() time.Duration { return b.leaseDuration }

// bucketName prefixes base with scope, as internal/state/nats's own bucketName
// does — the two packages share the idea (an install may point both at one
// server, design D-1) without sharing the code.
func bucketName(scope, base string) string {
	if scope == "" {
		return base
	}
	return scope + "_" + base
}

// bootstrapStream creates or updates a stream with direct gets turned off: a
// direct get may be served by any replica, including one that has not applied
// a write the mirror was just told about (gotchas.md's AllowDirect entry;
// internal/state/nats's own leaderReadsOnly is the same fix for the same
// reason, restated here rather than imported — design D-1 keeps the two
// backends independent). Retention is the limits policy and nothing else.
func bootstrapStream(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	cfg.Retention = jetstream.LimitsPolicy
	cfg.Storage = jetstream.FileStorage
	cfg.Discard = jetstream.DiscardOld
	cfg.AllowDirect = false
	s, err := js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: bootstrap stream %q: %w", cfg.Name, err)
	}
	return s, nil
}

// append writes ap's entry to its subject, conditional on the subject's last
// sequence still being the one the plan saw, and returns the sequence the log
// gave it. A rejected condition is isWrongLastSeq(err): the lost side of a
// race, for the caller to re-plan from a fresher view.
func (b *Bus) append(ctx context.Context, ap entrylog.Append) (uint64, error) {
	data, err := json.Marshal(ap.Entry)
	if err != nil {
		return 0, fmt.Errorf("commandbus/nats: encode %s entry of %s: %w", ap.Entry.Type, ap.Entry.UID, err)
	}
	ack, err := b.js.Publish(ctx, b.subj.forEntry(ap.Entry), data,
		jetstream.WithMsgID(msgID(ap.Entry)), jetstream.WithExpectLastSequencePerSubject(ap.ExpectLast))
	if err != nil {
		if isWrongLastSeq(err) {
			b.reconcileLast(ctx, ap.Entry)
		}
		return 0, err
	}
	b.bumpSelfSeq(ack.Sequence)
	return ack.Sequence, nil
}

// logInfo reads the queue log's current state from the server. It opens a
// fresh stream handle per call: a handle caches its info unguarded, so one
// shared across goroutines would race.
func (b *Bus) logInfo(ctx context.Context) (*jetstream.StreamInfo, error) {
	s, err := b.js.Stream(ctx, b.logName)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: log info: %w", err)
	}
	return s.CachedInfo(), nil
}

// reconcileLast re-learns the server's last sequence on the subject a rejected
// append was conditional on. The mirror's own belief goes stale in a way an
// ordered consumer cannot report: the log removes entries (a consumed task's,
// or those past its age limit) and a consumer is never told of a removal. After
// the mirror has caught up to what the server holds, the next plan is
// conditional on the truth, so a rejection is a lost race and never a standing
// one.
func (b *Bus) reconcileLast(ctx context.Context, e entrylog.Entry) {
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()
	var seq uint64
	last, err := b.logs.GetLastMsgForSubject(ctx, b.subj.forEntry(e))
	switch {
	case err == nil:
		seq = last.Sequence
	case !errors.Is(err, jetstream.ErrMsgNotFound):
		return
	}
	if err := b.mirror.waitCaughtUpTo(ctx, seq); err != nil {
		return
	}
	b.mirror.log.SetLast(e, seq)
}

// reconcileTimeout bounds reconcileLast: a rejected append is retried either
// way, so a reconcile that cannot finish quickly is simply skipped.
const reconcileTimeout = 2 * time.Second

// sync blocks until the mirror holds everything the log held when sync asked
// the server, so a decision read from it is never staler than the call. The
// target is the last entry the log still HOLDS, not its last sequence: the log
// removes the entries of consumed tasks, so its last sequence may name an entry
// no consumer will ever be delivered. A log holding nothing has nothing to
// catch up to.
func (b *Bus) sync(ctx context.Context) error {
	last, err := b.logs.GetLastMsgForSubject(ctx, b.subj.logFilter())
	switch {
	case errors.Is(err, jetstream.ErrMsgNotFound):
		return b.mirror.waitReady(ctx)
	case err != nil:
		return fmt.Errorf("commandbus/nats: last log entry: %w", err)
	}
	return b.mirror.waitCaughtUpTo(ctx, max(last.Sequence, b.selfSeq.Load()))
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
