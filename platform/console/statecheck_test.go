package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The static check of state-store-generic-contract design D-8: no package in
// this module reads a Conversation or a Signal from the Kubernetes API, and
// no package names the SignalSource status fields the store owns by their
// wire path. state.go is the one door — the console's apiSource for
// StateKinds, talking to the manager's /state/* surface instead.
//
// The console carries no Go TYPE for a Conversation or a Signal (everything
// here is untyped JSON, cache.go's Object), so this check cannot walk
// selectors onto a guarded struct the way the manager's own
// internal/state/static_test.go does. What it guards instead is the two
// concrete doors a kind STRING opens in this module's own shape: naming the
// kind in a Kinds/InstallKinds-shaped list (which drives the Kubernetes
// watch loop and the chart's Role), and naming it as the literal `kind`
// argument of an apiSource-shaped call (`List(ctx, kind)`, `Get(ctx, kind,
// name)`, `Watch(ctx, kind, rv, fn)`) — the one call shape that reaches the
// Kubernetes API, as opposed to Cache's own `List(kind)`/`Get(kind, name)`
// (no ctx, one fewer argument), which every business-logic file in this
// module calls freely and correctly, state-backed kinds included.

// guardedKindLiterals are the kind strings design D-8 names: naming one
// outside state.go is the same raw-Kubernetes-request bypass StateKinds and
// the composite router in state.go exist to close.
var guardedKindLiterals = map[string]bool{"conversations": true, "conversationinputs": true, "signals": true}

// guardedWireNames are the SignalSource status fields the manager's store
// owns, by their lowercase wire name — this module never reads a cooldown
// window or a receipt counter at all (D-6: the console reads cooldown
// STATS, a different, still-allowed thing, from GET /status, not from a
// SignalSource's own status), so no file, state.go included, may name them.
var guardedWireNames = []string{`"cooldown"`, `"receivedtotal"`, `"lastreceived"`}

// stateClientFile is the one file allowed to name a guarded kind literal —
// the console's apiSource for StateKinds (task 8.1).
const stateClientFile = "state.go"

// kindViolation is one place a guarded string was found.
type kindViolation struct {
	file string
	line int
	why  string
}

func (v kindViolation) String() string { return fmt.Sprintf("%s:%d: %s", v.file, v.line, v.why) }

// TestReadsNoStateFromKubernetes is the static check itself (design D-8):
// every .go file in this module outside state.go must never name
// "conversations", "conversationinputs" or "signals" as a Kinds/
// InstallKinds-shaped list entry or an apiSource-shaped call's kind
// argument, and no file at all — state.go included — may name a
// SignalSource status field the store owns by its lowercase wire path.
func TestReadsNoStateFromKubernetes(t *testing.T) {
	found, err := findKindViolations(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		lines := make([]string, len(found))
		for i, v := range found {
			lines[i] = v.String()
		}
		t.Fatalf("state is reachable only through state.go (the manager's /state/* surface):\n  %s",
			strings.Join(lines, "\n  "))
	}
}

// TestKindsAreDisjointFromStateKinds is design D-8's other half: Kinds (the
// Kubernetes watch, and what the chart derives its Role from) and StateKinds
// (the manager's state read surface) must never overlap, or the console
// would ask the Kubernetes API for a kind the chart no longer grants it.
func TestKindsAreDisjointFromStateKinds(t *testing.T) {
	for _, k := range Kinds {
		for _, s := range StateKinds {
			if k == s {
				t.Fatalf("Kinds and StateKinds both name %q — they must be disjoint (design D-8)", k)
			}
		}
	}
}

// findKindViolations parses every non-test .go file under root and reports
// the wire-name violations in all of them, state.go included (no
// exemption at all for those three — design D-8), plus the guarded-kind-
// literal doors in every one of them EXCEPT stateClientFile.
func findKindViolations(root string) ([]kindViolation, error) {
	var out []kindViolation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipKindDir(root, path, d)
		}
		// fixtures: never walked, matching the manager's own
		// internal/state/static_test.go, which excludes every _test.go
		// file from the whole check, not only the kind-literal half
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		vs, perr := fileKindViolations(path, filepath.ToSlash(rel))
		out = append(out, vs...)
		return perr
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].line < out[j].line
	})
	return out, err
}

// skipKindDir skips the UI's own tree (a different toolchain, never reaching
// this module's apiSource) and dot-directories.
func skipKindDir(root, path string, d fs.DirEntry) error {
	if d.Name() == "ui" || (path != root && strings.HasPrefix(d.Name(), ".")) {
		return filepath.SkipDir
	}
	return nil
}

// fileKindViolations parses one file and reports its violations.
func fileKindViolations(path, rel string) ([]kindViolation, error) {
	fset := token.NewFileSet()
	file, perr := parser.ParseFile(fset, path, nil, 0)
	if perr != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, perr)
	}
	out := wireNameViolations(fset, file, rel)
	if rel != stateClientFile {
		out = append(out, kindLiteralViolations(fset, file, rel)...)
	}
	return out, nil
}

