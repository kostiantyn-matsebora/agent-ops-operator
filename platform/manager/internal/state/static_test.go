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
	// SignalSpec and SignalStatus are the halves of a Signal an embedded or
	// copied value reaches without naming the whole.
	"Signal": true, "SignalList": true, "CooldownEntry": true,
	"SignalSpec": true, "SignalStatus": true,
}

// guardedStatusFields are the SignalSource status fields the manager writes
// through the store: the window and the receipt counters.
var guardedStatusFields = map[string]bool{"Cooldown": true, "ReceivedTotal": true, "LastReceived": true}

// guardedWireNames are the same fields by their JSON names.
var guardedWireNames = []string{`"cooldown"`, `"receivedtotal"`, `"lastreceived"`}

// guardedAnnotationPrefix is the adapter state annotation prefix (design
// D-8): the one place a backend may name where AdapterState's `kube` layout
// actually lives. A literal reference anywhere else is the same raw-annotation
// bypass the /channel/state and /signal/state handlers used before task 5.2
// moved them onto Stores.AdapterState.
const guardedAnnotationPrefix = "agentops.dev/adapter-state-"

// adapterStateReleaseExemptFile is the ONE call site outside the kube backend
// allowed to name the prefix: the owning reconciler's release of a deleted
// Channel's or SignalSource's own keys (internal/controller/adapterstate_release.go),
// which must read the object's own annotations directly because AdapterState
// declares no index to list them (task 5.2).
const adapterStateReleaseExemptFile = "internal/controller/adapterstate_release.go"

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
	files, err := checkedFiles(root)
	if err != nil {
		return nil, err
	}
	kindNames, err := guardedKindNames(files)
	if err != nil {
		return nil, err
	}
	var out []violation
	for _, f := range files {
		vs, err := violationsIn(f.path, f.rel, kindNames)
		out = append(out, vs...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

type checkedFile struct{ path, rel string }

// checkedFiles lists the non-test Go files outside the allowed places.
func checkedFiles(root string) ([]checkedFile, error) {
	var out []checkedFile
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
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, checkedFile{path, rel})
		}
		return nil
	})
	return out, err
}

// guardedKindNames finds every constant or variable, in any checked file, that
// holds a guarded kind as a string literal, so a function naming the kind
// through it is read as naming the kind. By name across the tree, not scoped.
func guardedKindNames(files []checkedFile) (map[string]string, error) {
	names := map[string]string{}
	for _, f := range files {
		file, err := parser.ParseFile(token.NewFileSet(), f.path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f.rel, err)
		}
		collectGuardedKinds(file, names)
	}
	return names, nil
}

// collectGuardedKinds records each value spec in file that binds a guarded kind
// to a name.
func collectGuardedKinds(file *ast.File, names map[string]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != len(spec.Values) {
			return true
		}
		for i, v := range spec.Values {
			if kind, isKind := guardedKindLiteral(v); isKind {
				names[spec.Names[i].Name] = kind
			}
		}
		return true
	})
}

// guardedKindLiteral returns the kind a string literal names, when it is one of
// the guarded kinds.
func guardedKindLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	kind, err := strconv.Unquote(lit.Value)
	return kind, err == nil && guardedTypes[kind]
}

// allowedDir names the places a guarded type may appear: the backend, the API
// types' own package and the black-box suites that drive a running manager.
func allowedDir(rel string) bool {
	return rel == "internal/state/kube" || rel == "api" || rel == "test" ||
		strings.HasPrefix(rel, "internal/state/kube/") || strings.HasPrefix(rel, "api/") || strings.HasPrefix(rel, "test/")
}

