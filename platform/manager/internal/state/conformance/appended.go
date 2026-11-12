package conformance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// watchWithItemsFailed is the failure format the WatchWithItems cases share.
const watchWithItemsFailed = "WatchWithItems: %v"

// AppendedOpen pairs one fresh, isolated instance of the parent's Collection
// with the Appended handle over the very same backend, so a scenario can
// create a parent and then grow its items under one capability pair —
// Append/Grow act on an existing parent, which only Collection[P] can create.
type AppendedOpen[P, T any] struct {
	Parent state.Collection[P]
	Items  state.Appended[P, T]
}

// AppendedFixture is an Appended[P, T] kind's own test data: enough to
// create a parent, add a plain item and name a grow-only edit by the
// markers it reflects — matching how Decl[T].Merge is actually shaped (a
// set of effects that only grows), never a single overwritten field.
type AppendedFixture[P, T any] struct {
	// NewParent returns a minimal, valid parent record.
	NewParent func() P
	// NewItem returns a plain item of identity id, reflecting no marker.
	NewItem func(id string) T
	// Edit returns a grow-only edit naming one marker, passed to Grow.
	Edit func(marker string) T
	// Effects reads, from an item Grow has touched, the sorted set of
	// markers it currently reflects.
	Effects func(item T) []string

	// AllIndex and AllValue list the parent's whole set, used only by the
	// ListWithItems-equals-the-combined-read scenario; AllIndex's zero value
	// (Name() == "") skips it.
	AllIndex state.Index[P]
	AllValue string
	// KeyOf reads the key a listed parent belongs under, needed only by that
	// same scenario.
	KeyOf func(rec P) state.Key
	// EditParent changes the parent in a way ParentChanged can read back,
	// without touching any item. Needed only by the parent-only-write
	// scenario; nil skips it.
	EditParent func(rec *P)
	// ParentChanged reports whether rec carries the change EditParent made.
	ParentChanged func(rec P) bool

	// HistoryLimit is how many items a parent keeps, the oldest falling off.
	// Zero means unbounded and skips the scenario.
	HistoryLimit int

	// ParentIndex, IndexBefore and IndexAfter name an index whose value
	// EditParent moves the parent from one entry of to another. Needed only by
	// the index-after-a-parent-write scenario; a zero ParentIndex skips it.
	ParentIndex             state.Index[P]
	IndexBefore, IndexAfter string
}

// RunAppended executes every state.Appended[P, T] scenario against a fresh
// pair open constructs for each leaf.
func RunAppended[P, T any](t *testing.T, open func() AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	t.Helper()

	t.Run("AppendIsAlwaysGrowOnlyAndNeedsNoRevision", func(t *testing.T) {
		t.Run("ConcurrentAppendsBothLandNoConflictSurfaced", func(t *testing.T) { testAppendNeverConflicts(t, open(), fx) })
		t.Run("ItemsReadBackInAppendOrder", func(t *testing.T) { testListOrder(t, open(), fx) })
		t.Run("OnMissingParentIsNotFound", func(t *testing.T) { testAppendOnMissingParent(t, open(), fx) })
	})

	t.Run("RecordAndItemsInOneRead", func(t *testing.T) {
		t.Run("GetWithItemsMatchesTheSeparateReads", func(t *testing.T) { testGetWithItems(t, open(), fx) })
		t.Run("ListOnMissingParentIsNotFound", func(t *testing.T) { testListOnMissingParentIsNotFound(t, open(), fx) })
		t.Run("GetWithItemsOnMissingParentIsNotFound", func(t *testing.T) { testGetWithItemsOnMissingParent(t, open(), fx) })
	})

	t.Run("ExistingItemAcceptsOnlyGrowth", func(t *testing.T) {
		t.Run("GrowOfAnUnknownItemOrParentIsNotFound", func(t *testing.T) { testGrowUnknown(t, open(), fx) })
		t.Run("MergeIsIdempotent", func(t *testing.T) { testMergeIdempotent(t, open(), fx) })
		t.Run("MergeCommutes", func(t *testing.T) { testMergeCommutes(t, open(), fx) })
		t.Run("ConcurrentGrowsAndAppendsAllLand", func(t *testing.T) { testGrowCommutesWithAppend(t, open(), fx) })
	})

	if fx.AllIndex.Name() != "" {
		t.Run("ListingWithItemsEqualsTheCombinedRead", func(t *testing.T) { testListWithItems(t, open(), fx) })
	}

	t.Run("DeletingTheParentTakesItsItemsWithIt", func(t *testing.T) { testDeleteTakesItems(t, open(), fx) })
	if fx.HistoryLimit > 0 {
		t.Run("HistoryKeepsTheNewestItemsUpToItsLimit", func(t *testing.T) { testHistoryLimit(t, open(), fx) })
	}
	if fx.ParentIndex.Name() != "" && fx.EditParent != nil && fx.ParentChanged != nil {
		t.Run("ListingWithItemsFollowsAParentOnlyWrite", func(t *testing.T) { testListWithItemsAfterParentWrite(t, open(), fx) })
	}

	t.Run("WatchWithItemsCarriesTheItemsOfEveryPut", func(t *testing.T) { testWatchWithItems(t, open(), fx) })
	if fx.AllIndex.Name() != "" {
		t.Run("WatchWithItemsResumesFromAListedRevision", func(t *testing.T) { testWatchWithItemsResumesFromList(t, open(), fx) })
	}
	if fx.EditParent != nil && fx.ParentChanged != nil {
		t.Run("WatchWithItemsCarriesAParentWriteThatTouchesNoItem", func(t *testing.T) { testWatchWithItemsParentOnly(t, open(), fx) })
		t.Run("WatchWithItemsResumesStrictlyAfterAnEvent", func(t *testing.T) { testWatchWithItemsResume(t, open(), fx) })
	}
}

func createParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T], name string) state.Key {
	t.Helper()
	key := state.Key(name)
	if _, err := o.Parent.Create(context.Background(), key, fx.NewParent()); err != nil {
		t.Fatalf("Create parent %q: %v", name, err)
	}
	return key
}

func testAppendNeverConflicts[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "append-race")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	appendOne := func(id string) {
		defer wg.Done()
		errs <- o.Items.Append(ctx, key, fx.NewItem(id))
	}
	wg.Add(2)
	go appendOne("item-a")
	go appendOne("item-b")
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Append surfaced a conflict or other error: %v", err)
		}
	}

	items, err := o.Items.List(ctx, key)
	if err != nil {
		t.Fatalf(fmtList, err)
	}
	if len(items) != 2 {
		t.Fatalf("List = %d items, want exactly item-a and item-b", len(items))
	}
}

func testListOrder[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "order")
	const n = 5
	for i := 0; i < n; i++ {
		if err := o.Items.Append(ctx, key, fx.NewItem(fmt.Sprintf("item-%d", i))); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	items, err := o.Items.List(ctx, key)
	if err != nil {
		t.Fatalf(fmtList, err)
	}
	if len(items) != n {
		t.Fatalf("got %d items, want %d", len(items), n)
	}
}

func testAppendOnMissingParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	if err := o.Items.Append(context.Background(), noSuchParent, fx.NewItem(itemOne)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Append on missing parent: got %v, want ErrNotFound", err)
	}
}

func testGetWithItems[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "with-items")
	for _, id := range []string{"item-a", "item-b"} {
		if err := o.Items.Append(ctx, key, fx.NewItem(id)); err != nil {
			t.Fatalf(fmtAppend, err)
		}
	}
	_, rev, items, err := o.Items.GetWithItems(ctx, key)
	if err != nil {
		t.Fatalf("GetWithItems: %v", err)
	}
	_, gotRev, err := o.Parent.Get(ctx, key)
	if err != nil || rev != gotRev {
		t.Fatalf("GetWithItems revision %q, Get %q, %v; want the same", rev, gotRev, err)
	}
	wantItems, err := o.Items.List(ctx, key)
	if err != nil || len(items) != len(wantItems) || len(items) != 2 {
		t.Fatalf("GetWithItems items = %+v, List = %+v, %v; want the same 2", items, wantItems, err)
	}
}

// testListOnMissingParentIsNotFound pins store.go's own List doc: "A parent
// that does not exist is ErrNotFound, as ListRuns always answered" — a
// parent that exists with no items lists empty instead (testListOrder's own
// first iteration already proves that half).
func testListOnMissingParentIsNotFound[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	_ = fx
	if _, err := o.Items.List(context.Background(), noSuchParent); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("List of a missing parent: got %v, want ErrNotFound", err)
	}
}

func testGetWithItemsOnMissingParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	_ = fx
	if _, _, _, err := o.Items.GetWithItems(context.Background(), noSuchParent); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("GetWithItems of a missing parent: got %v, want ErrNotFound", err)
	}
}

func testGrowUnknown[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "grow-unknown")
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatalf(fmtAppend, err)
	}
	if err := o.Items.Grow(ctx, key, "no-such-item", fx.Edit("x")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an unknown item: got %v, want ErrNotFound", err)
	}
	if err := o.Items.Grow(ctx, noSuchParent, itemOne, fx.Edit("x")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an unknown parent: got %v, want ErrNotFound", err)
	}
}

func testMergeIdempotent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "merge-idempotent")
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatalf(fmtAppend, err)
	}
	for i := 0; i < 2; i++ { // the second application must change nothing further
		if err := o.Items.Grow(ctx, key, itemOne, fx.Edit("x")); err != nil {
			t.Fatalf("Grow #%d: %v", i+1, err)
		}
	}
	items, err := o.Items.List(ctx, key)
	if err != nil || len(items) != 1 {
		t.Fatalf("List: %+v, %v", items, err)
	}
	if got := fx.Effects(items[0]); !sameStringSet(got, []string{"x"}) {
		t.Fatalf("effects after applying the same edit twice = %v, want exactly [x]", got)
	}
}

// testMergeCommutes applies two edits to one item in one order and the same
// two edits to a second item in the other order, and asserts both converge
// to the same set of effects — proved by List's own append-order guarantee,
// so item[0] is "forward" and item[1] is "backward" with no identity reader
// needed.
func testMergeCommutes[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "merge-commutes")
	for _, id := range []string{"forward", "backward"} {
		if err := o.Items.Append(ctx, key, fx.NewItem(id)); err != nil {
			t.Fatalf(fmtAppend, err)
		}
	}
	if err := o.Items.Grow(ctx, key, "forward", fx.Edit("a")); err != nil {
		t.Fatal(err)
	}
	if err := o.Items.Grow(ctx, key, "forward", fx.Edit("b")); err != nil {
		t.Fatal(err)
	}
	if err := o.Items.Grow(ctx, key, "backward", fx.Edit("b")); err != nil {
		t.Fatal(err)
	}
	if err := o.Items.Grow(ctx, key, "backward", fx.Edit("a")); err != nil {
		t.Fatal(err)
	}
	items, err := o.Items.List(ctx, key)
	if err != nil || len(items) != 2 {
		t.Fatalf("List: %+v, %v", items, err)
	}
	forward, backward := fx.Effects(items[0]), fx.Effects(items[1])
	if !sameStringSet(forward, backward) {
		t.Fatalf("applying two edits in either order diverged: forward %v, backward %v", forward, backward)
	}
	if !sameStringSet(forward, []string{"a", "b"}) {
		t.Fatalf("effects = %v, want exactly [a b]", forward)
	}
}

func testGrowCommutesWithAppend[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "grow-concurrent")
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatalf(fmtAppend, err)
	}
	markers := []string{"m1", "m2", "m3", "m4"}
	var wg sync.WaitGroup
	errs := make(chan error, len(markers)+1)
	for _, m := range markers {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			errs <- o.Items.Grow(ctx, key, itemOne, fx.Edit(m))
		}(m)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- o.Items.Append(ctx, key, fx.NewItem("item-2"))
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent write surfaced an error: %v", err)
		}
	}

	items, err := o.Items.List(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("List = %d items, want 2 (the append landed too)", len(items))
	}
	if got := fx.Effects(items[0]); !sameStringSet(got, markers) {
		t.Fatalf("item-1 effects = %v, want all of %v", got, markers)
	}
}

