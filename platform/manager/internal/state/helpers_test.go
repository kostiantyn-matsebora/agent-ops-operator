package state

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// casStore is the smallest Collection the write helpers need: one record, a
// revision counter, and a hook to lose a race on demand. It is generic, the way
// Mutate is. The conformance suite owns the contract; this only gives Mutate
// something to compare against.
type casStore[T any] struct {
	Collection[T] // every method the helpers do not touch panics through the nil embed
	rec           T
	rev           int
	found         bool
	// beforeUpdate runs ahead of each Update, to let another writer in.
	beforeUpdate func(s *casStore[T])
	updates      int
}

func newCASStore[T any](rec T) *casStore[T] {
	return &casStore[T]{rec: rec, rev: 1, found: true}
}

func (c *casStore[T]) Get(_ context.Context, _ Key) (T, Revision, error) {
	if !c.found {
		var zero T
		return zero, "", ErrNotFound
	}
	return c.rec, Revision(strconv.Itoa(c.rev)), nil
}

func (c *casStore[T]) Update(_ context.Context, _ Key, rev Revision, rec T) (Revision, error) {
	if c.beforeUpdate != nil {
		c.beforeUpdate(c)
	}
	if string(rev) != strconv.Itoa(c.rev) {
		return "", ErrConflict
	}
	c.rec, c.rev = rec, c.rev+1
	c.updates++
	return Revision(strconv.Itoa(c.rev)), nil
}

func TestMutateWritesTheChangeWithTheRevisionItRead(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c", Phase: PhaseIdle})
	got, err := Mutate(context.Background(), s, "c", func(r *ConversationRecord) error {
		r.Phase = PhaseWorking
		return nil
	})
	if err != nil || got.Phase != PhaseWorking || s.rec.Phase != PhaseWorking || s.updates != 1 {
		t.Fatalf("got (%+v, %v), stored %+v after %d updates", got, err, s.rec, s.updates)
	}
}

