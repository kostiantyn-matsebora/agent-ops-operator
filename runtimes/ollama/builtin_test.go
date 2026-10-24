package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newBuiltins(t *testing.T) (Builtins, string) {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "hello.txt"), []byte("hello world\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Builtins{Workspace: ws, OutputMax: 64, BashTimeout: 2 * time.Second}, ws
}

func call(t *testing.T, b Builtins, name, args string) (string, bool) {
	t.Helper()
	r := NewRegistry()
	b.Register(r)
	tool, ok := r.Get(name)
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	text, isErr, err := tool.Call(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: tool layer error: %v", name, err)
	}
	return text, isErr
}

func TestPathConfinement(t *testing.T) {
	b, ws := newBuiltins(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o644)
	os.Symlink(outside, filepath.Join(ws, "link"))
	os.Symlink(filepath.Join(outside, "secret"), filepath.Join(ws, "filelink"))
	for name, p := range map[string]string{
		"absolute":         filepath.Join(outside, "secret"),
		"dotdot":           "../" + filepath.Base(outside) + "/secret",
		"symlinked dir":    "link/secret",
		"symlinked file":   "filelink",
		"dotdot in middle": "sub/../../" + filepath.Base(outside) + "/secret",
	} {
		args, _ := json.Marshal(map[string]string{"path": p})
		text, isErr := call(t, b, "Read", string(args))
		if !isErr || !strings.Contains(text, "escapes") {
			t.Errorf("%s: expected an escape error, got isErr=%v %q", name, isErr, text)
		}
	}
	// a write through a symlinked directory is refused too, even to a new leaf
	args, _ := json.Marshal(map[string]string{"path": "link/new.txt", "content": "x"})
	if _, isErr := call(t, b, "Write", string(args)); !isErr {
		t.Error("Write through a symlinked escape must be refused")
	}
}

func TestReadGrepGlob(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Read", `{"path":"hello.txt"}`); isErr || !strings.HasPrefix(text, "hello world") {
		t.Errorf("Read: %v %q", isErr, text)
	}
	if text, _ := call(t, b, "Grep", `{"pattern":"second"}`); !strings.Contains(text, "hello.txt:2: second line") {
		t.Errorf("Grep: %q", text)
	}
	if text, _ := call(t, b, "Glob", `{"pattern":"**/*.txt"}`); text != "hello.txt" {
		t.Errorf("Glob: %q", text)
	}
	if text, isErr := call(t, b, "Grep", `{"pattern":"("}`); !isErr || !strings.Contains(text, "bad pattern") {
		t.Errorf("Grep bad pattern: %v %q", isErr, text)
	}
}

func TestOutputBound(t *testing.T) {
	b, ws := newBuiltins(t)
	os.WriteFile(filepath.Join(ws, "big.txt"), []byte(strings.Repeat("x", 1000)), 0o644)
	text, _ := call(t, b, "Read", `{"path":"big.txt"}`)
	if !strings.Contains(text, "[truncated: 64 of 1000 bytes shown]") {
		t.Errorf("truncation must be stated: %q", text)
	}
}

func TestEditWrite(t *testing.T) {
	b, ws := newBuiltins(t)
	if _, isErr := call(t, b, "Edit", `{"path":"hello.txt","old_string":"world","new_string":"there"}`); isErr {
		t.Error("edit failed")
	}
	data, _ := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if !strings.HasPrefix(string(data), "hello there") {
		t.Errorf("edit not applied: %q", data)
	}
	if text, isErr := call(t, b, "Edit", `{"path":"hello.txt","old_string":"nope","new_string":"x"}`); !isErr || !strings.Contains(text, "not found") {
		t.Errorf("Edit missing: %q", text)
	}
	if _, isErr := call(t, b, "Write", `{"path":"new/dir/f.txt","content":"hi"}`); isErr {
		t.Error("write failed")
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "new", "dir", "f.txt")); string(data) != "hi" {
		t.Error("write not applied")
	}
}

func TestBash(t *testing.T) {
	b, _ := newBuiltins(t)
	if text, isErr := call(t, b, "Bash", `{"command":"echo hi; exit 3"}`); !isErr || !strings.Contains(text, "hi") || !strings.Contains(text, "[exit 3]") {
		t.Errorf("Bash exit: %v %q", isErr, text)
	}
	b.BashTimeout = 200 * time.Millisecond
	start := time.Now()
	text, isErr := call(t, b, "Bash", `{"command":"sleep 5"}`)
	if !isErr || !strings.Contains(text, "timed out") || time.Since(start) > 4*time.Second {
		t.Errorf("Bash timeout: %v %q after %s", isErr, text, time.Since(start))
	}
}

func TestMalformedArguments(t *testing.T) {
	b, _ := newBuiltins(t)
	for _, name := range []string{"Read", "Grep", "Glob", "Edit", "Write", "Bash"} {
		for _, args := range []string{``, `not json`, `{"path":42}`, `[]`} {
			text, isErr := call(t, b, name, args)
			if !isErr || !strings.HasPrefix(text, "error:") {
				t.Errorf("%s(%q): want a readable tool error, got isErr=%v %q", name, args, isErr, text)
			}
		}
	}
}