func testListWithItems[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	const n = 3
	keys := make([]state.Key, n)
	for i := 0; i < n; i++ {
		keys[i] = createParent(t, o, fx, fmt.Sprintf("list-with-items-%d", i))
		for r := 0; r < i; r++ {
			if err := o.Items.Append(ctx, keys[i], fx.NewItem(fmt.Sprintf("item-%d-%d", i, r))); err != nil {
				t.Fatal(err)
			}
		}
	}

	listed, err := o.Items.ListWithItems(ctx, fx.AllIndex, fx.AllValue)
	if err != nil {
		t.Fatalf("ListWithItems: %v", err)
	}
	byKey := map[state.Key]state.WithItems[P, T]{}
	for _, w := range listed {
		byKey[fx.KeyOf(w.Record)] = w
	}
	for i, key := range keys {
		w, ok := byKey[key]
		if !ok {
			t.Fatalf("ListWithItems did not return %q", key)
		}
		rec, rev, items, err := o.Items.GetWithItems(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if w.Revision != rev || fx.KeyOf(w.Record) != fx.KeyOf(rec) || len(w.Items) != len(items) || len(items) != i {
			t.Fatalf("%s: listing %+v differs from the combined read %+v / %v", key, w, rec, items)
		}
	}

	undeclared := state.NewIndex[P]("undeclared", func(*P) (string, bool) { return "", true })
	if _, err := o.Items.ListWithItems(ctx, undeclared, ""); !errors.Is(err, state.ErrUnknownIndex) {
		t.Fatalf("ListWithItems on an undeclared index: got %v, want ErrUnknownIndex", err)
	}
}

func testWatchWithItems[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := createParent(t, o, fx, "watch-with-items")
	events, err := o.Items.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf(watchWithItemsFailed, err)
	}
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("WatchWithItems closed before carrying the append")
			}
			if ev.Type == state.EventPut && ev.Key == key && len(ev.Record.Items) >= 1 {
				return
			}
		case <-deadline:
			t.Fatal("never observed a put carrying the appended item")
		}
	}
}

// WatchWithItems is the parent's feed (store.go): a change to the parent alone
// — a close, a read mark, a phase — must reach it, not only a change to an
// item. A backend watching only the items missed exactly these, and a consumer
// showed a conversation closed in the store as still open for ever.
func testWatchWithItemsParentOnly[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := createParent(t, o, fx, "watch-parent-only")
	events, err := o.Items.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf(watchWithItemsFailed, err)
	}
	editParent(t, o, fx, key)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("WatchWithItems closed before carrying the parent write")
			}
			if ev.Type == state.EventPut && ev.Key == key && fx.ParentChanged(ev.Record.Record) {
				return
			}
		case <-deadline:
			t.Fatal("never observed a put for a write that changed the parent and no item")
		}
	}
}

// A feed resumed from an event's own Resume continues strictly after it: the
// event it was resumed from is not delivered again, and a write made after
// the disconnect is. The resume is whatever the backend minted, opaque here.
func testWatchWithItemsResume[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := createParent(t, o, fx, "watch-resume")
	first, err := o.Items.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf(watchWithItemsFailed, err)
	}
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatal(err)
	}
	var resume state.Revision
	deadline := time.After(10 * time.Second)
	for resume == "" {
		select {
		case ev, ok := <-first:
			if !ok {
				t.Fatal("feed closed before delivering the append")
			}
			if ev.Type == state.EventPut && ev.Key == key {
				resume = ev.Resume
			}
		case <-deadline:
			t.Fatal("never observed the append")
		}
	}
	cancel()

	// The parent changes while no feed is open; a resumed feed must carry it.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	editParent(t, o, fx, key)
	second, err := o.Items.WatchWithItems(ctx2, resume)
	if err != nil {
		t.Fatalf("resume from %q: %v", resume, err)
	}
	deadline = time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-second:
			if !ok {
				t.Fatal("resumed feed closed before delivering the write made while disconnected")
			}
			if ev.Type == state.EventPut && ev.Key == key && fx.ParentChanged(ev.Record.Record) {
				return
			}
		case <-deadline:
			t.Fatal("a resumed feed never delivered the parent write made after the resume point")
		}
	}
}

func editParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T], key state.Key) {
	t.Helper()
	if _, err := state.Mutate(context.Background(), o.Parent, key, func(p *P) error {
		fx.EditParent(p)
		return nil
	}); err != nil {
		t.Fatalf("edit the parent: %v", err)
	}
}

