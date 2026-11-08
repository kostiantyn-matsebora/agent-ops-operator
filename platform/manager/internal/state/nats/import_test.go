package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// fakeSourceNames is a SourceNames built directly from test fixtures, never
// a real Kubernetes list: cmd/manager (task 2.8) supplies the real one,
// over SignalSourceList/ChannelList and internal/state/kube's own
// EnumerateAdapterState. This package's own test exercises only the
// generic copy engine (import.go), never a Kubernetes type.
type fakeSourceNames struct {
	sourceNames  []string
	adapterState []AdapterStateEntry
}

func (f *fakeSourceNames) SignalSourceNames(context.Context) ([]string, error) {
	return f.sourceNames, nil
}
func (f *fakeSourceNames) AdapterStateEntries(context.Context) ([]AdapterStateEntry, error) {
	return f.adapterState, nil
}

// newFakeSource opens a second, independent nats-backed Backend standing in
// for "an older backend" (design D-10 names kube; the generic copy engine
// in import.go only ever calls state.Stores methods on it, so any backend
// proves the same logic — kube's own, Kubernetes-flavoured SourceNames
// implementation is cmd/manager's, task 2.8).
func newFakeSource(t testing.TB) *Backend {
	t.Helper()
	conn := sharedTestConn(t)
	return openBackend(t, conn, newScope(t))
}

func TestImporterFreshInstallImportsNothingButWritesTheMarker(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	dest := openBackend(t, sharedTestConn(t), newScope(t))

	imported, err := dest.Imported(ctx)
	if err != nil || imported {
		t.Fatalf("Imported before Run = %v, %v, want false, nil", imported, err)
	}

	im := &Importer{Source: source.Stores, SourceNames: &fakeSourceNames{}, Dest: dest}
	if err := im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	imported, err = dest.Imported(ctx)
	if err != nil || !imported {
		t.Fatalf("Imported after Run = %v, %v, want true, nil", imported, err)
	}
	if list, err := dest.Stores.Conversations.List(ctx, state.ByAll, ""); err != nil || len(list) != 0 {
		t.Fatalf("a fresh install must import nothing: %v, %v", list, err)
	}
}

// seedSource writes one conversation (with a run and a queued input backed
// by a blob), one signal, one source's cooldown window and receipt
// counters, and one adapter-state entry — every row of design D-10's table
// — directly onto source, standing in for "a kube install that already
// holds records".
func seedSource(t testing.TB, source *Backend) (convKey, sourceKey state.Key, blobKey state.Key) {
	t.Helper()
	ctx := context.Background()
	convKey = "conv-a"
	if _, err := source.Stores.Conversations.Create(ctx, convKey, state.ConversationRecord{
		ProfileRef: "p", Phase: "Working",
		Inputs: []state.InputItem{{ID: "in-1", Type: "task", PayloadRef: "blob-1"}},
	}); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	blobKey = "blob-1"
	if err := source.Stores.Blobs.Put(ctx, blobKey, state.Blob{ConversationName: string(convKey), Type: "task", Payload: "do the thing"}); err != nil {
		t.Fatalf("seed blob: %v", err)
	}
	if err := source.Stores.Runs.Append(ctx, convKey, state.Run{RunID: "run-1", Status: "succeeded"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := source.Stores.Signals.Create(ctx, "sig-a", state.SignalRecord{SourceRef: "src-a", Adapter: "cron", InitiationID: "init-1"}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	sourceKey = "src-a"
	if _, err := source.Stores.Cooldowns.Create(ctx, sourceKey, state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "init-1", At: time.Now().UTC().Truncate(time.Second)}}}); err != nil {
		t.Fatalf("seed cooldown: %v", err)
	}
	if _, err := source.Stores.Receipts.Create(ctx, sourceKey, state.Received{Total: 3}); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	return convKey, sourceKey, blobKey
}

