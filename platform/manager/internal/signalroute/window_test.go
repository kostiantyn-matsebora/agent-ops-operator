package signalroute

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// memCollection is the smallest Collection the window and receipt helpers
// need: keyed records with a revision counter. Everything else panics through
// the nil embed, so a test learns of an unexpected call.
type memCollection[T any] struct {
	state.Collection[T]
	recs map[state.Key]T
	revs map[state.Key]int
	// gone makes Create answer ErrNotFound, the way a vanished source does.
	gone bool
}

func newMem[T any]() *memCollection[T] {
	return &memCollection[T]{recs: map[state.Key]T{}, revs: map[state.Key]int{}}
}

func (m *memCollection[T]) Get(_ context.Context, k state.Key) (T, state.Revision, error) {
	rec, ok := m.recs[k]
	if !ok {
		var zero T
		return zero, "", state.ErrNotFound
	}
	return rec, state.Revision(strconv.Itoa(m.revs[k])), nil
}

func (m *memCollection[T]) Create(_ context.Context, k state.Key, rec T) (state.Revision, error) {
	if m.gone {
		return "", state.ErrNotFound
	}
	if _, ok := m.recs[k]; ok {
		return "", state.ErrAlreadyExists
	}
	m.recs[k], m.revs[k] = rec, 1
	return "1", nil
}

func (m *memCollection[T]) Update(_ context.Context, k state.Key, rev state.Revision, rec T) (state.Revision, error) {
	if _, ok := m.recs[k]; !ok {
		return "", state.ErrNotFound
	}
	if string(rev) != strconv.Itoa(m.revs[k]) {
		return "", state.ErrConflict
	}
	m.recs[k], m.revs[k] = rec, m.revs[k]+1
	return state.Revision(strconv.Itoa(m.revs[k])), nil
}

func TestCooldownWindowReadsEmptyWhenNothingIsRecorded(t *testing.T) {
	got, err := GetCooldownWindow(context.Background(), newMem[state.CooldownWindow](), "s")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v, want an empty window", got, err)
	}
}

func TestCooldownWindowReplaceRoundTripsAndReplacesWhole(t *testing.T) {
	ctx, c := context.Background(), newMem[state.CooldownWindow]()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first := []state.CooldownEntry{{InitiationID: "a", At: at}, {InitiationID: "b", At: at}}
	if err := PutCooldownWindow(ctx, c, "s", first); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := PutCooldownWindow(ctx, c, "s", first[:1]); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := GetCooldownWindow(ctx, c, "s")
	if err != nil || len(got) != 1 || got[0] != first[0] {
		t.Fatalf("got %v, %v, want only the replacing entry", got, err)
	}
}

func TestCooldownWindowOfAGoneSourceIsNotFound(t *testing.T) {
	c := newMem[state.CooldownWindow]()
	c.gone = true
	if err := PutCooldownWindow(context.Background(), c, "s", nil); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCountReceiptCountsEachAndKeepsTheLatest(t *testing.T) {
	ctx, c := context.Background(), newMem[state.Received]()
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	for _, at := range []time.Time{t1, t2} {
		if err := CountReceipt(ctx, c, "s", at); err != nil {
			t.Fatalf("count: %v", err)
		}
	}
	got, err := GetReceipts(ctx, c, "s")
	if err != nil || got.Total != 2 || got.Last == nil || !got.Last.Equal(t2) {
		t.Fatalf("got %+v, %v, want 2 receipts, last %v", got, err, t2)
	}
}

func TestCountReceiptLosesNoIncrementToARace(t *testing.T) {
	ctx, c := context.Background(), newMem[state.Received]()
	_ = CountReceipt(ctx, c, "s", time.Now())
	racing := &racyReceipts{memCollection: c}
	if err := CountReceipt(ctx, racing, "s", time.Now()); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got, _ := GetReceipts(ctx, c, "s"); got.Total != 3 {
		t.Fatalf("total = %d, want 3 (one racing writer, one retried)", got.Total)
	}
}

// racyReceipts lets another writer in before the first Update.
type racyReceipts struct {
	*memCollection[state.Received]
	raced bool
}

func (r *racyReceipts) Update(ctx context.Context, k state.Key, rev state.Revision, rec state.Received) (state.Revision, error) {
	if !r.raced {
		r.raced = true
		other := r.recs[k]
		other.Total++
		r.recs[k], r.revs[k] = other, r.revs[k]+1
	}
	return r.memCollection.Update(ctx, k, rev, rec)
}

// racyCreate simulates a second writer's Create winning a race for a key
// that held NOTHING yet: the first Get/Mutate attempt reads ErrNotFound, but
// by the time this writer's own Create runs, the key already exists —
// state.ErrAlreadyExists, not a conflict, since Create carries no revision.
// CountReceipt/PutCooldownWindow must retry through Mutate rather than fail.
type racyCreate[T any] struct {
	*memCollection[T]
	raced bool
}

func (r *racyCreate[T]) Create(ctx context.Context, k state.Key, rec T) (state.Revision, error) {
	if !r.raced {
		r.raced = true
		// The other writer's create lands first, with whatever it had.
		_, _ = r.memCollection.Create(ctx, k, rec)
		return "", state.ErrAlreadyExists
	}
	return r.memCollection.Create(ctx, k, rec)
}

func TestCountReceiptRetriesWhenAnotherWriterCreatesTheCounterFirst(t *testing.T) {
	ctx := context.Background()
	racing := &racyCreate[state.Received]{memCollection: newMem[state.Received]()}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := CountReceipt(ctx, racing, "s", at); err != nil {
		t.Fatalf("count: %v", err)
	}
	// Both this writer's increment and the racing writer's own Create landed:
	// the racing Create wrote Total=1 (the record this call passed in), and
	// the retried Mutate bumped it once more.
	got, err := GetReceipts(ctx, racing, "s")
	if err != nil || got.Total != 2 {
		t.Fatalf("got %+v, %v, want 2 (the racing create's write, plus this writer's retried bump)", got, err)
	}
}

func TestPutCooldownWindowRetriesWhenAnotherWriterCreatesTheWindowFirst(t *testing.T) {
	ctx := context.Background()
	racing := &racyCreate[state.CooldownWindow]{memCollection: newMem[state.CooldownWindow]()}
	entries := []state.CooldownEntry{{InitiationID: "a", At: time.Now()}}
	if err := PutCooldownWindow(ctx, racing, "s", entries); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := GetCooldownWindow(ctx, racing, "s")
	if err != nil || len(got) != 1 || got[0] != entries[0] {
		t.Fatalf("got %v, %v, want the replacing entry to have won the retry", got, err)
	}
}

func TestReceiptsOfAGoneSourceReadZeroAndCountIsNotFound(t *testing.T) {
	c := newMem[state.Received]()
	c.gone = true
	if got, err := GetReceipts(context.Background(), c, "s"); err != nil || got.Total != 0 || got.Last != nil {
		t.Fatalf("got %+v, %v, want zero", got, err)
	}
	if err := CountReceipt(context.Background(), c, "s", time.Now()); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
