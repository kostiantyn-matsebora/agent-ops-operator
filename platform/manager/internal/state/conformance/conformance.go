// Package conformance is the one suite every state.Store backend must pass
// to exist (design D-10 of openspec/changes/state-store/). Run walks every
// scenario of specs/state-store/spec.md as a subtest; Bench (bench.go)
// reports the tier-1 numbers the same design names: bytes written per run,
// allocations per write, records resident after a listing.
//
// newStore MUST return a fresh, empty, isolated Store on every call — Run
// relies on that for scope isolation between scenarios, calling it once per
// top-level scenario. A scenario needing continuity across what would be a
// process restart in production (the "new leader resumes" case) reuses the
// SAME Store value for its whole body instead of reconstructing it, since a
// resumable feed and a fresh backing store are different things.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Fixture values and failure formats the scenarios repeat.
const (
	profileDefault         = "profile-default"
	fmtCreate              = "Create: %v"
	fmtGet                 = "Get: %v"
	fmtWatch               = "Watch: %v"
	secondActor            = "second-actor"
	convCreateTwice        = "conv-create-twice"
	convID1                = "conv-id-1"
	pipe1                  = "pipe-1"
	convExpired            = "conv-expired"
	secondLife             = "second life"
	convSharedName         = "conv-shared-name"
	convCrashBetweenWrites = "conv-crash-between-writes"
	owedInput              = "owed-input"
)

// Run executes every conformance scenario against a Store newStore
// constructs fresh for it, one t.Run per scenario named for the spec
// requirement and scenario title it proves (tasks.md 3.1).
func Run(t *testing.T, newStore func() state.Store) {
	t.Helper()

	t.Run("WritesAreCAS", func(t *testing.T) {
		t.Run("TwoActorsRace_ExactlyOneWinsNoMerge", func(t *testing.T) {
			testCASTwoActorsRace(t, newStore())
		})
		t.Run("StaleClaimNeverMerged_ChangesNothing", func(t *testing.T) {
			testCASStaleClaimNeverMerged(t, newStore())
		})
		t.Run("CreateRefusedWhenKeyAlreadyExists", func(t *testing.T) {
			testCreateRefusedOnExistingKey(t, newStore())
		})
	})

	t.Run("LookupByKeyOrDeclaredIndex", func(t *testing.T) {
		t.Run("EqualityRoutingFindsOpenConversation", func(t *testing.T) {
			testIndexEqualityRouting(t, newStore())
		})
		t.Run("EveryDeclaredIndexReturnsExactlyMatchingSet", func(t *testing.T) {
			testEveryDeclaredIndex(t, newStore())
		})
		t.Run("QueryOutsideIndexSetRefused", func(t *testing.T) {
			testUnknownIndexRefused(t, newStore())
		})
	})

	t.Run("ChangeFeedIsAtLeastOnceAndResumable", func(t *testing.T) {
		t.Run("EventsCarryKeyRevisionRecord", func(t *testing.T) {
			testFeedEventShape(t, newStore())
		})
		t.Run("NewLeaderResumesFromStateAlone", func(t *testing.T) {
			testFeedResumeContinuity(t, newStore())
		})
		t.Run("NoGapAcrossConsumerDisconnect", func(t *testing.T) {
			testFeedNoGapAcrossDisconnect(t, newStore())
		})
		t.Run("StaleSnapshotCaughtByWriteNotRead", func(t *testing.T) {
			testFeedStaleSnapshotCaughtByCAS(t, newStore())
		})
	})

	t.Run("RecordMayCarryExpiry", func(t *testing.T) {
		t.Run("ExpiredRecordInvisibleBeforeRemoved", func(t *testing.T) {
			testExpiryInvisible(t, newStore())
		})
		t.Run("ExpiryObservableOnFeedAsDelete", func(t *testing.T) {
			testExpiryObservableOnFeed(t, newStore())
		})
	})

	t.Run("RunsAreAppendOnlySubRecords", func(t *testing.T) {
		t.Run("AppendNeedsNoRevisionAndNeverConflicts", func(t *testing.T) {
			testAppendRunNeverConflicts(t, newStore())
		})
		t.Run("RunsReadBackInAppendOrder", func(t *testing.T) {
			testListRunsOrder(t, newStore())
		})
	})

	t.Run("InputPayloadsAreBlobsByKey", func(t *testing.T) {
		t.Run("PutGetDeleteRoundTrip_PrunedInputReleasesPayload", func(t *testing.T) {
			testBlobLifecycle(t, newStore())
		})
	})

	t.Run("ScopeIsFixedAtConstruction", func(t *testing.T) {
		t.Run("TwoInstallsNeverSeeEachOther", func(t *testing.T) {
			testScopeIsolation(t, newStore, newStore)
		})
	})

	t.Run("ContractOffersNoMultiRecordAtomicity", func(t *testing.T) {
		t.Run("ConversationAndBlobWritesAreIndependent", func(t *testing.T) {
			testNoMultiRecordAtomicity(t, newStore())
		})
	})
}

