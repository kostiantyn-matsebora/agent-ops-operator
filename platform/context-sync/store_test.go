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

// skipIfRoot skips a permission-denial test that relies on chmod'd mode
// bits: the kernel skips the permission check entirely for uid 0, so a
// root-run container (the default for an unprivileged Docker image, as CI
// runs) would see the write/create succeed and the test fail for a reason
// that has nothing to do with the behavior under test.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced, so this denial cannot occur")
	}
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

// Current must distinguish "the pointer is stale" (IsNotExist, read as empty)
// from a real stat failure. A symlink pointing at itself hits ELOOP on Stat
// while Readlink itself succeeds, which is a genuinely different error path.
func TestCurrentPropagatesNonNotExistStatError(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, currentLink)
	if err := os.Symlink(link, link); err != nil {
		t.Fatal(err)
	}
	s := &Store{Root: root}
	if _, err := s.Current(); err == nil {
		t.Fatal("a symlink loop must surface as an error, not read as an empty store")
	}
}

// Meta's json.Unmarshal error path — a corrupt (not merely absent) metadata
// file — was never exercised.
func TestMetaFailsOnCorruptJSON(t *testing.T) {
	gen := t.TempDir()
	if err := os.WriteFile(filepath.Join(gen, metaFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{}
	if _, err := s.Meta(gen); err == nil {
		t.Fatal("corrupt metadata must be reported, not silently accepted")
	}
}

// Restore's "err != nil && !os.IsNotExist(err)" branch: a generation whose
// metadata is corrupt (not simply missing) must fail the restore loudly
// rather than proceed with a zero-value meta.
func TestRestoreFailsLoudlyOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	gen := filepath.Join(root, "gen-1")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, metaFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gen-1", filepath.Join(root, currentLink)); err != nil {
		t.Fatal(err)
	}

	s := &Store{Root: root}
	if _, _, err := s.Restore(t.TempDir()); err == nil {
		t.Fatal("corrupt generation metadata must fail the restore rather than silently proceeding")
	}
}

// The companion case: a generation with NO metadata file at all (predating
// the mechanism, or never written) must restore fine with a zero-value meta —
// this is the os.IsNotExist(err) tolerance branch in Restore.
func TestRestoreToleratesMissingMetadataFile(t *testing.T) {
	root := t.TempDir()
	gen := filepath.Join(root, "gen-1")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, "a.jsonl"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gen-1", filepath.Join(root, currentLink)); err != nil {
		t.Fatal(err)
	}

	s := &Store{Root: root}
	dst := t.TempDir()
	meta, ok, err := s.Restore(dst)
	if err != nil {
		t.Fatalf("a generation with no metadata file must still restore: %v", err)
	}
	if !ok {
		t.Fatal("want ok=true")
	}
	if meta.Quiesced {
		t.Fatal("a missing meta file must read as its zero value, not quiesced")
	}
	if _, err := os.Stat(filepath.Join(dst, "a.jsonl")); err != nil {
		t.Fatal(err)
	}
}

