package nats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

var errInjected = errors.New("injected failure")

// faultColl wraps a Collection and fails the calls a test names. A nil error
// field passes the call through.
type faultColl[T any] struct {
	state.Collection[T]
	listErr, getErr, createErr, updateErr error
}

func (f *faultColl[T]) List(ctx context.Context, idx state.Index[T], v string) ([]T, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.Collection.List(ctx, idx, v)
}

func (f *faultColl[T]) Get(ctx context.Context, k state.Key) (T, state.Revision, error) {
	if f.getErr != nil {
		var zero T
		return zero, "", f.getErr
	}
	return f.Collection.Get(ctx, k)
}

func (f *faultColl[T]) Create(ctx context.Context, k state.Key, rec T) (state.Revision, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.Collection.Create(ctx, k, rec)
}

func (f *faultColl[T]) Update(ctx context.Context, k state.Key, rev state.Revision, rec T) (state.Revision, error) {
	if f.updateErr != nil {
		return "", f.updateErr
	}
	return f.Collection.Update(ctx, k, rev, rec)
}

// faultRuns wraps the runs capability the same way.
type faultRuns struct {
	state.Appended[state.ConversationRecord, state.Run]
	listErr, appendErr, growErr error
}

func (f *faultRuns) List(ctx context.Context, p state.Key) ([]state.Run, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.Appended.List(ctx, p)
}

func (f *faultRuns) Append(ctx context.Context, p state.Key, r state.Run) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	return f.Appended.Append(ctx, p, r)
}

func (f *faultRuns) Grow(ctx context.Context, p state.Key, id string, r state.Run) error {
	if f.growErr != nil {
		return f.growErr
	}
	return f.Appended.Grow(ctx, p, id, r)
}

// faultBlobs wraps the blob capability.
type faultBlobs struct {
	state.Blobs
	getErr, putErr error
}

func (f *faultBlobs) Get(ctx context.Context, k state.Key) (state.Blob, error) {
	if f.getErr != nil {
		return state.Blob{}, f.getErr
	}
	return f.Blobs.Get(ctx, k)
}

func (f *faultBlobs) Put(ctx context.Context, k state.Key, b state.Blob) error {
	if f.putErr != nil {
		return f.putErr
	}
	return f.Blobs.Put(ctx, k, b)
}

// faultKV wraps the raw adapter-state bucket the import's marks live in. A
// get or put fails only for a key containing the named substring, so one mark
// can be refused while the rest are written.
type faultKV struct {
	jetstream.KeyValue
	getErrOn, putErrOn string
	err                error
}

func (f *faultKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if f.getErrOn != "" && strings.Contains(key, f.getErrOn) {
		return nil, f.err
	}
	return f.KeyValue.Get(ctx, key)
}

func (f *faultKV) Put(ctx context.Context, key string, v []byte) (uint64, error) {
	if f.putErrOn != "" && strings.Contains(key, f.putErrOn) {
		return 0, f.err
	}
	return f.KeyValue.Put(ctx, key, v)
}

// adapterNamesFail fails only the adapter-state listing.
type adapterNamesFail struct{ fakeSourceNames }

func (adapterNamesFail) AdapterStateEntries(context.Context) ([]AdapterStateEntry, error) {
	return nil, errInjected
}

// seeded is a source holding every kind, an empty destination, and the
// importer between them.
type seeded struct {
	im   *Importer
	dest *Backend
	conv state.Key
	src  state.Key
	blob state.Key
}

func newSeeded(t *testing.T) *seeded {
	t.Helper()
	source := newFakeSource(t)
	conv, src, blob := seedSource(t, source)
	dest := openBackend(t, sharedTestConn(t), newScope(t))
	return &seeded{
		im: &Importer{
			Source:      source.Stores,
			SourceNames: &fakeSourceNames{sourceNames: []string{string(src)}, adapterState: []AdapterStateEntry{{Key: state.AdapterStateKey(state.SurfaceChannel, "chan-a", "cursor"), Value: "42"}}},
			Dest:        dest,
		},
		dest: dest, conv: conv, src: src, blob: blob,
	}
}

