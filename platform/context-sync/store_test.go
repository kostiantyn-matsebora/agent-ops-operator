package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scanAll(t *testing.T, live string) Manifest {
	t.Helper()
	m, err := Scan(live, []string{"**/*"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCheckpointAndRestoreRoundTrip(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "projects/a.jsonl", "hello")
	write(t, live, "projects/deep/b.jsonl", "world")

	s := &Store{Root: root, Retain: 3}
	meta, err := s.Checkpoint(live, scanAll(t, live), true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Files != 2 || !meta.Quiesced {
		t.Fatalf("meta = %+v, want 2 files and quiesced", meta)
	}

	restored := t.TempDir()
	got, ok, err := s.Restore(restored)
	if err != nil || !ok {
		t.Fatalf("restore failed: ok=%v err=%v", ok, err)
	}
	if !got.Quiesced {
		t.Fatal("the restored generation's quiesced flag must survive")
	}
	for _, rel := range []string{"projects/a.jsonl", "projects/deep/b.jsonl"} {
		if _, err := os.Stat(filepath.Join(restored, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s missing after restore: %v", rel, err)
		}
	}
	// The metadata file is ours, not the agent's — it must not appear in the
	// restored context tree.
	if _, err := os.Stat(filepath.Join(restored, metaFile)); err == nil {
		t.Fatal("the store's own metadata leaked into the restored context")
	}
}

// An empty store is the normal first-start state, not a failure.
func TestRestoreFromEmptyStore(t *testing.T) {
	s := &Store{Root: t.TempDir(), Retain: 3}
	_, ok, err := s.Restore(t.TempDir())
	if err != nil {
		t.Fatalf("an empty store must not error: %v", err)
	}
	if ok {
		t.Fatal("an empty store must report that it restored nothing")
	}
}

// THE point of incremental: a checkpoint where nothing changed must transfer
// nothing, because at a two-minute cadence a full copy would hammer the very
// filesystem this design protects.
func TestUnchangedFilesAreHardlinkedNotCopied(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	write(t, live, "b.jsonl", "two")
	s := &Store{Root: root, Retain: 3}

	if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Change exactly one file.
	write(t, live, "b.jsonl", "two but longer")

	meta, err := s.Checkpoint(live, scanAll(t, live), true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Bytes != int64(len("two but longer")) {
		t.Fatalf("bytes written = %d, want only the changed file (%d)",
			meta.Bytes, len("two but longer"))
	}

	// The unchanged file must be the SAME inode as in the previous generation.
	gens, _ := s.generations()
	if len(gens) != 2 {
		t.Fatalf("want 2 generations, got %v", gens)
	}
	if !sameInode(t, filepath.Join(root, gens[0], "a.jsonl"), filepath.Join(root, gens[1], "a.jsonl")) {
		t.Fatal("an unchanged file must be hardlinked into the new generation, not copied")
	}
	if sameInode(t, filepath.Join(root, gens[0], "b.jsonl"), filepath.Join(root, gens[1], "b.jsonl")) {
		t.Fatal("a CHANGED file must be a real copy, or the old generation is corrupted by the new one")
	}
}

// A restore must produce a tree the next scan considers unchanged. If mtimes
// were reset, every restore would be followed by a full re-copy of everything.
func TestRestorePreservesChangeDetection(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &Store{Root: root, Retain: 3}
	before := scanAll(t, live)
	if _, err := s.Checkpoint(live, before, true, time.Now()); err != nil {
		t.Fatal(err)
	}

	restored := t.TempDir()
	if _, _, err := s.Restore(restored); err != nil {
		t.Fatal(err)
	}
	after := scanAll(t, restored)
	if after.Changed(before) {
		t.Fatal("a freshly restored tree must scan as unchanged, or every start re-copies everything")
	}
}

func TestRetainPrunesOldGenerations(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	s := &Store{Root: root, Retain: 2}
	for i := 0; i < 5; i++ {
		write(t, live, "a.jsonl", string(rune('a'+i)))
		if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	gens, _ := s.generations()
	if len(gens) != 2 {
		t.Fatalf("retain=2 kept %d generations: %v", len(gens), gens)
	}
	// The newest must still be the one `current` names.
	cur, err := s.Current()
	if err != nil || filepath.Base(cur) != gens[len(gens)-1] {
		t.Fatalf("current = %q, want the newest generation %q", cur, gens[len(gens)-1])
	}
}

// A mid-run copy is retained and LABELLED, never skipped: a long run is exactly
// what a crash would otherwise lose in full.
func TestMidRunCheckpointIsLabelled(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &Store{Root: root, Retain: 3}

	if _, err := s.Checkpoint(live, scanAll(t, live), false, time.Now()); err != nil {
		t.Fatal(err)
	}
	gen, _ := s.Current()
	meta, err := s.Meta(gen)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Quiesced {
		t.Fatal("a mid-run copy must be labelled best-effort, not quiesced")
	}
}

// A pointer to a generation that no longer exists is an empty store with a
// stale pointer — starting fresh beats wedging the conversation.
func TestStaleCurrentPointerReadsAsEmpty(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &Store{Root: root, Retain: 3}
	if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err != nil {
		t.Fatal(err)
	}
	gens, _ := s.generations()
	if err := os.RemoveAll(filepath.Join(root, gens[0])); err != nil {
		t.Fatal(err)
	}
	cur, err := s.Current()
	if err != nil {
		t.Fatalf("a stale pointer must not error: %v", err)
	}
	if cur != "" {
		t.Fatalf("a stale pointer must read as empty, got %q", cur)
	}
}

func sameInode(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(fa, fb)
}
