package conformance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Run executes every state.Collection[T] scenario against a collection open
// constructs fresh for each leaf, driven by decl (so it exercises exactly
// the indexes decl declares) and fx (so it never names a field of T). One
// t.Run per scenario, named for the capability it proves.
func Run[T any](t *testing.T, open func() state.Collection[T], decl state.Decl[T], fx Fixture[T]) {
	t.Helper()

	t.Run("WritesAreCAS", func(t *testing.T) {
		t.Run("TwoActorsRaceExactlyOneWinsNoMerge", func(t *testing.T) { testCASTwoActorsRace(t, open(), fx) })
		t.Run("StaleClaimNeverMergedChangesNothing", func(t *testing.T) { testCASStaleClaimNeverMerged(t, open(), fx) })
		t.Run("CreateRefusedWhenKeyAlreadyExists", func(t *testing.T) { testCreateRefusedOnExistingKey(t, open(), fx) })
	})

	t.Run("LookupByKeyOrDeclaredIndex", func(t *testing.T) {
		t.Run("EveryDeclaredIndexReturnsExactlyMatchingSet", func(t *testing.T) { testEveryDeclaredIndex(t, open(), decl, fx) })
		t.Run("QueryOutsideIndexSetRefused", func(t *testing.T) { testUnknownIndexRefused(t, open(), fx) })
	})

	t.Run("ChangeFeedIsAtLeastOnceAndResumable", func(t *testing.T) {
		t.Run("EventsCarryKeyRevisionRecord", func(t *testing.T) { testFeedEventShape(t, open(), fx) })
		t.Run("NewLeaderResumesFromStateAlone", func(t *testing.T) { testFeedResumeContinuity(t, open(), fx) })
		t.Run("NoGapAcrossConsumerDisconnect", func(t *testing.T) { testFeedNoGapAcrossDisconnect(t, open(), fx) })
		t.Run("StaleSnapshotCaughtByWriteNotRead", func(t *testing.T) { testFeedStaleSnapshotCaughtByCAS(t, open(), fx) })
	})

	t.Run("DeletionIsRequestedThenCompletedByReleasingFinalizers", func(t *testing.T) {
		t.Run("RecordWithoutFinalizerRemovedAtOnce", func(t *testing.T) { testDeleteWithoutFinalizer(t, open(), fx) })
		if fx.Hold != nil {
			t.Run("HeldRecordStaysVisibleWithMarker", func(t *testing.T) { testDeleteHeldRecordMarked(t, open(), fx) })
			t.Run("ReleasingLastFinalizerRemovesRecord", func(t *testing.T) { testReleaseLastFinalizerRemoves(t, open(), fx) })
			t.Run("FinalizerCannotBeAddedToMarkedRecord", func(t *testing.T) { testFinalizerAddRefusedWhenDeleting(t, open(), fx) })
			t.Run("CreateRefusedWhileMarkedRecordRemains", func(t *testing.T) { testCreateRefusedWhileDeleting(t, open(), fx) })
		}
	})

	t.Run("DeleteAtIsConditionalOnTheCallersRevision", func(t *testing.T) {
		t.Run("StaleRevisionRefusedAndRecordIntact", func(t *testing.T) { testDeleteAtStaleRevision(t, open(), fx) })
		t.Run("CurrentRevisionRemovesTheRecord", func(t *testing.T) { testDeleteAtCurrentRevision(t, open(), fx) })
		t.Run("AbsentKeyIsNotFound", func(t *testing.T) { testDeleteAtAbsentKey(t, open(), fx) })
		if fx.Hold != nil {
			t.Run("HeldRecordIsMarkedNotRemoved", func(t *testing.T) { testDeleteAtHeldRecordMarked(t, open(), fx) })
		}
	})

	if fx.Identity != nil {
		t.Run("RecordCarriesBackendMintedIdentity", func(t *testing.T) {
			t.Run("IdentityStableAcrossUpdatesAndNewAfterRecreate", func(t *testing.T) { testIdentityStableAndRenewed(t, open(), fx) })
			t.Run("CallerSuppliedIdentityIgnored", func(t *testing.T) { testIdentityCallerSuppliedIgnored(t, open(), fx) })
		})
	}

	t.Run("ScopeIsFixedAtConstruction", func(t *testing.T) {
		t.Run("TwoInstallsNeverSeeEachOther", func(t *testing.T) { testScopeIsolation(t, open, open, decl, fx) })
	})
}