func wantFailure(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("succeeded, want a failure containing %q", contains)
	}
	if !errors.Is(err, errInjected) && !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %v, want it to contain %q", err, contains)
	}
}

func TestImportSurfacesEveryFailureOfEveryStage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		break_   func(s *seeded)
		contains string
	}{
		{"listing the source's conversations", func(s *seeded) {
			s.im.Source.Conversations = &faultColl[state.ConversationRecord]{Collection: s.im.Source.Conversations, listErr: errInjected}
		}, "import conversations"},
		{"listing the source's runs", func(s *seeded) {
			s.im.Source.Runs = &faultRuns{Appended: s.im.Source.Runs, listErr: errInjected}
		}, "import conversations"},
		{"listing the destination's runs", func(s *seeded) {
			s.dest.Stores.Runs = &faultRuns{Appended: s.dest.Stores.Runs, listErr: errInjected}
		}, "import conversations"},
		{"appending a run to the destination", func(s *seeded) {
			s.dest.Stores.Runs = &faultRuns{Appended: s.dest.Stores.Runs, appendErr: errInjected}
		}, "import conversations"},
		{"reading a blob from the destination", func(s *seeded) {
			s.dest.Stores.Blobs = &faultBlobs{Blobs: s.dest.Stores.Blobs, getErr: errInjected}
		}, "import conversations"},
		{"reading a blob from the source", func(s *seeded) {
			s.im.Source.Blobs = &faultBlobs{Blobs: s.im.Source.Blobs, getErr: errInjected}
		}, "import conversations"},
		{"writing a blob to the destination", func(s *seeded) {
			s.dest.Stores.Blobs = &faultBlobs{Blobs: s.dest.Stores.Blobs, putErr: errInjected}
		}, "import conversations"},
		{"creating a conversation in the destination", func(s *seeded) {
			s.dest.Stores.Conversations = &faultColl[state.ConversationRecord]{Collection: s.dest.Stores.Conversations, createErr: errInjected}
		}, "import conversations"},
		{"listing the source's signals", func(s *seeded) {
			s.im.Source.Signals = &faultExpiring{Expiring: s.im.Source.Signals, listErr: errInjected}
		}, "import signals"},
		{"creating a signal in the destination", func(s *seeded) {
			s.dest.Stores.Signals = &faultExpiring{Expiring: s.dest.Stores.Signals, createErr: errInjected}
		}, "import signals"},
		{"listing the source names", func(s *seeded) {
			s.im.SourceNames = failingSourceNames{err: errInjected}
		}, "import cooldowns"},
		{"reading a source's cooldown window", func(s *seeded) {
			s.im.Source.Cooldowns = &faultColl[state.CooldownWindow]{Collection: s.im.Source.Cooldowns, getErr: errInjected}
		}, "import cooldowns"},
		{"reading a source's receipts", func(s *seeded) {
			s.im.Source.Receipts = &faultColl[state.Received]{Collection: s.im.Source.Receipts, getErr: errInjected}
		}, "import cooldowns"},
		{"creating a cooldown window in the destination", func(s *seeded) {
			s.dest.Stores.Cooldowns = &faultColl[state.CooldownWindow]{Collection: s.dest.Stores.Cooldowns, createErr: errInjected}
		}, "import cooldowns"},
		{"creating receipts in the destination", func(s *seeded) {
			s.dest.Stores.Receipts = &faultColl[state.Received]{Collection: s.dest.Stores.Receipts, createErr: errInjected}
		}, "import cooldowns"},
		{"listing adapter state", func(s *seeded) {
			s.im.SourceNames = &adapterNamesFail{fakeSourceNames{sourceNames: []string{string(s.src)}}}
		}, "import adapter state"},
		{"creating adapter state in the destination", func(s *seeded) {
			s.dest.Stores.AdapterState = &faultColl[string]{Collection: s.dest.Stores.AdapterState, createErr: errInjected}
		}, "import adapter state"},
		{"checking whether the import already ran", func(s *seeded) {
			s.dest.adapterStateKV = &faultKV{KeyValue: s.dest.adapterStateKV, getErrOn: importMarkerKey, err: errInjected}
		}, "check import marker"},
		{"writing the import marker", func(s *seeded) {
			s.dest.adapterStateKV = &faultKV{KeyValue: s.dest.adapterStateKV, putErrOn: importMarkerKey, err: errInjected}
		}, "write import marker"},
		{"writing a record's import mark", func(s *seeded) {
			s.dest.adapterStateKV = &faultKV{KeyValue: s.dest.adapterStateKV, putErrOn: "_import.", err: errInjected}
		}, "write import mark"},
		{"reading a record's import mark", func(s *seeded) {
			s.dest.adapterStateKV = &faultKV{KeyValue: s.dest.adapterStateKV, getErrOn: "_import.", err: errInjected}
		}, "read import mark"},
		{"asking for an old replica", func(s *seeded) {
			s.im.OldReplicaPresent = func(context.Context) (bool, error) { return false, errInjected }
		}, "old-version replica"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSeeded(t)
			tc.break_(s)
			wantFailure(t, s.im.Run(ctx), tc.contains)
			if tc.contains != "check import marker" && tc.contains != "write import marker" {
				if done, _ := s.dest.Imported(ctx); done {
					t.Fatal("the import marker was written although a stage failed")
				}
			}
		})
	}
}

