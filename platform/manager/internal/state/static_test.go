package state

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

// apiImportPath is the package whose Conversation and ConversationInput types
// the state store is the only door to.
const apiImportPath = "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"

// guardedTypes are the stored kinds no code outside the kube backend may name.
// Naming one is how a Kubernetes client call takes one: as the object to read
// into, the list to fill or the value to patch.
var guardedTypes = map[string]bool{
	"Conversation": true, "ConversationList": true,
	"ConversationInput": true, "ConversationInputList": true,
}

// A violation is one reference to a guarded type outside the allowed places.
type violation struct {
	pkg, file string
	line      int
	typ, call string
}

func (v violation) String() string {
	where := fmt.Sprintf("package %s (%s:%d) names v1alpha1.%s", v.pkg, v.file, v.line, v.typ)
	if v.call != "" {
		where += " in the call " + v.call
	}
	return where
}

// TestNoManagerPackageReachesAConversationExceptThroughTheStore is the static
// check of state-store design D-2/1.3: every manager package is walked, and a
// reference to a Conversation or ConversationInput (or their lists) fails the
// build unless it sits in internal/state/kube, the API types' own package, a
// test file or the black-box suites under test/.
//
// It is deliberately stricter than "a client call taking one": a type that is
// never named cannot be handed to a client, so there is no way around the check
// by holding the object in a variable first.
func TestNoManagerPackageReachesAConversationExceptThroughTheStore(t *testing.T) {
	root := filepath.Join("..", "..")
	found, err := findViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		lines := make([]string, len(found))
		for i, v := range found {
			lines[i] = v.String()
		}
		t.Fatalf("Conversation state is reachable only through state.Store (internal/state/kube is the one backend):\n  %s",
			strings.Join(lines, "\n  "))
	}
}

// The check must actually fail on what it forbids, and name where.
func TestStaticCheckNamesThePackageAndTheCall(t *testing.T) {
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
	const header = "package p\nimport v1 \"" + apiImportPath + "\"\nimport \"context\"\n"
	write("internal/chat/bad.go", header+`
type reader interface{ Get(context.Context, string, any) error }
func f(r reader) { _ = r.Get(context.Background(), "x", &v1.Conversation{}) }
`)
	write("internal/chat/bad_list.go", header+"var _ v1.ConversationList\n")
	write("internal/chat/bad_input.go", header+"var _ *v1.ConversationInput\n")
	write("internal/chat/ok_test.go", header+"var _ v1.Conversation\n")           // test files may seed objects
	write("internal/state/kube/ok.go", header+"var _ v1.Conversation\n")          // the backend
	write("api/v1alpha1/ok.go", "package v1alpha1\ntype Conversation struct{}\n") // the types themselves
	write("test/system/ok.go", header+"var _ v1.Conversation\n")                  // black-box suites
	write("internal/chat/other.go", header+"var _ v1.Channel\n")                  // another kind

	found, err := findViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range found {
		got = append(got, v.String())
	}
	sort.Strings(got)
	if len(got) != 3 {
		t.Fatalf("want exactly the three offending files flagged, got:\n%s", strings.Join(got, "\n"))
	}
	for _, line := range got {
		if !strings.Contains(line, "package internal/chat") {
			t.Errorf("a violation must name its package: %q", line)
		}
	}
	if !strings.Contains(strings.Join(got, "\n"), "in the call r.Get") {
		t.Errorf("a violation inside a call must name the call:\n%s", strings.Join(got, "\n"))
	}
}

// findViolations parses every non-test Go file under root and reports each
// reference to a guarded type from outside the allowed places.
func findViolations(root string) ([]violation, error) {
	var out []violation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if allowedDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		vs, err := violationsIn(path, rel)
		out = append(out, vs...)
		return err
	})
	return out, err
}

// allowedDir names the places a guarded type may appear: the backend, the API
// types' own package and the black-box suites that drive a running manager.
func allowedDir(rel string) bool {
	return rel == "internal/state/kube" || rel == "api" || rel == "test" ||
		strings.HasPrefix(rel, "internal/state/kube/") || strings.HasPrefix(rel, "api/") || strings.HasPrefix(rel, "test/")
}

func violationsIn(path, rel string) ([]violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, err)
	}
	aliases := apiAliases(file)
	if len(aliases) == 0 {
		return nil, nil
	}
	pkg := filepath.ToSlash(filepath.Dir(rel))
	var out []violation
	var calls []string // the stack of enclosing call names
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			calls = append(calls, callName(x))
			for _, child := range append([]ast.Expr{x.Fun}, x.Args...) {
				ast.Inspect(child, visit)
			}
			calls = calls[:len(calls)-1]
			return false
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if ok && aliases[id.Name] && guardedTypes[x.Sel.Name] {
				call := ""
				if len(calls) > 0 {
					call = calls[len(calls)-1]
				}
				out = append(out, violation{
					pkg: pkg, file: rel, line: fset.Position(x.Pos()).Line, typ: x.Sel.Name, call: call,
				})
			}
		}
		return true
	}
	ast.Inspect(file, visit)
	return out, nil
}

// apiAliases lists the local names a file imports the API package under.
func apiAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != apiImportPath {
			continue
		}
		name := "v1alpha1"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = true
	}
	return aliases
}

// callName renders a call's function for the report: recv.Method or name.
func callName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			return id.Name + "." + f.Sel.Name
		}
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}