func TestImporterUpgradeKeepsEveryKind(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	convKey, sourceKey, blobKey := seedSource(t, source)
	adapterKey := state.AdapterStateKey(state.SurfaceChannel, "chan-a", "cursor")
	dest := openBackend(t, sharedTestConn(t), newScope(t))

	im := &Importer{
		Source: source.Stores,
		SourceNames: &fakeSourceNames{
			sourceNames:  []string{string(sourceKey)},
			adapterState: []AdapterStateEntry{{Key: adapterKey, Value: "42"}},
		},
		Dest: dest,
	}
	if err := im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	conv, _, err := dest.Stores.Conversations.Get(ctx, convKey)
	if err != nil {
		t.Fatalf("Get conversation: %v", err)
	}
	if conv.Phase != "Working" {
		t.Fatalf("conversation phase = %q, want Working", conv.Phase)
	}
	runs, err := dest.Stores.Runs.List(ctx, convKey)
	if err != nil || len(runs) != 1 || runs[0].RunID != "run-1" {
		t.Fatalf("runs = %+v, %v, want exactly run-1", runs, err)
	}
	blob, err := dest.Stores.Blobs.Get(ctx, blobKey)
	if err != nil || blob.Payload != "do the thing" {
		t.Fatalf("blob = %+v, %v", blob, err)
	}
	sig, _, err := dest.Stores.Signals.Get(ctx, "sig-a")
	if err != nil || sig.InitiationID != "init-1" {
		t.Fatalf("signal = %+v, %v", sig, err)
	}
	cooldown, _, err := dest.Stores.Cooldowns.Get(ctx, sourceKey)
	if err != nil || len(cooldown.Entries) != 1 {
		t.Fatalf("cooldown = %+v, %v", cooldown, err)
	}
	received, _, err := dest.Stores.Receipts.Get(ctx, sourceKey)
	if err != nil || received.Total != 3 {
		t.Fatalf("received = %+v, %v", received, err)
	}
	adapterValue, _, err := dest.Stores.AdapterState.Get(ctx, adapterKey)
	if err != nil || adapterValue != "42" {
		t.Fatalf("adapter state = %q, %v, want 42", adapterValue, err)
	}

	// "The source is left intact": nothing the import does ever writes to
	// Source; re-reading it back proves nothing moved.
	srcConv, _, err := source.Stores.Conversations.Get(ctx, convKey)
	if err != nil || srcConv.Phase != "Working" {
		t.Fatalf("source conversation moved: %+v, %v", srcConv, err)
	}
}

func TestImporterRepeatedPassNeverDuplicatesAnAlreadyImportedRun(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	convKey, _, _ := seedSource(t, source)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	im := &Importer{Source: source.Stores, SourceNames: &fakeSourceNames{}, Dest: dest}

	// "An interrupted import is repaired": two passes, same as a process
	// restarted mid-import, must settle on exactly one copy of everything.
	if err := im.pass(ctx); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if err := im.pass(ctx); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	runs, err := dest.Stores.Runs.List(ctx, convKey)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs after two passes = %+v, %v, want exactly one", runs, err)
	}
}

func TestImporterLateOldReplicaWriteIsCaughtByTheFinalPass(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	im := &Importer{Source: source.Stores, SourceNames: &fakeSourceNames{}, Dest: dest}

	if err := im.pass(ctx); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	// An old-version replica writes after the first pass, before the final one.
	if _, err := source.Stores.Signals.Create(ctx, "sig-late", state.SignalRecord{SourceRef: "src-a", Adapter: "cron", InitiationID: "late"}); err != nil {
		t.Fatalf("late signal: %v", err)
	}
	if _, err := source.Stores.Conversations.Create(ctx, "conv-late", state.ConversationRecord{ProfileRef: "p"}); err != nil {
		t.Fatalf("late conversation: %v", err)
	}
	if err := im.pass(ctx); err != nil { // the final pass
		t.Fatalf("final pass: %v", err)
	}

	if _, _, err := dest.Stores.Signals.Get(ctx, "sig-late"); err != nil {
		t.Fatalf("late signal not imported: %v", err)
	}
	if _, _, err := dest.Stores.Conversations.Get(ctx, "conv-late"); err != nil {
		t.Fatalf("late conversation not imported: %v", err)
	}
}

