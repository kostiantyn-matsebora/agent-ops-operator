// The generic Appended scenarios (append order, concurrency, Grow's
// idempotence and commutativity, missing-parent behavior) now live in
// internal/state/conformance, driven by runFixture and run through
// TestStores (stores_test.go). What is left here is specific to this
// backend's own mapping of Run onto Conversation.status.runs[]: the
// lossless field-by-field round trip, and the bounded history every long-
// lived conversation's object is kept under.
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// TestAppendRunRoundTripsEveryField is Run's own lossless-mapping check, the
// same discipline fixtureRecord applies to ConversationRecord.
func TestAppendRunRoundTripsEveryField(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-run-fields")
	if _, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	startedAt := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	receivedAt := startedAt.Add(-time.Minute)
	exitCode := int32(1)

	want := state.Run{
		RunID: "run-full", JobKind: "task", Status: "failed",
		ExitCode: &exitCode, Result: "it broke",
		StartedAt: &startedAt, FinishedAt: &finishedAt,
		InputIDs: []string{"input-1", "input-2"}, DeliveryTracked: true,
		Delivered: []string{"chan-a"}, RoutedToParent: true,
		Inputs: []state.RecordedInput{
			{
				ID: "input-1", Type: "task", Text: "do the thing", Truncated: false,
				PayloadRef: "blob-1", Surface: "chan-a", Sender: "alice",
				Origin: "channel", ReceivedAt: &receivedAt,
			},
		},
	}
	if err := s.Runs.Append(ctx, state.Key(rec.Name), want); err != nil {
		t.Fatalf("Runs.Append: %v", err)
	}

	runs, err := s.Runs.List(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Runs.List: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	gotJSON, _ := json.MarshalIndent(runs[0], "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("run mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotJSON, wantJSON)
	}
}

// The stored history is bounded so a long-lived conversation cannot grow its
// object without limit: the oldest run falls off, order is kept.
func TestAppendRunKeepsOnlyTheNewestRuns(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-append-bound")
	if _, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 0; i < maxRunHistory+3; i++ {
		if err := s.Runs.Append(ctx, state.Key(rec.Name), state.Run{RunID: fmt.Sprintf("run-%d", i)}); err != nil {
			t.Fatalf("Runs.Append: %v", err)
		}
	}
	runs, err := s.Runs.List(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != maxRunHistory || runs[0].RunID != "run-3" || runs[len(runs)-1].RunID != fmt.Sprintf("run-%d", maxRunHistory+2) {
		t.Fatalf("want the newest %d runs in order, got %d starting %q", maxRunHistory, len(runs), runs[0].RunID)
	}
}
