package conformance

// Scenarios for the requirements added after the manager migration (design
// D-11 and D-12): record identity, the deletion lifecycle, run mutation, the
// indexes the manager's lookups needed, and UTC instants. Each test is named
// for the scenario of specs/state-store/spec.md it proves.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const (
	finalizerA = "example.com/hold-a"
	finalizerB = "example.com/hold-b"
)

func runLifecycleScenarios(t *testing.T, newStore func() state.Store) {
	t.Run("RecordCarriesBackendMintedIdentity", func(t *testing.T) {
		t.Run("IdentityStableAcrossUpdatesAndNewAfterRecreate", func(t *testing.T) {
			testIdentityStableAndRenewed(t, newStore())
		})
		t.Run("CallerSuppliedIdentityIgnored", func(t *testing.T) {
			testIdentityCallerSuppliedIgnored(t, newStore())
		})
	})

	t.Run("DeletionIsRequestedThenCompletedByReleasingFinalizers", func(t *testing.T) {
		t.Run("RecordWithoutFinalizerRemovedAtOnce", func(t *testing.T) {
			testDeleteWithoutFinalizer(t, newStore())
		})
		t.Run("HeldRecordStaysVisibleWithMarker", func(t *testing.T) {
			testDeleteHeldRecordMarked(t, newStore())
		})
		t.Run("ReleasingLastFinalizerRemovesRecord", func(t *testing.T) {
			testReleaseLastFinalizerRemoves(t, newStore())
		})
		t.Run("FinalizerCannotBeAddedToMarkedRecord", func(t *testing.T) {
			testFinalizerAddRefusedWhenDeleting(t, newStore())
		})
		t.Run("CreateRefusedWhileMarkedRecordRemains", func(t *testing.T) {
			testCreateRefusedWhileDeleting(t, newStore())
		})
	})

	t.Run("ExistingRunAcceptsOnlyGrowth", func(t *testing.T) {
		t.Run("UpdateAppliesAndIsIdempotent", func(t *testing.T) {
			testUpdateRunAppliesIdempotently(t, newStore())
		})
		t.Run("ConcurrentUpdatesAndAppendsAllLand", func(t *testing.T) {
			testUpdateRunCommutes(t, newStore())
		})
		t.Run("UnknownRunRefusedAsNotFound", func(t *testing.T) {
			testUpdateRunUnknown(t, newStore())
		})
	})

	t.Run("LookupByKeyOrDeclaredIndex", func(t *testing.T) {
		t.Run("MembersFoundByParent", func(t *testing.T) {
			testIndexCausedBy(t, newStore())
		})
		t.Run("NameBaseFindsEveryRecordOfAWordChain", func(t *testing.T) {
			testIndexNameBase(t, newStore())
		})
		t.Run("WholeSetListedWithoutFilter", func(t *testing.T) {
			testIndexAll(t, newStore())
		})
	})

	t.Run("InstantsAreUTC", func(t *testing.T) {
		t.Run("InstantsReadBackInUTCWhateverWasWritten", func(t *testing.T) {
			testInstantsUTC(t, newStore())
		})
	})
}

func createRecord(t *testing.T, store state.Store, rec state.ConversationRecord) state.Revision {
	t.Helper()
	rev, err := store.Create(context.Background(), state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}
	return rev
}

// --- Requirement: A record carries a backend-minted identity ---

func testIdentityStableAndRenewed(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-identity")
	before := time.Now().UTC().Add(-time.Minute)
	rev := createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-identity", Phase: "Pending"})

	first, rev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if first.ID == "" {
		t.Fatalf("ID is empty, want an opaque non-empty identity")
	}
	if first.CreatedAt.Before(before) || first.CreatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("CreatedAt = %v, want about now", first.CreatedAt)
	}
	if first.DeletionRequestedAt != nil {
		t.Fatalf("DeletionRequestedAt = %v on a fresh record, want nil", first.DeletionRequestedAt)
	}

	first.Phase = "Working"
	if _, err := store.Update(ctx, key, rev, first); err != nil {
		t.Fatalf("Update: %v", err)
	}
	second, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("identity moved across an Update: %q %v -> %q %v", first.ID, first.CreatedAt, second.ID, second.CreatedAt)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-identity", Phase: "Pending"})
	third, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if third.ID == "" || third.ID == first.ID {
		t.Fatalf("recreated record ID = %q, want a new identity distinct from %q", third.ID, first.ID)
	}
}

