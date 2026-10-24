package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// resolve("") is refused before any filesystem access -- distinct from the
// absolute/escaping-path cases TestPathConfinement already covers.
func TestResolveEmptyPathIsRefused(t *testing.T) {
	b, _ := newBuiltins(t)
	if _, err := b.resolve(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("got %v", err)
	}
}

// resolve fails naming the WORKSPACE itself when it cannot be evaluated --
// distinct from a path escaping a workspace that exists.
func TestResolveFailsWhenTheWorkspaceItselfIsGone(t *testing.T) {
	b := Builtins{Workspace: filepath.Join(t.TempDir(), "never-created")}
	if _, err := b.resolve("f.txt"); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Errorf("got %v", err)
	}
}

// A symlink cycle for the longest EXISTING prefix must fail resolve's second
// EvalSymlinks call, not the first (the workspace root itself is fine).
func TestResolveFailsOnASymlinkCycle(t *testing.T) {
	b, ws := newBuiltins(t)
	os.Symlink("loop-b", filepath.Join(ws, "loop-a"))
	os.Symlink("loop-a", filepath.Join(ws, "loop-b"))
	if _, err := b.resolve("loop-a"); err == nil {
		t.Error("want an error resolving a symlink cycle")
	}
}

// Grep on a path that escapes the workspace fails through its own resolve
// call, distinct from Read/Write's.
func TestGrepPathEscapeIsRefused(t *testing.T) {
	b, _ := newBuiltins(t)
	args, _ := json.Marshal(map[string]string{"pattern": "x", "path": "/etc"})
	if text, isErr := call(t, b, "Grep", string(args)); !isErr || !strings.Contains(text, "escapes") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
}

// Grep's walk visits a symlinked FILE as a leaf entry (not a directory), and
// os.ReadFile happily follows it -- a target outside the workspace must be
// refused rather than read, exactly like Read/Edit/Write's own resolve (O3).
func TestGrepRefusesASymlinkEscapingTheWorkspace(t *testing.T) {
	b, ws := newBuiltins(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("needle-outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(ws, "escape-link")); err != nil {
		t.Fatal(err)
	}
	text, isErr := call(t, b, "Grep", `{"pattern":"needle-outside"}`)
	if isErr {
		t.Errorf("an escaping symlink must be skipped, not a tool error: %q", text)
	}
	if text != "no matches" {
		t.Errorf("content reached through an escaping symlink must never be reported: %q", text)
	}
}

// Grep must not descend into .git, and must report "no matches" honestly
// when nothing else matches.
func TestGrepSkipsDotGitAndReportsNoMatches(t *testing.T) {
	b, ws := newBuiltins(t)
	os.MkdirAll(filepath.Join(ws, ".git"), 0o755)
	os.WriteFile(filepath.Join(ws, ".git", "config"), []byte("needle"), 0o644)
	text, isErr := call(t, b, "Grep", `{"pattern":"needle"}`)
	if isErr || text != "no matches" {
		t.Errorf(".git must be skipped: isErr=%v %q", isErr, text)
	}
}

// A file containing a NUL byte is treated as binary and skipped, even though
// its text would otherwise match.
func TestGrepSkipsBinaryFiles(t *testing.T) {
	b, ws := newBuiltins(t)
	os.WriteFile(filepath.Join(ws, "bin.dat"), []byte("needle\x00more"), 0o644)
	text, isErr := call(t, b, "Grep", `{"pattern":"needle"}`)
	if isErr || text != "no matches" {
		t.Errorf("binary file must be skipped: isErr=%v %q", isErr, text)
	}
}

// Grep stops early (errStop) once the output bound is exceeded, and that is
// reported as a truncated result, not a tool error.
func TestGrepStopsAtTheOutputBound(t *testing.T) {
	b, ws := newBuiltins(t)
	b.OutputMax = 20
	var lines strings.Builder
	for i := 0; i < 50; i++ {
		lines.WriteString("needle line that is reasonably long\n")
	}
	os.WriteFile(filepath.Join(ws, "many.txt"), []byte(lines.String()), 0o644)
	text, isErr := call(t, b, "Grep", `{"pattern":"needle"}`)
	if isErr {
		t.Errorf("hitting the bound must not be a tool error: %q", text)
	}
	if !strings.Contains(text, "truncated") {
		t.Errorf("want a truncation notice, got %q", text)
	}
}

// A cancelled context both short-circuits the walk (ctx.Err() branch) and
// propagates as the walk's own error, which Grep must surface as a tool
// error rather than an empty match set.
func TestGrepStopsOnACancelledContext(t *testing.T) {
	b, ws := newBuiltins(t)
	os.WriteFile(filepath.Join(ws, "a.txt"), []byte("needle"), 0o644)
	os.WriteFile(filepath.Join(ws, "b.txt"), []byte("needle"), 0o644)
	r := NewRegistry()
	b.Register(r)
	tool, _ := r.Get("Grep")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	text, isErr, err := tool.Call(ctx, json.RawMessage(`{"pattern":"needle"}`))
	if err != nil {
		t.Fatalf("tool layer error: %v", err)
	}
	if !isErr || !strings.Contains(text, "context canceled") {
		t.Errorf("a cancelled context must fail the walk readably: isErr=%v %q", isErr, text)
	}
}

// Glob against a gone workspace fails naming it, and skips .git the same way
// Grep does; a pattern matching nothing is reported honestly.
func TestGlobFailsOnAGoneWorkspaceSkipsDotGitAndReportsNoMatches(t *testing.T) {
	b := Builtins{Workspace: filepath.Join(t.TempDir(), "never-created")}
	if text, isErr := call(t, b, "Glob", `{"pattern":"*.go"}`); !isErr || !strings.Contains(text, "Glob:") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}

	b2, ws := newBuiltins(t)
	os.MkdirAll(filepath.Join(ws, ".git"), 0o755)
	os.WriteFile(filepath.Join(ws, ".git", "config"), []byte("x"), 0o644)
	if text, isErr := call(t, b2, "Glob", `{"pattern":"**/config"}`); isErr || text != "no matches" {
		t.Errorf(".git must be skipped: isErr=%v %q", isErr, text)
	}
	if text, isErr := call(t, b2, "Glob", `{"pattern":"**/*.nope"}`); isErr || text != "no matches" {
		t.Errorf("no matches must be reported honestly: isErr=%v %q", isErr, text)
	}
}

