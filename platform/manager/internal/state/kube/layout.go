// Package kube is the "kube" backend of the state contract (design D-1 of the
// state-store-generic-contract change): the manager's state kept on the custom
// resources it always lived on, behind the generic capabilities of
// internal/state, byte for byte.
//
// A backend implements each capability once, for every record type. What it
// needs to know about a kind of state is a Layout: the object kind, the record
// mapping both ways, whether a status subresource is written, the lookups the
// object serves cheaply, and where an expiry and an owner link live. Open,
// OpenExpiring, OpenAppended and OpenBlobs take a Layout and return the
// capability; the layouts of the closed inventory are in layouts.go, and no
// other file names a kind.
//
// It depends on nothing but client.WithWatch, with no ctrl.Manager and no
// cache.Cache, so it is constructable in a test (client.NewWithWatch against
// envtest). Every List and Watch is scoped with client.InNamespace, and Get,
// Update and Delete read straight from the apiserver through the client, the
// way the manager's HTTP handlers do (design D-4 of the state-store change).
package kube

import (
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// DefaultSweepInterval is how often an expiring collection sweeps the
// namespace for expired records when the caller takes the default.
const DefaultSweepInterval = 30 * time.Second

// statusUpdateMaxAttempts bounds every retry loop on the apiserver's own
// optimistic-concurrency conflict: a status write that lost to a commuting
// writer, an append, a grow, a part write.
const statusUpdateMaxAttempts = 20

// Layout maps a record type onto the object that holds it. A record is either
// the whole of an object this backend creates and deletes (Part nil: a
// Conversation, a Signal), or one part of an object that exists regardless
// (Part set: the window on a SignalSource, an annotation on a Channel).
type Layout[T any] struct {
	// Decl is the kind's declaration from the inventory: its name, indexes and
	// expiry. The backend takes every per-kind fact from it.
	Decl state.Decl[T]

	// New and NewList build an empty object and an empty list of the kind.
	New     func() client.Object
	NewList func() client.ObjectList

	// Read maps the object to the record it carries. ok is false when the
	// object holds no record of this kind (a SignalSource with no window).
	// It sets the backend-minted fields (ID, CreatedAt, DeletionRequestedAt)
	// from the object and reads an expiry back from its annotation.
	Read func(obj client.Object) (rec T, ok bool)

	// Init writes everything of a new object a record decides: publication
	// fields, the owner link, the lookup labels, the expiry annotation. Used
	// by Create only.
	Init func(obj client.Object, rec T)

	// WriteMain writes the mutable spec and metadata of an update: the fields
	// the main object endpoint accepts, the lookup labels, the expiry
	// annotation. It leaves every piece of metadata the record does not model
	// as the apiserver holds it.
	WriteMain func(obj client.Object, rec T)

	// WriteStatus writes the record's status-shaped fields. Nil means the kind
	// has no status subresource. It never writes the items of an Appended
	// kind, which no Update may rewrite.
	WriteStatus func(obj client.Object, rec T)

	// StatusFirst orders an update's two calls. False writes the main object
	// first, whose resourceVersion is the compare-and-swap, then the status
	// (a Conversation). True writes the status first and settles the
	// metadata after (a Signal, whose expiry must never be set ahead of the
	// terminal phase it follows).
	StatusFirst bool

	// StatusDrift reports whether the status changed between base and current
	// in a field the retry of a status write must not overwrite. Required
	// when WriteStatus is set and StatusFirst is not: it is what separates a
	// commuting writer (an append) from a competing one.
	StatusDrift func(base, current client.Object) bool

	// Finalizers reads the record's finalizer set. Nil means the kind holds
	// none and Delete removes at once.
	Finalizers func(rec T) []string

	// Prefilter answers an index's lookup from the object alone, before the
	// record is built, so a list of a large install maps only the matches. It
	// is keyed by index name and is an optimisation: the index's own Value
	// still decides.
	Prefilter map[string]func(obj client.Object, value string) bool

	// Part, when set, places the record on an object this backend neither
	// creates nor deletes.
	Part *Part[T]
}

// Part places a record on a part of an existing object: an annotation, or a
// field of its status. Create, Update and Delete are merge patches of that
// part, and the revision is the object's.
type Part[T any] struct {
	// Locate maps a key to the object that carries it and the part of that
	// object it names (an annotation key; empty when the object has one
	// record). A malformed key is an error.
	Locate func(key state.Key) (object, field string, err error)
	// Read maps the part to a record; ok is false when the part is empty.
	Read func(obj client.Object, field string) (rec T, ok bool)
	// Write sets the part to the record.
	Write func(obj client.Object, field string, rec T)
	// Clear empties the part.
	Clear func(obj client.Object, field string)
	// Enumerate lists every record an object carries with its key, which is
	// what the feed announces.
	Enumerate func(obj client.Object) []Entry[T]
	// Status writes the part through the status subresource.
	Status bool
}

// Entry is one record an object carries and its key.
type Entry[T any] struct {
	Key    state.Key
	Record T
}

// AppendedLayout maps the items of an Appended kind onto their parent's object
// (Conversation.status.runs[] for runs).
type AppendedLayout[P, T any] struct {
	// Parent is the parent kind's layout. The items live on its object.
	Parent Layout[P]
	// Items is the item kind's declaration: its identity and its grow-only
	// merge.
	Items state.Decl[T]
	// ReadItems maps the object's items, in append order.
	ReadItems func(obj client.Object) []T
	// WriteItems replaces the object's items.
	WriteItems func(obj client.Object, items []T)
	// MaxItems bounds the stored history, the oldest falling off. Zero means
	// unbounded.
	MaxItems int
}

// config is what the options set.
type config struct {
	lister        client.Reader
	sweepInterval time.Duration
}

// Option adjusts how a collection is opened.
type Option func(*config)

// WithCachedLists makes List answer from r, normally the manager's informer
// cache, instead of listing the namespace from the apiserver. Get, Create,
// Update and Delete stay uncached: they carry the compare-and-swap.
//
// A list is a lookup, not a precondition. The caller of a lookup that goes on
// to write re-reads the record it is about to change, and a write that raced a
// stale list loses on its revision. Serving it from memory is what the manager
// did before the state store, and a close that walks the child lookup twice
// per conversation cannot afford a 6 MB apiserver list each time (design D-4
// of the state-store change, amended).
func WithCachedLists(r client.Reader) Option {
	return func(c *config) { c.lister = r }
}

// WithSweepInterval overrides the expiry sweep's period, for a test that must
// observe a sweep inside its own timeout.
func WithSweepInterval(d time.Duration) Option {
	return func(c *config) { c.sweepInterval = d }
}

func newConfig(opts []Option) config {
	cfg := config{sweepInterval: DefaultSweepInterval}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// validate refuses a layout that cannot serve its kind, naming the kind.
func (l Layout[T]) validate() error {
	name := l.Decl.Name
	if name == "" {
		return fmt.Errorf("kube: layout has no declaration name")
	}
	if l.New == nil || l.NewList == nil {
		return fmt.Errorf("kube: layout %q lacks New or NewList", name)
	}
	if l.Part != nil {
		if l.Part.Locate == nil || l.Part.Read == nil || l.Part.Write == nil || l.Part.Clear == nil || l.Part.Enumerate == nil {
			return fmt.Errorf("kube: layout %q: a part needs Locate, Read, Write, Clear and Enumerate", name)
		}
		return nil
	}
	if l.Read == nil || l.Init == nil || l.WriteMain == nil {
		return fmt.Errorf("kube: layout %q lacks Read, Init or WriteMain", name)
	}
	if l.StatusFirst && l.WriteStatus == nil {
		return fmt.Errorf("kube: layout %q orders status first but writes none", name)
	}
	if l.WriteStatus != nil && !l.StatusFirst && l.StatusDrift == nil {
		return fmt.Errorf("kube: layout %q writes status after the main object and needs StatusDrift", name)
	}
	return nil
}
