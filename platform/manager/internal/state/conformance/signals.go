package conformance

// The SignalStore half of the suite (design D-14 of the state-store change).
// Every scenario is named for the requirement and scenario of
// specs/state-store/spec.md it proves, and every backend of SignalStore passes
// all of them to exist.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The source names the signal scenarios use. A backend that links a cooldown
// window to a configuration object it does not own (the kube backend keeps it
// on the SignalSource) must have these sources present before the scenarios
// run; SignalSources lists them for its factory.
const (
	SignalSourceA = "conf-source-a"
	SignalSourceB = "conf-source-b"
)

// SignalSources is every source name the signal scenarios use.
func SignalSources() []string { return []string{SignalSourceA, SignalSourceB} }

const (
	fmtSignalCreate = "Create signal: %v"
	fmtSignalGet    = "Get signal: %v"
	fmtSignalUpdate = "Update signal: %v"
	sigInitA        = "init-a"
	sigInitB        = "init-b"
)

// sigRec is a published signal on a source, as publication writes it.
func sigRec(name, source, initiation string) state.SignalRecord {
	return state.SignalRecord{
		Name: name, SourceRef: source, Adapter: "cron", InitiationID: initiation,
		SignalID: "id-" + name, Kind: "alert", Title: "title " + name, Severity: "warning",
		Labels: map[string]string{"k": "v"}, Reader: "reader-1", Payload: "payload of " + name,
	}
}