func TestImporterNeverOverwritesANewVersionWrite(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	convKey, _, _ := seedSource(t, source)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	im := &Importer{Source: source.Stores, SourceNames: &fakeSourceNames{}, Dest: dest}

	if err := im.pass(ctx); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	// The live (new-version) manager writes its own value.
	live, rev, err := dest.Stores.Conversations.Get(ctx, convKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	live.Title = "the live manager's own edit"
	if _, err := dest.Stores.Conversations.Update(ctx, convKey, rev, live); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// An old replica changes the source after that.
	srcRec, srcRev, err := source.Stores.Conversations.Get(ctx, convKey)
	if err != nil {
		t.Fatalf("Get source: %v", err)
	}
	srcRec.Title = "an old replica's conflicting edit"
	if _, err := source.Stores.Conversations.Update(ctx, convKey, srcRev, srcRec); err != nil {
		t.Fatalf("Update source: %v", err)
	}

	if err := im.pass(ctx); err != nil { // the final pass
		t.Fatalf("final pass: %v", err)
	}

	got, _, err := dest.Stores.Conversations.Get(ctx, convKey)
	if err != nil {
		t.Fatalf("Get after final pass: %v", err)
	}
	if got.Title != "the live manager's own edit" {
		t.Fatalf("Title = %q, want the live manager's own edit left untouched", got.Title)
	}
}

func TestImporterNonLeaderNeverRuns(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	seedSource(t, source)
	dest := openBackend(t, sharedTestConn(t), newScope(t))

	// A non-leader's whole contribution is simply never calling Run — the
	// leader-gating itself is cmd/manager's (task 2.8), not this package's.
	imported, err := dest.Imported(ctx)
	if err != nil || imported {
		t.Fatalf("a non-leader's Backend reports Imported = %v, %v, want false until some replica runs the import", imported, err)
	}
	if list, err := dest.Stores.Conversations.List(ctx, state.ByAll, ""); err != nil || len(list) != 0 {
		t.Fatalf("a non-leader must copy nothing: %v, %v", list, err)
	}
}

func TestImporterDoesNotRunAgainOnceTheMarkerExists(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	im := &Importer{Source: source.Stores, SourceNames: &fakeSourceNames{}, Dest: dest}
	if err := im.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Seed the source only after the marker exists: a second Run must see
	// none of it, "the import SHALL NOT run again once the marker exists".
	seedSource(t, source)
	if err := im.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if list, err := dest.Stores.Conversations.List(ctx, state.ByAll, ""); err != nil || len(list) != 0 {
		t.Fatalf("Run after the marker exists copied something: %v, %v", list, err)
	}
}

func TestImportMarkerRoundTrip(t *testing.T) {
	ctx := context.Background()
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	if imported, err := dest.Imported(ctx); err != nil || imported {
		t.Fatalf("Imported before marking = %v, %v", imported, err)
	}
	if err := dest.markImported(ctx); err != nil {
		t.Fatalf("markImported: %v", err)
	}
	if imported, err := dest.Imported(ctx); err != nil || !imported {
		t.Fatalf("Imported after marking = %v, %v, want true", imported, err)
	}
}

// The error path: a Source failure must surface, never be swallowed.
func TestImporterPropagatesASourceNamesFailure(t *testing.T) {
	ctx := context.Background()
	source := newFakeSource(t)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	wantErr := errors.New("boom")
	im := &Importer{Source: source.Stores, SourceNames: failingSourceNames{err: wantErr}, Dest: dest}
	if err := im.Run(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want it to wrap %v", err, wantErr)
	}
}

type failingSourceNames struct{ err error }

func (f failingSourceNames) SignalSourceNames(context.Context) ([]string, error) { return nil, f.err }
func (f failingSourceNames) AdapterStateEntries(context.Context) ([]AdapterStateEntry, error) {
	return nil, f.err
}
