package conformance

import (
	"context"
	"reflect"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// exercisedStoresFields is every field of state.Stores this file's RunStores
// drives through Run/RunExpiring/RunAppended/RunBlobs. Kept as its own
// value, not inlined, so testNothingLeftOut can walk state.Stores by
// reflection and name whichever field is missing from it — the "a kind of
// state cannot be left out" check design.md promises (D-1) and tasks.md 4.1
// asks for explicitly.
var exercisedStoresFields = map[string]bool{
	"Conversations": true,
	"Runs":          true,
	"Signals":       true,
	"Cooldowns":     true,
	"Receipts":      true,
	"AdapterState":  true,
	"Blobs":         true,
}

// RunStores is the suite's one entry point: every backend's own test is this
// one call. It walks the closed inventory field by field, each through the
// one generic engine its capability needs and the one Fixture written for
// it (fixtures.go), so a kind of state cannot be exercised twice and cannot
// be left out without testNothingLeftOut naming it.
func RunStores(t *testing.T, open func() state.Stores) {
	t.Helper()

	t.Run("Conversations", func(t *testing.T) {
		Run(t, requireField(t, open, "Conversations", func(s state.Stores) state.Collection[state.ConversationRecord] { return s.Conversations }),
			state.StoreDeclarations.Conversations, conversationFixture)
	})
	t.Run("Runs", func(t *testing.T) {
		seed := open()
		requireNonNil(t, "Conversations", seed.Conversations)
		requireNonNil(t, "Runs", seed.Runs)
		RunAppended(t, func() AppendedOpen[state.ConversationRecord, state.Run] {
			s := open()
			return AppendedOpen[state.ConversationRecord, state.Run]{Parent: s.Conversations, Items: s.Runs}
		}, runFixture)
	})
	t.Run("Signals", func(t *testing.T) {
		RunExpiring(t, requireField(t, open, "Signals", func(s state.Stores) state.Expiring[state.SignalRecord] { return s.Signals }),
			state.StoreDeclarations.Signals, signalFixture)
	})
	t.Run("Cooldowns", func(t *testing.T) {
		Run(t, requireField(t, open, "Cooldowns", func(s state.Stores) state.Collection[state.CooldownWindow] { return s.Cooldowns }),
			state.StoreDeclarations.Cooldowns, cooldownFixture)
	})
	t.Run("Receipts", func(t *testing.T) {
		Run(t, requireField(t, open, "Receipts", func(s state.Stores) state.Collection[state.Received] { return s.Receipts }),
			state.StoreDeclarations.Receipts, receiptFixture)
	})
	t.Run("AdapterState", func(t *testing.T) {
		Run(t, requireField(t, open, "AdapterState", func(s state.Stores) state.Collection[string] { return s.AdapterState }),
			state.StoreDeclarations.AdapterState, adapterStateFixture)
	})
	t.Run("Blobs", func(t *testing.T) {
		requireNonNil(t, "Blobs", open().Blobs)
		RunBlobs(t, func() state.Blobs { return open().Blobs })
	})

	t.Run("ContractOffersNoMultiRecordAtomicity", func(t *testing.T) { testNoMultiRecordAtomicity(t, open) })
	t.Run("NothingInTheInventoryIsLeftOut", func(t *testing.T) { testNothingLeftOut(t) })
}

// requireField checks field's nilness ONCE, synchronously, on t itself —
// never inside a closure Run/RunExpiring/RunAppended might later call from
// one of their own nested subtests, where a FailNow is refused as "called on
// a parent test". A backend's own wiring is static: once checked here, the
// plain accessor this returns never needs to check again.
func requireField[C any](t *testing.T, open func() state.Stores, field string, get func(state.Stores) C) func() C {
	t.Helper()
	requireNonNil(t, field, get(open()))
	return func() C { return get(open()) }
}

// requireNonNil fails, naming field, when v is a nil interface — go vet
// cannot compare a generic C to nil directly, so this goes through
// reflection once per open() call rather than constraining C to comparable.
func requireNonNil(t *testing.T, field string, v any) {
	t.Helper()
	if v == nil {
		t.Fatalf("state.Stores.%s is nil: open() does not wire it, so the suite cannot exercise it", field)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		if rv.IsNil() {
			t.Fatalf("state.Stores.%s is nil: open() does not wire it, so the suite cannot exercise it", field)
		}
	}
}

// testNoMultiRecordAtomicity is the "a crash between a conversation write
// and its input write" scenario: Collection carries no Begin/Commit (a
// compile-time fact of the interface shape, not something this test can
// assert at runtime), modelled by simply not making the second write yet and
// recovering from it later with nothing silently lost.
func testNoMultiRecordAtomicity(t *testing.T, open func() state.Stores) {
	ctx := context.Background()
	s := open()
	key := state.Key("crash-between-writes")
	if _, err := s.Conversations.Create(ctx, key, conversationFixture.New()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// "Crash" before the input payload's blob is written: the conversation
	// write already landed on its own, independent of the second write.
	got, _, err := s.Conversations.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after the conversation write alone: %v", err)
	}
	if got.ProfileRef != profileDefault {
		t.Fatalf("ProfileRef = %q — the conversation write must stand on its own", got.ProfileRef)
	}

	// The next reconciliation completes the owed input.
	blobKey := state.Key("crash-between-writes-input-1")
	if err := s.Blobs.Put(ctx, blobKey, state.Blob{ConversationName: string(key), Type: "task", Payload: owedInput}); err != nil {
		t.Fatalf("Put (completing the owed write): %v", err)
	}
	blob, err := s.Blobs.Get(ctx, blobKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if blob.Payload != owedInput {
		t.Fatalf("Payload = %q, want %q", blob.Payload, owedInput)
	}
}

// testNothingLeftOut fails, naming the field, when state.Stores carries one
// RunStores does not exercise — adding a kind without adding its scenario
// here must fail loudly rather than silently pass every existing backend.
func testNothingLeftOut(t *testing.T) {
	typ := reflect.TypeOf(state.Stores{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !exercisedStoresFields[name] {
			t.Fatalf("state.Stores.%s is not exercised by RunStores — add it to the generic suite before shipping it", name)
		}
	}
	if typ.NumField() != len(exercisedStoresFields) {
		t.Fatalf("RunStores exercises %d fields of state.Stores but it declares %d — a field was removed without removing its scenario here", len(exercisedStoresFields), typ.NumField())
	}
}