func violationsIn(path, rel string, kindNames map[string]string) ([]violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, err)
	}
	pkg := filepath.ToSlash(filepath.Dir(rel))
	out := untypedKindViolations(fset, file, pkg, rel, kindNames)
	out = append(out, lowercaseCooldownViolations(fset, file, pkg, rel)...)
	out = append(out, adapterStateAnnotationViolations(fset, file, pkg, rel)...)
	aliases, dotImported := apiAliases(file)
	if dotImported {
		// A dot import names every type unqualified, which the selector walk
		// below cannot see.
		out = append(out, violation{pkg: pkg, file: rel, line: 1, typ: "* (dot-import of the API package)"})
	}
	statusAliases := statusAliasNames(file)
	if len(aliases) == 0 && len(statusAliases) == 0 && !mentionsStatusCooldown(file) {
		return out, nil
	}
	w := &selectorWalk{fset: fset, pkg: pkg, rel: rel, aliases: aliases, statusAliases: statusAliases, out: out}
	ast.Inspect(file, w.visit)
	return w.out, nil
}

// selectorWalk finds the selector doors to a guarded type or status field.
type selectorWalk struct {
	fset          *token.FileSet
	pkg, rel      string
	aliases       map[string]bool
	statusAliases map[string]bool
	calls         []string // the stack of enclosing call names
	out           []violation
}

func (w *selectorWalk) visit(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.CallExpr:
		w.calls = append(w.calls, callName(x))
		for _, child := range append([]ast.Expr{x.Fun}, x.Args...) {
			ast.Inspect(child, w.visit)
		}
		w.calls = w.calls[:len(w.calls)-1]
		return false
	case *ast.SelectorExpr:
		w.selector(x)
	}
	return true
}

func (w *selectorWalk) selector(x *ast.SelectorExpr) {
	line := w.fset.Position(x.Pos()).Line
	id, ok := x.X.(*ast.Ident)
	if ok && w.aliases[id.Name] && guardedTypes[x.Sel.Name] {
		call := ""
		if len(w.calls) > 0 {
			call = w.calls[len(w.calls)-1]
		}
		w.out = append(w.out, violation{pkg: w.pkg, file: w.rel, line: line, typ: x.Sel.Name, call: call})
	}
	if guardedStatusFields[x.Sel.Name] && selectsStatus(x.X, w.statusAliases) {
		w.out = append(w.out, violation{pkg: w.pkg, file: w.rel, line: line, typ: "SignalSource Status." + x.Sel.Name})
	}
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
)

// untypedKindViolations finds a function that builds an unstructured or
// PartialObjectMetadata object and names a guarded kind in the same body (as a
// string literal or the controller's kind constant): the same door to a
// Conversation as naming the typed struct, just with no import to catch.
func untypedKindViolations(fset *token.FileSet, file *ast.File, pkg, rel string, kindNames map[string]string) []violation {
	var out []violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || (rel == ownerReferenceHelperFile && fn.Name.Name == ownerReferenceHelperFunc) {
			continue
		}
		scan := &bodyScan{fn: fn, kindNames: kindNames}
		ast.Inspect(fn.Body, scan.visit)
		if scan.unresolved != "" && scan.untypedAt.IsValid() {
			out = append(out, violation{
				pkg: pkg, file: rel, line: fset.Position(fn.Pos()).Line,
				typ:  "a kind taken from the parameter " + scan.unresolved + " (via an unstructured/PartialObjectMetadata object)",
				call: fn.Name.Name,
			})
		}
		if scan.untypedAt.IsValid() && scan.kind != "" {
			out = append(out, violation{
				pkg: pkg, file: rel, line: fset.Position(scan.untypedAt).Line, typ: scan.kind + " (via an unstructured/PartialObjectMetadata object)",
				call: fn.Name.Name,
			})
		}
	}
	return out
}

// bodyScan reads one function body for an untyped object and a guarded kind.
type bodyScan struct {
	fn         *ast.FuncDecl
	kindNames  map[string]string
	untypedAt  token.Pos
	kind       string
	unresolved string
}

func (s *bodyScan) visit(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.SelectorExpr:
		if untypedKinds[x.Sel.Name] && !s.untypedAt.IsValid() {
			s.untypedAt = x.Pos()
		}
	case *ast.BasicLit:
		if v, ok := guardedKindLiteral(x); ok {
			s.kind = v
		}
	case *ast.Ident:
		if k, named := s.kindNames[x.Name]; named {
			s.kind = k
		}
	case *ast.CallExpr:
		if param := kindParameter(x, s.fn); param != "" {
			s.unresolved = param
		}
	}
	return true
}

