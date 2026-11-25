package namewords

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// fakeConversations is the minimal state.Collection[state.ConversationRecord]
// AllocateName needs: List(ByNameBase) answers whatever this fake already
// holds under a base, and Create mints an ID the way a real backend does —
// everything else panics through the nil embed, so a test learns of a call
// AllocateName is not supposed to make.
type fakeConversations struct {
	state.Collection[state.ConversationRecord]
	byName map[state.Key]state.ConversationRecord
	nextID int
}

func newFakeConversations() *fakeConversations {
	return &fakeConversations{byName: map[state.Key]state.ConversationRecord{}}
}

func (f *fakeConversations) List(_ context.Context, idx state.Index[state.ConversationRecord], value string) ([]state.ConversationRecord, error) {
	var out []state.ConversationRecord
	for _, rec := range f.byName {
		if v, ok := idx.Value(&rec); ok && v == value {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (f *fakeConversations) Create(_ context.Context, key state.Key, rec state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	if _, ok := f.byName[key]; ok {
		var zero state.ConversationRecord
		return zero, "", state.ErrAlreadyExists
	}
	f.nextID++
	rec.ID = "fake-id-" + strconv.Itoa(f.nextID)
	f.byName[key] = rec
	return rec, "1", nil
}

// AllocateName's own three callers never read the record's ID off the local
// *state.ConversationRecord before this change — the backend minted it and
// nobody ever asked for it back. This proves both halves of the fix: the
// return value carries the minted ID, and *rec is brought up to date with it
// too, so a caller still holding rec after the call sees the same thing.
func TestAllocateNameReturnsARecordWithTheBackendMintedID(t *testing.T) {
	store := newFakeConversations()
	rec := &state.ConversationRecord{ProfileRef: "stub"}
	created, err := AllocateName(context.Background(), store, "task", "hello-world", rec)
	if err != nil {
		t.Fatalf("AllocateName: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("AllocateName returned a record with an empty ID")
	}
	if created.Name != "task-hello-world" {
		t.Fatalf("Name = %q, want the bare base name", created.Name)
	}
	if rec.ID != created.ID || rec.Name != created.Name {
		t.Fatalf("rec = %+v after AllocateName, want it brought up to date with the returned %+v", rec, created)
	}
}

func TestAllocateNameSecondCallOnTheSameBaseGetsADistinctID(t *testing.T) {
	store := newFakeConversations()
	first, err := AllocateName(context.Background(), store, "member", "alpha", &state.ConversationRecord{ProfileRef: "stub"})
	if err != nil {
		t.Fatalf("first AllocateName: %v", err)
	}
	second, err := AllocateName(context.Background(), store, "member", "alpha", &state.ConversationRecord{ProfileRef: "stub"})
	if err != nil {
		t.Fatalf("second AllocateName: %v", err)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("first.ID = %q, second.ID = %q, want both non-empty and distinct", first.ID, second.ID)
	}
	if second.Name != "member-alpha-2" {
		t.Fatalf("second.Name = %q, want the next free suffix", second.Name)
	}
}

// alwaysConflictConversations is a Collection whose List always succeeds
// empty and whose Create always refuses with ErrAlreadyExists — every
// attempt collides, as a genuine race between concurrent creators would,
// without ever actually persisting a record.
type alwaysConflictConversations struct {
	state.Collection[state.ConversationRecord]
}

func (alwaysConflictConversations) List(context.Context, state.Index[state.ConversationRecord], string) ([]state.ConversationRecord, error) {
	return nil, nil
}

func (alwaysConflictConversations) Create(context.Context, state.Key, state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	return state.ConversationRecord{}, "", state.ErrAlreadyExists
}

// A genuine race retries through sleepCtx's own backoff (desynchronizing
// this attempt's List from every other loser's) — a context that ends
// while that retry is waiting must fail AllocateName with the context's own
// error, never swallow it and try again.
func TestAllocateNamePropagatesACanceledContextFromTheRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AllocateName(ctx, alwaysConflictConversations{}, "task", "hello-world", &state.ConversationRecord{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled from the retry backoff, got %v", err)
	}
}

// listFailsConversations is a Collection whose List always fails outright —
// never a conflict, a genuine read failure.
type listFailsConversations struct {
	state.Collection[state.ConversationRecord]
}

var errListBoom = errors.New("list boom")

func (listFailsConversations) List(context.Context, state.Index[state.ConversationRecord], string) ([]state.ConversationRecord, error) {
	return nil, errListBoom
}

// AllocateName's own List, over ByNameBase, must fail the whole call rather
// than guess at a free name from nothing.
func TestAllocateNamePropagatesAListFailure(t *testing.T) {
	_, err := AllocateName(context.Background(), listFailsConversations{}, "task", "hello-world", &state.ConversationRecord{})
	if !errors.Is(err, errListBoom) {
		t.Fatalf("want the list failure wrapped and returned, got %v", err)
	}
}

// createFailsConversations is a Collection whose List always succeeds empty
// and whose Create always fails with something OTHER than ErrAlreadyExists
// — a genuine write failure, never the create-conflict retry path.
type createFailsConversations struct {
	state.Collection[state.ConversationRecord]
}

var errCreateBoom = errors.New("create boom")

func (createFailsConversations) List(context.Context, state.Index[state.ConversationRecord], string) ([]state.ConversationRecord, error) {
	return nil, nil
}

func (createFailsConversations) Create(context.Context, state.Key, state.ConversationRecord) (state.ConversationRecord, state.Revision, error) {
	return state.ConversationRecord{}, "", errCreateBoom
}

// A non-conflict Create failure must be returned verbatim, on the FIRST
// attempt, never retried as if it were a name collision.
func TestAllocateNamePropagatesANonConflictCreateFailure(t *testing.T) {
	_, err := AllocateName(context.Background(), createFailsConversations{}, "task", "hello-world", &state.ConversationRecord{})
	if !errors.Is(err, errCreateBoom) {
		t.Fatalf("want a non-conflict create failure returned verbatim, got %v", err)
	}
}