// --- Requirement: Writes are compare-and-swap on an opaque revision ---

func testCASTwoActorsRace[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("cas-race")
	rev, err := s.Create(ctx, key, fx.New())
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	write := func(marker int) {
		defer wg.Done()
		_, err := s.Update(ctx, key, rev, fx.Edit(fx.New(), marker))
		results <- err
	}
	wg.Add(2)
	go write(1)
	go write(2)
	wg.Wait()
	close(results)

	var oks, conflicts int
	for err := range results {
		switch {
		case err == nil:
			oks++
		case errors.Is(err, state.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if oks != 1 || conflicts != 1 {
		t.Fatalf("got %d successes and %d conflicts, want exactly one of each", oks, conflicts)
	}

	final, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if m := fx.Marker(final); m != 1 && m != 2 {
		t.Fatalf("final marker = %d, want exactly one actor's write, never a merge", m)
	}
}

func testCASStaleClaimNeverMerged[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("cas-stale")
	staleRev, err := s.Create(ctx, key, fx.New())
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	if _, err := s.Update(ctx, key, staleRev, fx.Edit(fx.New(), 1)); err != nil {
		t.Fatalf("Update (winning write): %v", err)
	}

	if _, err := s.Update(ctx, key, staleRev, fx.Edit(fx.New(), 2)); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("stale Update error = %v, want ErrConflict", err)
	}

	got, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if fx.Marker(got) != 1 {
		t.Fatalf("marker = %d after a refused stale write, want 1 unchanged", fx.Marker(got))
	}
}

func testCreateRefusedOnExistingKey[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("create-twice")
	if _, err := s.Create(ctx, key, fx.Edit(fx.New(), 1)); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if _, err := s.Create(ctx, key, fx.Edit(fx.New(), 2)); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create error = %v, want ErrAlreadyExists", err)
	} else if errors.Is(err, state.ErrConflict) {
		t.Fatalf("ErrAlreadyExists must be distinguishable from ErrConflict")
	}

	got, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if fx.Marker(got) != 1 {
		t.Fatalf("marker = %d after a refused create, want 1 unchanged", fx.Marker(got))
	}
}

// --- Requirement: Lookup is by key or by a declared index ---

func testEveryDeclaredIndex[T any](t *testing.T, s state.Collection[T], decl state.Decl[T], fx Fixture[T]) {
	if len(decl.Indexes) == 0 {
		t.Skip("this kind declares no index")
	}
	ctx := context.Background()
	for _, kv := range fx.Indexed {
		if _, err := s.Create(ctx, kv.Key, kv.Record); err != nil {
			t.Fatalf("Create %q: %v", kv.Key, err)
		}
	}
	for _, idx := range decl.Indexes {
		checkIndex(t, ctx, s, idx, fx)
	}
}

// checkIndex lists every value the fixture's pool puts under idx and requires
// exactly the keys that carry it.
func checkIndex[T any](t *testing.T, ctx context.Context, s state.Collection[T], idx state.Index[T], fx Fixture[T]) {
	groups := map[string][]state.Key{}
	for _, kv := range fx.Indexed {
		rec := kv.Record
		v, listed := idx.Value(&rec)
		if listed {
			groups[v] = append(groups[v], kv.Key)
		}
	}
	if len(groups) == 0 {
		t.Fatalf("index %q: the fixture's pool lists nothing under it — add a record that does", idx.Name())
	}
	for v, want := range groups {
		got, err := s.List(ctx, idx, v)
		if err != nil {
			t.Fatalf("List(%s, %q): %v", idx.Name(), v, err)
		}
		gotKeys := make([]state.Key, len(got))
		for i, r := range got {
			gotKeys[i] = fx.KeyOf(r)
		}
		if !sameKeySet(gotKeys, want) {
			t.Fatalf("List(%s, %q) = %v, want exactly %v", idx.Name(), v, gotKeys, want)
		}
	}
}

func testUnknownIndexRefused[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	_ = fx
	undeclared := state.NewIndex[T]("not-a-declared-index", func(*T) (string, bool) { return "", true })
	if _, err := s.List(context.Background(), undeclared, ""); !errors.Is(err, state.ErrUnknownIndex) {
		t.Fatalf("List with unknown index: got %v, want ErrUnknownIndex", err)
	}
}

// --- Requirement: The change feed is at-least-once and resumable ---

func testFeedEventShape[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}

	key := fx.Key("feed-shape")
	rev, err := s.Create(ctx, key, fx.Edit(fx.New(), 1))
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	evt := mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventPut && e.Key == key && e.Revision == rev
	})
	if fx.Marker(evt.Record) != 1 {
		t.Fatalf("event marker = %d, want 1", fx.Marker(evt.Record))
	}
}