func testIdentityCallerSuppliedIgnored(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-identity-ignored")
	fake := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	rec := state.ConversationRecord{
		ProfileRef: profileDefault, Name: "conv-identity-ignored", Phase: "Pending",
		ID: "caller-chosen", CreatedAt: fake, DeletionRequestedAt: &fake,
	}
	createRecord(t, store, rec)

	got, rev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if got.ID == "caller-chosen" || got.CreatedAt.Equal(fake) || got.DeletionRequestedAt != nil {
		t.Fatalf("Create kept caller-supplied identity: %+v", got)
	}

	rec.Phase = "Working"
	if _, err := store.Update(ctx, key, rev, rec); err != nil {
		t.Fatalf("Update: %v", err)
	}
	again, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if again.ID != got.ID || !again.CreatedAt.Equal(got.CreatedAt) || again.DeletionRequestedAt != nil {
		t.Fatalf("Update kept caller-supplied identity: %+v", again)
	}
}

// --- Requirement: Deletion is requested, then completed by releasing finalizers ---

func testDeleteWithoutFinalizer(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-delete-plain")
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-delete-plain", Phase: "Working"})

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of an absent key: %v", err)
	}
}

func testDeleteHeldRecordMarked(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-delete-held")
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-delete-held", Phase: "Working", Finalizers: []string{finalizerA}})

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get of a held record after Delete: %v", err)
	}
	if got.DeletionRequestedAt == nil {
		t.Fatalf("DeletionRequestedAt is nil on a held record after Delete")
	}
	marked := *got.DeletionRequestedAt

	listed, err := store.List(ctx, state.ByPhase, state.ListKey{Phase: "Working"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].DeletionRequestedAt == nil {
		t.Fatalf("List = %+v, want the held record, marked", listed)
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event) bool {
		return e.Type == state.EventPut && e.Key == key && e.Record.DeletionRequestedAt != nil
	})

	time.Sleep(1100 * time.Millisecond) // past whole-second timestamp precision
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	again, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if again.DeletionRequestedAt == nil || !again.DeletionRequestedAt.Equal(marked) {
		t.Fatalf("second Delete moved DeletionRequestedAt: %v -> %v", marked, again.DeletionRequestedAt)
	}
}

func testReleaseLastFinalizerRemoves(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-release")
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-release", Phase: "Working", Finalizers: []string{finalizerA, finalizerB}})
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// One of two finalizers released: the record stays, still marked.
	rec, rev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	rec.Finalizers = []string{finalizerB}
	rev, err = store.Update(ctx, key, rev, rec)
	if err != nil {
		t.Fatalf("Update releasing one finalizer: %v", err)
	}
	rec, _, err = store.Get(ctx, key)
	if err != nil || rec.DeletionRequestedAt == nil || len(rec.Finalizers) != 1 {
		t.Fatalf("after releasing one of two: %+v, %v — want the record, marked, one finalizer", rec, err)
	}

	// The last one, together with a status change in the same write: the
	// write lands, then the record is gone.
	rec.Finalizers = nil
	rec.Phase = "Closed"
	if _, err := store.Update(ctx, key, rev, rec); err != nil {
		t.Fatalf("Update releasing the last finalizer: %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after the last release error = %v, want ErrNotFound", err)
	}
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
}

func testFinalizerAddRefusedWhenDeleting(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-no-new-hold")
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-no-new-hold", Phase: "Working", Finalizers: []string{finalizerA}})
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rec, rev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	rec.Finalizers = []string{finalizerA, finalizerB}
	rec.Phase = "Closed"
	if _, err := store.Update(ctx, key, rev, rec); !errors.Is(err, state.ErrDeleting) {
		t.Fatalf("Update adding a finalizer error = %v, want ErrDeleting", err)
	}
	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if got.Phase != "Working" || len(got.Finalizers) != 1 {
		t.Fatalf("refused Update changed the record: %+v", got)
	}
}

