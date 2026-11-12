// Package memory is the commandbus.Bus backend for tests and local
// development (design D-1 of openspec/changes/async-command-bus/) — never
// selected by a production install, which runs commandbus/nats instead.
//
// It is the same append-only model the nats backend runs (design D-2): the
// shared entrylog.Log plans every write, and this package only gives each
// entry the next sequence under one mutex. Nothing is ever rewritten or
// removed, and the log is uncapped for as long as the Backing lives.
//
// A Backing is one isolated store; Bus opens an independent handle onto it.
// Two handles from one Backing see each other's writes at once, which is
// how this package models what a real cross-process backend gives for
// free: a claim made through one handle completes through another, because
// Settle and Fail check only the lease token Claim returned, never which
// handle is asking (design D-2).
package memory

import (
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// defaultLeaseDuration matches internal/chat's own OpQueue.ReclaimAfter —
// no design value is named for commandbus specifically, and reusing the
// existing number is a smaller surprise than inventing a second one.
const defaultLeaseDuration = 5 * time.Minute

// defaultPollInterval bounds how long Claim ever sleeps with nothing
// scheduled before rechecking — a safety net behind the wake channel, not
// the normal wake path. Memory being test/dev-only, a short real-time poll
// costs nothing a production backend would ever pay.
const defaultPollInterval = 200 * time.Millisecond

// Option configures a Backing at construction.
type Option func(*store)

// WithClock overrides the store's base clock (default time.Now) — a fixed
// instant for a test that wants reproducible timestamps. Backing.Advance
// fast-forwards on top of whichever base clock is in use, in either case.
func WithClock(base func() time.Time) Option {
	return func(s *store) { s.baseClock = base }
}

// WithInventory overrides the queue declarations a store resolves Submit,
// Fail and Stats against. Default commandbus.DefaultInventory().
func WithInventory(inv []commandbus.Declaration) Option {
	return func(s *store) { s.inventory = inv }
}

// WithLeaseDuration overrides how long a Claim's lease runs before the
// store treats it as lapsed.
func WithLeaseDuration(d time.Duration) Option {
	return func(s *store) { s.leaseDuration = d }
}

// Backing is memory's shared backing store. It holds every task and the
// whole event feed for as long as the process keeps it, which is the one
// place this backend diverges from a production one on purpose: it is
// declared for tests and local development only, so there is no retention
// cap to get wrong.
type Backing struct {
	s *store
}

// New opens a Backing with a fresh, empty store.
func New(opts ...Option) *Backing {
	s := &store{
		baseClock:     time.Now,
		inventory:     commandbus.DefaultInventory(),
		leaseDuration: defaultLeaseDuration,
		wake:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.log = entrylog.New(s.inventory)
	return &Backing{s: s}
}

// Bus opens a new, independent handle onto b's store. Calling it more than
// once is how commandbus/conformance proves a claim completes through a
// handle other than the one that claimed it.
func (b *Backing) Bus() commandbus.Bus { return &handle{s: b.s} }

// Advance fast-forwards b's clock by d (see WithClock). Local development
// never needs it — wall-clock time already advances on its own — it exists
// for a deterministic test to skip past a backoff, a lease or a TTL.
func (b *Backing) Advance(d time.Duration) { b.s.advance(d) }

// LeaseDuration reports how long every Claim's lease runs before this
// Backing treats it as lapsed — the default, or whatever WithLeaseDuration
// set. A test advances past exactly this rather than an arbitrary guess.
func (b *Backing) LeaseDuration() time.Duration { return b.s.leaseDur() }

// Compact does nothing: this backend removes a consumed task's entries inside
// Settle.
func (b *Backing) Compact() {}

// Entries reads back every entry appended so far, in order. The conformance
// suite uses it to prove no entry is ever rewritten or written twice.
func (b *Backing) Entries() []entrylog.Entry { return b.s.entriesSnapshot() }

// Open is the single-handle convenience most callers want: a fresh store
// and its one handle, in one call.
func Open(opts ...Option) commandbus.Bus { return New(opts...).Bus() }

// handle is one connection onto a shared store. It carries no state of its
// own — every call reaches straight into the store under its own lock —
// which is the point: nothing about which handle claimed a task is needed
// to settle or fail it.
type handle struct{ s *store }

var _ commandbus.Bus = (*handle)(nil)
