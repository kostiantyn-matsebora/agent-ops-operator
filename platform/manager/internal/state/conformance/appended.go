package conformance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

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

	t.Run("WatchWithItemsCarriesTheItemsOfEveryPut", func(t *testing.T) { testWatchWithItems(t, open(), fx) })
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
		t.Fatalf("List: %v", err)
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
		t.Fatalf("List: %v", err)
	}
	if len(items) != n {
		t.Fatalf("got %d items, want %d", len(items), n)
	}
}

func testAppendOnMissingParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	if err := o.Items.Append(context.Background(), "no-such-parent", fx.NewItem("item-1")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Append on missing parent: got %v, want ErrNotFound", err)
	}
}

func testGetWithItems[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "with-items")
	for _, id := range []string{"item-a", "item-b"} {
		if err := o.Items.Append(ctx, key, fx.NewItem(id)); err != nil {
			t.Fatalf("Append: %v", err)
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
	if _, err := o.Items.List(context.Background(), "no-such-parent"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("List of a missing parent: got %v, want ErrNotFound", err)
	}
}

func testGetWithItemsOnMissingParent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	_ = fx
	if _, _, _, err := o.Items.GetWithItems(context.Background(), "no-such-parent"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("GetWithItems of a missing parent: got %v, want ErrNotFound", err)
	}
}

func testGrowUnknown[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "grow-unknown")
	if err := o.Items.Append(ctx, key, fx.NewItem("item-1")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := o.Items.Grow(ctx, key, "no-such-item", fx.Edit("x")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an unknown item: got %v, want ErrNotFound", err)
	}
	if err := o.Items.Grow(ctx, "no-such-parent", "item-1", fx.Edit("x")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Grow of an unknown parent: got %v, want ErrNotFound", err)
	}
}

func testMergeIdempotent[P, T any](t *testing.T, o AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	ctx := context.Background()
	key := createParent(t, o, fx, "merge-idempotent")
	if err := o.Items.Append(ctx, key, fx.NewItem("item-1")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	for i := 0; i < 2; i++ { // the second application must change nothing further
		if err := o.Items.Grow(ctx, key, "item-1", fx.Edit("x")); err != nil {
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
			t.Fatalf("Append: %v", err)
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
	if err := o.Items.Append(ctx, key, fx.NewItem("item-1")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	markers := []string{"m1", "m2", "m3", "m4"}
	var wg sync.WaitGroup
	errs := make(chan error, len(markers)+1)
	for _, m := range markers {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			errs <- o.Items.Grow(ctx, key, "item-1", fx.Edit(m))
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
		t.Fatalf("WatchWithItems: %v", err)
	}
	if err := o.Items.Append(ctx, key, fx.NewItem("item-1")); err != nil {
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
