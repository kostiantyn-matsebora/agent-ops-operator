// Task 2.1: Get/Create/Update/Delete, the CAS round trip, and the
// lossless mapping of every field record.go declares.
package kube

import (
	"context"
	"errors"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestCreateGetRoundTripsEveryField(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	want := fixtureRecord("conv-roundtrip")
	rev, err := s.Create(ctx, state.Key(want.Name), want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rev == "" {
		t.Fatal("Create returned an empty revision")
	}

	got, gotRev, err := s.Get(ctx, state.Key(want.Name))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotRev != rev {
		t.Fatalf("Get revision %q != Create revision %q", gotRev, rev)
	}
	assertRecordsEqual(t, got, want)
}

func TestCreateOnExistingKeyIsAlreadyExists(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-dup")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := s.Create(ctx, state.Key(rec.Name), rec)
	if !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create: got %v, want ErrAlreadyExists", err)
	}
	if errors.Is(err, state.ErrConflict) {
		t.Fatalf("second Create: ErrAlreadyExists must be distinguishable from ErrConflict, got %v", err)
	}
}

func TestUpdateWithCurrentRevisionSucceeds(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-update-ok")
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := rec
	updated.Phase = "Idle"
	updated.Title = "updated title"
	newRev, err := s.Update(ctx, state.Key(rec.Name), rev, updated)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if newRev == rev {
		t.Fatalf("Update did not move the revision: %q", newRev)
	}

	got, gotRev, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if gotRev != newRev {
		t.Fatalf("Get revision %q != Update revision %q", gotRev, newRev)
	}
	assertRecordsEqual(t, got, updated)
}

// TestUpdateWithStaleRevisionIsRefused is the spec's own "A stale claim is
// never merged" scenario: a write presenting a revision older than the
// current one is refused in full, and the record carries none of it.
func TestUpdateWithStaleRevisionIsRefused(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-update-stale")
	staleRev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Move the revision forward once, so staleRev no longer names the
	// current record.
	afterFirstUpdate := rec
	afterFirstUpdate.Title = "first update"
	if _, err := s.Update(ctx, state.Key(rec.Name), staleRev, afterFirstUpdate); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	// Writing against the now-stale revision must be refused, and must
	// change nothing.
	staleAttempt := rec
	staleAttempt.Title = "stale write, must not land"
	_, err = s.Update(ctx, state.Key(rec.Name), staleRev, staleAttempt)
	if !errors.Is(err, state.ErrConflict) {
		t.Fatalf("Update with stale revision: got %v, want ErrConflict", err)
	}

	got, _, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get after refused update: %v", err)
	}
	if got.Title != afterFirstUpdate.Title {
		t.Fatalf("stale write changed the record: title = %q, want %q", got.Title, afterFirstUpdate.Title)
	}
}

func TestUpdateOnMissingKeyIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	_, err := s.Update(ctx, state.Key("no-such-conversation"), state.Revision("1"), fixtureRecord("no-such-conversation"))
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Update on missing key: got %v, want ErrNotFound", err)
	}
}

func TestDeleteThenGetIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-delete")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, state.Key(rec.Name)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Get(ctx, state.Key(rec.Name)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after delete: got %v, want ErrNotFound", err)
	}

	// Not an error when the key is already absent (store.go's Store.Delete
	// contract).
	if err := s.Delete(ctx, state.Key(rec.Name)); err != nil {
		t.Fatalf("second Delete on an already-absent key: %v", err)
	}
}

func TestGetOnMissingKeyIsNotFound(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	if _, _, err := s.Get(ctx, state.Key("never-created")); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on missing key: got %v, want ErrNotFound", err)
	}
}

// TestUpdatePreservesRunsAppendedBetweenReadAndWrite pins the one
// documented caveat in Update's own comment: the control-record write
// must never clobber status.runs[], which AppendRun alone owns.
func TestUpdatePreservesRunsAppendedBetweenReadAndWrite(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	rec := fixtureRecord("conv-update-preserves-runs")
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendRun(ctx, state.Key(rec.Name), state.Run{RunID: "run-x", Status: "succeeded"}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	// AppendRun moves the control record's own revision (design D-5: "an
	// append moves the control record's revision"), so Update's own CAS
	// must read the current one rather than the one Create returned.
	_, rev, err := s.Get(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("Get before update: %v", err)
	}

	updated := rec
	updated.Title = "after append"
	if _, err := s.Update(ctx, state.Key(rec.Name), rev, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	runs, err := s.ListRuns(ctx, state.Key(rec.Name))
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != "run-x" {
		t.Fatalf("Update clobbered runs: %+v", runs)
	}
}
