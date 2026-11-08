package state

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// typeCheckWithSnippet type-checks this package's non-test files plus one
// extra file, body, and returns the type errors. The package imports only the
// standard library, so no module resolution is needed: it is the repository's
// way to state a compile-time negative ("this does not compile") as a test.
func typeCheckWithSnippet(t *testing.T, body string) []error {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	snippet, err := parser.ParseFile(fset, "snippet.go", "package state\n"+body, 0)
	if err != nil {
		t.Fatalf("snippet does not parse: %v", err)
	}
	files = append(files, snippet)

	var errs []error
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil), Error: func(err error) { errs = append(errs, err) }}
	_, _ = conf.Check("state", fset, files, nil)
	return errs
}

func TestInventoryTypesRefuseWhatAKindLacks(t *testing.T) {
	const ctxImport = "import \"context\"\nvar _ = context.Background\n"
	cases := []struct {
		name, body, wantErr string // wantErr "" means the snippet must compile
	}{
		{"the control reads compile", ctxImport + `func f(s Stores) { s.Conversations.List(context.Background(), ByPhase, "Idle"); s.Signals.List(context.Background(), SignalBySource, "x") }`, ""},
		{"runs append and grow compile", ctxImport + `func f(s Stores) { _ = s.Runs.Append(context.Background(), "c", Run{}); _ = s.Runs.Grow(context.Background(), "c", "r", Run{}) }`, ""},
		{"conversations have no Append", ctxImport + `func f(s Stores) { _ = s.Conversations.Append }`, "Append"},
		{"conversations have no Grow", ctxImport + `func f(s Stores) { _ = s.Conversations.Grow }`, "Grow"},
		{"conversations have no expiry", ctxImport + `func f(s Stores) { _ = s.Conversations.Expiry }`, "Expiry"},
		{"signals have no Append", ctxImport + `func f(s Stores) { _ = s.Signals.Append }`, "Append"},
		{"a conversation index does not list signals", ctxImport + `func f(s Stores) { s.Signals.List(context.Background(), ByPhase, "Idle") }`, "ByPhase"},
		{"a signal index does not list conversations", ctxImport + `func f(s Stores) { s.Conversations.List(context.Background(), SignalAll, "") }`, "SignalAll"},
		{"runs are not a collection", ctxImport + `func f(s Stores) { _ = s.Runs.Get }`, "Get"},
		{"only signals expire in the inventory", ctxImport + `func f(s Stores) { _ = s.Cooldowns.Expiry }`, "Expiry"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := typeCheckWithSnippet(t, c.body)
			if c.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("must compile, got %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("must not compile (mentioning %q) but type-checked clean", c.wantErr)
			}
			if !strings.Contains(errs[0].Error(), c.wantErr) {
				t.Fatalf("first error %q does not mention %q", errs[0], c.wantErr)
			}
		})
	}
}

// The package is the contract: a backend's types never cross it.
func TestStatePackageImportsNothingFromKubernetes(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(path, "k8s.io/") || strings.HasPrefix(path, "sigs.k8s.io/") {
				t.Errorf("%s imports %s: no Kubernetes type may cross the state contract", e.Name(), path)
			}
		}
	}
}

func TestSignalIndexesPartitionByPhaseAndSource(t *testing.T) {
	live := &SignalRecord{SourceRef: "a", InitiationID: "i", Phase: SignalPending}
	absent := &SignalRecord{SourceRef: "a", InitiationID: "i"}
	done := &SignalRecord{SourceRef: "a", InitiationID: "i", Phase: SignalRouted}
	for _, tc := range []struct {
		idx      Index[SignalRecord]
		rec      *SignalRecord
		want     string
		isListed bool
	}{
		{SignalNonTerminalBySource, live, "a", true},
		{SignalNonTerminalBySource, absent, "a", true},
		{SignalNonTerminalBySource, done, "", false},
		{SignalNonTerminalByInitiation, live, IndexValue("a", "i"), true},
		{SignalNonTerminalByInitiation, done, "", false},
		{SignalBySource, done, "a", true},
		{SignalAll, done, "", true},
	} {
		v, ok := tc.idx.Value(tc.rec)
		if ok != tc.isListed || (ok && v != tc.want) {
			t.Errorf("%s on %q: got (%q, %v), want (%q, %v)", tc.idx.Name(), tc.rec.Phase, v, ok, tc.want, tc.isListed)
		}
	}
	if IndexValue("a", "bc") == IndexValue("ab", "c") {
		t.Fatal("different part lists must not share a value")
	}
}