// kindParameter returns the name of a function parameter handed to SetKind or
// SetGroupVersionKind, a kind the walk cannot resolve to a literal.
func kindParameter(c *ast.CallExpr, fn *ast.FuncDecl) string {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "SetKind" && sel.Sel.Name != "SetGroupVersionKind") || len(c.Args) == 0 {
		return ""
	}
	id, ok := c.Args[0].(*ast.Ident)
	if !ok {
		return ""
	}
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == id.Name {
				return id.Name
			}
		}
	}
	return ""
}

// lowercaseCooldownViolations finds the cooldown window reached by its wire
// path instead of the Go field: a function holding both a "status" and a
// "cooldown" string (a merge patch, a SetNestedField path), in any form.
func lowercaseCooldownViolations(fset *token.FileSet, file *ast.File, pkg, rel string) []violation {
	var out []violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var scan wireScan
		ast.Inspect(fn.Body, scan.visit)
		if scan.status.IsValid() && scan.cooldown.IsValid() {
			out = append(out, violation{
				pkg: pkg, file: rel, line: fset.Position(scan.cooldown).Line,
				typ: "a SignalSource status field the store owns, by its wire path", call: fn.Name.Name,
			})
		}
	}
	return out
}

// wireScan records the first "status" and the first guarded wire-name string
// literal of a function body.
type wireScan struct{ status, cooldown token.Pos }

func (s *wireScan) visit(n ast.Node) bool {
	lit, isLit := n.(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return true
	}
	text := strings.ToLower(lit.Value)
	if strings.Contains(text, `"status"`) && !s.status.IsValid() {
		s.status = lit.Pos()
	}
	if !s.cooldown.IsValid() && namesWireField(text) {
		s.cooldown = lit.Pos()
	}
	return true
}

// adapterStateAnnotationViolations flags a literal reference to the adapter
// state annotation prefix (design D-8) anywhere outside the kube backend
// (already excluded by allowedDir) and the one release call site task 5.2
// adds, adapterStateReleaseExemptFile.
func adapterStateAnnotationViolations(fset *token.FileSet, file *ast.File, pkg, rel string) []violation {
	if rel == adapterStateReleaseExemptFile {
		return nil
	}
	var out []violation
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err == nil && strings.Contains(v, guardedAnnotationPrefix) {
			out = append(out, violation{pkg: pkg, file: rel, line: fset.Position(lit.Pos()).Line,
				typ: "the adapter state annotation prefix, outside the kube backend and its one release call site"})
		}
		return true
	})
	return out
}

func namesWireField(text string) bool {
	for _, wire := range guardedWireNames {
		if strings.Contains(text, wire) {
			return true
		}
	}
	return false
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

// selectsStatus reports whether expr is a `.Status` selection or a name this
// file bound to one (see statusAliasNames).
func selectsStatus(expr ast.Expr, aliases map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return e.Sel.Name == "Status"
	case *ast.Ident:
		return aliases[e.Name]
	}
	return false
}

// statusAliasNames lists the names a file binds to a `.Status` value or pointer
// (`st := &src.Status`, `st := src.Status`, `var st = &src.Status`), so that
// `st.Cooldown` is read as `src.Status.Cooldown`. The table is per file and by
// name, not scoped: a coarse net is the right one for a guard.
//
// Not covered, on purpose: a kind passed in a variable, and a lowercase or
// reflected field path. Those need type information the AST walk lacks, and the
// guard's job is to stop the plain doors.
func statusAliasNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			bindStatusAliases(names, x.Lhs, x.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(x.Names))
			for i, id := range x.Names {
				lhs[i] = id
			}
			bindStatusAliases(names, lhs, x.Values)
		}
		return true
	})
	return names
}

// bindStatusAliases records each name on the left bound to a `.Status` value or
// pointer on the right, pairwise.
func bindStatusAliases(names map[string]bool, lhs, rhs []ast.Expr) {
	if len(lhs) != len(rhs) {
		return
	}
	for i := range lhs {
		id, ok := lhs[i].(*ast.Ident)
		if !ok {
			continue
		}
		val := rhs[i]
		if u, isAddr := val.(*ast.UnaryExpr); isAddr && u.Op == token.AND {
			val = u.X
		}
		if sel, isSel := val.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Status" {
			names[id.Name] = true
		}
	}
}

