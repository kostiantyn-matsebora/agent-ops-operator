// Task 2.2: every member of state.DeclaredIndexes, plus ErrUnknownIndex
// for anything outside it.
package kube

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func createForList(t *testing.T, ctx context.Context, s *Store, name, conversationID, pipelineRef, phase string) {
	t.Helper()
	rec := fixtureRecord(name)
	rec.ConversationID = conversationID
	rec.PipelineRef = pipelineRef
	rec.Phase = phase
	if _, err := s.Create(ctx, state.Key(name), rec); err != nil {
		t.Fatalf("Create %q: %v", name, err)
	}
}

func namesOf(t *testing.T, recs []state.ConversationRecord) []string {
	t.Helper()
	names := make([]string, len(recs))
	for i, r := range recs {
		names[i] = r.Name
	}
	sort.Strings(names)
	return names
}

func TestListByConversationID(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	createForList(t, ctx, s, "conv-a", "id-match", "pipe-1", "Working")
	createForList(t, ctx, s, "conv-b", "id-other", "pipe-1", "Working")
	createForList(t, ctx, s, "conv-c", "id-match", "pipe-2", "Idle")

	got, err := s.List(ctx, state.ByConversationID, state.ListKey{ConversationID: "id-match"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"conv-a", "conv-c"}
	if names := namesOf(t, got); !equalStrings(names, want) {
		t.Fatalf("ByConversationID: got %v, want %v", names, want)
	}
}

func TestListByPipelineAndPhase(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	createForList(t, ctx, s, "conv-a", "id-1", "pipe-x", "Working")
	createForList(t, ctx, s, "conv-b", "id-2", "pipe-x", "Idle")
	createForList(t, ctx, s, "conv-c", "id-3", "pipe-y", "Working")

	got, err := s.List(ctx, state.ByPipelineAndPhase, state.ListKey{PipelineRef: "pipe-x", Phase: "Working"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"conv-a"}
	if names := namesOf(t, got); !equalStrings(names, want) {
		t.Fatalf("ByPipelineAndPhase: got %v, want %v", names, want)
	}
}

func TestListByPhase(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	createForList(t, ctx, s, "conv-a", "id-1", "pipe-x", "Working")
	createForList(t, ctx, s, "conv-b", "id-2", "pipe-y", "Working")
	createForList(t, ctx, s, "conv-c", "id-3", "pipe-z", "Idle")

	got, err := s.List(ctx, state.ByPhase, state.ListKey{Phase: "Working"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"conv-a", "conv-b"}
	if names := namesOf(t, got); !equalStrings(names, want) {
		t.Fatalf("ByPhase: got %v, want %v", names, want)
	}
}

// TestListEveryDeclaredIndexIsServed walks state.DeclaredIndexes itself
// (not a locally hand-kept copy) — the same list store.go's own comment
// says internal/state/conformance walks, so an Index added there without
// a case in matchesIndex fails here instead of silently matching nothing
// only on this one backend.
func TestListEveryDeclaredIndexIsServed(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	createForList(t, ctx, s, "conv-a", "id-1", "pipe-x", "Working")

	for _, idx := range state.DeclaredIndexes {
		if _, err := s.List(ctx, idx, state.ListKey{ConversationID: "id-1", PipelineRef: "pipe-x", Phase: "Working"}); err != nil {
			t.Fatalf("List(%s): %v", idx, err)
		}
	}
}

func TestListWithUnknownIndexFails(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	_, err := s.List(ctx, state.Index("not-a-declared-index"), state.ListKey{})
	if !errors.Is(err, state.ErrUnknownIndex) {
		t.Fatalf("List with unknown index: got %v, want ErrUnknownIndex", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