// globMatch's own branches, exercised directly: a plain pattern with no `**`
// (path.Match only), a prefix that does not match the candidate, an
// all-directories suffix ("**" with nothing after it), and a suffix that
// never matches at any depth.
func TestGlobMatchBranches(t *testing.T) {
	if !globMatch("*.go", "main.go") || globMatch("*.go", "sub/main.go") {
		t.Error("a plain pattern (no **) must be an ordinary path.Match")
	}
	if globMatch("docs/**", "other/readme.md") {
		t.Error("a prefix that does not match the candidate must not match")
	}
	if !globMatch("docs/**", "docs/a/b/c.md") {
		t.Error("an empty suffix after ** must match anything under the prefix")
	}
	if globMatch("**/*.nope", "a/b/c.md") {
		t.Error("a suffix that matches nothing at any depth must not match")
	}
}

// Read's own os.ReadFile failure: a path that resolves cleanly (nothing
// escapes) but names a file that simply is not there.
func TestReadFailsOnAMissingFile(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Read", `{"path":"missing.txt"}`); !isErr || !strings.Contains(text, "no such file") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
}

// Glob's walk callback swallows a per-entry error (werr, e.g. permission
// denied on a subdirectory) rather than failing the whole listing.
func TestGlobSwallowsAPerEntryWalkError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission enforcement is platform-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-permission bit")
	}
	b, ws := newBuiltins(t)
	locked := filepath.Join(ws, "locked")
	os.MkdirAll(locked, 0o755)
	os.WriteFile(filepath.Join(locked, "inside.go"), []byte("x"), 0o644)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	// hello.txt (from newBuiltins) must still be found even though the locked
	// subdirectory cannot be entered.
	text, isErr := call(t, b, "Glob", `{"pattern":"**/*.txt"}`)
	if isErr || text != "hello.txt" {
		t.Errorf("an unreadable sibling directory must not fail the listing: isErr=%v %q", isErr, text)
	}
}