// mustRecvEvent reads the next event off events, failing the test if none
// arrives within timeout.
func mustRecvEvent(t *testing.T, events <-chan state.Event, timeout time.Duration) state.Event {
	t.Helper()
	select {
	case evt, ok := <-events:
		if !ok {
			t.Fatalf("event channel closed before an event arrived")
		}
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event within %s", timeout)
	}
	return state.Event{}
}

// mustRecvEventMatching drains events, within timeout, until one satisfies
// want, and returns it.
//
// A backend may land one logical Create/Update as more than one
// underlying apiserver write — the kube backend's Update does exactly
// this (store.go's own comment: the main object, then the status
// subresource, as two REST calls) — and each is its own real, correctly
// ordered notification. A scenario that cares about a write's FINAL state
// looks for it among what the write actually produced rather than
// assuming the very first notification already is it.
func mustRecvEventMatching(t *testing.T, events <-chan state.Event, timeout time.Duration, want func(state.Event) bool) state.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed before a matching event arrived")
			}
			if want(evt) {
				return evt
			}
		case <-deadline:
			t.Fatalf("no matching event within %s", timeout)
		}
	}
}

// drainQuiet collects every event delivered on events until quiet passes
// with nothing new arriving, used where a scenario cares about the whole
// sequence a backend produced rather than one event — draining by a quiet
// period rather than a fixed count tolerates a backend that lands one
// logical write as more than one notification (mustRecvEventMatching's own
// comment).
func drainQuiet(events <-chan state.Event, quiet time.Duration) []state.Event {
	var got []state.Event
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, evt)
		case <-time.After(quiet):
			return got
		}
	}
}

// --- Requirement: Writes are compare-and-swap on an opaque revision ---