func testFeedResumeContinuity[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	formerLeaderEvents, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("resume-leader")
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	createEvt := mustRecvEvent(t, formerLeaderEvents, 5*time.Second)
	resumePoint := createEvt.Resume

	// The former leader is gone: nothing more is read from
	// formerLeaderEvents, while a change still lands.
	cur, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if _, err := s.Update(ctx, key, rev, fx.Edit(cur, 1)); err != nil {
		t.Fatalf(fmtUpdate, err)
	}

	resumed, err := s.Watch(ctx, resumePoint)
	if err != nil {
		t.Fatalf("Watch(resume): %v", err)
	}
	mustRecvEventMatching(t, resumed, 5*time.Second, func(e state.Event[T]) bool {
		return e.Key == key && e.Type == state.EventPut && fx.Marker(e.Record) == 1
	})
}

func testFeedNoGapAcrossDisconnect[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("no-gap")
	if _, err := s.Create(ctx, key, fx.Edit(fx.New(), 0)); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	firstEvt := mustRecvEvent(t, events, 5*time.Second)
	lastSeen := firstEvt.Resume

	const duringDisconnect = 20
	cur, currentRev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	for i := 1; i <= duringDisconnect; i++ {
		newRev, err := s.Update(ctx, key, currentRev, fx.Edit(cur, i))
		if err != nil {
			t.Fatalf("Update %d: %v", i, err)
		}
		currentRev = newRev
	}

	resumed, err := s.Watch(ctx, lastSeen)
	if err != nil {
		t.Fatalf("Watch(resume): %v", err)
	}
	var markers []int
	for _, evt := range drainQuiet(resumed, 500*time.Millisecond) {
		markers = append(markers, fx.Marker(evt.Record))
	}

	var collapsed []int
	for _, m := range markers {
		if len(collapsed) == 0 || collapsed[len(collapsed)-1] != m {
			collapsed = append(collapsed, m)
		}
	}
	for len(collapsed) > 0 && collapsed[0] == 0 {
		collapsed = collapsed[1:]
	}
	if len(collapsed) != duringDisconnect {
		t.Fatalf("got markers %v after collapsing duplicates, want exactly %d entries (1..%d) — gap or reorder across the disconnect", collapsed, duringDisconnect, duringDisconnect)
	}
	for i, m := range collapsed {
		if want := i + 1; m != want {
			t.Fatalf("event %d marker = %d, want %d — gap or reorder across the disconnect", i, m, want)
		}
	}
}

