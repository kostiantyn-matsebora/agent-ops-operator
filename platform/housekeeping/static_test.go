package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guardedResourceLiterals are the Kubernetes API resource names the old
// kube.go read this job's conversations through — the one door back to the
// API this change closes (state-store-generic-contract design D-8).
// housekeeping never read signals (the design's own reader table), so unlike
// the console's equivalent check this list omits it.
//
// Matched EXACTLY, not as a substring: a REST client names a resource with a
// bare literal like this ("conversations", the way the old kube.go's path
// ended), which is a different thing from the word appearing inside an
// ordinary English sentence such as reclaim.go's "listing conversations:
// %w" error text — that prose names nothing and must stay untouched.
var guardedResourceLiterals = map[string]bool{"conversations": true, "conversationinputs": true}

// guardedStoreFieldLiterals are the cooldown window and receipt counter
// fields the manager's state store keeps on a SignalSource — the signal half
// of the store, no business of this job's, guarded with NO exemption for any
// file, state.go included. Matched exactly, in the field's own casing, for
// the same reason as guardedResourceLiterals.
var guardedStoreFieldLiterals = map[string]bool{"cooldown": true, "receivedTotal": true, "lastReceived": true}

// stateClientFile is the one file allowed to name a guarded resource
// literal: this job's own client for the manager's /state/* read surface.
const stateClientFile = "state.go"

// stateLeak is one string literal naming what this job must reach only
// through state.go, or never at all.
type stateLeak struct {
	file string
	line int
	text string
}

func (v stateLeak) String() string { return fmt.Sprintf("%s:%d: names %q", v.file, v.line, v.text) }

// TestReadsNoStateFromKubernetes is this module's static check of design
// D-8: every non-test .go file outside state.go is walked for a string
// literal naming a conversation/conversationinput Kubernetes resource — the
// direct API access this change replaces with the manager's state read
// surface — and every such file, state.go included, is walked for the
// cooldown window or receipt counter fields, which belong to the
// SignalSource half of the store this job never touches at all.
func TestReadsNoStateFromKubernetes(t *testing.T) {
	found, err := findStateLeaks(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		lines := make([]string, len(found))
		for i, v := range found {
			lines[i] = v.String()
		}
		t.Fatalf("conversation state is reachable only through state.go's client of the manager's /state/* surface:\n  %s",
			strings.Join(lines, "\n  "))
	}
}

// findStateLeaks parses every non-test Go file under root and reports each
// guarded literal it names, by the rules above.
func findStateLeaks(root string) ([]stateLeak, error) {
	var out []stateLeak
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		vs, scanErr := leaksIn(path, rel)
		out = append(out, vs...)
		return scanErr
	})
	return out, err
}

// leaksIn reports every guarded literal one file names.
func leaksIn(path, rel string) ([]stateLeak, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, err)
	}
	exemptResource := rel == stateClientFile
	var out []stateLeak
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		line := fset.Position(lit.Pos()).Line
		if !exemptResource && guardedResourceLiterals[v] {
			out = append(out, stateLeak{file: rel, line: line, text: v})
		}
		if guardedStoreFieldLiterals[v] {
			out = append(out, stateLeak{file: rel, line: line, text: v})
		}
		return true
	})
	return out, nil
}

// The check must actually fire on what it forbids, name the file, and leave
// the exempt file and an unrelated literal alone.
func TestStateLeakCheckFiresOnASyntheticViolation(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("bad_path.go", "package main\nconst resource = \"conversations\"\n")
	write("bad_inputs.go", "package main\nconst resource = \"conversationinputs\"\n")
	write("ok_prose.go", "package main\nimport \"fmt\"\nfunc f() error { return fmt.Errorf(\"listing conversations: %w\", nil) }\n")
	write("bad_cooldown.go", "package main\nconst f = \"cooldown\"\n")
	write("bad_received.go", "package main\nconst f = \"receivedTotal\"\n")
	write("bad_last.go", "package main\nconst f = \"lastReceived\"\n")
	write("ok.go", "package main\nconst f = \"unrelated\"\n")
	// The exempt file: an exact "conversations" literal inside it must not
	// be flagged as a resource-literal violation.
	write("state.go", "package main\nconst resource = \"conversations\"\n")
	// A DIFFERENT file naming a guarded store field has no exemption, ever.
	write("another.go", "package main\nconst f = \"cooldown\"\n")

	found, err := findStateLeaks(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, v := range found {
		seen[v.file] = true
	}
	for _, want := range []string{
		"bad_path.go", "bad_inputs.go", "bad_cooldown.go", "bad_received.go", "bad_last.go", "another.go",
	} {
		if !seen[want] {
			t.Errorf("want %s flagged, found %v", want, found)
		}
	}
	if seen["ok.go"] {
		t.Errorf("an unrelated literal must not be flagged: %v", found)
	}
	if seen["ok_prose.go"] {
		t.Errorf("the word appearing inside an ordinary error sentence must not be flagged: %v", found)
	}
	if seen["state.go"] {
		t.Errorf("state.go's own exact \"conversations\" literal must be exempt from the resource-literal check: %v", found)
	}
}
