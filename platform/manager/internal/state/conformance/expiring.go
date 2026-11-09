package conformance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// RunExpiring executes every state.Collection[T] scenario (Run) plus the
// expiry scenarios only an Expiring[T] has: a record past its expiry is
// invisible to Get and List, and its eventual removal is delivered as a
// delete on the feed.
func RunExpiring[T any](t *testing.T, open func() state.Expiring[T], decl state.Decl[T], fx Fixture[T]) {
	t.Helper()
	asCollection := func() state.Collection[T] { return open() }
	Run(t, asCollection, decl, fx)

	t.Run("RecordMayCarryExpiry", func(t *testing.T) {
		t.Run("ExpiredRecordInvisibleBeforeRemoved", func(t *testing.T) { testExpiryInvisible(t, open(), decl, fx) })
		t.Run("ExpiryObservableOnFeedAsDelete", func(t *testing.T) { testExpiryObservableOnFeed(t, open(), fx) })
		t.Run("ExpiryWhileDisconnectedArrivesOnTheResumedFeed", func(t *testing.T) { testExpiryAcrossResume(t, open(), fx) })
	})
}

// A fixture for an Expiring kind reads the marker -1 on Edit as "set a past
// expiry" (fixtures.go) — the one convention this file relies on, since the
// generic Fixture carries an int marker and an expiry is a time.Time.

func testExpiryInvisible[T any](t *testing.T, s state.Expiring[T], decl state.Decl[T], fx Fixture[T]) {
	ctx := context.Background()
	key := fx.Key("expired")
	rec := fx.New()
	rec = fx.Edit(rec, -1) // fixtures.go: a negative marker on an expiring kind sets a past expiry
	if _, err := s.Create(ctx, key, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get(expired) error = %v, want ErrNotFound", err)
	}

	for _, idx := range decl.Indexes {
		v, listed := idx.Value(&rec)
		if !listed {
			continue
		}
		got, err := s.List(ctx, idx, v)
		if err != nil {
			t.Fatalf("List(%s): %v", idx.Name(), err)
		}
		for _, r := range got {
			if fx.KeyOf(r) == key {
				t.Fatalf("List(%s) returned the expired record", idx.Name())
			}
		}
	}
}

func testExpiryObservableOnFeed[T any](t *testing.T, s state.Expiring[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	key := fx.Key("expiry-feed")
	rec := fx.Edit(fx.New(), -1)
	if _, err := s.Create(ctx, key, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	putEvt := mustRecvEvent(t, events, 5*time.Second)
	if putEvt.Type != state.EventPut {
		t.Fatalf("first event Type = %v, want EventPut", putEvt.Type)
	}

	// How the removal happens is the backend's own; a later lookup gives it
	// the chance to run.
	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get after expiry error = %v, want ErrNotFound", err)
	}

	mustRecvEventMatching(t, events, 8*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
}

// testExpiryAcrossResume lets a record expire while no feed is open. A feed
// resumed from before it must deliver the removal, or a consumer that cached
// the record keeps serving it for good.
func testExpiryAcrossResume[T any](t *testing.T, s state.Expiring[T], fx Fixture[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first, firstCancel := context.WithCancel(ctx)
	events, err := s.Watch(first, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	key := fx.Key("expiry-resume")
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	put := mustRecvEvent(t, events, 5*time.Second)
	if put.Type != state.EventPut || put.Resume == "" {
		t.Fatalf("first event = %+v, want a put carrying a resume point", put)
	}
	firstCancel() // the consumer disconnects here

	if _, err := state.Mutate(ctx, s, key, func(r *T) error {
		*r = fx.Edit(*r, -1)
		return nil
	}); err != nil {
		t.Fatalf("expire the record: %v", err)
	}
	if _, _, err := s.Get(ctx, key); !errors.Is(err, state.ErrNotFound) { // a lookup lets a lazy backend remove it
		t.Fatalf("Get after expiry = %v, want ErrNotFound", err)
	}

	resumed, err := s.Watch(ctx, put.Resume)
	if err != nil {
		t.Fatalf("resume from %q: %v", put.Resume, err)
	}
	mustRecvEventMatching(t, resumed, 10*time.Second, func(e state.Event[T]) bool {
		return e.Type == state.EventDelete && e.Key == key
	})
}