// Edit's own resolve/read failures, distinct from Read's.
func TestEditResolveAndReadFailures(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Edit", `{"path":"/etc/passwd","old_string":"a","new_string":"b"}`); !isErr || !strings.Contains(text, "escapes") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
	if text, isErr := call(t, b, "Edit", `{"path":"missing.txt","old_string":"a","new_string":"b"}`); !isErr {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
}

// An empty old_string, and an old_string occurring more than once, are both
// refused rather than silently editing the wrong (or every) occurrence.
func TestEditRefusesEmptyOrAmbiguousOldString(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Edit", `{"path":"hello.txt","old_string":"","new_string":"x"}`); !isErr || !strings.Contains(text, "empty") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
	if text, isErr := call(t, b, "Edit", `{"path":"hello.txt","old_string":"l","new_string":"x"}`); !isErr || !strings.Contains(text, "occurs") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
}

// A read-only target file makes the Edit's WriteFile fail -- skipped on a
// filesystem/user combination where that permission bit is not enforced.
func TestEditFailsWhenTheFileCannotBeWritten(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission enforcement is platform-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the write-permission bit")
	}
	b, ws := newBuiltins(t)
	target := filepath.Join(ws, "hello.txt")
	if err := os.Chmod(target, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o644)
	if text, isErr := call(t, b, "Edit", `{"path":"hello.txt","old_string":"world","new_string":"there"}`); !isErr {
		t.Errorf("want a write failure, got isErr=%v %q", isErr, text)
	}
}

// Write's MkdirAll fails when a path component is an existing FILE, not a
// directory.
func TestWriteFailsWhenAPathComponentIsAFile(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Write", `{"path":"hello.txt/sub/f.txt","content":"x"}`); !isErr {
		t.Errorf("want a MkdirAll failure, got isErr=%v %q", isErr, text)
	}
}

// Write's own os.WriteFile failure: an existing directory with no write
// permission.
func TestWriteFailsWhenTheDirectoryIsNotWritable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission enforcement is platform-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the write-permission bit")
	}
	b, ws := newBuiltins(t)
	locked := filepath.Join(ws, "locked")
	os.MkdirAll(locked, 0o755)
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	if text, isErr := call(t, b, "Write", `{"path":"locked/f.txt","content":"x"}`); !isErr {
		t.Errorf("want a write failure, got isErr=%v %q", isErr, text)
	}
}

// boundedWriter's whole point: the BUFFER never grows past its cap no matter
// how much is offered, even though it keeps an honest running total (O4).
func TestBoundedWriterCapsMemoryRegardlessOfTotalWritten(t *testing.T) {
	w := &boundedWriter{cap: 10}
	for i := 0; i < 1000; i++ {
		w.Write([]byte("0123456789"))
	}
	if w.buf.Len() != 10 {
		t.Errorf("the buffer itself must never grow past its cap, got %d bytes", w.buf.Len())
	}
	if w.total != 10000 {
		t.Errorf("total must still count every byte offered, got %d", w.total)
	}
}

// Bash's capture must stay bounded even when OutputMax is left unset (0,
// historically "no truncation at all") -- a hard ceiling applies regardless,
// so a flooding command cannot grow this process's memory without limit.
func TestBashIsBoundedEvenWithOutputMaxUnset(t *testing.T) {
	b, _ := newBuiltins(t)
	b.OutputMax = 0
	text, isErr := call(t, b, "Bash", `{"command":"yes | head -c 11000000"}`)
	if isErr {
		t.Errorf("hitting the hard ceiling must not itself be a tool error: %q", text)
	}
	if !strings.Contains(text, "truncated") {
		t.Errorf("an unconfigured OutputMax must still hit the hard memory ceiling, got a %d-byte result", len(text))
	}
}

// Bash's plain success path (no error, exit 0) is the one branch the
// existing exit-code and timeout tests never reach.
func TestBashPlainSuccess(t *testing.T) {
	b, _ := newBuiltins(t)
	text, isErr := call(t, b, "Bash", `{"command":"echo plain-success"}`)
	if isErr || !strings.Contains(text, "plain-success") {
		t.Errorf("got isErr=%v %q", isErr, text)
	}
}

// Bash's own error path that is neither a deadline nor an *exec.ExitError --
// a working directory that does not exist fails at process start.
func TestBashFailsToStartInAMissingWorkspace(t *testing.T) {
	b := Builtins{Workspace: filepath.Join(string(os.PathSeparator), "does-not-exist-agentops"), OutputMax: 1024, BashTimeout: 5 * time.Second}
	text, isErr := call(t, b, "Bash", `{"command":"echo hi"}`)
	if !isErr || !strings.HasPrefix(text, "error:") {
		t.Errorf("want a start failure, got isErr=%v %q", isErr, text)
	}
}