// testCASTwoActorsRace is the "Two actors race for one record" scenario:
// the leader's reconciler and an HTTP handler each read revision N and both
// write — exactly one write succeeds, the other gets a conflict, and the
// record carries exactly one of the two changes, never a merge.
func testCASTwoActorsRace(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-cas-race")
	base := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-cas-race", Phase: "Working"}
	rev, err := store.Create(ctx, key, base)
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	write := func(title string) {
		defer wg.Done()
		rec := base
		rec.Title = title
		_, err := store.Update(ctx, key, rev, rec)
		results <- err
	}
	wg.Add(2)
	go write("actor-A")
	go write("actor-B")
	wg.Wait()
	close(results)

	var oks, conflicts int
	for err := range results {
		switch {
		case err == nil:
			oks++
		case errors.Is(err, state.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if oks != 1 || conflicts != 1 {
		t.Fatalf("got %d successes and %d conflicts, want exactly one of each", oks, conflicts)
	}

	final, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if final.Title != "actor-A" && final.Title != "actor-B" {
		t.Fatalf("final Title = %q, want exactly one actor's write, never a merge", final.Title)
	}
}

// testCASStaleClaimNeverMerged is the "A stale claim is never merged"
// scenario: a writer presenting a revision older than current is refused
// without any field of its write applying.
func testCASStaleClaimNeverMerged(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-cas-stale")
	base := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-cas-stale", Phase: "Working", Title: "original"}
	staleRev, err := store.Create(ctx, key, base)
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	winning := base
	winning.Title = secondActor
	if _, err := store.Update(ctx, key, staleRev, winning); err != nil {
		t.Fatalf("Update (winning write): %v", err)
	}

	stale := base
	stale.Title = "stale-actor"
	if _, err := store.Update(ctx, key, staleRev, stale); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("stale Update error = %v, want ErrConflict", err)
	}

	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if got.Title != secondActor {
		t.Fatalf("Title = %q after refused stale write, want %q unchanged", got.Title, secondActor)
	}
}

// testCreateRefusedOnExistingKey covers "A create SHALL be refused when the
// key already exists" with state.ErrAlreadyExists, and that the existing
// record is left untouched.
func testCreateRefusedOnExistingKey(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key(convCreateTwice)
	first := state.ConversationRecord{ProfileRef: profileDefault, Name: convCreateTwice, Title: "first"}
	if _, err := store.Create(ctx, key, first); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	second := state.ConversationRecord{ProfileRef: profileDefault, Name: convCreateTwice, Title: "second"}
	if _, err := store.Create(ctx, key, second); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create error = %v, want ErrAlreadyExists", err)
	}

	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	if got.Title != "first" {
		t.Fatalf("Title = %q after refused create, want %q unchanged", got.Title, "first")
	}
}

// --- Requirement: Lookup is by key or by a declared index ---

// testIndexEqualityRouting is the "Equality routing finds the open
// conversation" scenario.
func testIndexEqualityRouting(t *testing.T, store state.Store) {
	ctx := context.Background()
	target := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-target", ConversationID: convID1, Phase: "Working"}
	other := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-other", ConversationID: "conv-id-2", Phase: "Working"}
	if _, err := store.Create(ctx, state.Key(target.Name), target); err != nil {
		t.Fatalf("Create target: %v", err)
	}
	if _, err := store.Create(ctx, state.Key(other.Name), other); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	got, err := store.List(ctx, state.ByConversationID, state.ListKey{ConversationID: convID1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ConversationID != convID1 {
		t.Fatalf("List(ByConversationID, conv-id-1) = %+v, want exactly the one matching record", got)
	}
}

// testEveryDeclaredIndex is the "A query outside the index set is not
// expressible" requirement's positive half: every declared index returns
// exactly its matching set, nothing more. The length check on
// state.DeclaredIndexes fails loudly if a new index is declared without a
// matching scenario here, the same way tasks.md 2.2 fails the kube backend.
func testEveryDeclaredIndex(t *testing.T, store state.Store) {
	ctx := context.Background()
	records := []state.ConversationRecord{
		{ProfileRef: profileDefault, Name: "conv-a", ConversationID: "id-a", PipelineRef: pipe1, Phase: "Working"},
		{ProfileRef: profileDefault, Name: "conv-b", ConversationID: "id-b", PipelineRef: pipe1, Phase: "Closed"},
		{ProfileRef: profileDefault, Name: "conv-c", ConversationID: "id-c", PipelineRef: "pipe-2", Phase: "Working"},
	}
	for _, r := range records {
		if _, err := store.Create(ctx, state.Key(r.Name), r); err != nil {
			t.Fatalf("Create %s: %v", r.Name, err)
		}
	}

	wantIndexes := []state.Index{state.ByConversationID, state.ByPipelineAndPhase, state.ByPhase}
	if len(state.DeclaredIndexes) != len(wantIndexes) {
		t.Fatalf("state.DeclaredIndexes = %v, this suite covers only %v — add a scenario for the new index before shipping it", state.DeclaredIndexes, wantIndexes)
	}

	byConvID, err := store.List(ctx, state.ByConversationID, state.ListKey{ConversationID: "id-b"})
	if err != nil {
		t.Fatalf("List ByConversationID: %v", err)
	}
	if len(byConvID) != 1 || byConvID[0].Name != "conv-b" {
		t.Fatalf("List(ByConversationID, id-b) = %+v, want exactly conv-b", byConvID)
	}

	byPipelinePhase, err := store.List(ctx, state.ByPipelineAndPhase, state.ListKey{PipelineRef: pipe1, Phase: "Working"})
	if err != nil {
		t.Fatalf("List ByPipelineAndPhase: %v", err)
	}
	if len(byPipelinePhase) != 1 || byPipelinePhase[0].Name != "conv-a" {
		t.Fatalf("List(ByPipelineAndPhase, pipe-1/Active) = %+v, want exactly conv-a", byPipelinePhase)
	}

	byPhase, err := store.List(ctx, state.ByPhase, state.ListKey{Phase: "Working"})
	if err != nil {
		t.Fatalf("List ByPhase: %v", err)
	}
	if len(byPhase) != 2 {
		t.Fatalf("List(ByPhase, Active) returned %d records, want exactly the 2 Active ones", len(byPhase))
	}
}

// testUnknownIndexRefused covers the contract's closed index set: a lookup
// outside DeclaredIndexes is not expressible, and List reports
// ErrUnknownIndex rather than silently matching nothing.
func testUnknownIndexRefused(t *testing.T, store state.Store) {
	ctx := context.Background()
	_, err := store.List(ctx, state.Index("no-such-index"), state.ListKey{})
	if !errors.Is(err, state.ErrUnknownIndex) {
		t.Fatalf("List on an undeclared index error = %v, want ErrUnknownIndex", err)
	}
}

// --- Requirement: The change feed is at-least-once and resumable ---

// testFeedEventShape asserts design D-3's "(key, revision, record)" shape
// on a Put event.
func testFeedEventShape(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}

	key := state.Key("conv-feed-shape")
	rev, err := store.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-feed-shape", Title: "v1"})
	if err != nil {
		t.Fatalf(fmtCreate, err)
	}

	// A backend may land one Create as more than one underlying write
	// (mustRecvEventMatching's own comment); the event carrying Create's
	// own returned revision is the one this asserts against, among
	// whatever notifications the write actually produced.
	evt := mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event) bool {
		return e.Type == state.EventPut && e.Key == key && e.Revision == rev
	})
	if evt.Record.Title != "v1" {
		t.Fatalf("event.Record.Title = %q, want %q", evt.Record.Title, "v1")
	}
}