// RunSignals executes every SignalStore scenario against a store newStore
// constructs fresh for each. The same rules as Run: newStore returns an empty,
// isolated store every call.
func RunSignals(t *testing.T, newStore func() state.SignalStore) {
	t.Helper()

	t.Run("SignalCreateIsTheArbiterOfADuplicate", func(t *testing.T) {
		t.Run("SecondCreateUnderOneKeyRefusedAndChangesNothing", func(t *testing.T) {
			testSignalCreateTwice(t, newStore())
		})
		t.Run("ConcurrentCreatesExactlyOneWins", func(t *testing.T) {
			testSignalCreateRace(t, newStore())
		})
		t.Run("DuplicateIsRefusedInEveryPhase", func(t *testing.T) {
			testSignalCreateAfterTerminal(t, newStore())
		})
	})

	t.Run("SignalWritesAreCASOnStatusOnly", func(t *testing.T) {
		t.Run("StaleStatusWriteRefusedAndNeverMerged", func(t *testing.T) {
			testSignalStaleWrite(t, newStore())
		})
		t.Run("UpdateNeverRewritesThePublicationFields", func(t *testing.T) {
			testSignalPublicationFieldsImmutable(t, newStore())
		})
		t.Run("ClaimantsAndStatusRoundTrip", func(t *testing.T) {
			testSignalStatusRoundTrip(t, newStore())
		})
		t.Run("UpdateOfAnAbsentSignalIsNotFound", func(t *testing.T) {
			testSignalUpdateAbsent(t, newStore())
		})
	})

	t.Run("SignalCarriesBackendMintedIdentity", func(t *testing.T) {
		t.Run("IdentityMintedAndCallerSuppliedIgnored", func(t *testing.T) {
			testSignalIdentity(t, newStore())
		})
		t.Run("ArrivalOrderIsCreatedAtThenName", func(t *testing.T) {
			testSignalArrivalOrder(t, newStore())
		})
	})

	t.Run("SignalLookupIsByKeyOrDeclaredIndex", func(t *testing.T) {
		t.Run("NonTerminalBySourceExcludesTerminalAndOtherSources", func(t *testing.T) {
			testSignalIndexNonTerminalBySource(t, newStore())
		})
		t.Run("NonTerminalByInitiationFindsEarlierMessagesOfOneConversation", func(t *testing.T) {
			testSignalIndexNonTerminalByInitiation(t, newStore())
		})
		t.Run("WholeSetListedWithoutFilter", func(t *testing.T) {
			testSignalIndexAll(t, newStore())
		})
		t.Run("QueryOutsideIndexSetRefused", func(t *testing.T) {
			testSignalUnknownIndex(t, newStore())
		})
	})

	t.Run("SignalChangeFeedIsAtLeastOnceAndResumable", func(t *testing.T) {
		t.Run("EventsCarryKeyRevisionRecordAndDeleteOnRemoval", func(t *testing.T) {
			testSignalFeedShape(t, newStore())
		})
		t.Run("ResumeSeesEveryChangeMadeWhileAway", func(t *testing.T) {
			testSignalFeedResume(t, newStore())
		})
	})

	t.Run("SignalMayCarryExpiry", func(t *testing.T) {
		t.Run("ExpiredSignalInvisibleBeforeRemoved", func(t *testing.T) {
			testSignalExpiryInvisible(t, newStore())
		})
		t.Run("CreateReplacesAnExpiredSignal", func(t *testing.T) {
			testSignalCreateReplacesExpired(t, newStore())
		})
		t.Run("ExpiryObservableOnFeedAsDelete", func(t *testing.T) {
			testSignalExpiryOnFeed(t, newStore())
		})
		t.Run("TerminalWriteCarriesItsExpiryInTheSameCAS", func(t *testing.T) {
			testSignalExpiryWrittenWithTerminalPhase(t, newStore())
		})
	})

	t.Run("SignalDeletionIsImmediate", func(t *testing.T) {
		t.Run("DeleteRemovesAtOnceAndAbsentIsNotAnError", func(t *testing.T) {
			testSignalDelete(t, newStore())
		})
	})

	t.Run("SourceCooldownWindowIsStateToo", func(t *testing.T) {
		t.Run("EmptyBeforeAnythingIsRecorded", func(t *testing.T) {
			testCooldownEmpty(t, newStore())
		})
		t.Run("PutReplacesTheWholeWindowPerSource", func(t *testing.T) {
			testCooldownReplace(t, newStore())
		})
		t.Run("InstantsReadBackInUTC", func(t *testing.T) {
			testCooldownUTC(t, newStore())
		})
	})

	t.Run("ReleasingASourceRemovesItsSignals", func(t *testing.T) {
		t.Run("ReceiptsAreCountedWithoutLosingAnIncrement", func(t *testing.T) {
			testRecordReceived(t, newStore())
		})
		t.Run("ASuccessorsSignalsOutliveTheDepartedIdentitysRelease", func(t *testing.T) {
			testReleaseSparesSuccessor(t, newStore())
		})
		t.Run("EverySignalOfTheSourceAnyPhaseAndItsWindowGoOthersStay", func(t *testing.T) {
			testReleaseSource(t, newStore())
		})
	})

	t.Run("SignalScopeIsFixedAtConstruction", func(t *testing.T) {
		t.Run("TwoInstallsNeverSeeEachOther", func(t *testing.T) {
			testSignalScope(t, newStore(), newStore())
		})
	})

	t.Run("SignalInstantsAreUTC", func(t *testing.T) {
		t.Run("InstantsReadBackInUTCWhateverWasWritten", func(t *testing.T) {
			testSignalInstantsUTC(t, newStore())
		})
	})
}

func mustCreateSignal(t *testing.T, s state.SignalStore, rec state.SignalRecord) state.Revision {
	t.Helper()
	rev, err := s.Create(context.Background(), state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf(fmtSignalCreate, err)
	}
	return rev
}

// finish returns rec as the consumer writes it when it makes the phase terminal.
func finish(rec state.SignalRecord, phase string) state.SignalRecord {
	at := time.Now().UTC().Truncate(time.Second)
	rec.Phase, rec.CompletedAt, rec.ConsumedAt, rec.ConsumedBy = phase, &at, &at, "leader-1"
	return rec
}

func signalNames(recs []state.SignalRecord) map[string]bool {
	out := map[string]bool{}
	for _, r := range recs {
		out[r.Name] = true
	}
	return out
}

