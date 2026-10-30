// Task 2.4: AppendRun/ListRuns.
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestListRunsReturnsAppendOrder(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-runs-order")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	const n = 5
	for i := 0; i < n; i++ {
		run := state.Run{RunID: fmt.Sprintf("run-%d", i), Status: "succeeded"}
		if err := s.AppendRun(ctx, state.Key(rec.Name), run); err != nil {
			t.Fatalf("AppendRun %d: %v", i, err)
		}
	}

	runs, err := s.ListRuns(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != n {
		t.Fatalf("got %d runs, want %d", len(runs), n)
	}
	for i, r := range runs {
		want := fmt.Sprintf("run-%d", i)
		if r.RunID != want {
			t.Fatalf("position %d: got RunID %q, want %q", i, r.RunID, want)
		}
	}
}

// TestAppendRunRoundTripsEveryField is Run's own lossless-mapping check,
// the same discipline fixtureRecord applies to ConversationRecord.
func TestAppendRunRoundTripsEveryField(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-run-fields")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
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
	if err := s.AppendRun(ctx, state.Key(rec.Name), want); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	runs, err := s.ListRuns(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
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

// TestAppendRunConcurrentNoConflictSurfaced: task 2.4's own scenario —
// two goroutines appending 10 runs each to one conversation at once must
// land all 20, in no caller-visible conflict, and ListRuns must read all
// of them back with each goroutine's own sequence still in its original
// relative order (interleaving BETWEEN the two goroutines is expected and
// not asserted on, since neither commits in lockstep with the other).
func TestAppendRunConcurrentNoConflictSurfaced(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-append-concurrent")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const perGoroutine = 10
	prefixes := []string{"a", "b"}
	var wg sync.WaitGroup
	errs := make(chan error, len(prefixes)*perGoroutine)
	for _, prefix := range prefixes {
		wg.Add(1)
		go func(prefix string) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				run := state.Run{RunID: fmt.Sprintf("%s-%d", prefix, i), Status: "succeeded"}
				if err := s.AppendRun(ctx, state.Key(rec.Name), run); err != nil {
					errs <- err
				}
			}
		}(prefix)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if errors.Is(err, state.ErrConflict) {
			t.Fatalf("AppendRun surfaced a conflict to the caller, which the contract forbids: %v", err)
		}
		t.Fatalf("AppendRun: %v", err)
	}

	runs, err := s.ListRuns(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	want := len(prefixes) * perGoroutine
	if len(runs) != want {
		t.Fatalf("got %d runs, want %d", len(runs), want)
	}

	position := make(map[string]int, len(runs))
	for i, r := range runs {
		position[r.RunID] = i
	}
	for _, prefix := range prefixes {
		last := -1
		for i := 0; i < perGoroutine; i++ {
			id := fmt.Sprintf("%s-%d", prefix, i)
			pos, ok := position[id]
			if !ok {
				t.Fatalf("run %q missing from the listing", id)
			}
			if pos <= last {
				t.Fatalf("run %q landed out of its own goroutine's order: position %d after %d", id, pos, last)
			}
			last = pos
		}
	}
}

func TestAppendRunOnMissingKeyIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	err := s.AppendRun(ctx, state.Key("no-such-conversation"), state.Run{RunID: "run-1"})
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("AppendRun on missing key: got %v, want ErrNotFound", err)
	}
}

func TestListRunsOnMissingKeyIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	_, err := s.ListRuns(ctx, state.Key("no-such-conversation"))
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("ListRuns on missing key: got %v, want ErrNotFound", err)
	}
}