func testCreateRefusedWhileDeleting(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-still-held")
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-still-held", Finalizers: []string{finalizerA}})
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-still-held"})
	if !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("Create over a marked record error = %v, want ErrAlreadyExists", err)
	}
}

// --- Requirement: An existing run accepts only growth ---

func seedRuns(t *testing.T, store state.Store, name string, runIDs ...string) state.Key {
	t.Helper()
	key := state.Key(name)
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: name})
	for _, id := range runIDs {
		if err := store.AppendRun(context.Background(), key, state.Run{RunID: id, Status: "Succeeded"}); err != nil {
			t.Fatalf("AppendRun %s: %v", id, err)
		}
	}
	return key
}

func runByID(t *testing.T, store state.Store, key state.Key, id string) state.Run {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), key)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	for _, r := range runs {
		if r.RunID == id {
			return r
		}
	}
	t.Fatalf("run %q not in %+v", id, runs)
	return state.Run{}
}

func testUpdateRunAppliesIdempotently(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := seedRuns(t, store, "conv-run-update", "run-1", "run-2")

	upd := state.RunUpdate{MarkDeliveryTracked: true, MarkDelivered: []string{"chan-a", "chan-b"}, MarkRoutedToParent: true}
	for i := 0; i < 2; i++ { // the second application is a no-op, not an error
		if err := store.UpdateRun(ctx, key, "run-1", upd); err != nil {
			t.Fatalf("UpdateRun #%d: %v", i+1, err)
		}
	}
	if err := store.UpdateRun(ctx, key, "run-1", state.RunUpdate{MarkDelivered: []string{"chan-b", "chan-c"}}); err != nil {
		t.Fatalf("UpdateRun adding to the set: %v", err)
	}

	got := runByID(t, store, key, "run-1")
	if !got.DeliveryTracked || !got.RoutedToParent {
		t.Fatalf("flags not set: %+v", got)
	}
	if want := []string{"chan-a", "chan-b", "chan-c"}; len(got.Delivered) != 3 || got.Delivered[0] != want[0] || got.Delivered[1] != want[1] || got.Delivered[2] != want[2] {
		t.Fatalf("Delivered = %v, want %v (union, first-appearance order)", got.Delivered, want)
	}
	if other := runByID(t, store, key, "run-2"); other.DeliveryTracked || other.RoutedToParent || len(other.Delivered) != 0 {
		t.Fatalf("UpdateRun touched another run: %+v", other)
	}
	if err := store.UpdateRun(ctx, key, "run-1", state.RunUpdate{}); err != nil {
		t.Fatalf("zero RunUpdate: %v", err)
	}
}

func testUpdateRunCommutes(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := seedRuns(t, store, "conv-run-commute", "run-1")

	channels := []string{"chan-1", "chan-2", "chan-3", "chan-4"}
	var wg sync.WaitGroup
	errs := make(chan error, len(channels)+3)
	for _, ch := range channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.UpdateRun(ctx, key, "run-1", state.RunUpdate{MarkDelivered: []string{ch}})
		}()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- store.UpdateRun(ctx, key, "run-1", state.RunUpdate{MarkRoutedToParent: true})
	}()
	go func() {
		defer wg.Done()
		errs <- store.AppendRun(ctx, key, state.Run{RunID: "run-2", Status: "Succeeded"})
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent run write surfaced an error: %v", err)
		}
	}

	got := runByID(t, store, key, "run-1")
	if len(got.Delivered) != len(channels) || !got.RoutedToParent {
		t.Fatalf("run-1 = %+v, want all %d channels and RoutedToParent", got, len(channels))
	}
	if runs, _ := store.ListRuns(ctx, key); len(runs) != 2 {
		t.Fatalf("ListRuns = %d runs, want 2 (the append landed too)", len(runs))
	}
}

