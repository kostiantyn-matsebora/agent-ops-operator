// The generic CAS/lookup/feed/deletion scenarios every Collection[T] must
// pass now live in internal/state/conformance, driven by conversationFixture
// and run once for every field of state.Stores through TestStores
// (stores_test.go). What is left here is specific to this backend's own
// mapping: the lossless field-by-field round trip, and the one interaction
// between Update and Runs.Append that only kube's split main+status writes
// can show.
package kube

import (
	"context"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestCreateGetRoundTripsEveryField(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	want := fixtureRecord("conv-roundtrip")
	rev, err := s.Conversations.Create(ctx, state.Key(want.Name), want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rev == "" {
		t.Fatal("Create returned an empty revision")
	}

	got, gotRev, err := s.Conversations.Get(ctx, state.Key(want.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotRev != rev {
		t.Fatalf("Get revision %q != Create revision %q", gotRev, rev)
	}
	assertRecordsEqual(t, got, want)
}

// TestUpdatePreservesRunsAppendedBetweenReadAndWrite pins the one documented
// caveat in Update's own comment: the control-record write must never
// clobber status.runs[], which Runs.Append alone owns.
func TestUpdatePreservesRunsAppendedBetweenReadAndWrite(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-update-preserves-runs")
	if _, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Runs.Append(ctx, state.Key(rec.Name), state.Run{RunID: "run-x", Status: "succeeded"}); err != nil {
		t.Fatalf("Runs.Append: %v", err)
	}

	// Append moves the control record's own revision, so Update's own CAS
	// must read the current one rather than the one Create returned.
	_, rev, err := s.Conversations.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get before update: %v", err)
	}

	updated := rec
	updated.Title = "after append"
	if _, err := s.Conversations.Update(ctx, state.Key(rec.Name), rev, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	runs, err := s.Runs.List(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Runs.List: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != "run-x" {
		t.Fatalf("Update clobbered runs: %+v", runs)
	}
}
