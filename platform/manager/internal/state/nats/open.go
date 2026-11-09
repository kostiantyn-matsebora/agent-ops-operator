package nats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The bucket/object-store names of the closed inventory (design D-1), before
// Scope's own prefix (bucketName): one per kind, so per-kind history, limits
// and replica count stay independent.
const (
	bucketConversations = "conversations"
	bucketRuns          = "runs"
	bucketSignals       = "signals"
	bucketCooldowns     = "cooldowns"
	bucketReceipts      = "receipts"
	bucketAdapterState  = "adapterstate"
	objectStoreBlobs    = "blobs"
)

// Option adjusts how OpenStores opens a collection.
type Option func(*openConfig)

type openConfig struct {
	sweepInterval time.Duration
	// orphanInterval is the period of the sweep that removes what no
	// conversation owns (cascade.go).
	orphanInterval time.Duration
}

// WithSweepInterval overrides the expiry sweep's period, for a test that must
// observe a sweep inside its own timeout.
func WithSweepInterval(d time.Duration) Option {
	return func(c *openConfig) { c.sweepInterval = d }
}

func newOpenConfig(opts []Option) openConfig {
	cfg := openConfig{sweepInterval: DefaultSweepInterval, orphanInterval: DefaultOrphanSweepInterval}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// bucketName prefixes base with scope — the install-isolation boundary this
// backend carries by bucket name, where kube carries it by namespace (design
// D-2 of state-store-generic-contract: "scope fixed at construction").
func bucketName(scope, base string) string {
	if scope == "" {
		return base
	}
	return scope + "_" + base
}

// kvStreamName is the stream name NATS itself gives a KV bucket's backing
// stream ("KV_<bucket>", nats.go's own kvBucketNamePre convention) — the one
// place this backend reaches past the KeyValue handle to the raw stream, for
// List's own fence (collection.go's latestSeq) and Watch's ordered consumer
// (rawwatch.go).
func kvStreamName(bucket string) string { return "KV_" + bucket }

// Backend is the nats backend as the manager wires it: the closed inventory
// over one Conn, every bucket and the object store created with the
// configured replica count, and the expiry sweep the Signals field needs
// running beside the manager — the nats analogue of kube.Backend.
type Backend struct {
	// Stores is the inventory, every field implemented by this backend.
	Stores state.Stores

	conn  *Conn
	scope string

	// The concrete handles the one-time import (import.go) needs beyond
	// what the state.Stores interface exposes: the raw adapterstate bucket
	// the marker lives in, and typed access to each collection's own
	// raw-envelope write path.
	adapterStateKV jetstream.KeyValue
	conversations  *collection[state.ConversationRecord]
	signals        *collection[state.SignalRecord]
	cooldowns      *collection[state.CooldownWindow]
	receipts       *collection[state.Received]
	adapterState   *collection[string]
	runsKV         jetstream.KeyValue
	blobsStore     jetstream.ObjectStore

	sweeps []interface{ Run(context.Context) }

	// owned removes what a conversation owns outside its record (cascade.go).
	owned *children
}

// OpenStores bootstraps every bucket and the object store of the closed
// inventory, scoped by scope, and opens every field of state.Stores over
// them. The sweep is not started: Run does that, once the caller has
// somewhere to run it (mirroring kube.OpenStores).
func OpenStores(ctx context.Context, conn *Conn, scope string, opts ...Option) (_ *Backend, err error) {
	cfg := newOpenConfig(opts)
	replicas := conn.cfg.replicas()
	b := &Backend{conn: conn, scope: scope}

	// Each bucket's mirror starts its watch the moment its collection is built.
	// A later bucket failing must not leave the earlier ones running: a caller
	// that retries would pile up one set per attempt.
	var stops []func()
	defer func() {
		if err != nil {
			for _, stop := range stops {
				stop()
			}
		}
	}()

	convKV, convBucket, err := b.bootstrapKV(ctx, bucketConversations, replicas)
	if err != nil {
		return nil, err
	}
	conversations := newCollection[state.ConversationRecord](conn.js, convKV, convBucket, kvStreamName(convBucket), ConversationsLayout())
	stops = append(stops, conversations.mirror.stop)

	runsKV, runsBucket, err := b.bootstrapKV(ctx, bucketRuns, replicas)
	if err != nil {
		return nil, err
	}
	runsAppended, err := newAppended[state.ConversationRecord, state.Run](
		conversations, func(r *state.ConversationRecord) state.Key { return state.Key(r.Name) },
		conn.js, runsKV, runsBucket, state.StoreDeclarations.Runs,
	)
	if err != nil {
		return nil, err
	}

	runsAppended.limit = state.RunHistoryLimit

	signalsKV, signalsBucket, err := b.bootstrapKV(ctx, bucketSignals, replicas)
	if err != nil {
		return nil, err
	}
	signalsColl := newCollection[state.SignalRecord](conn.js, signalsKV, signalsBucket, kvStreamName(signalsBucket), SignalsLayout())
	stops = append(stops, signalsColl.mirror.stop)
	signalsExpiring, err := newExpiring(signalsColl, cfg.sweepInterval)
	if err != nil {
		return nil, err
	}

	cooldownsKV, cooldownsBucket, err := b.bootstrapKV(ctx, bucketCooldowns, replicas)
	if err != nil {
		return nil, err
	}
	cooldowns := newCollection[state.CooldownWindow](conn.js, cooldownsKV, cooldownsBucket, kvStreamName(cooldownsBucket), CooldownsLayout())
	stops = append(stops, cooldowns.mirror.stop)

	receiptsKV, receiptsBucket, err := b.bootstrapKV(ctx, bucketReceipts, replicas)
	if err != nil {
		return nil, err
	}
	receipts := newCollection[state.Received](conn.js, receiptsKV, receiptsBucket, kvStreamName(receiptsBucket), ReceiptsLayout())
	stops = append(stops, receipts.mirror.stop)

	adapterStateKV, adapterStateBucket, err := b.bootstrapKV(ctx, bucketAdapterState, replicas)
	if err != nil {
		return nil, err
	}
	adapterState := newCollection[string](conn.js, adapterStateKV, adapterStateBucket, kvStreamName(adapterStateBucket), AdapterStateLayout())
	stops = append(stops, adapterState.mirror.stop)

	blobsBucket := bucketName(scope, objectStoreBlobs)
	blobsStore, err := conn.js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: blobsBucket, Replicas: replicas})
	if err != nil {
		return nil, fmt.Errorf("nats: bootstrap object store %q: %w", blobsBucket, err)
	}

	b.Stores = state.Stores{
		Conversations: conversations,
		Runs:          runsAppended,
		Signals:       signalsExpiring,
		Cooldowns:     cooldowns,
		Receipts:      receipts,
		AdapterState:  adapterState,
		Blobs:         newBlobs(blobsStore),
	}
	b.adapterStateKV, b.conversations, b.signals, b.cooldowns, b.receipts, b.adapterState = adapterStateKV, conversations, signalsColl, cooldowns, receipts, adapterState
	b.runsKV, b.blobsStore = runsKV, blobsStore
	owned := &children{conversations: conversations, runsKV: runsKV, blobs: blobsStore, now: time.Now}
	b.owned = owned
	conversations.onRemoved = owned.removed
	b.sweeps = []interface{ Run(context.Context) }{signalsExpiring, sweepEvery{owned, cfg.orphanInterval}}
	return b, nil
}

