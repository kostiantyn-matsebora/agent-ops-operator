package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanIncludesOnlyWhatIsDeclared(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude/projects/-data-workspace/a.jsonl", "one")
	write(t, root, ".claude/projects/-data-workspace/deep/b.jsonl", "two")
	// The whole reason includes beat excludes: a package cache is enormous and
	// nobody has to remember to name it.
	write(t, root, ".npm/_cacache/big", "cache")
	write(t, root, ".cache/whatever", "junk")

	m, err := Scan(root, []string{".claude/projects/-data-workspace/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("scanned %d entries, want 2: %+v", len(m.Entries), m.Entries)
	}
	for _, e := range m.Entries {
		if filepath.Ext(e.Path) != ".jsonl" {
			t.Fatalf("a cache file was scanned: %q", e.Path)
		}
	}
}

func TestScanAppliesExcludes(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude/a.jsonl", "keep")
	write(t, root, ".claude/session.lock", "churn")
	write(t, root, ".claude/sub/tmp.tmp", "churn")

	m, err := Scan(root, []string{".claude/**"}, []string{"**/*.lock", "**/*.tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Path != ".claude/a.jsonl" {
		t.Fatalf("excludes not applied: %+v", m.Entries)
	}
}

// A missing include is the NORMAL first-start state — a runtime that has not
// written any context yet. Failing here would make every fresh conversation
// look broken.
func TestScanToleratesMissingPaths(t *testing.T) {
	root := t.TempDir()
	m, err := Scan(root, []string{".claude/projects/**"}, nil)
	if err != nil {
		t.Fatalf("a missing include must not be an error: %v", err)
	}
	if len(m.Entries) != 0 {
		t.Fatalf("expected an empty manifest, got %+v", m.Entries)
	}
}

// Changed IS the skip-when-unchanged rule. If it were wrong in the "unchanged"
// direction the sidecar would write to the fragile volume every cycle forever;
// wrong the other way and context would silently stop being persisted.
func TestChangedDetectsRealChangesOnly(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude/a.jsonl", "one")
	base, err := Scan(root, []string{".claude/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	again, _ := Scan(root, []string{".claude/**"}, nil)
	if again.Changed(base) {
		t.Fatal("an untouched tree must report NO change, or the timer writes every cycle")
	}

	// Content of a different length.
	write(t, root, ".claude/a.jsonl", "one more")
	grown, _ := Scan(root, []string{".claude/**"}, nil)
	if !grown.Changed(base) {
		t.Fatal("a changed file must be detected")
	}

	// Same size, different mtime — the case a size-only check would miss.
	p := write(t, root, ".claude/a.jsonl", "one")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	touched, _ := Scan(root, []string{".claude/**"}, nil)
	if !touched.Changed(base) {
		t.Fatal("a same-size rewrite must still count as a change")
	}

	// A new file.
	write(t, root, ".claude/b.jsonl", "two")
	added, _ := Scan(root, []string{".claude/**"}, nil)
	if !added.Changed(base) {
		t.Fatal("a new file must be detected")
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"**/*.lock", "a.lock", true},
		{"**/*.lock", "deep/nested/a.lock", true},
		{"**/*.lock", "a.jsonl", false},
		{".claude/**", ".claude/a", true},
		{".claude/**", ".claude/deep/a", true},
		{".claude/**", ".claudex/a", false},
		{"*.tmp", "a.tmp", true},
		{"*.tmp", "deep/a.tmp", false},
	}
	for _, tc := range cases {
		if got := matchGlob(tc.pattern, tc.name); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// The `**/*` form silently matched NOTHING before the walker was unified: the
// literal-prefix walk plus one matcher is what makes every pattern shape go
// down the same path.
func TestScanSupportsEveryPatternShape(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.jsonl", "top")
	write(t, root, "deep/b.jsonl", "nested")
	write(t, root, ".claude/c.jsonl", "dotted")

	cases := []struct {
		pattern string
		want    int
	}{
		{"**", 3},
		{"**/*", 3},
		{"**/*.jsonl", 3},
		{".claude/**", 1},
		{"deep/**", 1},
		{"a.jsonl", 1},
		{"nothing/**", 0},
	}
	for _, tc := range cases {
		m, err := Scan(root, []string{tc.pattern}, nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.pattern, err)
		}
		if len(m.Entries) != tc.want {
			t.Errorf("Scan(%q) found %d entries, want %d: %+v",
				tc.pattern, len(m.Entries), tc.want, m.Entries)
		}
	}
}

// Two includes that overlap must not double-count: the manifest is a set.
func TestScanDeduplicatesOverlappingIncludes(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude/a.jsonl", "one")
	m, err := Scan(root, []string{".claude/**", "**/*.jsonl"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("overlapping includes double-counted: %+v", m.Entries)
	}
}
