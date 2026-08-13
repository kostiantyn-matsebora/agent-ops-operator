package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubLister lets a test create a conversation BETWEEN the disk scan and the
// listing — the race the workspace ordering exists to eliminate.
type stubLister struct {
	convs   []Conversation
	onList  func()
	listErr error
}

func (s *stubLister) ListConversations(context.Context) ([]Conversation, error) {
	if s.onList != nil {
		s.onList()
	}
	return s.convs, s.listErr
}

func mkDirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func mkSession(t *testing.T, root, id string, age time.Duration) {
	t.Helper()
	p := filepath.Join(root, id+".jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

func dirExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// A deleted conversation's directory is what this job is FOR.
func TestOrphanWorkspaceIsReclaimed(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, "conv-live", "conv-gone")
	lister := &stubLister{convs: []Conversation{{Name: "conv-live"}}}

	rep, err := ReclaimWorkspaces(context.Background(), root, lister, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 1 || rep.Removed[0] != "conv-gone" {
		t.Fatalf("want conv-gone reclaimed, got %+v", rep.Removed)
	}
	if !dirExists(t, filepath.Join(root, "conv-live")) {
		t.Fatal("a live conversation's workspace must survive")
	}
	if dirExists(t, filepath.Join(root, "conv-gone")) {
		t.Fatal("the orphan was not removed")
	}
}

// THE ordering test. A conversation created after the scan but before the
// listing must never be reclaimed — its directory exists, but the listing that
// decides is taken later, so it is seen.
func TestConversationCreatedMidScanIsNeverReclaimed(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, "conv-racing")
	lister := &stubLister{}
	// The conversation appears only when the listing happens — i.e. strictly
	// after the directory scan.
	lister.onList = func() { lister.convs = []Conversation{{Name: "conv-racing"}} }

	rep, err := ReclaimWorkspaces(context.Background(), root, lister, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 0 {
		t.Fatalf("a conversation created mid-scan must survive, removed %+v", rep.Removed)
	}
	if !dirExists(t, filepath.Join(root, "conv-racing")) {
		t.Fatal("the racing conversation lost its workspace")
	}
}

// D8, and the one a future "optimisation" would break: the listing is
// PHASE-BLIND, so a closed conversation is protected by the ordinary rule.
// Nothing here knows what a phase is — which is exactly the point.
func TestClosedConversationKeepsItsWorkspaceAndTranscripts(t *testing.T) {
	wsRoot, sessRoot := t.TempDir(), t.TempDir()
	mkDirs(t, wsRoot, "conv-closed")
	mkSession(t, sessRoot, "ctx-closed", 30*24*time.Hour) // long past any grace

	// A CLOSED conversation still has a CR and still carries its context
	// handle. The lister returns it exactly as it returns a live one.
	lister := &stubLister{convs: []Conversation{{Name: "conv-closed", ContextID: "ctx-closed"}}}

	wsRep, err := ReclaimWorkspaces(context.Background(), wsRoot, lister, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(wsRep.Removed) != 0 {
		t.Fatalf("a closed conversation's workspace must survive: %+v", wsRep.Removed)
	}
	sessRep, err := ReclaimSessions(context.Background(), sessRoot, lister,
		Options{SessionGrace: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessRep.Removed) != 0 {
		t.Fatalf("a closed conversation's transcripts must survive: %+v", sessRep.Removed)
	}
}

// A transcript for a run IN FLIGHT is unreferenced — the context handle is
// written by /work/done, after the file exists — so only the grace period
// protects it. This is why the ordering argument cannot be reused here.
func TestTranscriptFromARunInFlightIsKept(t *testing.T) {
	root := t.TempDir()
	mkSession(t, root, "ctx-in-flight", time.Minute)
	lister := &stubLister{convs: []Conversation{{Name: "conv-1"}}} // no handle yet

	rep, err := ReclaimSessions(context.Background(), root, lister,
		Options{SessionGrace: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 0 {
		t.Fatalf("an in-flight transcript must be kept: %+v", rep.Removed)
	}
}

func TestOldUnreferencedTranscriptIsReclaimed(t *testing.T) {
	root := t.TempDir()
	mkSession(t, root, "ctx-abandoned", 30*24*time.Hour)
	mkSession(t, root, "ctx-referenced", 30*24*time.Hour)
	lister := &stubLister{convs: []Conversation{{Name: "c", ContextID: "ctx-referenced"}}}

	rep, err := ReclaimSessions(context.Background(), root, lister,
		Options{SessionGrace: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 1 || rep.Removed[0] != "ctx-abandoned.jsonl" {
		t.Fatalf("want only the abandoned transcript reclaimed: %+v", rep.Removed)
	}
}

// The retired sessionId spelling still references a transcript. Missing it
// would make an older manager's conversations lose their history.
func TestRetiredSessionIDSpellingStillReferences(t *testing.T) {
	root := t.TempDir()
	mkSession(t, root, "ctx-old-spelling", 30*24*time.Hour)
	lister := &stubLister{convs: []Conversation{{Name: "c", ContextID: "ctx-old-spelling"}}}

	rep, err := ReclaimSessions(context.Background(), root, lister, Options{SessionGrace: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 0 {
		t.Fatalf("a handle read from the retired spelling must still protect: %+v", rep.Removed)
	}
}

func TestDryRunRemovesNothing(t *testing.T) {
	wsRoot := t.TempDir()
	mkDirs(t, wsRoot, "conv-gone")
	lister := &stubLister{}

	rep, err := ReclaimWorkspaces(context.Background(), wsRoot, lister, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 1 {
		t.Fatalf("dry run must still REPORT what it would remove: %+v", rep)
	}
	if !dirExists(t, filepath.Join(wsRoot, "conv-gone")) {
		t.Fatal("dry run deleted something")
	}
	if !rep.DryRun {
		t.Error("the report must say it was a dry run")
	}
}

// The first run on an established install is the dangerous one.
func TestPerRunBoundIsHonoredAndReported(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, "a", "b", "c", "d")
	lister := &stubLister{}

	rep, err := ReclaimWorkspaces(context.Background(), root, lister, Options{MaxDeletions: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 2 {
		t.Fatalf("the bound must cap one run: %+v", rep.Removed)
	}
	if rep.Skipped != 2 {
		t.Fatalf("what was withheld must be reported, got %d", rep.Skipped)
	}
	// deterministic order, so a bounded run makes progress rather than
	// revisiting the same two every time
	if rep.Removed[0] != "a" || rep.Removed[1] != "b" {
		t.Fatalf("bounded runs must proceed in a stable order: %+v", rep.Removed)
	}
}

// A listing that fails must reclaim NOTHING: without it, every directory looks
// like an orphan.
func TestAFailedListingReclaimsNothing(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, "conv-1", "conv-2")
	lister := &stubLister{listErr: context.DeadlineExceeded}

	rep, err := ReclaimWorkspaces(context.Background(), root, lister, Options{})
	if err == nil {
		t.Fatal("a failed listing must be an error, not an empty world")
	}
	if len(rep.Removed) != 0 || !dirExists(t, filepath.Join(root, "conv-1")) {
		t.Fatal("nothing may be removed when the listing failed")
	}
}

func TestMissingRootIsNotAnError(t *testing.T) {
	rep, err := ReclaimWorkspaces(context.Background(), filepath.Join(t.TempDir(), "nope"),
		&stubLister{}, Options{})
	if err != nil {
		t.Fatalf("an unmounted claim is a configuration, not a failure: %v", err)
	}
	if rep.Scanned != 0 || len(rep.Removed) != 0 {
		t.Fatalf("nothing to do: %+v", rep)
	}
}