// faultExpiring wraps the signals capability.
type faultExpiring struct {
	state.Expiring[state.SignalRecord]
	listErr, getErr, createErr error
}

func (f *faultExpiring) List(ctx context.Context, idx state.Index[state.SignalRecord], v string) ([]state.SignalRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.Expiring.List(ctx, idx, v)
}

func (f *faultExpiring) Get(ctx context.Context, k state.Key) (state.SignalRecord, state.Revision, error) {
	if f.getErr != nil {
		return state.SignalRecord{}, "", f.getErr
	}
	return f.Expiring.Get(ctx, k)
}

func (f *faultExpiring) Create(ctx context.Context, k state.Key, r state.SignalRecord) (state.Revision, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.Expiring.Create(ctx, k, r)
}

func TestImportSkipsARecordThatVanishedBetweenTheListAndTheRead(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	s.im.Source.Conversations = &faultColl[state.ConversationRecord]{Collection: s.im.Source.Conversations, getErr: state.ErrNotFound}
	s.im.Source.Signals = &faultExpiring{Expiring: s.im.Source.Signals, getErr: state.ErrNotFound}
	if err := s.im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, _, err := s.dest.Stores.Conversations.Get(ctx, s.conv); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("a conversation that could not be read was imported: %v", err)
	}
	if _, _, err := s.dest.Stores.Signals.Get(ctx, "sig-a"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("a signal that could not be read was imported: %v", err)
	}
}