// bucketHistory is every bucket's per-key history depth, the NATS server's
// own ceiling (KeyValueConfig.History: "Max is 64"). A KV bucket enforces
// this as MaxMsgsPerSubject on its backing stream, so History: 1 would
// PURGE a key's own earlier revisions the instant a newer one lands — not
// merely stop serving them — and Watch's resumable ordered consumer
// (rawwatch.go) replays exactly those revisions to close a gap across a
// disconnect (design D-2's "no gap"). The ceiling bounds, never eliminates,
// how long a resume point may age before it falls outside what the bucket
// still holds; design D-2's bookmark exists for precisely that edge, and a
// resume older than this depth is the same "outage before a loss" the
// manager's storage breaker already answers for an unreachable server.
const bucketHistory = 64

// bootstrapKV creates or opens a scoped KV bucket and returns its scoped
// name alongside the handle.
func (b *Backend) bootstrapKV(ctx context.Context, base string, replicas int) (jetstream.KeyValue, string, error) {
	name := bucketName(b.scope, base)
	kv, err := b.conn.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: name, Replicas: replicas, History: bucketHistory})
	if err != nil {
		return nil, "", fmt.Errorf("nats: bootstrap bucket %q: %w", name, err)
	}
	return kv, name, nil
}

// Run runs every expiry sweep until ctx is done.
func (b *Backend) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, s := range b.sweeps {
		wg.Add(1)
		s := s
		go func() {
			defer wg.Done()
			s.Run(ctx)
		}()
	}
	wg.Wait()
}

// Close ends every collection's background mirror watch. Not part of
// production's lifecycle (a Backend lives for the manager's whole life) —
// a test opening many isolated instances calls it to avoid leaking one
// goroutine and subscription per scenario.
func (b *Backend) Close() {
	b.conversations.mirror.stop()
	b.signals.mirror.stop()
	b.cooldowns.mirror.stop()
	b.receipts.mirror.stop()
	b.adapterState.mirror.stop()
}

// DefaultOrphanSweepInterval is how often the orphan sweep looks for runs and
// blobs whose conversation is gone.
const DefaultOrphanSweepInterval = 5 * time.Minute

// WithOrphanSweepInterval overrides the orphan sweep's period, for a test that
// cannot wait minutes.
func WithOrphanSweepInterval(d time.Duration) Option {
	return func(c *openConfig) { c.orphanInterval = d }
}

// sweepEvery adapts children's interval-taking Run to the one-argument shape
// Backend.Run starts.
type sweepEvery struct {
	c        *children
	interval time.Duration
}

func (s sweepEvery) Run(ctx context.Context) { s.c.Run(ctx, s.interval) }

// OpenStoresWhenReady is OpenStores that waits for the server instead of
// failing on the first refusal: a JetStream cluster that has not yet formed a
// quorum cannot place a replicated bucket, and the manager that gives up and
// exits turns a few seconds of startup into a crash loop. It retries every
// interval until ctx ends, then returns the last refusal.
func OpenStoresWhenReady(ctx context.Context, conn *Conn, scope string, interval time.Duration, opts ...Option) (*Backend, error) {
	if interval <= 0 {
		interval = DefaultOpenRetryInterval
	}
	for {
		b, err := OpenStores(ctx, conn, scope, opts...)
		if err == nil {
			return b, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w (last attempt: %v)", ctx.Err(), err)
		case <-time.After(interval):
		}
	}
}

// DefaultOpenRetryInterval is how long OpenStoresWhenReady waits between attempts.
const DefaultOpenRetryInterval = 3 * time.Second
