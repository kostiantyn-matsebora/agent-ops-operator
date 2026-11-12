package conformance

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Scenarios of the retained feed (design D-16): an ordered log read by cursor
// with no claim, lease or acknowledgement, trimmed by age and count and never
// by delivery. Walked for every backend.

var feedScenarios = []struct {
	name string
	run  func(t *testing.T, open func() Backing)
}{
	{"replays after a cursor in log order", testFeedReplaysAfterCursor},
	{"an unknown or empty cursor replays the retained log", testFeedUnknownCursor},
	{"delivery removes nothing", testFeedDeliveryRemovesNothing},
	{"two followers both receive everything", testFeedTwoFollowers},
	{"a follower that never reads strands nothing", testFeedStalledFollower},
	{"a repeat inside the dedup window appends once", testFeedDedupWindow},
	{"retention trims by count", testFeedTrimsByCount},
	{"retention trims by age", testFeedTrimsByAge},
	{"an unknown feed and an empty id are refused", testFeedRefusals},
}

func runFeed(t *testing.T, open func() Backing) {
	for _, sc := range feedScenarios {
		sc := sc
		t.Run("retained feed/"+sc.name, func(t *testing.T) { sc.run(t, open) })
	}
}

const testFeed = commandbus.Queue("feed.conformance")

func publishN(t *testing.T, bus commandbus.Bus, prefix string, n int) []commandbus.CommandID {
	t.Helper()
	var ids []commandbus.CommandID
	for i := 0; i < n; i++ {
		id := commandbus.CommandID(fmt.Sprintf("%s-%d", prefix, i))
		if _, created, err := bus.Publish(context.Background(), testFeed, id, []byte("p-"+string(id))); err != nil || !created {
			t.Fatalf("Publish %s: created=%v err=%v", id, created, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// collect reads exactly n entries from a follow, failing on a timeout.
func collect(t *testing.T, ch <-chan commandbus.FeedEntry, n int) []commandbus.CommandID {
	t.Helper()
	var got []commandbus.CommandID
	timeout := time.After(scenarioTimeout)
	for len(got) < n {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatalf("follow closed after %d of %d entries: %v", len(got), n, got)
			}
			got = append(got, e.ID)
		case <-timeout:
			t.Fatalf("received %d of %d entries: %v", len(got), n, got)
		}
	}
	return got
}

// quiet asserts nothing more arrives for a short while.
func quiet(t *testing.T, ch <-chan commandbus.FeedEntry) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("unexpected entry %q", e.ID)
	case <-time.After(300 * time.Millisecond):
	}
}

func sameIDs(t *testing.T, got, want []commandbus.CommandID) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func testFeedReplaysAfterCursor(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	ids := publishN(t, bus, "e", 5)
	ch, err := bus.Follow(ctx, testFeed, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, collect(t, ch, 3), ids[2:])
	// then new entries are pushed, in order
	more := publishN(t, bus, "live", 2)
	sameIDs(t, collect(t, ch, 2), more)
}

func testFeedUnknownCursor(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	ids := publishN(t, bus, "e", 3)
	for _, cursor := range []commandbus.CommandID{"", "never-seen"} {
		ch, err := bus.Follow(ctx, testFeed, cursor)
		if err != nil {
			t.Fatal(err)
		}
		sameIDs(t, collect(t, ch, 3), ids)
	}
}

func testFeedDeliveryRemovesNothing(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	ids := publishN(t, bus, "e", 3)
	for i := 0; i < 2; i++ {
		fctx, stop := context.WithCancel(ctx)
		ch, err := bus.Follow(fctx, testFeed, "")
		if err != nil {
			t.Fatal(err)
		}
		sameIDs(t, collect(t, ch, 3), ids)
		stop()
	}
}

func testFeedTwoFollowers(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	first, second := b.Bus(), b.Bus()
	a, err := first.Follow(ctx, testFeed, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := second.Follow(ctx, testFeed, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := publishN(t, first, "e", 4)
	sameIDs(t, collect(t, a, 4), ids)
	sameIDs(t, collect(t, c, 4), ids)
}

func testFeedStalledFollower(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	if _, err := bus.Follow(ctx, testFeed, ""); err != nil { // never read
		t.Fatal(err)
	}
	ids := publishN(t, bus, "e", 5) // the producer is not blocked by it
	live, err := bus.Follow(ctx, testFeed, "")
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, collect(t, live, 5), ids)
}

func testFeedDedupWindow(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()
	first, created, err := bus.Publish(ctx, testFeed, "same", []byte("1"))
	if err != nil || !created {
		t.Fatalf("first publish: created=%v err=%v", created, err)
	}
	again, created, err := bus.Publish(ctx, testFeed, "same", []byte("2"))
	if err != nil || created || string(again.Payload) != "1" {
		t.Fatalf("a repeat inside the window must return the first entry: created=%v payload=%q err=%v", created, again.Payload, err)
	}
	b.Advance(commandbus.DefaultFeedDedup + time.Second)
	if _, created, err = bus.Publish(ctx, testFeed, "same", []byte("3")); err != nil || !created {
		t.Fatalf("a repeat past the window appends again: created=%v err=%v", created, err)
	}
	_ = first
}

func testFeedTrimsByCount(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	b.SetFeedPolicy(commandbus.FeedPolicy{MaxAge: time.Hour, MaxEntries: 3, DedupWindow: time.Minute})
	bus := b.Bus()
	ids := publishN(t, bus, "e", 5)
	ch, err := bus.Follow(ctx, testFeed, "")
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, collect(t, ch, 3), ids[2:])
	quiet(t, ch)
}

func testFeedTrimsByAge(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	const maxAge = 2 * time.Second
	b.SetFeedPolicy(commandbus.FeedPolicy{MaxAge: maxAge, MaxEntries: 100, DedupWindow: time.Second})
	bus := b.Bus()
	publishN(t, bus, "old", 2)
	b.AgeFeed(maxAge + time.Second)
	fresh := publishN(t, bus, "new", 1)
	deadline := time.Now().Add(20 * time.Second)
	for {
		fctx, stop := context.WithCancel(ctx)
		ch, err := bus.Follow(fctx, testFeed, "")
		if err != nil {
			t.Fatal(err)
		}
		var got []commandbus.CommandID
	read:
		for {
			select {
			case e, ok := <-ch:
				if !ok {
					break read
				}
				got = append(got, e.ID)
			case <-time.After(400 * time.Millisecond):
				break read
			}
		}
		stop()
		if fmt.Sprint(got) == fmt.Sprint(fresh) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("entries older than the max age are still retained: %v, want only %v", got, fresh)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func testFeedRefusals(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	if _, _, err := bus.Publish(ctx, "out.console", "x", nil); !errors.Is(err, commandbus.ErrUnknownQueue) {
		t.Fatalf("a task queue is not a feed: %v", err)
	}
	if _, err := bus.Follow(ctx, "conv", ""); !errors.Is(err, commandbus.ErrUnknownQueue) {
		t.Fatalf("following a task queue: %v", err)
	}
	if _, _, err := bus.Publish(ctx, testFeed, "", nil); !errors.Is(err, commandbus.ErrInvalidRequest) {
		t.Fatalf("an empty id: %v", err)
	}
}