func TestImportSkipsABlobTheSourceNoLongerHasAndOneWithNoReference(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	if _, err := s.im.Source.Conversations.Create(ctx, "no-refs", state.ConversationRecord{
		ProfileRef: "p", Inputs: []state.InputItem{{ID: "in-inline", Type: "task"}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.im.Source.Blobs = &faultBlobs{Blobs: s.im.Source.Blobs, getErr: state.ErrNotFound}
	if err := s.im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.dest.Stores.Blobs.Get(ctx, s.blob); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("a blob the source lacks appeared in the destination: %v", err)
	}
	if _, _, err := s.dest.Stores.Conversations.Get(ctx, "no-refs"); err != nil {
		t.Fatalf("the conversation with an inline input was not imported: %v", err)
	}
}

func TestImportLeavesABlobTheDestinationAlreadyHolds(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	if err := s.dest.Stores.Blobs.Put(ctx, s.blob, state.Blob{ConversationName: string(s.conv), Payload: "newer, written by the live manager"}); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	if err := s.im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := s.dest.Stores.Blobs.Get(ctx, s.blob)
	if err != nil || got.Payload != "newer, written by the live manager" {
		t.Fatalf("blob = %+v, %v, want the destination's own", got, err)
	}
}

func TestImportGrowsARunTheDestinationAlreadyHasAndToleratesItAgingOut(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	if _, err := s.dest.Stores.Conversations.Create(ctx, s.conv, state.ConversationRecord{ProfileRef: "p"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.dest.Stores.Runs.Append(ctx, s.conv, state.Run{RunID: "run-1", Status: "succeeded"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	// A run that aged out of the bounded history between the list and the grow
	// is not a failure.
	s.dest.Stores.Runs = &faultRuns{Appended: s.dest.Stores.Runs, growErr: state.ErrNotFound}
	if err := s.im.Run(ctx); err != nil {
		t.Fatalf("Run with a run that aged out: %v", err)
	}
	// Any other failure of the grow is.
	s2 := newSeeded(t)
	if _, err := s2.dest.Stores.Conversations.Create(ctx, s2.conv, state.ConversationRecord{ProfileRef: "p"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s2.dest.Stores.Runs.Append(ctx, s2.conv, state.Run{RunID: "run-1", Status: "succeeded"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	s2.dest.Stores.Runs = &faultRuns{Appended: s2.dest.Stores.Runs, growErr: errInjected}
	wantFailure(t, s2.im.Run(ctx), "import conversations")
}

func TestImportRepeatsPassesWhileAnOldReplicaMayStillWrite(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	asked := 0
	s.im.PassInterval = time.Millisecond
	s.im.OldReplicaPresent = func(context.Context) (bool, error) {
		asked++
		return asked < 3, nil
	}
	if err := s.im.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if asked != 3 {
		t.Fatalf("asked %d times, want 3: two passes while the old replica lived and one final", asked)
	}
	if done, _ := s.dest.Imported(ctx); !done {
		t.Fatal("not marked imported")
	}
}

func TestImportStopsWhenTheContextEndsWhileWaitingForAnOldReplica(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newSeeded(t)
	s.im.PassInterval = time.Hour
	s.im.OldReplicaPresent = func(context.Context) (bool, error) {
		cancel()
		return true, nil
	}
	if err := s.im.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if done, _ := s.dest.Imported(context.Background()); done {
		t.Fatal("marked imported although the wait was abandoned")
	}
}

func TestImportFailsOnTheFinalPassToo(t *testing.T) {
	ctx := context.Background()
	s := newSeeded(t)
	calls := 0
	s.im.PassInterval = time.Millisecond
	s.im.OldReplicaPresent = func(context.Context) (bool, error) {
		calls++
		// After the first pass, break the source: the final pass must fail.
		s.im.Source.Signals = &faultExpiring{Expiring: s.im.Source.Signals, listErr: errInjected}
		return calls < 2, nil
	}
	wantFailure(t, s.im.Run(ctx), "import signals")
}

// copyRecord, directly: the branches no seeded importer reaches.

func openMarks(t *testing.T) *Backend {
	t.Helper()
	return openBackend(t, sharedTestConn(t), newScope(t))
}

func TestCopyRecordRefusesACorruptMark(t *testing.T) {
	ctx := context.Background()
	b := openMarks(t)
	if _, err := b.adapterStateKV.Put(ctx, importTrackKey("cooldowns", "k"), []byte("not json")); err != nil {
		t.Fatalf("seed mark: %v", err)
	}
	err := copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{}, b.Stores.Cooldowns)
	wantFailure(t, err, "decode import mark")
}

func TestCopyRecordSurfacesAFailedLookupOfTheDestination(t *testing.T) {
	ctx := context.Background()
	b := openMarks(t)
	dest := &faultColl[state.CooldownWindow]{Collection: b.Stores.Cooldowns, getErr: errInjected}
	wantFailure(t, copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{}, dest), "import get")
}

func TestCopyRecordTreatsAnAlreadyExistingRecordAsDone(t *testing.T) {
	ctx := context.Background()
	b := openMarks(t)
	dest := &faultColl[state.CooldownWindow]{Collection: b.Stores.Cooldowns, getErr: state.ErrNotFound, createErr: state.ErrAlreadyExists}
	if err := copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{}, dest); err != nil {
		t.Fatalf("copyRecord = %v, want nil: another writer got there first", err)
	}
}

func TestCopyRecordSurfacesAFailedCreate(t *testing.T) {
	ctx := context.Background()
	b := openMarks(t)
	dest := &faultColl[state.CooldownWindow]{Collection: b.Stores.Cooldowns, getErr: state.ErrNotFound, createErr: errInjected}
	wantFailure(t, copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{}, dest), "import create")
}

// recopy sets up a record imported at source version "1", ready for a pass
// that sees version "2".
func recopy(t *testing.T) (*Backend, state.Collection[state.CooldownWindow]) {
	t.Helper()
	ctx := context.Background()
	b := openMarks(t)
	if err := copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "a"}}}, b.Stores.Cooldowns); err != nil {
		t.Fatalf("first copy: %v", err)
	}
	return b, b.Stores.Cooldowns
}

func TestCopyRecordRecopiesAChangedSourceWhileNothingElseWrote(t *testing.T) {
	ctx := context.Background()
	b, dest := recopy(t)
	next := state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "b"}}}
	if err := copyRecord(ctx, b, "cooldowns", "k", "2", next, dest); err != nil {
		t.Fatalf("recopy: %v", err)
	}
	got, _, _ := dest.Get(ctx, "k")
	if len(got.Entries) != 1 || got.Entries[0].InitiationID != "b" {
		t.Fatalf("record = %+v, want the new version", got)
	}
	// And the same version again is a no-op.
	if err := copyRecord(ctx, b, "cooldowns", "k", "2", state.CooldownWindow{}, dest); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	got, _, _ = dest.Get(ctx, "k")
	if len(got.Entries) != 1 {
		t.Fatalf("an unchanged source version rewrote the record: %+v", got)
	}
}