func sameNames(t *testing.T, what string, got []state.SignalRecord, want ...string) {
	t.Helper()
	names := signalNames(got)
	if len(got) != len(want) {
		t.Fatalf("%s returned %d records (%v), want %v", what, len(got), names, want)
	}
	for _, w := range want {
		if !names[w] {
			t.Fatalf("%s = %v, want %v", what, names, want)
		}
	}
}

// --- Requirement: A signal create is the arbiter of a duplicate ---

func testSignalCreateTwice(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	first := sigRec("sig-twice", SignalSourceA, sigInitA)
	mustCreateSignal(t, s, first)
	second := sigRec("sig-twice", SignalSourceA, sigInitA)
	second.Payload = "second"
	if _, err := s.Create(ctx, "sig-twice", second); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create error = %v, want ErrAlreadyExists", err)
	}
	got, _, err := s.Get(ctx, "sig-twice")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.Payload != first.Payload {
		t.Fatalf("Payload = %q after a refused create, want %q unchanged", got.Payload, first.Payload)
	}
}

func testSignalCreateRace(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Create(ctx, "sig-race", sigRec("sig-race", SignalSourceA, sigInitA))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var ok, dup int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, state.ErrAlreadyExists):
			dup++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 7 {
		t.Fatalf("got %d creates and %d duplicates, want exactly 1 and 7", ok, dup)
	}
}

func testSignalCreateAfterTerminal(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-done", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	if _, err := s.Update(ctx, "sig-done", rev, finish(rec, state.SignalRouted)); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	if _, err := s.Create(ctx, "sig-done", rec); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("Create over a Routed signal error = %v, want ErrAlreadyExists", err)
	}
}

// --- Requirement: Signal writes are compare-and-swap on status only ---

func testSignalStaleWrite(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-stale", SignalSourceA, sigInitA)
	stale := mustCreateSignal(t, s, rec)
	win := rec
	win.Attempts = 1
	if _, err := s.Update(ctx, "sig-stale", stale, win); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	lose := rec
	lose.Attempts = 9
	lose.Phase = state.SignalFailed
	if _, err := s.Update(ctx, "sig-stale", stale, lose); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("stale Update error = %v, want ErrConflict", err)
	}
	got, _, err := s.Get(ctx, "sig-stale")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.Attempts != 1 || got.Phase == state.SignalFailed {
		t.Fatalf("record = attempts %d phase %q after a refused write, want attempts 1 and the winner's phase", got.Attempts, got.Phase)
	}
}

func testSignalPublicationFieldsImmutable(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-fixed", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	changed := rec
	changed.SourceRef, changed.InitiationID, changed.Payload, changed.Title = SignalSourceB, "other", "rewritten", "rewritten"
	changed.Labels = map[string]string{"x": "y"}
	changed.Attempts = 2
	if _, err := s.Update(ctx, "sig-fixed", rev, changed); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, _, err := s.Get(ctx, "sig-fixed")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.SourceRef != rec.SourceRef || got.InitiationID != rec.InitiationID || got.Payload != rec.Payload ||
		got.Title != rec.Title || got.Labels["k"] != "v" || len(got.Labels) != 1 {
		t.Fatalf("publication fields = %+v, want those of the create", got)
	}
	if got.Attempts != 2 {
		t.Fatalf("Attempts = %d, want the status write to land", got.Attempts)
	}
}