// testFeedResumeContinuity is the "A new leader resumes from state alone"
// scenario, phrased the backend-agnostic way: the same Store, watching from
// a resume point taken earlier, observes a record written since — modelling
// a process restart with no in-memory hand-off, without assuming a double
// shares state across newStore() calls.
func testFeedResumeContinuity(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	formerLeaderEvents, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-resume-leader")
	rec := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-resume-leader", Phase: "Working"}
	if _, err := store.Create(ctx, key, rec); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	createEvt := mustRecvEvent(t, formerLeaderEvents, 5*time.Second)
	resumePoint := createEvt.Resume

	// The former leader is gone. Nothing more is read from
	// formerLeaderEvents — there is no in-memory hand-off — while a change
	// still lands.
	_, rev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	rec.Phase = "Closed"
	if _, err := store.Update(ctx, key, rev, rec); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// The new leader resumes from the point it kept, from state alone. A
	// backend may redeliver a notification the former leader's own Create
	// already produced a sibling of (mustRecvEventMatching's own comment),
	// so this looks for the Closed update among what resuming delivers
	// rather than assuming it is the very first thing redelivered.
	resumed, err := store.Watch(ctx, resumePoint)
	if err != nil {
		t.Fatalf("Watch(resume): %v", err)
	}
	mustRecvEventMatching(t, resumed, 5*time.Second, func(e state.Event) bool {
		return e.Key == key && e.Type == state.EventPut && e.Record.Phase == "Closed"
	})
}