func TestConversationIndexesReadTheRecordsOwnFields(t *testing.T) {
	r := &ConversationRecord{ConversationID: "cid", PipelineRef: "p", Phase: PhaseIdle, NameBase: "base", CausedBy: &Provenance{Parent: "root"}}
	for _, tc := range []struct {
		idx  Index[ConversationRecord]
		want string
	}{
		{ByConversationID, "cid"}, {ByPipelineAndPhase, IndexValue("p", PhaseIdle)},
		{ByPhase, PhaseIdle}, {ByCausedBy, "root"}, {ByNameBase, "base"}, {ByAll, ""},
	} {
		if v, ok := tc.idx.Value(r); !ok || v != tc.want {
			t.Errorf("%s: got (%q, %v), want %q", tc.idx.Name(), v, ok, tc.want)
		}
	}
	if _, ok := ByCausedBy.Value(&ConversationRecord{}); ok {
		t.Error("a root conversation is not listed under causedBy")
	}
}

func TestStoreDeclarationsAreCompleteAndDeclareTheirIndexes(t *testing.T) {
	d := StoreDeclarations
	if !d.Conversations.Declares(ByPhase) || d.Conversations.Declares(NewIndex("undeclared", ByAll.value)) {
		t.Fatal("an index is declared by name")
	}
	if len(d.Signals.Indexes) != 4 || d.Signals.Expiry == nil || d.Conversations.Expiry != nil {
		t.Fatal("signals declare four indexes and an expiry; conversations expire never")
	}
	exp := time.Unix(10, 0)
	if got := d.Signals.Expiry(&SignalRecord{ExpiresAt: &exp}); got == nil || !got.Equal(exp) {
		t.Fatal("the signal expiry reads ExpiresAt")
	}
	names := map[string]bool{}
	v := reflect.ValueOf(d)
	for i := 0; i < v.NumField(); i++ {
		name := v.Field(i).FieldByName("Name").String()
		if name == "" || names[name] {
			t.Fatalf("declaration %s has an empty or duplicate name %q", v.Type().Field(i).Name, name)
		}
		names[name] = true
	}
	// Stores has one more field than Declarations: Blobs, which has no record.
	if reflect.TypeOf(Stores{}).NumField() != v.NumField()+1 {
		t.Fatal("every Stores field except Blobs needs a declaration")
	}
}

func TestRunsMergeIsIdempotentAndCommutative(t *testing.T) {
	merge := StoreDeclarations.Runs.Merge
	a := Run{DeliveryTracked: true, Delivered: []string{"x"}}
	b := Run{RoutedToParent: true, Delivered: []string{"y", "x"}}

	once := Run{RunID: "r"}
	merge(&once, a)
	merge(&once, b)
	twice := once
	twice.Delivered = append([]string(nil), once.Delivered...)
	merge(&twice, a)
	merge(&twice, b)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("not idempotent: %+v vs %+v", once, twice)
	}
	if !once.DeliveryTracked || !once.RoutedToParent || !reflect.DeepEqual(once.Delivered, []string{"x", "y"}) {
		t.Fatalf("merge result %+v", once)
	}

	other := Run{RunID: "r"}
	merge(&other, b)
	merge(&other, a)
	if other.DeliveryTracked != once.DeliveryTracked || other.RoutedToParent != once.RoutedToParent || len(other.Delivered) != len(once.Delivered) {
		t.Fatalf("not commutative as a set: %+v vs %+v", other, once)
	}
	if got := StoreDeclarations.Runs.ID(&Run{RunID: "r1"}); got != "r1" {
		t.Fatalf("ID = %q", got)
	}
	if AdapterStateKey(SurfaceChannel, "n", "k") == AdapterStateKey(SurfaceSignalSource, "n", "k") {
		t.Fatal("two surfaces must never share a key")
	}
}