// Restore's copyTree error path: a destination blocked by an existing file
// must fail rather than silently drop files.
func TestRestoreFailsWhenCopyTreeCannotWriteDestination(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &Store{Root: root, Retain: 2}
	if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err != nil {
		t.Fatal(err)
	}

	dstParent := t.TempDir()
	dst := filepath.Join(dstParent, "notadir")
	if err := os.WriteFile(dst, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.Restore(dst); err == nil {
		t.Fatal("restoring into a path blocked by an existing file must fail, not silently drop files")
	}
}

// generations() on a store whose Root has never been created must read as
// empty, not error — the same "not yet started" tolerance Current() applies.
func TestGenerationsOnMissingRootIsEmpty(t *testing.T) {
	s := &Store{Root: filepath.Join(t.TempDir(), "never-created")}
	gens, err := s.generations()
	if err != nil {
		t.Fatalf("a missing root must not error: %v", err)
	}
	if gens != nil {
		t.Fatalf("want no generations, got %v", gens)
	}
}

// Checkpoint's own os.MkdirAll(s.Root) failure: a Root whose parent forbids
// creating it must fail the checkpoint loudly.
func TestCheckpointFailsWhenRootCannotBeCreated(t *testing.T) {
	skipIfRoot(t)
	live := t.TempDir()
	write(t, live, "a.jsonl", "x")
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	s := &Store{Root: filepath.Join(parent, "store"), Retain: 2}
	if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err == nil {
		t.Fatal("a store root that cannot be created must fail the checkpoint")
	}
}

// nextGenDir's collision case: a plain FILE already occupying the path the
// next generation would use must fail the checkpoint's own MkdirAll rather
// than silently overwrite it.
func TestCheckpointFailsWhenNextGenerationPathIsAFile(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	if err := os.WriteFile(filepath.Join(root, "gen-1"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{Root: root, Retain: 2}
	if _, err := s.Checkpoint(live, scanAll(t, live), true, time.Now()); err == nil {
		t.Fatal("a name collision on the next generation directory must fail loudly")
	}
}

// A manifest entry whose source file vanished between the scan and the copy
// (the comment on this branch literally says "not ours to mourn") must be
// skipped rather than fail the whole checkpoint.
func TestCheckpointSkipsFilesThatVanishBetweenScanAndCopy(t *testing.T) {
	live, root := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	m := scanAll(t, live)
	if err := os.Remove(filepath.Join(live, "a.jsonl")); err != nil {
		t.Fatal(err)
	}

	s := &Store{Root: root, Retain: 2}
	if _, err := s.Checkpoint(live, m, true, time.Now()); err != nil {
		t.Fatalf("a vanished source file must not fail the checkpoint: %v", err)
	}
}

// copyFile's os.OpenFile(dst) failure: a destination directory that does not
// exist must be reported, not attempted blindly.
func TestCopyFileFailsWhenDestinationDirMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "missing", "dst.txt")
	if _, err := copyFile(src, dst); err == nil {
		t.Fatal("want an error when the destination directory does not exist")
	}
}

// writeMeta's failure path: a generation directory that forbids writes must
// fail writing its own metadata file rather than silently skip it. Tested
// directly rather than through Checkpoint, because nextGenDir always assigns
// a brand-new, previously-nonexistent number — pre-creating that directory to
// chmod it ahead of time would itself count as a generation and shift the
// number Checkpoint actually targets.
func TestWriteMetaFailsWhenGenerationDirIsReadOnly(t *testing.T) {
	skipIfRoot(t)
	gen := t.TempDir()
	if err := os.Chmod(gen, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gen, 0o755) })

	if err := writeMeta(gen, GenerationMeta{}); err == nil {
		t.Fatal("want an error when the generation directory forbids writes")
	}
}

// swapCurrent's Symlink failure: a non-empty directory sitting at the
// temporary path cannot be cleared by the best-effort os.Remove, so the
// Symlink call underneath must fail rather than silently lose the pointer.
func TestSwapCurrentFailsWhenTmpPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	tmp := filepath.Join(root, ".current.tmp")
	if err := os.MkdirAll(filepath.Join(tmp, "x"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := &Store{Root: root}
	if err := s.swapCurrent(filepath.Join(root, "gen-1")); err == nil {
		t.Fatal("a blocked temp path must fail the swap rather than silently losing the pointer")
	}
}

// prune's os.RemoveAll failure: a generation slated for removal whose
// permissions block deleting its own contents must surface the error rather
// than leave a torn, half-removed generation unreported.
func TestPruneReturnsErrorWhenRemovalFails(t *testing.T) {
	skipIfRoot(t)
	root := t.TempDir()
	for _, n := range []string{"gen-1", "gen-2"} {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "gen-1", "a.jsonl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gen-2", filepath.Join(root, currentLink)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "gen-1"), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "gen-1"), 0o755) })

	s := &Store{Root: root, Retain: 1}
	if err := s.prune(); err == nil {
		t.Fatal("a removal failure during prune must be surfaced, not swallowed")
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