// mentionsStatusCooldown reports whether a file selects a Status.Cooldown
// field, which needs no import of the API package to be written.
func mentionsStatusCooldown(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && guardedStatusFields[sel.Sel.Name] {
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
	write("internal/chat/bad_received.go", "package p\ntype src struct{ Status struct{ ReceivedTotal int64 } }\nfunc f(s *src) { s.Status.ReceivedTotal++ }\n")
	write("internal/chat/bad_status.go", header+"var _ v1.SignalStatus\n")
	write("internal/chat/bad_spec.go", header+"var _ *v1.SignalSpec\n")
	write("internal/chat/bad_alias.go", "package p\ntype src struct{ Status struct{ Cooldown []int } }\nfunc f(s *src) { st := &s.Status; st.Cooldown = nil }\n")
	write("internal/chat/bad_alias_value.go", "package p\ntype src struct{ Status struct{ Cooldown []int } }\nfunc f(s *src) { var st = s.Status; _ = st.Cooldown }\n")
	write("internal/chat/ok_alias.go", "package p\ntype src struct{ Status struct{ Phase string }; Other struct{ Cooldown int } }\nfunc f(s *src) { st := &s.Status; o := &s.Other; _, _ = st.Phase, o.Cooldown }\n") // no Status.Cooldown
	write("internal/chat/ok_source.go", header+"var _ v1.SignalSource\nvar _ v1.SignalAdapter\n")                                                                                                                     // other kinds
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
		"internal/chat/bad_alias.go", "internal/chat/bad_alias_value.go",
		"internal/chat/bad_entry.go", "internal/chat/bad_field.go", "internal/chat/bad_list.go",
		"internal/chat/bad_received.go", "internal/chat/bad_signal.go", "internal/chat/bad_spec.go", "internal/chat/bad_status.go", "internal/controller/bad_conversation.go", "internal/signalroute/bad_route.go",
	}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("flagged %v, want %v", files, want)
	}
}

// The two bypasses a type-only walk would miss: a kind held in a constant or
// handed in as a parameter, and the cooldown window named by its wire path.
func TestStaticCheckClosesTheKindVariableAndLowercasePathBypasses(t *testing.T) {
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
	const un = "import \"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured\"\n"
	write("internal/chat/kinds.go", "package p\nconst sneaky = \"Signal\"\n")
	write("internal/chat/bad_const.go", "package p\n"+un+"func f() any { u := &unstructured.Unstructured{}; u.SetKind(sneaky); return u }\n")
	write("internal/chat/bad_param.go", "package p\n"+un+"func f(kind string) any { u := &unstructured.Unstructured{}; u.SetKind(kind); return u }\n")
	write("internal/chat/ok_other.go", "package p\n"+un+"func f() any { u := &unstructured.Unstructured{}; u.SetKind(\"Pod\"); return u }\n")
	write("internal/chat/bad_patch.go", "package p\nfunc f() []byte { return []byte(`{\"status\":{\"cooldown\":[]}}`) }\n")
	write("internal/chat/bad_path.go", "package p\nfunc f() []string { return []string{\"status\", \"cooldown\"} }\n")
	write("internal/chat/ok_status.go", "package p\nfunc f() []string { return []string{\"status\", \"phase\"} }\n")
	write("internal/chat/ok_cooldown.go", "package p\nfunc f() string { return \"cooldownHours\" }\n")
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
	want := "internal/chat/bad_const.go,internal/chat/bad_param.go,internal/chat/bad_patch.go,internal/chat/bad_path.go"
	if strings.Join(files, ",") != want {
		t.Fatalf("flagged %v, want %s", files, want)
	}
}