func testFeedStaleSnapshotCaughtByCAS[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("stale-snapshot")
	if _, err := s.Create(ctx, key, fx.Edit(fx.New(), 1)); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	snapshot := mustRecvEvent(t, events, 5*time.Second)

	current, currentRev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get (second actor): %v", err)
	}
	if _, err := s.Update(ctx, key, currentRev, fx.Edit(current, 2)); err != nil {
		t.Fatalf("Update (second actor): %v", err)
	}

	if _, err := s.Update(ctx, key, snapshot.Revision, fx.Edit(snapshot.Record, 3)); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("stale consumer Update error = %v, want ErrConflict", err)
	}

	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event[T]) bool {
		return e.Key == key && fx.Marker(e.Record) == 2
	})
}

// --- Requirement: Deletion is requested, then completed by releasing finalizers ---

func testDeleteWithoutFinalizer[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("delete-plain")
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}
	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of an absent key: %v", err)
	}
}

func testDeleteHeldRecordMarked[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("delete-held")
	if _, err := s.Create(ctx, key, fx.Hold.Hold(fx.New())); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}
	got, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get of a held record after Delete: %v", err)
	}
	if !fx.Hold.Marked(got) {
		t.Fatalf("a held record after Delete is not marked")
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventPut && e.Key == key && fx.Hold.Marked(e.Record)
	})

	time.Sleep(1100 * time.Millisecond) // past whole-second timestamp precision
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	again, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if !fx.Hold.Marked(again) {
		t.Fatalf("second Delete unmarked a held record")
	}
}

func testReleaseLastFinalizerRemoves[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := fx.Key("release")
	held := fx.Hold.AddSecond(fx.Hold.Hold(fx.New()))
	if _, err := s.Create(ctx, key, held); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}

	rec, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	// fx.Hold.Hold sets the finalizer set to exactly one, modelling "one of
	// two released".
	rev, err = s.Update(ctx, key, rev, fx.Hold.Hold(rec))
	if err != nil {
		t.Fatalf("Update releasing one finalizer: %v", err)
	}
	rec, _, err = s.Get(ctx, key)
	if err != nil || fx.Hold.NumFinalizers(rec) != 1 || !fx.Hold.Marked(rec) {
		t.Fatalf("after releasing one of two: %+v, %v — want the record, marked, one finalizer", rec, err)
	}

	if _, err := s.Update(ctx, key, rev, fx.Hold.Release(rec)); err != nil {
		t.Fatalf("Update releasing the last finalizer: %v", err)
	}
	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after the last release error = %v, want ErrNotFound", err)
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
}

func testFinalizerAddRefusedWhenDeleting[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("no-new-hold")
	if _, err := s.Create(ctx, key, fx.Hold.Hold(fx.New())); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}
	rec, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if _, err := s.Update(ctx, key, rev, fx.Hold.AddSecond(rec)); !errors.Is(err, state.ErrDeleting) {
		t.Fatalf("Update adding a finalizer error = %v, want ErrDeleting", err)
	}
	got, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if fx.Hold.NumFinalizers(got) != 1 {
		t.Fatalf("refused Update changed the finalizer set: %+v", got)
	}
}

func testCreateRefusedWhileDeleting[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("still-held")
	if _, err := s.Create(ctx, key, fx.Hold.Hold(fx.New())); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}
	if _, err := s.Create(ctx, key, fx.New()); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("Create over a marked record error = %v, want ErrAlreadyExists", err)
	}
}

// --- The conditional delete added by state-store-generic-contract (D-10) ---

func testDeleteAtStaleRevision[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("deleteat-stale")
	stale, err := s.Create(ctx, key, fx.Edit(fx.New(), 1))
	if err != nil {
		t.Fatal(err)
	}
	rewritten := fx.Edit(fx.New(), 2)
	if _, err := s.Update(ctx, key, stale, rewritten); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteAt(ctx, key, stale); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("DeleteAt on a stale revision = %v, want ErrConflict", err)
	}
	got, _, err := s.Get(ctx, key)
	if err != nil || fx.Marker(got) != 2 {
		t.Fatalf("the record after a refused DeleteAt = %+v, %v; want it intact", got, err)
	}
}