// testFeedNoGapAcrossDisconnect is the "No gap across a consumer disconnect"
// scenario: a consumer stops reading, many changes land, and resuming from
// its kept point observes every one, in order. Duplicates are permitted by
// the spec; a gap is not, which is what this asserts.
func testFeedNoGapAcrossDisconnect(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-no-gap")
	rec := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-no-gap", Title: "0"}
	if _, err := store.Create(ctx, key, rec); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	firstEvt := mustRecvEvent(t, events, 5*time.Second)
	lastSeen := firstEvt.Resume

	// Disconnect: stop reading from events entirely, and keep writing.
	const duringDisconnect = 20
	_, currentRev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf(fmtGet, err)
	}
	for i := 1; i <= duringDisconnect; i++ {
		rec.Title = fmt.Sprintf("%d", i)
		newRev, err := store.Update(ctx, key, currentRev, rec)
		if err != nil {
			t.Fatalf("Update %d: %v", i, err)
		}
		currentRev = newRev
	}

	// Reconnect: a brand new feed, resumed from the kept point. Collect
	// every delivered title until the feed goes quiet rather than reading
	// exactly duringDisconnect events — a backend may land one logical
	// write as more than one underlying notification
	// (mustRecvEventMatching's own comment), so consecutive repeats of one
	// title are real, permitted duplicates, not a gap.
	resumed, err := store.Watch(ctx, lastSeen)
	if err != nil {
		t.Fatalf("Watch(resume): %v", err)
	}
	var titles []string
	for _, evt := range drainQuiet(resumed, 500*time.Millisecond) {
		titles = append(titles, evt.Record.Title)
	}

	// Collapse consecutive duplicates, then drop any leading titles left
	// over from before the disconnect (Create's own "0", already seen) —
	// what remains must be exactly "1"..".duringDisconnect", in order,
	// with nothing skipped.
	var collapsed []string
	for _, title := range titles {
		if len(collapsed) == 0 || collapsed[len(collapsed)-1] != title {
			collapsed = append(collapsed, title)
		}
	}
	for len(collapsed) > 0 && collapsed[0] == "0" {
		collapsed = collapsed[1:]
	}
	if len(collapsed) != duringDisconnect {
		t.Fatalf("got titles %v after collapsing duplicates, want exactly %d entries (1..%d) — gap or reorder across the disconnect", collapsed, duringDisconnect, duringDisconnect)
	}
	for i, title := range collapsed {
		want := fmt.Sprintf("%d", i+1)
		if title != want {
			t.Fatalf("event %d title = %q, want %q — gap or reorder across the disconnect", i, title, want)
		}
	}
}

// testFeedStaleSnapshotCaughtByCAS is the "A stale snapshot is caught by the
// write, not the read" scenario: a consumer acting on an event's record
// after it changed gets refused on write, and a later event carries the
// newer record.
func testFeedStaleSnapshotCaughtByCAS(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-stale-snapshot")
	rec := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-stale-snapshot", Title: "v1"}
	if _, err := store.Create(ctx, key, rec); err != nil {
		t.Fatalf(fmtCreate, err)
	}
	snapshot := mustRecvEvent(t, events, 5*time.Second)

	// A second actor moves the record on with its OWN fresh read, not the
	// consumer's watched snapshot — a backend that lands one logical
	// write as more than one underlying notification
	// (mustRecvEventMatching's own comment) may hand the consumer a
	// snapshot already one write behind the record's true current
	// revision, and reusing it here would conflict against that rather
	// than against this second actor, which is not what this scenario is
	// about.
	current, currentRev, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get (second actor): %v", err)
	}
	current.Title = "v2-by-another-actor"
	if _, err := store.Update(ctx, key, currentRev, current); err != nil {
		t.Fatalf("Update (second actor): %v", err)
	}

	// The consumer acts on its now-stale snapshot: the write is refused —
	// the record changed since the consumer read it, whether that change
	// was the second actor's above or the record's own trailing write.
	staleWrite := snapshot.Record
	staleWrite.Title = "v2-by-the-consumer"
	if _, err := store.Update(ctx, key, snapshot.Revision, staleWrite); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("stale consumer Update error = %v, want ErrConflict", err)
	}

	// A later event carries the newer record.
	mustRecvEventMatching(t, events, 5*time.Second, func(e state.Event) bool {
		return e.Key == key && e.Record.Title == "v2-by-another-actor"
	})
}

