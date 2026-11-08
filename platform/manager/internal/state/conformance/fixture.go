package conformance

import (
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Keyed pairs a record with the key it is created under, the shape a pool of
// pre-built fixtures needs: Create takes a Key and a T separately, and
// nothing about T alone says which key it belongs under.
type Keyed[T any] struct {
	Key    state.Key
	Record T
}

// HoldFixture is supplied only for a kind whose record can carry a
// finalizer (ConversationRecord alone, today). Its absence (a nil
// *HoldFixture on Fixture) skips the held-record scenarios, since nothing
// can build one for a kind with no such notion — Signals, Cooldowns,
// Receipts and AdapterState hold nothing to release.
type HoldFixture[T any] struct {
	// Hold returns a copy of rec carrying exactly one finalizer.
	Hold func(rec T) T
	// AddSecond returns a copy of rec carrying a second finalizer alongside
	// whatever it already holds.
	AddSecond func(rec T) T
	// Release returns a copy of rec with every finalizer cleared.
	Release func(rec T) T
	// NumFinalizers counts rec's held finalizers.
	NumFinalizers func(rec T) int
	// Marked reports whether rec is marked for deletion (its
	// DeletionRequestedAt-equivalent is set).
	Marked func(rec T) bool
}

// IdentityFixture is supplied only for a kind that mints its own identity
// back to the caller (Conversations and Signals). Its absence skips the
// identity scenarios for a plain value like a CooldownWindow, which carries
// none.
type IdentityFixture[T any] struct {
	// ID reads the backend-minted identity.
	ID func(rec T) string
	// CreatedAt reads the backend-minted arrival instant.
	CreatedAt func(rec T) time.Time
	// Forge returns a copy of rec carrying an obviously fake ID and
	// CreatedAt, to prove Create and Update ignore caller-supplied identity.
	Forge func(rec T) T
}

// Fixture is one kind's own test data and hooks for the generic suite,
// written once per field of state.Stores (fixtures.go) and reused by every
// backend: Run, RunExpiring and RunAppended take every generic operation
// from the capability interfaces of internal/state and only this value from
// the kind, so the suite's own body never names a concrete field of a
// concrete record type.
type Fixture[T any] struct {
	// New returns a minimal, valid, zero-marker record — the fixture every
	// CAS, feed, deletion and expiry scenario starts from, which care about
	// the record's identity, never its content.
	New func() T

	// Edit returns a copy of rec with its one observable marker field set to
	// marker, read back by Marker. Every write scenario proves a write
	// landed, or was refused, through this one field alone.
	Edit func(rec T, marker int) T
	// Marker reads the field Edit sets, from a record that may carry fields
	// New never set (a minted identity) — so a scenario proves a write's
	// effect, never a whole-record equality a minted field would break.
	Marker func(rec T) int

	// Key turns a scenario's own label into a valid key for this kind: the
	// identity function for a kind keyed by its own name, and a composed key
	// for one keyed through another object (AdapterState's surface/name/key,
	// a Cooldown or a Receipt's owning source).
	Key func(label string) state.Key

	// KeyOf reads the key a record returned by List belongs under, needed
	// only when Indexes is non-empty: a Collection declaring no index never
	// calls it.
	KeyOf func(rec T) state.Key
	// Indexed is a pool of records, each under its own key, built so every
	// index of the kind's own Decl.Indexes partitions it into at least one
	// matching group and at least one excluded record — nil for a kind that
	// declares none.
	Indexed []Keyed[T]

	Hold     *HoldFixture[T]
	Identity *IdentityFixture[T]
}
