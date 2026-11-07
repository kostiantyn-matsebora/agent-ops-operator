// Package state is the only way the manager reaches the state it writes at
// run time (design D-1 of the state-store-generic-contract change,
// openspec/changes/state-store-generic-contract/): conversations, their runs,
// signals, each source's cooldown window and receipt counters, adapter
// key-value state and queued-input blobs. No Kubernetes type, no selector
// string and no transaction crosses this boundary.
//
// The contract is a handful of capability interfaces generic over the record
// type (Collection, Expiring, Appended, Blobs) and one closed inventory,
// Stores, whose every field is typed by exactly the capabilities that kind of
// state has. A backend implements each capability once, for every record type;
// adding a kind of state is one field plus its declaration (Decl), never a new
// operation. internal/state/kube is the one backend shipped, and
// internal/state/conformance is the suite every backend must pass to exist.
package state

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Get, Update and Blobs.Get for a key that does not
// exist, or that exists but has passed its expiry.
var ErrNotFound = errors.New("state: record not found")

// ErrConflict is returned by Update when a write loses a compare-and-swap
// race. Distinguish it from any other failure with
// errors.Is(err, state.ErrConflict) — never by string-matching a backend's
// own error text.
var ErrConflict = errors.New("state: conflicting write")

// ErrAlreadyExists is returned by Create for a key that already has a
// record. Distinct from ErrConflict: a create names no revision to lose a
// race over, it simply refuses to create a second record under one key.
var ErrAlreadyExists = errors.New("state: record already exists")

// ErrDeleting is returned by Update for a write that adds a finalizer to a
// record already marked for deletion (record.go: Finalizers can only shrink
// on such a record). Nothing of the write is applied.
var ErrDeleting = errors.New("state: record is being deleted")

// ErrUnknownIndex is returned by List for an Index outside the collection's
// declared Indexes.
var ErrUnknownIndex = errors.New("state: unknown index")

// Index names one declared lookup over records of type T (design D-1: "a
// named extractor typed on the record"). It carries the rule that puts a
// record under a value, so a backend that evaluates in memory needs nothing
// else and one that serves the lookup natively (a label, a column) keys its
// own mapping on Name.
//
// The type parameter is the point: an Index[ConversationRecord] cannot be
// handed to a collection of signals, so an index of one kind is refused for
// another by the compiler. There is no open-ended query — adding an index is a
// declaration plus a conformance scenario.
type Index[T any] struct {
	name  string
	value func(*T) (string, bool)
}

// NewIndex declares an index. value returns the lookup value a record is
// listed under and whether it is listed at all: a record for which it reports
// false is absent from the index, which is how a "non-terminal" lookup never
// returns a terminal record. Compose a multi-part value with IndexValue.
func NewIndex[T any](name string, value func(*T) (string, bool)) Index[T] {
	return Index[T]{name: name, value: value}
}

// Name is the index's stable identifier, unique within one collection.
func (i Index[T]) Name() string { return i.name }

// Value returns the value rec is listed under and whether it is listed.
func (i Index[T]) Value(rec *T) (string, bool) {
	if i.value == nil {
		return "", false
	}
	return i.value(rec)
}

// indexValueSeparator joins the parts of a composite index value. A NUL
// cannot occur in a name, a phase or an initiation id, so two different
// part lists never produce one value.
const indexValueSeparator = "\x00"

// IndexValue composes the value of an index over several record fields, the
// one a caller of List passes for it (for example a source and an initiation
// id).
func IndexValue(parts ...string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += indexValueSeparator
		}
		out += p
	}
	return out
}

// EventType says what happened to a record on a Watch feed.
type EventType string

const (
	// EventPut is a create or an update — the feed does not distinguish
	// them, exactly as an informer's AddFunc/UpdateFunc collapse to one
	// reconcile request today.
	EventPut EventType = "put"
	// EventDelete is a Delete, or a record crossing its expiry.
	EventDelete EventType = "delete"
	// EventBookmark carries no change: only a Resume point, advanced past
	// whatever the backend has seen since its last event, so an idle feed's
	// resume point does not age out of the backend's history. Key and Record
	// are zero. A consumer records Resume and does nothing else with it.
	EventBookmark EventType = "bookmark"
)

// Event is one change on a Watch feed: "(key, revision, record)", plus the
// Type a consumer needs to tell a delete from a put and the Resume token for
// the point the feed has delivered through. Record is the zero value on
// EventDelete and EventBookmark.
type Event[T any] struct {
	Type     EventType
	Key      Key
	Revision Revision
	Record   T
	// Resume is this event's own Revision, offered back from Watch's resume
	// parameter to continue strictly after it. Equal to Revision on
	// EventPut; a backend still mints one on EventDelete so a consumer that
	// stops there has somewhere to resume from.
	Resume Revision
}

// WithItems is a record read together with its appended items, as they stood
// at the same instant: what GetWithItems returns, one element of
// ListWithItems, and the payload of a put on WatchWithItems.
type WithItems[P, T any] struct {
	Record   P
	Revision Revision
	Items    []T
}

// Decl is a kind of state's declaration (design D-1): everything a backend
// needs to serve it that is not an operation. It sits beside the Stores field
// it describes, so adding a kind is one field and one Decl and touches no
// backend primitive.
type Decl[T any] struct {
	// Name identifies the kind in the inventory and in a conformance report.
	Name string
	// Indexes is the collection's closed lookup set. List with an Index not
	// in it is ErrUnknownIndex. The conformance suite walks it.
	Indexes []Index[T]
	// Expiry reads a record's expiry instant, nil for none. Declared only by
	// an Expiring collection; a record past it is invisible to Get and List
	// and delivered as a delete on the feed.
	Expiry func(*T) *time.Time
	// ID reads an appended item's identity. Declared only by the item type of
	// an Appended collection.
	ID func(*T) string
	// Merge applies a grow-only edit to an existing appended item. Declared
	// only by the item type of an Appended collection. It MUST be idempotent
	// (merging an edit twice equals once) and commutative (two edits give one
	// result in either order), which the conformance suite proves, because
	// that property is what lets Grow take no revision and never conflict.
	Merge func(item *T, edit T)
}