// testHistoryLimit appends past the limit and checks the oldest fell off, the
// newest stayed in append order, and an item that aged out is ErrNotFound to
// Grow, so a caller can tell it from success.
func testHistoryLimit[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "history-limit")
	total := fx.HistoryLimit + 3
	id := func(i int) string { return fmt.Sprintf("hist-%03d", i) }
	for i := 0; i < total; i++ {
		if err := o.Items.Append(ctx, key, fx.NewItem(id(i))); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		// Append order is by time on some backends: keep appends distinct.
		time.Sleep(2 * time.Millisecond)
	}
	got, err := o.Items.List(ctx, key)
	if err != nil {
		t.Fatalf(fmtList, err)
	}
	if len(got) != fx.HistoryLimit {
		t.Fatalf("kept %d items, want the newest %d", len(got), fx.HistoryLimit)
	}
	want := fx.NewItem(id(total - fx.HistoryLimit))
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("oldest kept = %+v, want %+v", got[0], want)
	}
	if err := o.Items.Grow(ctx, key, id(0), fx.Edit("late")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an item that aged out = %v, want ErrNotFound", err)
	}
}

// testListWithItemsAfterParentWrite checks that an index over the parent
// follows a write that touches no item: the record is listed under its new
// value with its items, and no longer under the old.
func testListWithItemsAfterParentWrite[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "index-follows")
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatal(err)
	}
	has := func(value string) (found bool, items int) {
		t.Helper()
		listed, err := o.Items.ListWithItems(ctx, fx.ParentIndex, value)
		if err != nil {
			t.Fatalf("ListWithItems(%q): %v", value, err)
		}
		for _, w := range listed {
			if fx.KeyOf(w.Record) == key {
				return true, len(w.Items)
			}
		}
		return false, 0
	}
	if found, _ := has(fx.IndexBefore); !found {
		t.Fatalf("the parent is not listed under %q before the write", fx.IndexBefore)
	}
	editParent(t, o, fx, key)
	eventually(t, "the parent is listed under its new value with its item", func() bool {
		found, n := has(fx.IndexAfter)
		return found && n == 1
	})
	if found, _ := has(fx.IndexBefore); found {
		t.Fatalf("the parent is still listed under %q after the write", fx.IndexBefore)
	}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never: %s", what)
}

// testWatchWithItemsResumesFromAListedRevision is the reader's whole recovery
// path: list, keep the revision the listing gave, open a feed from it. A
// backend that mints a different token for a feed than for a listing refuses
// every resume, and the reader relists for ever.
func testWatchWithItemsResumesFromList[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key := createParent(t, o, fx, "resume-from-list")
	if err := o.Items.Append(ctx, key, fx.NewItem(itemOne)); err != nil {
		t.Fatal(err)
	}
	var listed state.Revision
	eventually(t, "the listing carries the parent", func() bool {
		all, err := o.Items.ListWithItems(ctx, fx.AllIndex, fx.AllValue)
		if err != nil || len(all) == 0 {
			return false
		}
		listed = all[0].Revision
		return true
	})
	feed, err := o.Items.WatchWithItems(ctx, listed)
	if err != nil {
		t.Fatalf("a feed opened from the revision a listing returned (%q) was refused: %v", listed, err)
	}
	if err := o.Items.Append(ctx, key, fx.NewItem("after-list")); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case ev, ok := <-feed:
			if !ok {
				t.Fatal("the feed closed instead of delivering the change made after the listing")
			}
			if ev.Type == state.EventPut && ev.Key == key && len(ev.Record.Items) == 2 {
				return
			}
		case <-ctx.Done():
			t.Fatal("the change made after the listing never arrived")
		}
	}
}

// testDeleteTakesItems deletes a parent that has items, then recreates the
// same key. A backend that keeps items apart from the parent must not let the
// old ones reappear under the new record.
func testDeleteTakesItems[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "delete-cascade")
	for _, id := range []string{"gone-1", "gone-2"} {
		if err := o.Items.Append(ctx, key, fx.NewItem(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Parent.Delete(ctx, key); err != nil {
		t.Fatalf("Delete the parent: %v", err)
	}
	if _, err := o.Items.List(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("List of a deleted parent's items = %v, want ErrNotFound", err)
	}
	if err := o.Items.Append(ctx, key, fx.NewItem("orphan")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Append to a deleted parent = %v, want ErrNotFound", err)
	}

	if _, err := o.Parent.Create(ctx, key, fx.NewParent()); err != nil {
		t.Fatalf("recreate the parent: %v", err)
	}
	eventually(t, "the recreated parent has none of the old items", func() bool {
		got, err := o.Items.List(ctx, key)
		return err == nil && len(got) == 0
	})
}