func testDeleteAtCurrentRevision[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("deleteat-current")
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatal(err)
	}
	_, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAt(ctx, key, rev); err != nil {
		t.Fatalf("DeleteAt on the current revision: %v", err)
	}
	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after DeleteAt = %v, want ErrNotFound", err)
	}
}

func testDeleteAtAbsentKey[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	key := fx.Key("deleteat-absent")
	if err := s.DeleteAt(context.Background(), key, "1"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("DeleteAt of an absent key = %v, want ErrNotFound", err)
	}
}

func testDeleteAtHeldRecordMarked[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("deleteat-finalizer")
	if _, err := s.Create(ctx, key, fx.Hold.Hold(fx.New())); err != nil {
		t.Fatal(err)
	}
	_, rev, _ := s.Get(ctx, key)
	if err := s.DeleteAt(ctx, key, rev); err != nil {
		t.Fatalf("DeleteAt: %v", err)
	}
	marked, _, err := s.Get(ctx, key)
	if err != nil || !fx.Hold.Marked(marked) {
		t.Fatalf("a held record after DeleteAt = %+v, %v; want it readable and marked", marked, err)
	}
}

// --- Requirement: A record carries a backend-minted identity ---

func testIdentityStableAndRenewed[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("identity")
	before := time.Now().UTC().Add(-time.Minute)
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	first, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	id := fx.Identity
	if id.ID(first) == "" {
		t.Fatalf("ID is empty, want an opaque non-empty identity")
	}
	if id.CreatedAt(first).Before(before) || id.CreatedAt(first).After(time.Now().Add(time.Minute)) {
		t.Fatalf("CreatedAt = %v, want about now", id.CreatedAt(first))
	}

	if _, err := s.Update(ctx, key, rev, fx.Edit(first, 1)); err != nil {
		t.Fatalf(fmtUpdate, err)
	}
	second, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if id.ID(second) != id.ID(first) || !id.CreatedAt(second).Equal(id.CreatedAt(first)) {
		t.Fatalf("identity moved across an Update: %q %v -> %q %v", id.ID(first), id.CreatedAt(first), id.ID(second), id.CreatedAt(second))
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf(fmtDelete, err)
	}
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf("Create (recreate): %v", err)
	}
	third, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if id.ID(third) == "" || id.ID(third) == id.ID(first) {
		t.Fatalf("recreated record ID = %q, want a new identity distinct from %q", id.ID(third), id.ID(first))
	}
}

func testIdentityCallerSuppliedIgnored[T any](t *testing.T, s state.Collection[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("identity-ignored")
	id := fx.Identity
	if _, err := s.Create(ctx, key, id.Forge(fx.New())); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	got, rev, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if id.ID(got) == "" {
		t.Fatalf("Create dropped identity entirely: %+v", got)
	}

	if _, err := s.Update(ctx, key, rev, id.Forge(fx.Edit(got, 1))); err != nil {
		t.Fatalf(fmtUpdate, err)
	}
	again, _, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if id.ID(again) != id.ID(got) || !id.CreatedAt(again).Equal(id.CreatedAt(got)) {
		t.Fatalf("Update kept caller-supplied identity: %+v", again)
	}
}

// --- Requirement: Scope is fixed when the collection is constructed ---

func testScopeIsolation[T any](t *testing.T, openA, openB func() state.Collection[T], decl state.Decl[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a, b := openA(), openB()
	key := fx.Key("shared-name")
	if _, err := a.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf("Create on A: %v", err)
	}

	if _, _, err := b.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on B for A's key error = %v, want ErrNotFound", err)
	}

	if len(decl.Indexes) > 0 {
		listed, err := b.List(ctx, decl.Indexes[0], "")
		if err == nil {
			for _, r := range listed {
				if fx.KeyOf(r) == key {
					t.Fatalf("B's listing includes A's record")
				}
			}
		}
	}

	eventsB, err := b.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch on B: %v", err)
	}
	select {
	case evt := <-eventsB:
		t.Fatalf("B's feed delivered an event from A's install: %+v", evt)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing from A ever reaches B.
	}
}