// The adapter state annotation prefix (design D-8): a bypass outside the
// kube backend is flagged, the one named release call site is not, and
// another kind of literal naming no such prefix is left alone.
func TestStaticCheckGuardsTheAdapterStateAnnotationPrefix(t *testing.T) {
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
	write("internal/chat/bad_prefix.go", "package p\nconst x = \"agentops.dev/adapter-state-\"\n")
	write("internal/chat/bad_use.go", "package p\nfunc f(ch map[string]string) string { return ch[\"agentops.dev/adapter-state-offset\"] }\n")
	write("internal/chat/ok_other.go", "package p\nconst y = \"agentops.dev/other-prefix-\"\n")
	write(adapterStateReleaseExemptFile, "package controller\nconst exempt = \"agentops.dev/adapter-state-\"\n")

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
	want := []string{"internal/chat/bad_prefix.go", "internal/chat/bad_use.go"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("flagged %v, want %v (the release file and the unrelated prefix must not be)", files, want)
	}
}

// natsGoImportPrefix is the client library the nats backend introduces
// (design D-1 of nats-state-store-default): the package itself and every
// subpackage (notably jetstream). internal/state/nats is the only package
// allowed to import it, mirroring this file's own guard on
// internal/state/kube's Conversation/Signal types above — a backend's own
// dependency must stay exactly as contained as the types it maps.
const natsGoImportPrefix = "github.com/nats-io/nats.go"

// natsBackendDir is the one package the guard excludes, test files included:
// a leaked import in a _test.go file is the same bypass as one in ordinary
// code, since go test still builds and links it.
const natsBackendDir = "internal/state/nats"

// TestOnlyTheNatsBackendImportsTheNatsClientLibrary is the static check task
// 2.1 of nats-state-store-default asks for: nothing outside
// internal/state/nats may import nats.go or one of its subpackages.
func TestOnlyTheNatsBackendImportsTheNatsClientLibrary(t *testing.T) {
	root := filepath.Join("..", "..")
	found, err := findNatsGoImports(root, natsBackendDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("only %s may import %s (design D-1 of nats-state-store-default):\n  %s",
			natsBackendDir, natsGoImportPrefix, strings.Join(found, "\n  "))
	}
}

// findNatsGoImports walks every .go file under root (test files included,
// vendor-equivalent dirs skipped by a module-only repo having none) and
// reports each file outside exemptDir that imports natsGoImportPrefix or one
// of its subpackages.
func findNatsGoImports(root, exemptDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == exemptDir || strings.HasPrefix(rel, exemptDir+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", rel, err)
		}
		for _, imp := range file.Imports {
			p, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if p == natsGoImportPrefix || strings.HasPrefix(p, natsGoImportPrefix+"/") {
				out = append(out, fmt.Sprintf("%s imports %s", rel, p))
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// The check must actually fire outside the backend and leave the backend
// itself alone.
func TestStaticCheckGuardsTheNatsClientLibraryImport(t *testing.T) {
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
	write("internal/chat/bad.go", "package p\nimport \""+natsGoImportPrefix+"\"\nvar _ = nats.ErrInvalidArg\n")
	write("internal/chat/bad_sub.go", "package p\nimport \""+natsGoImportPrefix+"/jetstream\"\nvar _ = jetstream.ErrKeyNotFound\n")
	write("internal/chat/bad_test.go", "package p\nimport \""+natsGoImportPrefix+"\"\nvar _ = nats.ErrInvalidArg\n")
	write("internal/state/nats/ok.go", "package nats\nimport \""+natsGoImportPrefix+"\"\nvar _ = nats.ErrInvalidArg\n")
	write("internal/state/nats/ok_test.go", "package nats\nimport \""+natsGoImportPrefix+"\"\nvar _ = nats.ErrInvalidArg\n")
	write("internal/chat/ok_other.go", "package p\nimport \"github.com/nats-io/nuid\"\nvar _ = nuid.Next\n")

	found, err := findNatsGoImports(root, natsBackendDir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, v := range found {
		files = append(files, strings.SplitN(v, " ", 2)[0])
	}
	sort.Strings(files)
	want := []string{"internal/chat/bad.go", "internal/chat/bad_sub.go", "internal/chat/bad_test.go"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("flagged %v, want %v (the backend's own files and the unrelated nuid import must not be)", files, want)
	}
}