func testSignalStatusRoundTrip(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-status", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	first := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	rec = finish(rec, state.SignalRouted)
	rec.Reason, rec.Attempts, rec.FirstAttemptAt = "", 3, &first
	rec.Claimants = []state.SignalClaimant{
		{Kind: state.ClaimantKindPipeline, Name: "p1", State: state.ClaimantRouted, ConversationRef: "conv-1"},
		{Kind: state.ClaimantKindCoordinator, Name: "c1", State: state.ClaimantFailed, Reason: "overflow"},
		{Kind: state.ClaimantKindPipeline, Name: "p2", State: state.ClaimantSkipped, Reason: "gone"},
	}
	if _, err := s.Update(ctx, "sig-status", rev, rec); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, _, err := s.Get(ctx, "sig-status")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.Phase != state.SignalRouted || got.Attempts != 3 || got.ConsumedBy != "leader-1" ||
		got.FirstAttemptAt == nil || !got.FirstAttemptAt.Equal(first) ||
		got.CompletedAt == nil || !got.CompletedAt.Equal(*rec.CompletedAt) ||
		got.ConsumedAt == nil || len(got.Claimants) != 3 {
		t.Fatalf("status = %+v, want what was written", got)
	}
	if got.Claimants[0] != rec.Claimants[0] || got.Claimants[1] != rec.Claimants[1] || got.Claimants[2] != rec.Claimants[2] {
		t.Fatalf("claimants = %+v, want %+v in order", got.Claimants, rec.Claimants)
	}
}

func testSignalUpdateAbsent(t *testing.T, s state.SignalStore) {
	if _, err := s.Update(context.Background(), "sig-absent", "1", sigRec("sig-absent", SignalSourceA, sigInitA)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Update of an absent signal error = %v, want ErrNotFound", err)
	}
}

// --- Requirement: A signal carries a backend-minted identity ---

