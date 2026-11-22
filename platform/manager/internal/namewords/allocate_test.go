package namewords

import (
	"context"
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
