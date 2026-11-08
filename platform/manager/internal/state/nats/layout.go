package nats

import (
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Layout is what this backend needs to know about a kind that is not an
// operation (the nats analogue of kube.Layout, much smaller: there is no
// object/status split, only one JSON value per key).
//
// Identity and Hold are nil for a kind whose record models neither — exactly
// the kinds conformance's own Fixture leaves Identity/Hold nil for
// (CooldownWindow, Received, AdapterState's plain string): the envelope
// still carries id/createdAt/finalizers/deletionRequestedAt (design D-3),
// they are simply never copied onto a record with no field to hold them.
type Layout[T any] struct {
	Decl     state.Decl[T]
	Identity *IdentityLayout[T]
	Hold     *HoldLayout[T]
	// SetName writes the record's own Name field to the key it is stored
	// under (ConversationRecord.Name, SignalRecord.Name) — the record's own
	// name IS its key, by the same convention state.Stores' own fixtures
	// read it back by (Fixture.KeyOf: "state.Key(rec.Name)"). Nil for a kind
	// with no such field (CooldownWindow, Received, AdapterState's string).
	SetName func(rec *T, name string)
}

// IdentityLayout reads and writes the backend-minted identity fields a
// record type models directly (ConversationRecord.ID/CreatedAt,
// SignalRecord.ID/CreatedAt) — the generic equivalent of kube's per-kind
// Read/Init closures, narrowed to the one thing this backend cannot infer
// generically.
type IdentityLayout[T any] struct {
	Get func(rec *T) (id string, createdAt time.Time)
	Set func(rec *T, id string, createdAt time.Time)
}

// HoldLayout reads and writes the caller-owned finalizer set and the
// backend-minted deletion marker a record type models directly
// (ConversationRecord.Finalizers/DeletionRequestedAt). Nil for a kind with
// no notion of either — the kube backend's own Decl.Finalizers plus a typed
// DeletionRequestedAt field, generalised the same way.
type HoldLayout[T any] struct {
	Finalizers             func(rec *T) []string
	SetFinalizers          func(rec *T, fs []string)
	DeletionRequestedAt    func(rec *T) *time.Time
	SetDeletionRequestedAt func(rec *T, at *time.Time)
}

// clear removes every backend-minted/caller-owned field this Layout models
// from rec before it is marshalled, so the stored JSON carries them exactly
// once, at the envelope's own top level (design D-3), never duplicated
// inside "record" too.
func (l Layout[T]) clear(rec *T) {
	if l.Identity != nil {
		l.Identity.Set(rec, "", time.Time{})
	}
	if l.Hold != nil {
		l.Hold.SetFinalizers(rec, nil)
		l.Hold.SetDeletionRequestedAt(rec, nil)
	}
}

// apply writes the envelope's own identity/hold fields onto rec, the inverse
// of clear, done once on every read before a record reaches a caller.
func (l Layout[T]) apply(rec *T, e *envelope[T]) {
	if l.Identity != nil {
		l.Identity.Set(rec, e.ID, e.CreatedAt)
	}
	if l.Hold != nil {
		l.Hold.SetFinalizers(rec, e.Finalizers)
		l.Hold.SetDeletionRequestedAt(rec, e.DeletionRequestedAt)
	}
}

// setName writes rec's own Name field to key, a no-op for a kind with none.
func (l Layout[T]) setName(rec *T, key state.Key) {
	if l.SetName != nil {
		l.SetName(rec, string(key))
	}
}

// finalizersOf reads the finalizers a caller's rec carries, "" (nil) for a
// kind with no notion of one.
func (l Layout[T]) finalizersOf(rec *T) []string {
	if l.Hold == nil {
		return nil
	}
	return l.Hold.Finalizers(rec)
}
