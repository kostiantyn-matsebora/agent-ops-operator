// The built-in tools — Read, Grep, Glob, Edit, Write, Bash — implemented
// natively, so the chart's agentops-observe / -shell / -edit toolsets mean the
// same thing on this runtime as on the reference one.
//
// Every path is confined to the workspace after symlink resolution, every
// result is size-bounded, and Bash is exactly what it says: the pod's shell,
// with whatever the route's identity can reach. That risk is not mitigated
// here and is not described as if it were — it is why the vocabulary ships
// risk-split, and why "should this agent have a shell" is a Pipeline binding.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Builtins configures the native tools.
type Builtins struct {
	Workspace   string
	OutputMax   int
	BashTimeout time.Duration
}

// Register adds the six built-ins to a registry.
func (b Builtins) Register(r *Registry) {
	r.Add(Tool{Name: "Read", Description: "Read a file in the workspace. Returns its text, truncated past the output limit.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"workspace-relative file path"}},"required":["path"]}`),
		Call:   b.read})
	r.Add(Tool{Name: "Grep", Description: "Search file contents in the workspace with a regular expression. Returns matching lines as path:line: text.",
		Schema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Go regular expression"},"path":{"type":"string","description":"workspace-relative directory or file to search, default the whole workspace"}},"required":["pattern"]}`),
		Call:   b.grep})
	r.Add(Tool{Name: "Glob", Description: "List workspace files matching a glob pattern such as **/*.go or docs/*.md.",
		Schema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`),
		Call:   b.glob})
	r.Add(Tool{Name: "Edit", Description: "Replace one exact occurrence of old_string with new_string in a workspace file.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"]}`),
		Call:   b.edit})
	r.Add(Tool{Name: "Write", Description: "Write a workspace file, creating or overwriting it.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		Call:   b.write})
	r.Add(Tool{Name: "Bash", Description: "Run a shell command in the workspace. Returns combined output and the exit code.",
		Schema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		Call:   b.bash})
}

var errEscapes = errors.New("path escapes the workspace")

// resolve confines a path to the workspace: absolute paths, `..` and symlinked
// escapes are all refused after evaluating whatever part of the path exists.
func (b Builtins) resolve(p string) (string, error) {
	if p == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(p) {
		return "", errEscapes
	}
	root, err := filepath.EvalSymlinks(b.Workspace)
	if err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	full := filepath.Join(root, p)
	if !within(root, full) {
		return "", errEscapes
	}
	// Evaluate the longest existing prefix so a symlink inside the workspace
	// pointing outside it is caught even when the leaf does not exist yet.
	existing, rest := full, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = filepath.Dir(existing)
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	if !within(root, real) {
		return "", errEscapes
	}
	return filepath.Join(real, rest), nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (b Builtins) bound(s string) string {
	if b.OutputMax > 0 && len(s) > b.OutputMax {
		return s[:b.OutputMax] + fmt.Sprintf("\n… [truncated: %d of %d bytes shown]", b.OutputMax, len(s))
	}
	return s
}

func decodeArgs(raw json.RawMessage, into any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("no arguments")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	return dec.Decode(into)
}

func (b Builtins) read(_ context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct{ Path string }
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Read: bad arguments: %v", err)
	}
	p, err := b.resolve(a.Path)
	if err != nil {
		return toolErr("Read %q: %v", a.Path, err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return toolErr("Read %q: %v", a.Path, err)
	}
	return b.bound(string(data)), false, nil
}

func (b Builtins) grep(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct{ Pattern, Path string }
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Grep: bad arguments: %v", err)
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return toolErr("Grep: bad pattern: %v", err)
	}
	if a.Path == "" {
		a.Path = "."
	}
	start, err := b.resolve(a.Path)
	if err != nil {
		return toolErr("Grep %q: %v", a.Path, err)
	}
	root, _ := filepath.EvalSymlinks(b.Workspace)
	var out strings.Builder
	walk := func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil // unreadable or binary
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				fmt.Fprintf(&out, "%s:%d: %s\n", rel, i+1, line)
				if b.OutputMax > 0 && out.Len() > b.OutputMax {
					return errStop
				}
			}
		}
		return nil
	}
	if err := filepath.WalkDir(start, walk); err != nil && !errors.Is(err, errStop) {
		return toolErr("Grep: %v", err)
	}
	if out.Len() == 0 {
		return "no matches", false, nil
	}
	return b.bound(out.String()), false, nil
}

var errStop = errors.New("stop")

func (b Builtins) glob(_ context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct{ Pattern string }
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Glob: bad arguments: %v", err)
	}
	if a.Pattern == "" || filepath.IsAbs(a.Pattern) || strings.Contains(a.Pattern, "..") {
		return toolErr("Glob: pattern must be workspace-relative")
	}
	root, err := filepath.EvalSymlinks(b.Workspace)
	if err != nil {
		return toolErr("Glob: %v", err)
	}
	var matches []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if globMatch(a.Pattern, rel) {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return toolErr("Glob: %v", err)
	}
	if len(matches) == 0 {
		return "no matches", false, nil
	}
	return b.bound(strings.Join(matches, "\n")), false, nil
}

// globMatch supports `**` as "any directories" on top of path.Match.
func globMatch(pattern, rel string) bool {
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, rel)
		return ok
	}
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && !strings.HasPrefix(rel, prefix+"/") {
		return false
	}
	if suffix == "" {
		return true
	}
	// The suffix may match the leaf at any depth.
	segs := strings.Split(rel, "/")
	for i := range segs {
		if ok, _ := filepath.Match(suffix, strings.Join(segs[i:], "/")); ok {
			return true
		}
	}
	return false
}

func (b Builtins) edit(_ context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Edit: bad arguments: %v", err)
	}
	p, err := b.resolve(a.Path)
	if err != nil {
		return toolErr("Edit %q: %v", a.Path, err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return toolErr("Edit %q: %v", a.Path, err)
	}
	s := string(data)
	switch n := strings.Count(s, a.OldString); {
	case a.OldString == "":
		return toolErr("Edit: old_string is empty")
	case n == 0:
		return toolErr("Edit %q: old_string not found", a.Path)
	case n > 1:
		return toolErr("Edit %q: old_string occurs %d times; make it unique", a.Path, n)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(s, a.OldString, a.NewString, 1)), 0o644); err != nil {
		return toolErr("Edit %q: %v", a.Path, err)
	}
	return "edited " + a.Path, false, nil
}

func (b Builtins) write(_ context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct{ Path, Content string }
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Write: bad arguments: %v", err)
	}
	p, err := b.resolve(a.Path)
	if err != nil {
		return toolErr("Write %q: %v", a.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return toolErr("Write %q: %v", a.Path, err)
	}
	if err := os.WriteFile(p, []byte(a.Content), 0o644); err != nil {
		return toolErr("Write %q: %v", a.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), false, nil
}

func (b Builtins) bash(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct{ Command string }
	if err := decodeArgs(raw, &a); err != nil {
		return toolErr("Bash: bad arguments: %v", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return toolErr("Bash: command is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, b.BashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", a.Command)
	cmd.Dir = b.Workspace
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	text := b.bound(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("%s\n[timed out after %s]", text, b.BashTimeout), true, nil
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return fmt.Sprintf("%s\n[exit %d]", text, ee.ExitCode()), true, nil
		}
		return toolErr("Bash: %v", err)
	}
	return text, false, nil
}
