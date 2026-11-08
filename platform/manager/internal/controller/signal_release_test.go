package controller

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// releaseFake is a signals collection with hooks, enough to script a source
// recreated between the listing and the delete.
type releaseFake struct {
	state.Collection[state.SignalRecord]
	recs       map[state.Key]state.SignalRecord
	afterList  func()
	deleteErrs []error // consumed one per DeleteAt call
	deletes    int
	revs       map[state.Key]int // revision per key, absent = 1
	atDelete   func()            // runs before each DeleteAt, to rewrite a record mid-decision
}

func (f *releaseFake) rewrite(k state.Key, r state.SignalRecord) {
	f.recs[k] = r
	if f.revs == nil {
		f.revs = map[state.Key]int{}
	}
	f.revs[k] = f.rev(k) + 1
}

func (f *releaseFake) rev(k state.Key) int {
	if n, ok := f.revs[k]; ok {
		return n
	}
	return 1
}

func (f *releaseFake) List(_ context.Context, idx state.Index[state.SignalRecord], v string) ([]state.SignalRecord, error) {
	var out []state.SignalRecord
	for _, r := range f.recs {
		r := r
		if got, ok := idx.Value(&r); ok && got == v {
			out = append(out, r)
		}
	}
	if f.afterList != nil {
		f.afterList()
	}
	return out, nil
}

func (f *releaseFake) Get(_ context.Context, k state.Key) (state.SignalRecord, state.Revision, error) {
	r, ok := f.recs[k]
	if !ok {
		return r, "", state.ErrNotFound
	}
	return r, state.Revision(strconv.Itoa(f.rev(k))), nil
}

func (f *releaseFake) DeleteAt(_ context.Context, k state.Key, rev state.Revision) error {
	if f.atDelete != nil {
		hook := f.atDelete
		f.atDelete = nil
		hook()
	}
	if _, ok := f.recs[k]; !ok {
		return state.ErrNotFound
	}
	if string(rev) != strconv.Itoa(f.rev(k)) {
		return state.ErrConflict
	}
	f.deletes++
	if len(f.deleteErrs) > 0 {
		err := f.deleteErrs[0]
		f.deleteErrs = f.deleteErrs[1:]
		if err != nil {
			return err
		}
	}
	delete(f.recs, k)
	return nil
}

type windowFake struct {
	state.Collection[state.CooldownWindow]
	win map[state.Key]state.CooldownWindow
}

func (w *windowFake) Get(_ context.Context, k state.Key) (state.CooldownWindow, state.Revision, error) {
	r, ok := w.win[k]
	if !ok {
		return r, "", state.ErrNotFound
	}
	return r, "1", nil
}

func (w *windowFake) Update(_ context.Context, k state.Key, _ state.Revision, r state.CooldownWindow) (state.Revision, error) {
	w.win[k] = r
	return "2", nil
}

func newRelease(sigs ...state.SignalRecord) (*SourceReleaser, *releaseFake, *windowFake, *string) {
	f := &releaseFake{recs: map[state.Key]state.SignalRecord{}}
	for _, s := range sigs {
		f.recs[state.Key(s.Name)] = s
	}
	w := &windowFake{win: map[state.Key]state.CooldownWindow{
		"src":   {Entries: []state.CooldownEntry{{InitiationID: "x"}}},
		"other": {Entries: []state.CooldownEntry{{InitiationID: "y"}}},
	}}
	live := new(string) // identity of the source holding "src"; empty = none
	r := &SourceReleaser{Signals: f, Cooldowns: w, Backoff: time.Millisecond,
		Live: func(context.Context, string) (string, bool, error) { return *live, *live != "", nil }}
	return r, f, w, live
}

func sig(name, source, id string) state.SignalRecord {
	return state.SignalRecord{Name: name, SourceRef: source, SourceID: id}
}