// kindLiteralViolations finds the two doors a guarded kind string opens
// outside the state client: a Kinds/InstallKinds-shaped top-level
// []string{...} entry, and the kind argument of an apiSource-shaped call.
func kindLiteralViolations(fset *token.FileSet, file *ast.File, rel string) []kindViolation {
	var out []kindViolation
	for _, decl := range file.Decls {
		if lit := topLevelStringSliceLiteral(decl); lit != nil {
			for _, elt := range lit.Elts {
				if kind, ok := unquotedKind(elt); ok {
					out = append(out, kindViolation{
						file: rel, line: fset.Position(elt.Pos()).Line,
						why: fmt.Sprintf("%q named in a top-level []string{...} (a Kinds/InstallKinds-shaped list)", kind),
					})
				}
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if v, ok := kindCallViolation(fset, n, rel); ok {
			out = append(out, v)
		}
		return true
	})
	return out
}

// kindCallArgs maps an apiSource-shaped method to its argument count.
var kindCallArgs = map[string]int{"List": 2, "Get": 3, "Watch": 4}

// kindCallViolation reports n when it is an apiSource-shaped call naming a
// guarded kind as its second argument.
func kindCallViolation(fset *token.FileSet, n ast.Node, rel string) (kindViolation, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return kindViolation{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != kindCallArgs[sel.Sel.Name] {
		return kindViolation{}, false
	}
	kind, ok := unquotedKind(call.Args[1])
	if !ok {
		return kindViolation{}, false
	}
	return kindViolation{
		file: rel, line: fset.Position(call.Pos()).Line,
		why: fmt.Sprintf("%q named as the kind argument of %s.%s(ctx, ...) (an apiSource-shaped call)", kind, exprString(sel.X), sel.Sel.Name),
	}, true
}

// topLevelStringSliceLiteral returns decl's composite literal when decl is a
// file-scope `var Name = []string{...}` (or `var Name []string = []string{...}`,
// though Go code never writes it that way) — Kinds and InstallKinds' own
// shape, and nothing a function body or a map literal (Singular,
// groupVersion — pure display/routing lookups, no door to the API) matches.
func topLevelStringSliceLiteral(decl ast.Decl) *ast.CompositeLit {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return nil
	}
	for _, spec := range gen.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			if lit := stringSliceValue(vs); lit != nil {
				return lit
			}
		}
	}
	return nil
}

// stringSliceValue returns the first []string{...} literal among vs's values.
func stringSliceValue(vs *ast.ValueSpec) *ast.CompositeLit {
	for _, v := range vs.Values {
		lit, ok := v.(*ast.CompositeLit)
		if !ok {
			continue
		}
		arr, ok := lit.Type.(*ast.ArrayType)
		if !ok || arr.Len != nil {
			continue // fixed-size array, not a slice literal
		}
		if ident, ok := arr.Elt.(*ast.Ident); ok && ident.Name == "string" {
			return lit
		}
	}
	return nil
}

// unquotedKind reports whether e is a string literal naming a guarded kind.
func unquotedKind(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil && guardedKindLiterals[v]
}

// exprString renders a call's receiver for the violation message — best
// effort, never load-bearing for the check itself.
func exprString(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "_"
}

// wireNameViolations finds a literal reference to a guarded SignalSource
// status wire name, in ANY file — no exemption, not even state.go, since
// this module has no legitimate reason to ever name one (design D-8).
func wireNameViolations(fset *token.FileSet, file *ast.File, rel string) []kindViolation {
	var out []kindViolation
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text := strings.ToLower(lit.Value)
		for _, wire := range guardedWireNames {
			if strings.Contains(text, wire) {
				out = append(out, kindViolation{
					file: rel, line: fset.Position(lit.Pos()).Line,
					why: fmt.Sprintf("names the SignalSource status field %s by its wire path — the store owns it, read it through GET /status instead", wire),
				})
				break
			}
		}
		return true
	})
	return out
}

// The check must actually fire on what it forbids, and leave everything else
// alone — a synthetic tree, the same technique the manager's own
// internal/state/static_test.go uses, so this test does not merely assert
// against the real tree it is also meant to guard.
func TestStaticCheckFindsTheThreeDoorsAndNothingElse(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("bad_list.go", "package main\nvar BadKinds = []string{\"pipelines\", \"conversations\"}\n")
	write("bad_call.go", "package main\nimport \"context\"\nfunc f(k *Kube) { k.List(context.Background(), \"signals\") }\n")
	write("bad_get.go", "package main\nimport \"context\"\nfunc g(k *Kube) { k.Get(context.Background(), \"conversationinputs\", \"x\") }\n")
	write("bad_wire.go", "package main\nconst x = `{\"status\":{\"cooldown\":1}}`\n")
	write("ok_cache_call.go", "package main\nfunc h(c *Cache) { c.List(\"conversations\"); c.Get(\"signals\", \"x\") }\n") // Cache, not apiSource — no ctx, fewer args
	write("ok_map.go", "package main\nvar OkSingular = map[string]string{\"conversations\": \"Conversation\"}\n")          // display map, not a kind-list door
	write("ok_other_kind.go", "package main\nvar OkKinds = []string{\"pipelines\", \"channels\"}\n")
	write("ok_test.go", "package main\nfunc init() { _ = \"conversations\" }\n")                // a _test.go file: fixtures are exempt, same as the real tree's own obj() helpers
	write(stateClientFile, "package main\nvar ok = []string{\"conversations\", \"signals\"}\n") // the one exempt file

	found, err := findKindViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, v := range found {
		files = append(files, v.file)
	}
	sort.Strings(files)
	want := []string{"bad_call.go", "bad_get.go", "bad_list.go", "bad_wire.go"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("flagged %v, want exactly %v (ok_*.go and %s must be left alone)", files, want, stateClientFile)
	}
}