func testUpdateRunUnknown(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := seedRuns(t, store, "conv-run-unknown", "run-1")
	if err := store.UpdateRun(ctx, key, "no-such-run", state.RunUpdate{MarkRoutedToParent: true}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("UpdateRun of an unknown run error = %v, want ErrNotFound", err)
	}
	if err := store.UpdateRun(ctx, "no-such-conversation", "run-1", state.RunUpdate{MarkRoutedToParent: true}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("UpdateRun of an unknown conversation error = %v, want ErrNotFound", err)
	}
}

// --- Requirement: Lookup is by key or by a declared index (extended) ---

func names(recs []state.ConversationRecord) map[string]bool {
	out := map[string]bool{}
	for _, r := range recs {
		out[r.Name] = true
	}
	return out
}

func testIndexCausedBy(t *testing.T, store state.Store) {
	ctx := context.Background()
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "root"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "member-1", CausedBy: &state.Provenance{Parent: "root", Entry: "a"}})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "member-2", CausedBy: &state.Provenance{Parent: "root", Entry: "b"}})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "grandchild", CausedBy: &state.Provenance{Parent: "member-1", Entry: "c"}})

	got, err := store.List(ctx, state.ByCausedBy, state.ListKey{CausedBy: "root"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := names(got); len(got) != 2 || !n["member-1"] || !n["member-2"] {
		t.Fatalf("List(ByCausedBy, root) = %v, want exactly the two direct members", n)
	}
}

func testIndexNameBase(t *testing.T, store state.Store) {
	ctx := context.Background()
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "task-red-fox", NameBase: "task-red-fox"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "task-red-fox-2", NameBase: "task-red-fox"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "task-blue-owl", NameBase: "task-blue-owl"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "unbased"})

	got, err := store.List(ctx, state.ByNameBase, state.ListKey{NameBase: "task-red-fox"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := names(got); len(got) != 2 || !n["task-red-fox"] || !n["task-red-fox-2"] {
		t.Fatalf("List(ByNameBase, task-red-fox) = %v, want exactly the two sharing it", n)
	}
	if got[0].NameBase != "task-red-fox" {
		t.Fatalf("NameBase did not round-trip: %+v", got[0])
	}
}

func testIndexAll(t *testing.T, store state.Store) {
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "all-a", Phase: "Working"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "all-b", Phase: "Closed"})
	createRecord(t, store, state.ConversationRecord{ProfileRef: profileDefault, Name: "all-expired", ExpiresAt: &past})

	got, err := store.List(ctx, state.ByAll, state.ListKey{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := names(got); len(got) != 2 || !n["all-a"] || !n["all-b"] {
		t.Fatalf("List(ByAll) = %v, want every record that has not expired", n)
	}
}

// --- Requirement: Instants are UTC ---

func testInstantsUTC(t *testing.T, store state.Store) {
	ctx := context.Background()
	zone := time.FixedZone("UTC+2", 2*60*60)
	at := time.Date(2026, 1, 2, 5, 0, 0, 0, zone)
	key := state.Key("conv-utc")
	createRecord(t, store, state.ConversationRecord{
		ProfileRef: profileDefault, Name: "conv-utc",
		LastActivity: &at,
		Inflight:     &state.InflightRun{RunID: "r", DispatchedAt: at},
	})
	if err := store.AppendRun(ctx, key, state.Run{RunID: "r", StartedAt: &at}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	runs, err := store.ListRuns(ctx, key)
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListRuns = %+v, %v", runs, err)
	}
	for name, got := range map[string]time.Time{
		"CreatedAt":             got.CreatedAt,
		"LastActivity":          *got.LastActivity,
		"Inflight.DispatchedAt": got.Inflight.DispatchedAt,
		"Run.StartedAt":         *runs[0].StartedAt,
	} {
		if got.Location() != time.UTC {
			t.Errorf("%s location = %v, want UTC", name, got.Location())
		}
		if name != "CreatedAt" && !got.Equal(at) {
			t.Errorf("%s = %v, want the instant %v", name, got, at)
		}
	}
}