func TestReleaseRemovesOneSourceAndNoOtherTwice(t *testing.T) {
	r, f, w, _ := newRelease(sig("a", "src", "1"), sig("b", "src", ""), sig("c", "other", "2"))
	for i := 0; i < 2; i++ {
		if err := r.Release(context.Background(), "src"); err != nil {
			t.Fatalf("release %d: %v", i, err)
		}
	}
	if len(f.recs) != 1 || f.recs["c"].Name != "c" {
		t.Fatalf("signals left = %v, want only the other source's", f.recs)
	}
	if len(w.win["src"].Entries) != 0 || len(w.win["other"].Entries) != 1 {
		t.Fatalf("windows = %v, want src cleared and other untouched", w.win)
	}
}

func TestReleaseOfASourceWithNoSignalsIsNotAnError(t *testing.T) {
	r, _, _, _ := newRelease()
	if err := r.Release(context.Background(), "gone"); err != nil {
		t.Fatalf("release: %v, want nil", err)
	}
}

func TestReleaseKeepsASuccessorsSignalsAndWindow(t *testing.T) {
	r, f, w, live := newRelease(sig("old", "src", "1"), sig("new", "src", "2"))
	*live = "2"
	if err := r.Release(context.Background(), "src"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := f.recs["old"]; ok {
		t.Fatal("the departed incarnation's signal survived")
	}
	if _, ok := f.recs["new"]; !ok {
		t.Fatal("the successor's signal was deleted")
	}
	if len(w.win["src"].Entries) != 1 {
		t.Fatalf("successor's window cleared: %v", w.win["src"])
	}
}

func TestReleaseSparesASourceRecreatedMidRelease(t *testing.T) {
	r, f, w, live := newRelease(sig("old", "src", "1"), sig("new", "src", "2"))
	// the name is free when the signals are listed; a successor appears before
	// the first delete is decided
	f.afterList = func() { *live = "2" }
	if err := r.Release(context.Background(), "src"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := f.recs["new"]; !ok {
		t.Fatal("a source recreated mid-release lost its signal")
	}
	if _, ok := f.recs["old"]; ok {
		t.Fatal("the departed incarnation's signal survived")
	}
	if len(w.win["src"].Entries) != 1 {
		t.Fatalf("recreated source's window cleared: %v", w.win["src"])
	}
}

func TestReleaseRetriesAConflictAndDecidesAgain(t *testing.T) {
	r, f, _, _ := newRelease(sig("a", "src", "1"))
	f.deleteErrs = []error{state.ErrConflict, state.ErrConflict, nil}
	if err := r.Release(context.Background(), "src"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if f.deletes != 3 || len(f.recs) != 0 {
		t.Fatalf("deletes = %d, left = %d, want 3 and none", f.deletes, len(f.recs))
	}
}

func TestReleaseGivesUpAfterBoundedConflicts(t *testing.T) {
	r, f, _, _ := newRelease(sig("a", "src", "1"))
	f.deleteErrs = []error{state.ErrConflict, state.ErrConflict, state.ErrConflict,
		state.ErrConflict, state.ErrConflict, state.ErrConflict}
	err := r.Release(context.Background(), "src")
	if !errors.Is(err, state.ErrConflict) || f.deletes != releaseMaxAttempts {
		t.Fatalf("err = %v after %d deletes, want ErrConflict after %d", err, f.deletes, releaseMaxAttempts)
	}
}

func TestReleaseDoesNotDeleteASignalRewrittenAfterTheDecision(t *testing.T) {
	r, f, _, live := newRelease(sig("a", "src", "1"))
	// the name is free at the first decision, then the signal is rewritten
	// under a successor before the delete lands
	f.atDelete = func() {
		*live = "2"
		f.rewrite("a", sig("a", "src", "2"))
	}
	if err := r.Release(context.Background(), "src"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := f.recs["a"]; !ok {
		t.Fatal("a signal rewritten after the decision was deleted on the stale revision")
	}
	if f.deletes != 0 {
		t.Fatalf("deletes = %d, want 0 once the re-decision spares it", f.deletes)
	}
}

func TestReleaseTreatsASignalGoneAtDeleteAsDone(t *testing.T) {
	r, f, _, _ := newRelease(sig("a", "src", "1"))
	f.atDelete = func() { delete(f.recs, "a") }
	if err := r.Release(context.Background(), "src"); err != nil {
		t.Fatalf("release: %v, want nil", err)
	}
}
