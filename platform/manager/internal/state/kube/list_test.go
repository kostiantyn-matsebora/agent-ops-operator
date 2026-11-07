// Task 2.2: every declared conversation index, plus ErrUnknownIndex for
// anything outside them.
package kube

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func createForList(t *testing.T, ctx context.Context, s state.Stores, name, conversationID, pipelineRef, phase string) {
	t.Helper()
	rec := fixtureRecord(name)
	rec.ConversationID = conversationID
	rec.PipelineRef = pipelineRef
	rec.Phase = phase
	if _, err := s.Conversations.Create(ctx, state.Key(name), rec); err != nil {
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

	got, err := s.Conversations.List(ctx, state.ByConversationID, "id-match")
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

	got, err := s.Conversations.List(ctx, state.ByPipelineAndPhase, state.IndexValue("pipe-x", "Working"))
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

	got, err := s.Conversations.List(ctx, state.ByPhase, "Working")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"conv-a", "conv-b"}
	if names := namesOf(t, got); !equalStrings(names, want) {
		t.Fatalf("ByPhase: got %v, want %v", names, want)
	}
}

// TestListEveryDeclaredIndexIsServed walks the conversation declaration's own
// indexes (not a locally hand-kept copy), so an index declared there without
// being served fails here instead of silently matching nothing only on this one
// backend. Each index must list the one fixture record under its own value.
func TestListEveryDeclaredIndexIsServed(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	createForList(t, ctx, s, "conv-a", "id-1", "pipe-x", "Working")
	rec := fixtureRecord("conv-a")
	rec.ConversationID, rec.PipelineRef, rec.Phase = "id-1", "pipe-x", "Working"

	for _, idx := range state.StoreDeclarations.Conversations.Indexes {
		value, listed := idx.Value(&rec)
		if !listed {
			t.Fatalf("fixture is not listed under %s", idx.Name())
		}
		got, err := s.Conversations.List(ctx, idx, value)
		if err != nil || len(got) != 1 || got[0].Name != "conv-a" {
			t.Fatalf("List(%s, %q) = %v, %v; want the fixture", idx.Name(), value, namesOf(t, got), err)
		}
	}
}

func TestListWithUnknownIndexFails(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	undeclared := state.NewIndex("not-a-declared-index", func(*state.ConversationRecord) (string, bool) { return "", true })
	_, err := s.Conversations.List(ctx, undeclared, "")
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
