package activity

import (
	"testing"
	"time"
)

func emitN(l *Log, n int) {
	for i := 0; i < n; i++ {
		l.Emit(Event{Kind: KindInputQueued, Conversation: "c"})
	}
}

func TestCursorsAreMonotonicAndOrdered(t *testing.T) {
	l := New(8)
	emitN(l, 5)
	got, resync := l.Since("", 0)
	if resync {
		t.Fatalf("first connect must not demand a resync")
	}
	if len(got) != 5 {
		t.Fatalf("want 5 events, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Cursor >= got[i].Cursor {
			t.Fatalf("cursors not increasing: %q then %q", got[i-1].Cursor, got[i].Cursor)
		}
	}
	if l.Latest() != got[len(got)-1].Cursor {
		t.Fatalf("Latest %q != last event cursor %q", l.Latest(), got[len(got)-1].Cursor)
	}
}

func TestSinceReturnsOnlyEventsAfterTheCursor(t *testing.T) {
	l := New(16)
	emitN(l, 3)
	first, _ := l.Since("", 0)
	rest, resync := l.Since(first[0].Cursor, 0)
	if resync {
		t.Fatalf("a live cursor must not resync")
	}
	if len(rest) != 2 || rest[0].Cursor != first[1].Cursor {
		t.Fatalf("Since(cursor) is not exclusive-after: %+v", rest)
	}
	// A cursor at the head yields nothing, not the whole buffer.
	tail, _ := l.Since(l.Latest(), 0)
	if len(tail) != 0 {
		t.Fatalf("want no events after the newest cursor, got %d", len(tail))
	}
}

func TestRingEvictsOldestFirst(t *testing.T) {
	l := New(4)
	for i := 0; i < 10; i++ {
		l.Emit(Event{Kind: KindInputQueued, Detail: string(rune('a' + i))})
	}
	got, _ := l.Since("", 0)
	if len(got) != 4 {
		t.Fatalf("ring grew past its capacity: %d events held", len(got))
	}
	// events 7..10 survive; 1..6 are gone
	if got[0].Detail != "g" || got[3].Detail != "j" {
		t.Fatalf("wrong events survived: %q..%q", got[0].Detail, got[3].Detail)
	}
}

func TestEvictedCursorDemandsResync(t *testing.T) {
	l := New(4)
	emitN(l, 2)
	stale, _ := l.Since("", 0)
	emitN(l, 10) // evicts everything the client saw

	got, resync := l.Since(stale[0].Cursor, 0)
	if !resync {
		t.Fatalf("an evicted cursor must resync rather than return a silent gap")
	}
	if len(got) == 0 {
		t.Fatalf("resync should still hand back what the buffer holds")
	}
}

func TestEmitNeverBlocksOnAFullSubscriber(t *testing.T) {
	l := New(64)
	sub := l.Subscribe() // never drained
	defer sub.Close()

	done := make(chan struct{})
	go func() {
		emitN(l, subscriberBuffer*4)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Emit blocked on a subscriber that stopped reading")
	}
	if !sub.Lagged() {
		t.Fatalf("a subscriber that dropped events must report itself lagged")
	}
}

func TestSubscriberReceivesLiveEvents(t *testing.T) {
	l := New(16)
	sub := l.Subscribe()
	defer sub.Close()
	l.Emit(Event{Kind: KindRunDispatched, RunID: "r-1"})
	select {
	case e := <-sub.Events():
		if e.RunID != "r-1" || e.Cursor == "" {
			t.Fatalf("subscriber got %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no event delivered to subscriber")
	}
}

func TestClosedSubscriptionStopsReceivingAndEmitStillWorks(t *testing.T) {
	l := New(16)
	sub := l.Subscribe()
	sub.Close()
	sub.Close() // idempotent
	l.Emit(Event{Kind: KindRunCompleted})
	select {
	case <-sub.Events():
		t.Fatalf("a closed subscription must not receive further events")
	default:
	}
}

func TestEmitNormalizesStatusAndTimestamp(t *testing.T) {
	l := New(4)
	l.Emit(Event{Kind: KindSignalReceived})
	got, _ := l.Since("", 0)
	if got[0].Status != StatusOK {
		t.Fatalf("status not defaulted: %q", got[0].Status)
	}
	if got[0].TS.IsZero() {
		t.Fatalf("timestamp not stamped")
	}
}

func TestObserversSeeEveryEvent(t *testing.T) {
	l := New(4)
	var seen []Event
	l.AddObserver(ObserverFunc(func(e Event) { seen = append(seen, e) }))
	emitN(l, 3)
	if len(seen) != 3 {
		t.Fatalf("observer saw %d of 3 events", len(seen))
	}
	if seen[0].Cursor == "" {
		t.Fatalf("observers must see the assigned cursor, got %+v", seen[0])
	}
}

func TestNilLogIsInert(t *testing.T) {
	var l *Log
	l.Emit(Event{Kind: KindInputQueued}) // must not panic
	if got, resync := l.Since("", 0); got != nil || resync {
		t.Fatalf("nil log returned %v/%v", got, resync)
	}
	if l.Subscribe() != nil {
		t.Fatalf("nil log handed out a subscription")
	}
	if l.Latest() != "" {
		t.Fatalf("nil log reported a cursor")
	}
}