// --- Requirement: A record may carry an expiry ---

// testExpiryInvisible is the "An expired record is invisible before it is
// gone" scenario.
func testExpiryInvisible(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key(convExpired)
	past := time.Now().Add(-time.Hour)
	rec := state.ConversationRecord{ProfileRef: profileDefault, Name: convExpired, Phase: "Working", ExpiresAt: &past}
	if _, err := store.Create(ctx, key, rec); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	if _, _, err := store.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get(expired) error = %v, want ErrNotFound", err)
	}

	listed, err := store.List(ctx, state.ByPhase, state.ListKey{Phase: "Working"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range listed {
		if r.Name == convExpired {
			t.Fatalf("List(ByPhase, Active) returned the expired record")
		}
	}
}

// testExpiryObservableOnFeed is the "Expiry is observable on the feed"
// scenario: once the backend removes an expired record, the feed carries a
// delete event for its key.
func testExpiryObservableOnFeed(t *testing.T, store state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := store.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	key := state.Key("conv-expiry-feed")
	past := time.Now().Add(-time.Hour)
	rec := state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-expiry-feed", Phase: "Working", ExpiresAt: &past}
	if _, err := store.Create(ctx, key, rec); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	putEvt := mustRecvEvent(t, events, 5*time.Second)
	if putEvt.Type != state.EventPut {
		t.Fatalf("first event Type = %v, want EventPut", putEvt.Type)
	}

	// How the removal happens is the backend's own (spec) — a later lookup
	// is enough to give it the chance to run.
	if _, _, err := store.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after expiry error = %v, want ErrNotFound", err)
	}

	// A backend may still deliver a sibling of the Create above before the
	// delete (mustRecvEventMatching's own comment), and a sweep-based
	// backend's removal is not instantaneous (spec: "SHALL eventually
	// remove it") — this looks for the delete among what arrives rather
	// than assuming it is the very next thing.
	mustRecvEventMatching(t, events, 8*time.Second, func(e state.Event) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
}

// --- Requirement: Runs are append-only sub-records of a conversation ---

// testAppendRunNeverConflicts is the "An append needs no revision and never
// conflicts" scenario: two actors append a run each to the same
// conversation at once, neither holding a revision, and both succeed.
func testAppendRunNeverConflicts(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-append-race")
	if _, err := store.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-append-race"}); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	appendOne := func(runID string) {
		defer wg.Done()
		errs <- store.AppendRun(ctx, key, state.Run{RunID: runID, Status: "Succeeded"})
	}
	wg.Add(2)
	go appendOne("run-a")
	go appendOne("run-b")
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("AppendRun surfaced a conflict or other error: %v", err)
		}
	}

	runs, err := store.ListRuns(ctx, key)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range runs {
		seen[r.RunID] = true
	}
	if !seen["run-a"] || !seen["run-b"] || len(runs) != 2 {
		t.Fatalf("ListRuns = %+v, want exactly run-a and run-b", runs)
	}
}

// testListRunsOrder is the "Runs read back in order" scenario.
func testListRunsOrder(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("conv-runs-order")
	if _, err := store.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: "conv-runs-order"}); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	const n = 5
	for i := 0; i < n; i++ {
		if err := store.AppendRun(ctx, key, state.Run{RunID: fmt.Sprintf("run-%d", i)}); err != nil {
			t.Fatalf("AppendRun %d: %v", i, err)
		}
	}

	runs, err := store.ListRuns(ctx, key)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != n {
		t.Fatalf("ListRuns returned %d runs, want %d", len(runs), n)
	}
	for i, r := range runs {
		want := fmt.Sprintf("run-%d", i)
		if r.RunID != want {
			t.Fatalf("runs[%d].RunID = %q, want %q — appended order not preserved", i, r.RunID, want)
		}
	}
}

// --- Requirement: Input payloads are blobs by key ---