func TestCopyRecordLosesARaceQuietlyAndFailsOnAnythingElse(t *testing.T) {
	ctx := context.Background()
	b, dest := recopy(t)
	conflicted := &faultColl[state.CooldownWindow]{Collection: dest, updateErr: state.ErrConflict}
	if err := copyRecord(ctx, b, "cooldowns", "k", "2", state.CooldownWindow{}, conflicted); err != nil {
		t.Fatalf("a conflict = %v, want nil: the next pass tries again", err)
	}
	broken := &faultColl[state.CooldownWindow]{Collection: dest, updateErr: errInjected}
	wantFailure(t, copyRecord(ctx, b, "cooldowns", "k", "2", state.CooldownWindow{}, broken), "import update")
}

func TestCopyRecordSurfacesAFailedMarkAfterARecopy(t *testing.T) {
	ctx := context.Background()
	b, dest := recopy(t)
	b.adapterStateKV = &faultKV{KeyValue: b.adapterStateKV, putErrOn: "_import.", err: errInjected}
	wantFailure(t, copyRecord(ctx, b, "cooldowns", "k", "2", state.CooldownWindow{}, dest), "write import mark")
}

func TestCopyRecordNeverOverwritesWhatTheLiveManagerWrote(t *testing.T) {
	ctx := context.Background()
	b, dest := recopy(t)
	_, rev, _ := dest.Get(ctx, "k")
	if _, err := dest.Update(ctx, "k", rev, state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "live"}}}); err != nil {
		t.Fatalf("live write: %v", err)
	}
	if err := copyRecord(ctx, b, "cooldowns", "k", "2", state.CooldownWindow{}, dest); err != nil {
		t.Fatalf("copyRecord: %v", err)
	}
	got, _, _ := dest.Get(ctx, "k")
	if got.Entries[0].InitiationID != "live" {
		t.Fatalf("record = %+v, the live write was overwritten", got)
	}
}

func TestCopyRecordWithNoMarkLeavesAnExistingRecordAlone(t *testing.T) {
	ctx := context.Background()
	b := openMarks(t)
	if _, err := b.Stores.Cooldowns.Create(ctx, "k", state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "pre"}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := copyRecord(ctx, b, "cooldowns", "k", "1", state.CooldownWindow{}, b.Stores.Cooldowns); err != nil {
		t.Fatalf("copyRecord: %v", err)
	}
	got, _, _ := b.Stores.Cooldowns.Get(ctx, "k")
	if got.Entries[0].InitiationID != "pre" {
		t.Fatalf("record = %+v, a record the import did not write was overwritten", got)
	}
}