func testSignalIdentity(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-ident", SignalSourceA, sigInitA)
	rec.ID, rec.CreatedAt = "forged", time.Unix(1, 0)
	rev := mustCreateSignal(t, s, rec)
	got, _, err := s.Get(ctx, "sig-ident")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.ID == "" || got.ID == "forged" || got.CreatedAt.Year() < 2000 {
		t.Fatalf("identity = %q created %v, want the backend's own", got.ID, got.CreatedAt)
	}
	upd := got
	upd.ID, upd.CreatedAt, upd.Attempts = "forged-again", time.Unix(2, 0), 1
	if _, err := s.Update(ctx, "sig-ident", rev, upd); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	again, _, err := s.Get(ctx, "sig-ident")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if again.ID != got.ID || !again.CreatedAt.Equal(got.CreatedAt) {
		t.Fatalf("identity changed across an update: %q %v then %q %v", got.ID, got.CreatedAt, again.ID, again.CreatedAt)
	}
	if err := s.Delete(ctx, "sig-ident"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	mustCreateSignal(t, s, sigRec("sig-ident", SignalSourceA, sigInitA))
	renewed, _, _ := s.Get(ctx, "sig-ident")
	if renewed.ID == got.ID {
		t.Fatalf("a recreated signal kept the old identity %q", got.ID)
	}
}

func testSignalArrivalOrder(t *testing.T, s state.SignalStore) {
	earlier := state.SignalRecord{Name: "a", CreatedAt: time.Unix(10, 0)}
	later := state.SignalRecord{Name: "b", CreatedAt: time.Unix(20, 0)}
	sameInstant := state.SignalRecord{Name: "c", CreatedAt: time.Unix(10, 0)}
	if !state.SignalArrivesBefore(&earlier, &later) || state.SignalArrivesBefore(&later, &earlier) {
		t.Fatalf("an earlier CreatedAt must arrive first")
	}
	if !state.SignalArrivesBefore(&earlier, &sameInstant) || state.SignalArrivesBefore(&sameInstant, &earlier) {
		t.Fatalf("equal CreatedAt must order by Name")
	}
	_ = s
}

// --- Requirement: Signal lookup is by key or by a declared index ---

func testSignalIndexNonTerminalBySource(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	live := sigRec("sig-live", SignalSourceA, sigInitA)
	mustCreateSignal(t, s, live)
	mustCreateSignal(t, s, sigRec("sig-other-source", SignalSourceB, sigInitA))
	done := sigRec("sig-terminal", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, done)
	if _, err := s.Update(ctx, "sig-terminal", rev, finish(done, state.SignalDropped)); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	part := sigRec("sig-part", SignalSourceA, sigInitB)
	rev = mustCreateSignal(t, s, part)
	part.Phase = state.SignalPartiallyRouted
	if _, err := s.Update(ctx, "sig-part", rev, part); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, err := s.List(ctx, state.SignalNonTerminalBySource, state.SignalListKey{Source: SignalSourceA})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sameNames(t, "NonTerminalBySource(a)", got, "sig-live", "sig-part")
}

func testSignalIndexNonTerminalByInitiation(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	mustCreateSignal(t, s, sigRec("sig-1", SignalSourceA, sigInitA))
	mustCreateSignal(t, s, sigRec("sig-2", SignalSourceA, sigInitA))
	mustCreateSignal(t, s, sigRec("sig-other-init", SignalSourceA, sigInitB))
	mustCreateSignal(t, s, sigRec("sig-other-src", SignalSourceB, sigInitA))
	done := sigRec("sig-3", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, done)
	if _, err := s.Update(ctx, "sig-3", rev, finish(done, state.SignalFailed)); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, err := s.List(ctx, state.SignalNonTerminalByInitiation, state.SignalListKey{Source: SignalSourceA, InitiationID: sigInitA})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sameNames(t, "NonTerminalByInitiation(a, init-a)", got, "sig-1", "sig-2")
}

func testSignalIndexAll(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	if len(state.DeclaredSignalIndexes) != 3 {
		t.Fatalf("state.DeclaredSignalIndexes = %v, this suite covers three — add a scenario for the new index", state.DeclaredSignalIndexes)
	}
	mustCreateSignal(t, s, sigRec("sig-x", SignalSourceA, sigInitA))
	done := sigRec("sig-y", SignalSourceB, sigInitB)
	rev := mustCreateSignal(t, s, done)
	if _, err := s.Update(ctx, "sig-y", rev, finish(done, state.SignalRouted)); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, err := s.List(ctx, state.SignalAll, state.SignalListKey{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sameNames(t, "All", got, "sig-x", "sig-y")
}

func testSignalUnknownIndex(t *testing.T, s state.SignalStore) {
	if _, err := s.List(context.Background(), state.SignalIndex("no-such-index"), state.SignalListKey{}); !errors.Is(err, state.ErrUnknownIndex) {
		t.Fatalf("List on an undeclared index error = %v, want ErrUnknownIndex", err)
	}
}

// --- Requirement: The signal change feed is at-least-once and resumable ---

func recvSignalEvent(t *testing.T, events <-chan state.SignalEvent, want func(state.SignalEvent) bool) state.SignalEvent {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("signal feed closed before a matching event arrived")
			}
			if want(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("no matching signal event within 10s")
		}
	}
}

func testSignalFeedShape(t *testing.T, s state.SignalStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	rec := sigRec("sig-feed", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	ev := recvSignalEvent(t, events, func(e state.SignalEvent) bool {
		return e.Type == state.EventPut && e.Key == "sig-feed" && e.Revision == rev
	})
	if ev.Record.Payload != rec.Payload || ev.Record.InitiationID != sigInitA {
		t.Fatalf("event record = %+v, want the created record", ev.Record)
	}
	if err := s.Delete(ctx, "sig-feed"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	recvSignalEvent(t, events, func(e state.SignalEvent) bool { return e.Type == state.EventDelete && e.Key == "sig-feed" })
}

func testSignalFeedResume(t *testing.T, s state.SignalStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first, stop := context.WithCancel(ctx)
	events, err := s.Watch(first, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	rev := mustCreateSignal(t, s, sigRec("sig-before", SignalSourceA, sigInitA))
	got := recvSignalEvent(t, events, func(e state.SignalEvent) bool { return e.Type == state.EventPut && e.Key == "sig-before" })
	resume := got.Resume
	stop()
	for range events { // wait for the feed to close
	}
	mustCreateSignal(t, s, sigRec("sig-away", SignalSourceA, sigInitA))
	rec, _, err := s.Get(ctx, "sig-before")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	rec.Attempts = 1
	if _, err := s.Update(ctx, "sig-before", rev, rec); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	resumed, err := s.Watch(ctx, resume)
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	recvSignalEvent(t, resumed, func(e state.SignalEvent) bool { return e.Type == state.EventPut && e.Key == "sig-away" })
	recvSignalEvent(t, resumed, func(e state.SignalEvent) bool {
		return e.Type == state.EventPut && e.Key == "sig-before" && e.Record.Attempts == 1
	})
}

// --- Requirement: A signal may carry an expiry ---

func testSignalExpiryInvisible(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	rec := sigRec("sig-expired", SignalSourceA, sigInitA)
	rec.ExpiresAt = &past
	mustCreateSignal(t, s, rec)
	if _, _, err := s.Get(ctx, "sig-expired"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get(expired) error = %v, want ErrNotFound", err)
	}
	for _, idx := range []state.SignalIndex{state.SignalAll, state.SignalNonTerminalBySource, state.SignalNonTerminalByInitiation} {
		got, err := s.List(ctx, idx, state.SignalListKey{Source: SignalSourceA, InitiationID: sigInitA})
		if err != nil {
			t.Fatalf("List(%s): %v", idx, err)
		}
		if signalNames(got)["sig-expired"] {
			t.Fatalf("List(%s) returned the expired record", idx)
		}
	}
}

func testSignalCreateReplacesExpired(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	stale := sigRec("sig-reuse", SignalSourceA, sigInitA)
	stale.ExpiresAt = &past
	stale.Payload = "first"
	mustCreateSignal(t, s, stale)
	fresh := sigRec("sig-reuse", SignalSourceA, sigInitA)
	fresh.Payload = "second"
	mustCreateSignal(t, s, fresh)
	got, _, err := s.Get(ctx, "sig-reuse")
	if err != nil || got.Payload != "second" {
		t.Fatalf("Get after re-creating over an expired record = %+v, %v, want the new payload", got, err)
	}
	if _, err := s.Create(ctx, "sig-reuse", fresh); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("a create over the live replacement error = %v, want ErrAlreadyExists", err)
	}
}

func testSignalExpiryOnFeed(t *testing.T, s state.SignalStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf(fmtWatch, err)
	}
	past := time.Now().Add(-time.Hour)
	rec := sigRec("sig-expiry-feed", SignalSourceA, sigInitA)
	rec.ExpiresAt = &past
	mustCreateSignal(t, s, rec)
	recvSignalEvent(t, events, func(e state.SignalEvent) bool { return e.Type == state.EventDelete && e.Key == "sig-expiry-feed" })
}

// testSignalExpiryWrittenWithTerminalPhase is the shape the consumer writes: the
// phase, completedAt and the expiry in one compare-and-swap, so a terminal
// signal removes itself with no second deleter.
func testSignalExpiryWrittenWithTerminalPhase(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	rec := sigRec("sig-ttl", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	soon := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	done := finish(rec, state.SignalRouted)
	done.ExpiresAt = &soon
	if _, err := s.Update(ctx, "sig-ttl", rev, done); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, rev2, err := s.Get(ctx, "sig-ttl")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(soon) || got.Phase != state.SignalRouted {
		t.Fatalf("record = phase %q expires %v, want Routed expiring %v", got.Phase, got.ExpiresAt, soon)
	}
	past := time.Now().Add(-time.Minute)
	done.ExpiresAt = &past
	if _, err := s.Update(ctx, "sig-ttl", rev2, done); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	if _, _, err := s.Get(ctx, "sig-ttl"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after the expiry passed error = %v, want ErrNotFound", err)
	}
}

// --- Requirement: Signal deletion is immediate ---

func testSignalDelete(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	mustCreateSignal(t, s, sigRec("sig-del", SignalSourceA, sigInitA))
	if err := s.Delete(ctx, "sig-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Get(ctx, "sig-del"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "sig-del"); err != nil {
		t.Fatalf("Delete of an absent key: %v, want nil", err)
	}
}

// --- Requirement: The source's cooldown window is state too ---

func testCooldownEmpty(t *testing.T, s state.SignalStore) {
	got, err := s.GetCooldown(context.Background(), SignalSourceA)
	if err != nil || len(got) != 0 {
		t.Fatalf("GetCooldown of a source with nothing recorded = %v, %v, want an empty window and no error", got, err)
	}
}

func testCooldownReplace(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	if err := s.PutCooldown(ctx, SignalSourceA, []state.CooldownEntry{{InitiationID: "i1", At: at}, {InitiationID: "i2", At: at}}); err != nil {
		t.Fatalf("PutCooldown: %v", err)
	}
	if err := s.PutCooldown(ctx, SignalSourceB, []state.CooldownEntry{{InitiationID: "i9", At: at}}); err != nil {
		t.Fatalf("PutCooldown: %v", err)
	}
	if err := s.PutCooldown(ctx, SignalSourceA, []state.CooldownEntry{{InitiationID: "i3", At: at}}); err != nil {
		t.Fatalf("PutCooldown (replace): %v", err)
	}
	a, err := s.GetCooldown(ctx, SignalSourceA)
	if err != nil {
		t.Fatalf("GetCooldown: %v", err)
	}
	if len(a) != 1 || a[0].InitiationID != "i3" || !a[0].At.Equal(at) {
		t.Fatalf("window of a = %+v, want exactly the replacement i3", a)
	}
	b, _ := s.GetCooldown(ctx, SignalSourceB)
	if len(b) != 1 || b[0].InitiationID != "i9" {
		t.Fatalf("window of b = %+v, want its own i9 untouched", b)
	}
	if err := s.PutCooldown(ctx, SignalSourceA, nil); err != nil {
		t.Fatalf("PutCooldown (clear): %v", err)
	}
	if a, _ := s.GetCooldown(ctx, SignalSourceA); len(a) != 0 {
		t.Fatalf("window of a = %+v after a clear, want empty", a)
	}
}

func testCooldownUTC(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	zone := time.FixedZone("UTC+3", 3*3600)
	at := time.Now().In(zone).Truncate(time.Second)
	if err := s.PutCooldown(ctx, SignalSourceA, []state.CooldownEntry{{InitiationID: "i1", At: at}}); err != nil {
		t.Fatalf("PutCooldown: %v", err)
	}
	got, _ := s.GetCooldown(ctx, SignalSourceA)
	if len(got) != 1 || got[0].At.Location() != time.UTC || !got[0].At.Equal(at) {
		t.Fatalf("window = %+v, want the same instant in UTC", got)
	}
}

// --- Requirement: Releasing a source removes its signals ---

func testReleaseSource(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	mustCreateSignal(t, s, sigRec("sig-a-live", SignalSourceA, sigInitA))
	done := sigRec("sig-a-done", SignalSourceA, sigInitB)
	rev := mustCreateSignal(t, s, done)
	if _, err := s.Update(ctx, "sig-a-done", rev, finish(done, state.SignalRouted)); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	mustCreateSignal(t, s, sigRec("sig-b-live", SignalSourceB, sigInitA))
	at := time.Now().UTC().Truncate(time.Second)
	if err := s.PutCooldown(ctx, SignalSourceA, []state.CooldownEntry{{InitiationID: "i1", At: at}}); err != nil {
		t.Fatalf("PutCooldown: %v", err)
	}
	if err := s.PutCooldown(ctx, SignalSourceB, []state.CooldownEntry{{InitiationID: "i2", At: at}}); err != nil {
		t.Fatalf("PutCooldown: %v", err)
	}

	for i := 0; i < 2; i++ { // idempotent
		if err := s.ReleaseSource(ctx, SignalSourceA, ""); err != nil {
			t.Fatalf("ReleaseSource: %v", err)
		}
	}
	all, err := s.List(ctx, state.SignalAll, state.SignalListKey{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sameNames(t, "after releasing a", all, "sig-b-live")
	if a, _ := s.GetCooldown(ctx, SignalSourceA); len(a) != 0 {
		t.Fatalf("window of a = %+v after release, want empty", a)
	}
	if b, _ := s.GetCooldown(ctx, SignalSourceB); len(b) != 1 {
		t.Fatalf("window of b = %+v, want untouched", b)
	}
}

func testRecordReceived(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	if got, err := s.GetReceived(ctx, SignalSourceA); err != nil || got.Total != 0 || got.Last != nil {
		t.Fatalf("GetReceived before any receipt = %+v, %v, want the zero value", got, err)
	}
	const n = 12
	at := time.Now().UTC().Truncate(time.Second)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.RecordReceived(ctx, SignalSourceA, at)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RecordReceived: %v", err)
		}
	}
	got, err := s.GetReceived(ctx, SignalSourceA)
	if err != nil || got.Total != n || got.Last == nil || !got.Last.Equal(at) || got.Last.Location() != time.UTC {
		t.Fatalf("GetReceived = %+v, %v, want %d receipts at %v in UTC", got, err, n, at)
	}
	if other, _ := s.GetReceived(ctx, SignalSourceB); other.Total != 0 {
		t.Fatalf("another source counted %d receipts, want none", other.Total)
	}
}

func testReleaseSparesSuccessor(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	old := sigRec("sig-old", SignalSourceA, sigInitA)
	old.SourceID = "incarnation-1"
	mustCreateSignal(t, s, old)
	next := sigRec("sig-next", SignalSourceA, sigInitB)
	next.SourceID = "incarnation-2"
	mustCreateSignal(t, s, next)
	if err := s.ReleaseSource(ctx, SignalSourceA, "incarnation-1"); err != nil {
		t.Fatalf("ReleaseSource: %v", err)
	}
	all, err := s.List(ctx, state.SignalAll, state.SignalListKey{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sameNames(t, "after releasing the departed identity", all, "sig-next")
}

// --- Requirement: Scope is fixed when the store is constructed ---

func testSignalScope(t *testing.T, a, b state.SignalStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mustCreateSignal(t, a, sigRec("sig-shared", SignalSourceA, sigInitA))
	if _, _, err := b.Get(ctx, "sig-shared"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on B for A's key error = %v, want ErrNotFound", err)
	}
	listed, err := b.List(ctx, state.SignalAll, state.SignalListKey{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("B's listing = %v, %v, want empty", listed, err)
	}
}

// --- Requirement: Instants are UTC ---

func testSignalInstantsUTC(t *testing.T, s state.SignalStore) {
	ctx := context.Background()
	zone := time.FixedZone("UTC-5", -5*3600)
	rec := sigRec("sig-utc", SignalSourceA, sigInitA)
	rev := mustCreateSignal(t, s, rec)
	at := time.Now().In(zone).Truncate(time.Second)
	exp := at.Add(time.Hour)
	rec = finish(rec, state.SignalRouted)
	rec.CompletedAt, rec.ConsumedAt, rec.FirstAttemptAt, rec.ExpiresAt = &at, &at, &at, &exp
	if _, err := s.Update(ctx, "sig-utc", rev, rec); err != nil {
		t.Fatalf(fmtSignalUpdate, err)
	}
	got, _, err := s.Get(ctx, "sig-utc")
	if err != nil {
		t.Fatalf(fmtSignalGet, err)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt zone = %v, want UTC", got.CreatedAt.Location())
	}
	for name, p := range map[string]*time.Time{"CompletedAt": got.CompletedAt, "ConsumedAt": got.ConsumedAt, "FirstAttemptAt": got.FirstAttemptAt, "ExpiresAt": got.ExpiresAt} {
		if p == nil || p.Location() != time.UTC || !p.Equal(map[string]time.Time{"CompletedAt": at, "ConsumedAt": at, "FirstAttemptAt": at, "ExpiresAt": exp}[name]) {
			t.Fatalf("%s = %v, want the written instant in UTC", name, p)
		}
	}
}