// testBlobLifecycle is the "A pruned input releases its payload" scenario,
// generalized to the full put/get/delete/re-put lifecycle.
func testBlobLifecycle(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key("input-blob-1")
	blob := state.Blob{ConversationName: "conv-with-input", Type: "task", Payload: "hello"}

	if err := store.PutBlob(ctx, key, blob); err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	got, err := store.GetBlob(ctx, key)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if got.Payload != "hello" {
		t.Fatalf("GetBlob.Payload = %q, want %q", got.Payload, "hello")
	}

	if err := store.DeleteBlob(ctx, key); err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}
	if _, err := store.GetBlob(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("GetBlob after delete error = %v, want ErrNotFound", err)
	}

	// A pruned input's key may be reused later without special-casing.
	if err := store.PutBlob(ctx, key, state.Blob{ConversationName: "conv-with-input", Type: "task", Payload: secondLife}); err != nil {
		t.Fatalf("PutBlob (re-put after delete): %v", err)
	}
	got2, err := store.GetBlob(ctx, key)
	if err != nil {
		t.Fatalf("GetBlob (after re-put): %v", err)
	}
	if got2.Payload != secondLife {
		t.Fatalf("GetBlob.Payload after re-put = %q, want %q", got2.Payload, secondLife)
	}
}

// --- Requirement: Scope is fixed when the store is constructed ---

// testScopeIsolation is the "Two installs never see each other" scenario.
func testScopeIsolation(t *testing.T, newA, newB func() state.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	storeA := newA()
	storeB := newB()

	key := state.Key(convSharedName)
	if _, err := storeA.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: convSharedName, Title: "install-A-only"}); err != nil {
		t.Fatalf("Create on A: %v", err)
	}

	if _, _, err := storeB.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on B for A's key error = %v, want ErrNotFound", err)
	}

	listed, err := storeB.List(ctx, state.ByPhase, state.ListKey{Phase: ""})
	if err != nil {
		t.Fatalf("List on B: %v", err)
	}
	for _, r := range listed {
		if r.Name == convSharedName {
			t.Fatalf("B's listing includes A's record")
		}
	}

	eventsB, err := storeB.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch on B: %v", err)
	}
	select {
	case evt := <-eventsB:
		t.Fatalf("B's feed delivered an event from A's install: %+v", evt)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing from A ever reaches B.
	}
}

// --- Requirement: The contract offers no multi-record atomicity ---

// testNoMultiRecordAtomicity is the "A crash between a conversation write
// and its input write" scenario. Store carries no Begin/Commit (a
// compile-time fact of the interface shape in store.go, not something this
// test can assert at runtime) — the two writes below are two independent
// calls a caller may interleave with a failure between them, modelled here
// by simply not making the second call yet, and recovering from it later
// with nothing silently lost.
func testNoMultiRecordAtomicity(t *testing.T, store state.Store) {
	ctx := context.Background()
	key := state.Key(convCrashBetweenWrites)
	if _, err := store.Create(ctx, key, state.ConversationRecord{ProfileRef: profileDefault, Name: convCrashBetweenWrites, Phase: "Working"}); err != nil {
		t.Fatalf(fmtCreate, err)
	}

	// "Crash" before the input payload's blob is written: the conversation
	// write already landed on its own, independent of the second write.
	got, _, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after the conversation write alone: %v", err)
	}
	if got.Phase != "Working" {
		t.Fatalf("Phase = %q, want %q — the conversation write must stand on its own", got.Phase, "Working")
	}

	// The next reconciliation completes the owed input.
	blobKey := state.Key("conv-crash-between-writes-input-1")
	if err := store.PutBlob(ctx, blobKey, state.Blob{ConversationName: convCrashBetweenWrites, Type: "task", Payload: owedInput}); err != nil {
		t.Fatalf("PutBlob (completing the owed write): %v", err)
	}
	blob, err := store.GetBlob(ctx, blobKey)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if blob.Payload != owedInput {
		t.Fatalf("Payload = %q, want %q", blob.Payload, owedInput)
	}
}
