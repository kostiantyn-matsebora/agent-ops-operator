package conformance

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// fakeConvCollection is the minimal state.Collection[state.ConversationRecord]
// testCreateReturnsStoredRecord needs: Create mints an ID and CreatedAt the
// way a real backend does and remembers what it stored, Get answers from
// that memory. Every wrapper below embeds a *fakeConvCollection and overrides
// exactly the one method a scenario needs to misbehave, so the rest of the
// contract (a real Create/Get pair) stays genuine.
type fakeConvCollection struct {
	state.Collection[state.ConversationRecord]
	byKey map[state.Key]state.ConversationRecord
	next  int
}

func newFakeConvCollection() *fakeConvCollection {
	return &fakeConvCollection{byKey: map[state.Key]state.ConversationRecord{}}
}

func (f *fakeConvCollection) Get(_ context.Context, key state.Key) (state.ConversationRecord, state.Revision, error) {
	rec, ok := f.byKey[key]
	if !ok {
		return state.ConversationRecord{}, "", state.ErrNotFound
	}
	return rec, "1", nil
}

func (f *fakeConvCollection) Create(_ context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	if _, ok := f.byKey[key]; ok {
		return state.ConversationRecord{}, "", state.ErrAlreadyExists
	}
	f.next++
	rec.ID = "minted-" + strconv.Itoa(f.next)
	rec.CreatedAt = time.Now()
	f.byKey[key] = rec
	return rec, "1", nil
}

// runIsolated runs f against a *testing.T of its own, in its own goroutine,
// so a t.Fatalf inside f (the scenario this file drives on purpose) ends
// only that goroutine, never this test's — Goexit unwinds the calling
// goroutine alone, and an orphan *testing.T has no parent to propagate Fail
// into. It reports whether f's *testing.T passed.
func runIsolated(f func(t *testing.T)) (passed bool) {
	ft := &testing.T{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f(ft)
	}()
	<-done
	return !ft.Failed()
}

// TestCreateReturnsStoredRecordPassesAConformingBackend is the ordinary,
// self-contained run of the scenario Run's own "CreateReturnsTheStoredRecord"
// subtest drives per backend (design D-2 of conversation-identity: Create
// hands back the record it just stored, ID and CreatedAt included) — kept
// here, generic and backend-free, rather than relying on kube or nats alone
// to exercise it.
func TestCreateReturnsStoredRecordPassesAConformingBackend(t *testing.T) {
	testCreateReturnsStoredRecord(t, newFakeConvCollection(), conversationFixture)
}

// createAlwaysFails is a Collection whose Create always refuses with a
// genuine, non-AlreadyExists failure.
type createAlwaysFails struct {
	*fakeConvCollection
}

var errFakeCreateBoom = errors.New("create boom")

func (createAlwaysFails) Create(context.Context, state.Key, state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	return state.ConversationRecord{}, "", errFakeCreateBoom
}

func TestCreateReturnsStoredRecordCatchesACreateFailure(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, createAlwaysFails{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail a backend whose Create genuinely errors")
	}
}

// createReturnsStaleRecord stores exactly what it is given, but hands the
// caller back a record with a different marker than what it just stored —
// as if Create answered with the pre-edit value instead of the stored one.
type createReturnsStaleRecord struct {
	*fakeConvCollection
}

func (c createReturnsStaleRecord) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	stored, rev, err := c.fakeConvCollection.Create(ctx, key, rec)
	if err != nil {
		return stored, rev, err
	}
	wrong := stored
	wrong.Title = "0"
	return wrong, rev, nil
}

func TestCreateReturnsStoredRecordCatchesCreateAnsweringWithTheWrongRecord(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, createReturnsStaleRecord{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail a Create whose returned record does not match what it stored")
	}
}

// getAlwaysFails is a Collection whose Create behaves, but whose Get always
// refuses with a genuine failure.
type getAlwaysFails struct {
	*fakeConvCollection
}

var errFakeGetBoom = errors.New("get boom")

func (getAlwaysFails) Get(context.Context, state.Key) (state.ConversationRecord, state.Revision, error) {
	return state.ConversationRecord{}, "", errFakeGetBoom
}

func TestCreateReturnsStoredRecordCatchesAGetFailureRightAfterCreate(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, getAlwaysFails{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail a backend whose Get right after Create genuinely errors")
	}
}

// getReturnsStaleRecord answers Get with a record carrying a different
// marker than what Create actually stored — as if a read landed on a stale
// copy.
type getReturnsStaleRecord struct {
	*fakeConvCollection
}

func (c getReturnsStaleRecord) Get(ctx context.Context, key state.Key) (state.ConversationRecord, state.Revision, error) {
	rec, rev, err := c.fakeConvCollection.Get(ctx, key)
	if err != nil {
		return rec, rev, err
	}
	rec.Title = "0"
	return rec, rev, nil
}

func TestCreateReturnsStoredRecordCatchesGetDisagreeingWithWhatCreateStored(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, getReturnsStaleRecord{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail a Get whose content disagrees with the record Create just stored")
	}
}

// createReturnsEmptyID stores a record exactly as given (minted ID
// included), but hands Create's own caller a copy with that ID wiped — as
// if Create forgot to mint an identity back to its own return value.
type createReturnsEmptyID struct {
	*fakeConvCollection
}

func (c createReturnsEmptyID) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	stored, rev, err := c.fakeConvCollection.Create(ctx, key, rec)
	if err != nil {
		return stored, rev, err
	}
	wrong := stored
	wrong.ID = ""
	return wrong, rev, nil
}

func TestCreateReturnsStoredRecordCatchesAnEmptyMintedID(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, createReturnsEmptyID{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail a Create whose returned record carries no ID")
	}
}

// createReturnsWrongID stores the record with its genuinely minted ID, but
// hands Create's own caller back a different, non-empty ID — so a
// subsequent Get (reading the real stored ID) disagrees with what Create
// itself reported.
type createReturnsWrongID struct {
	*fakeConvCollection
}

func (c createReturnsWrongID) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	stored, rev, err := c.fakeConvCollection.Create(ctx, key, rec)
	if err != nil {
		return stored, rev, err
	}
	wrong := stored
	wrong.ID = "different-" + stored.ID
	return wrong, rev, nil
}

func TestCreateReturnsStoredRecordCatchesCreatesIDDisagreeingWithGet(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, createReturnsWrongID{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail when Create's own returned ID disagrees with a Get right after")
	}
}

// createReturnsWrongCreatedAt stores the record with its genuine CreatedAt,
// but hands Create's own caller back a different instant.
type createReturnsWrongCreatedAt struct {
	*fakeConvCollection
}

func (c createReturnsWrongCreatedAt) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	stored, rev, err := c.fakeConvCollection.Create(ctx, key, rec)
	if err != nil {
		return stored, rev, err
	}
	wrong := stored
	wrong.CreatedAt = stored.CreatedAt.Add(time.Hour)
	return wrong, rev, nil
}

func TestCreateReturnsStoredRecordCatchesCreatesCreatedAtDisagreeingWithGet(t *testing.T) {
	if runIsolated(func(t *testing.T) {
		testCreateReturnsStoredRecord(t, createReturnsWrongCreatedAt{newFakeConvCollection()}, conversationFixture)
	}) {
		t.Fatal("want the scenario to fail when Create's own returned CreatedAt disagrees with a Get right after")
	}
}