// Declares reports whether idx is one of d's declared indexes.
func (d Decl[T]) Declares(idx Index[T]) bool {
	for _, have := range d.Indexes {
		if have.Name() == idx.Name() {
			return true
		}
	}
	return false
}

// Collection is the base capability: records of type T keyed by Key, written
// under compare-and-swap, listed by declared index and watched. A Collection
// is bound to one install's scope at construction, so no method takes a
// namespace and none takes a selector string.
type Collection[T any] interface {
	// Get returns the current record and its revision, or ErrNotFound.
	Get(ctx context.Context, key Key) (T, Revision, error)

	// Create writes a new record and returns its revision. ErrAlreadyExists
	// when key already names a record — including one marked for deletion
	// and still held by a finalizer: a create carries no revision to lose a
	// race over, it is refused outright. Identity and timestamps the record
	// type carries as backend-minted (ID, CreatedAt, DeletionRequestedAt) are
	// minted here, whatever rec holds.
	Create(ctx context.Context, key Key, rec T) (Revision, error)

	// Update writes rec in place of the record at rev, returning the new
	// revision. ErrConflict when rev is not the record's current revision —
	// the caller's own get-modify-put raced another writer and must Get again
	// (Mutate does). ErrNotFound when the key is absent. ErrDeleting when rec
	// adds a finalizer to a record marked for deletion. Updating a marked
	// record's last finalizer away removes the record (see Delete).
	Update(ctx context.Context, key Key, rev Revision, rec T) (Revision, error)

	// Delete requests a record's removal. A record holding no finalizer is
	// removed at once and the feed carries EventDelete. A record holding one
	// stays readable with its deletion time set, the feed carries an EventPut
	// for the marking, and the record is removed by the Update that leaves
	// its finalizers empty. A second Delete of a marked record changes
	// nothing. Not an error when the key is already absent.
	Delete(ctx context.Context, key Key) error

	// List returns every visible record that idx lists under value (compose
	// a multi-part value with IndexValue; "" for an index over the whole
	// set). Order is unspecified. ErrUnknownIndex for an index the
	// collection's Decl does not declare.
	List(ctx context.Context, idx Index[T], value string) ([]T, error)

	// Watch delivers an at-least-once feed of Event, resuming strictly after
	// resume (empty Revision starts from now). The channel closes when ctx is
	// done; a backend reports a terminal failure by closing the channel
	// early, with no error value crossing this contract.
	Watch(ctx context.Context, resume Revision) (<-chan Event[T], error)
}

// Expiring is a Collection whose records carry an expiry. A record past it is
// invisible to Get and List and delivered as a delete on the feed, so a
// terminal record removes itself and nothing deletes it by hand. Only a kind
// whose field is typed Expiring has this; a Conversation never expires.
type Expiring[T any] interface {
	Collection[T]

	// Expiry returns the instant after which rec is invisible, nil for none.
	// It is the Decl's own extractor, offered so a consumer reads the same
	// instant the store applies.
	Expiry(rec T) *time.Time
}

// Appended is an append-only sub-collection of items of type T hanging off a
// parent record of type P, keyed by the parent's Key. It does not embed
// Collection: an item is never created, replaced or deleted on its own, only
// appended and grown, and it lives and dies with its parent.
type Appended[P, T any] interface {
	// Append adds item to parent's history. It takes no revision: appends
	// commute, and a backend meets this with its own internal retry rather
	// than surfacing a conflict. ErrNotFound when parent does not exist.
	Append(ctx context.Context, parent Key, item T) error

	// List returns parent's items in append order. A parent with none, and
	// one that does not exist, list empty.
	List(ctx context.Context, parent Key) ([]T, error)

	// Grow applies edit to the item with identity id by the Decl's Merge. It
	// takes no revision and never conflicts, because the merge is idempotent
	// and commutative: applying an edit already applied changes nothing, and
	// edits commute with each other and with Append. ErrNotFound when the
	// parent or the item does not exist, so a caller whose item aged out of a
	// bounded history can tell it from success.
	Grow(ctx context.Context, parent Key, id string, edit T) error

	// GetWithItems is the parent's Get and List in ONE read: the record, its
	// revision and its items as they stood at the same instant, or
	// ErrNotFound by Get's rule.
	GetWithItems(ctx context.Context, parent Key) (P, Revision, []T, error)

	// ListWithItems lists the parent collection by one of its declared
	// indexes, each record with exactly the items GetWithItems returns for
	// it. How a backend joins the items to the records is its own.
	ListWithItems(ctx context.Context, idx Index[P], value string) ([]WithItems[P, T], error)

	// WatchWithItems is the parent's feed with each put carrying the
	// record's items as of that revision. Semantics as Collection.Watch.
	WatchWithItems(ctx context.Context, resume Revision) (<-chan Event[WithItems[P, T]], error)
}

// Blobs stores the out-of-line body of a queued input, keyed by Key.
type Blobs interface {
	// Put stores blob under key, replacing any value already there.
	Put(ctx context.Context, key Key, blob Blob) error

	// Get returns the blob stored under key, or ErrNotFound.
	Get(ctx context.Context, key Key) (Blob, error)

	// Delete removes a blob. Not an error when the key is already absent.
	Delete(ctx context.Context, key Key) error
}