func TestMutateWritesNothingWhenNothingChanged(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c", Phase: PhaseIdle})
	if _, err := Mutate(context.Background(), s, "c", func(*ConversationRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if s.updates != 0 || s.rev != 1 {
		t.Fatalf("an unchanged record must not be written: %d updates, revision %d", s.updates, s.rev)
	}
}

// The loser of a race reads again and applies its change to what the winner
// wrote, so neither write is lost: the property a merge patch gave for free.
func TestMutateRereadsAfterLosingARaceAndKeepsTheWinnersWrite(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c"})
	raced := false
	s.beforeUpdate = func(c *casStore[ConversationRecord]) {
		if !raced {
			raced = true
			c.rec.Brief = "from the other writer"
			c.rev++
		}
	}
	attempts := 0
	_, err := Mutate(context.Background(), s, "c", func(r *ConversationRecord) error {
		attempts++
		r.Phase = PhaseQueued
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || s.rec.Brief != "from the other writer" || s.rec.Phase != PhaseQueued {
		t.Fatalf("attempts=%d stored=%+v, want both writes kept after one retry", attempts, s.rec)
	}
}

func TestMutateGivesUpAfterTheBoundAndWrapsTheConflict(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c"})
	s.beforeUpdate = func(c *casStore[ConversationRecord]) { c.rev++ } // always loses
	_, err := Mutate(context.Background(), s, "c", func(r *ConversationRecord) error {
		r.Phase = PhaseQueued
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want a wrapped ErrConflict, got %v", err)
	}
}

func TestMutateStopsOnTheCallersError(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c"})
	boom := errors.New("boom")
	if _, err := Mutate(context.Background(), s, "c", func(*ConversationRecord) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v, want the caller's error", err)
	}
	if s.updates != 0 {
		t.Fatal("nothing may be written after the caller's error")
	}
}

func TestMutateReportsAMissingRecord(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c"})
	s.found = false
	_, err := Mutate(context.Background(), s, "c", func(*ConversationRecord) error { return nil })
	if !errors.Is(err, ErrNotFound) || IgnoreNotFound(err) != nil {
		t.Fatalf("got %v, want ErrNotFound that IgnoreNotFound swallows", err)
	}
	other := fmt.Errorf("other")
	if IgnoreNotFound(other) != other {
		t.Fatal("IgnoreNotFound must pass every other error through")
	}
}

func TestPatchReplacesTheCallersCopyWithWhatWasStored(t *testing.T) {
	s := newCASStore(ConversationRecord{Name: "c", Brief: "stored"})
	stale := ConversationRecord{Name: "c", Brief: "stale"}
	if err := Patch(context.Background(), s, &stale, func(r *ConversationRecord) { r.Phase = PhaseIdle }); err != nil {
		t.Fatal(err)
	}
	if stale.Brief != "stored" || stale.Phase != PhaseIdle {
		t.Fatalf("the caller's copy must read as the stored state, got %+v", stale)
	}
}

func TestSetConditionFollowsTheMetaSemantics(t *testing.T) {
	r := ConversationRecord{}
	if !r.SetCondition(Condition{Type: "T", Status: ConditionFalse, Reason: "A"}) {
		t.Fatal("a new condition is a change")
	}
	stamped := r.FindCondition("T").LastTransitionTime
	if stamped.IsZero() {
		t.Fatal("a new condition is stamped")
	}
	time.Sleep(2 * time.Millisecond)
	if !r.SetCondition(Condition{Type: "T", Status: ConditionFalse, Reason: "B"}) {
		t.Fatal("a new reason is a change")
	}
	if !r.FindCondition("T").LastTransitionTime.Equal(stamped) {
		t.Fatal("the transition time moves only when the status does")
	}
	if r.SetCondition(Condition{Type: "T", Status: ConditionFalse, Reason: "B"}) {
		t.Fatal("restating a condition changes nothing")
	}
	r.SetCondition(Condition{Type: "T", Status: ConditionTrue, Reason: "B"})
	if !r.FindCondition("T").LastTransitionTime.After(stamped) {
		t.Fatal("a status change moves the transition time")
	}
	if !r.RemoveCondition("T") || r.RemoveCondition("T") || r.FindCondition("T") != nil {
		t.Fatal("RemoveCondition reports a removal once")
	}
}

func TestThreadHelpersReadTheBindings(t *testing.T) {
	r := ConversationRecord{
		ChannelRefs:     []Ref{"c1", "c2"},
		Threads:         []ThreadBinding{{Channel: "c1", ThreadID: "t1"}, {Channel: "c2"}},
		ThreadsArchived: []string{"c1"},
	}
	if id := r.ThreadFor("c1"); id == nil || *id != "t1" {
		t.Fatalf("ThreadFor(c1) = %v", id)
	}
	if r.ThreadFor("c2") != nil || r.ThreadFor("nope") != nil {
		t.Fatal("a claim-only binding and an unbound channel have no thread")
	}
	if !r.BoundTo("c1") || r.BoundTo("nope") || !r.ThreadArchived("c1") || r.ThreadArchived("c2") {
		t.Fatal("BoundTo / ThreadArchived disagree with the record")
	}
}

func TestWatermarksAreOpaqueStringsComparedAtFullPrecision(t *testing.T) {
	at := "2026-10-31T10:00:00.123456789Z"
	parsed, ok := ParseWatermark(at)
	if !ok || parsed.Nanosecond() != 123456789 {
		t.Fatalf("ParseWatermark(%q) = (%v, %v)", at, parsed, ok)
	}
	if _, ok := ParseWatermark("garbage"); ok {
		t.Fatal("a malformed watermark is not a time")
	}
	b := ThreadBinding{ReadTracked: true, ReadAt: &at}
	before := parsed.Add(-time.Nanosecond)
	if b.Unread(&before, "") {
		t.Fatal("activity older than the mark is read")
	}
	after := parsed.Add(time.Nanosecond)
	if !b.Unread(&after, "") {
		t.Fatal("activity one nanosecond past the mark is unread")
	}
	if (&ThreadBinding{ReadAt: &at}).Unread(&after, "") {
		t.Fatal("an untracked binding predates the mechanism and reads as read")
	}
}

// Mutate is generic over the record: the same loop serves a kind that is not a
// conversation, and creates nothing when the key is absent.
func TestMutateIsGenericOverTheRecordType(t *testing.T) {
	s := newCASStore(CooldownWindow{})
	raced := false
	s.beforeUpdate = func(c *casStore[CooldownWindow]) {
		if !raced {
			raced = true
			c.rec.Entries = append(c.rec.Entries, CooldownEntry{InitiationID: "theirs"})
			c.rev++
		}
	}
	got, err := Mutate(context.Background(), s, "src", func(w *CooldownWindow) error {
		w.Entries = append(w.Entries, CooldownEntry{InitiationID: "mine"})
		return nil
	})
	if err != nil || len(got.Entries) != 2 || s.updates != 1 {
		t.Fatalf("got (%+v, %v) after %d updates, want the winner's entry kept and ours added", got, err, s.updates)
	}

	str := newCASStore("")
	str.found = false
	if _, err := Mutate(context.Background(), str, "k", func(*string) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound for an absent string record", err)
	}
}
