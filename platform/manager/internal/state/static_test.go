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
	// The Signal half (design D-14): a Signal, its list and the cooldown entry
	// the router keeps on a SignalSource. "Status.Cooldown" is the field read
	// and written through a selector, checked below.
	"Signal": true, "SignalList": true, "CooldownEntry": true,
}

// cooldownField is the SignalSource status field the router reads and writes.
const cooldownField = "Cooldown"

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

	write("internal/chat/bad_dot.go", "package p\nimport . \""+apiImportPath+"\"\n")
	write("internal/chat/bad_partial.go", "package p\nimport metav1 \"k8s.io/apimachinery/pkg/apis/meta/v1\"\n"+
		"func g() any { return &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{Kind: \"ConversationInput\"}} }\n")
	write("internal/chat/bad_unstructured.go", "package p\nimport \"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured\"\n"+
		"func h() any { u := &unstructured.Unstructured{}; u.SetKind(\"Conversation\"); return u }\n")
	write("internal/chat/ok_unstructured.go", "package p\nimport \"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured\"\n"+
		"func h() any { u := &unstructured.Unstructured{}; u.SetKind(\"Pod\"); return u }\n") // another kind
	write("internal/controller/conversation_controller.go", "package p\nimport metav1 \"k8s.io/apimachinery/pkg/apis/meta/v1\"\n"+
		"const conversationKind = \"Conversation\"\n"+
		"func ownerOf() any { return &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{Kind: conversationKind}} }\n") // the one allowed helper

	found, err := findViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range found {
		got = append(got, v.String())
	}
	sort.Strings(got)
	if len(got) != 6 {
		t.Fatalf("want exactly the six offending files flagged, got:\n%s", strings.Join(got, "\n"))
	}
	for _, line := range got {
		if !strings.Contains(line, "package internal/chat") {
			t.Errorf("a violation must name its package: %q", line)
		}
	}
	for _, want := range []string{"dot-import", "bad_partial.go", "bad_unstructured.go"} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("the check missed %s:\n%s", want, strings.Join(got, "\n"))
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
	pkg := filepath.ToSlash(filepath.Dir(rel))
	out := untypedKindViolations(fset, file, pkg, rel)
	aliases, dotImported := apiAliases(file)
	if dotImported {
		// A dot import names every type unqualified, which the selector walk
		// below cannot see.
		out = append(out, violation{pkg: pkg, file: rel, line: 1, typ: "* (dot-import of the API package)"})
	}
	if len(aliases) == 0 && !mentionsStatusCooldown(file) {
		return out, nil
	}
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
			if inner, isSel := x.X.(*ast.SelectorExpr); isSel && inner.Sel.Name == "Status" &&
				x.Sel.Name == cooldownField {
				out = append(out, violation{
					pkg: pkg, file: rel, line: fset.Position(x.Pos()).Line, typ: "SignalSource Status." + cooldownField,
				})
			}
		}
		return true
	}
	ast.Inspect(file, visit)
	return out, nil
}

// apiAliases lists the local names a file imports the API package under, and
// whether it dot-imports it.
func apiAliases(file *ast.File) (aliases map[string]bool, dotImported bool) {
	aliases = map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != apiImportPath {
			continue
		}
		name := "v1alpha1"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			dotImported = true
			continue
		}
		aliases[name] = true
	}
	return aliases, dotImported
}

// untypedKinds are the metadata-only or schemaless object types that can name
// a kind without importing the API package.
var untypedKinds = map[string]bool{
	"PartialObjectMetadata": true, "PartialObjectMetadataList": true,
	"Unstructured": true, "UnstructuredList": true,
}

// ownerReferenceHelper is the one function outside the backend allowed to
// build a PartialObjectMetadata for a Conversation: the owner a runtime pod
// carries a controller reference to, built from identity alone (design D-7).
const (
	ownerReferenceHelperFile = "internal/controller/conversation_controller.go"
	ownerReferenceHelperFunc = "ownerOf"
	conversationKindIdent    = "conversationKind"
)

// untypedKindViolations finds a function that builds an unstructured or
// PartialObjectMetadata object and names a guarded kind in the same body (as a
// string literal or the controller's kind constant): the same door to a
// Conversation as naming the typed struct, just with no import to catch.
func untypedKindViolations(fset *token.FileSet, file *ast.File, pkg, rel string) []violation {
	var out []violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if rel == ownerReferenceHelperFile && fn.Name.Name == ownerReferenceHelperFunc {
			continue
		}
		var untypedAt token.Pos
		var kind string
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if untypedKinds[x.Sel.Name] && !untypedAt.IsValid() {
					untypedAt = x.Pos()
				}
			case *ast.BasicLit:
				if v, err := strconv.Unquote(x.Value); err == nil && x.Kind == token.STRING && guardedTypes[v] {
					kind = v
				}
			case *ast.Ident:
				if x.Name == conversationKindIdent {
					kind = "Conversation"
				}
			}
			return true
		})
		if untypedAt.IsValid() && kind != "" {
			out = append(out, violation{
				pkg: pkg, file: rel, line: fset.Position(untypedAt).Line, typ: kind + " (via an unstructured/PartialObjectMetadata object)",
				call: fn.Name.Name,
			})
		}
	}
	return out
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

// mentionsStatusCooldown reports whether a file selects a Status.Cooldown
// field, which needs no import of the API package to be written.
func mentionsStatusCooldown(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == cooldownField {
			if inner, isSel := sel.X.(*ast.SelectorExpr); isSel && inner.Sel.Name == "Status" {
				found = true
			}
		}
		return !found
	})
	return found
}

// The Signal half names the same doors: a Signal or its list, the cooldown
// entry, and the SignalSource status field, from any package except the
// backend, with no package excused.
func TestStaticCheckGuardsSignalsInEveryPackageOutsideTheBackend(t *testing.T) {
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
	const header = "package p\nimport v1 \"" + apiImportPath + "\"\n"
	write("internal/chat/bad_signal.go", header+"var _ *v1.Signal\n")
	write("internal/chat/bad_list.go", header+"var _ v1.SignalList\n")
	write("internal/chat/bad_entry.go", header+"var _ v1.CooldownEntry\n")
	write("internal/chat/bad_field.go", "package p\ntype src struct{ Status struct{ Cooldown []int } }\nfunc f(s *src) { s.Status.Cooldown = nil }\n")
	write("internal/chat/ok_source.go", header+"var _ v1.SignalSource\nvar _ v1.SignalAdapter\n") // other kinds
	write("internal/signalroute/bad_route.go", header+"var _ *v1.Signal\ntype src struct{ Status struct{ Cooldown []int } }\nfunc f(s *src) { _ = s.Status.Cooldown }\n")
	write("internal/controller/bad_conversation.go", header+"var _ v1.Conversation\n")
	write("internal/state/kube/ok.go", header+"var _ v1.Signal\n")

	found, err := findViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var files []string
	for _, v := range found {
		if !seen[v.file] {
			seen[v.file] = true
			files = append(files, v.file)
		}
	}
	sort.Strings(files)
	want := []string{
		"internal/chat/bad_entry.go", "internal/chat/bad_field.go", "internal/chat/bad_list.go",
		"internal/chat/bad_signal.go", "internal/controller/bad_conversation.go", "internal/signalroute/bad_route.go",
	}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("flagged %v, want %v", files, want)
	}
}
