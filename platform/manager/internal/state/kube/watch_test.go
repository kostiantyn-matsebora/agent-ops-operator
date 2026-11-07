// Task 2.3: Watch resume ordering.
package kube

import (
	"context"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// recvEvent blocks for one event or fails the test after a generous
// timeout — envtest's own watch latency is normally well under a second,
// so a real gap means a bug, not a slow CI box.
func recvEvent(t *testing.T, events <-chan state.Event) state.Event {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event channel closed unexpectedly")
		}
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a watch event")
	}
	return state.Event{}
}

// TestWatchResumeOrdering is the spec's own feed-resume scenario: create,
// two updates, delete; resuming from the first update's own revision must
// observe the second update and the delete, in order, and nothing from
// before that point.
//
// Update's own two-call shape (store.go's Update: one apiserver write for
// spec+metadata, one for status) means a single logical Update can emit
// more than one raw watch event — so this does not assert "exactly one
// Put then one Delete", only that nothing OLDER than the resume point
// leaks through, and that the second update's content is observed before
// the delete.
func TestWatchResumeOrdering(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := fixtureRecord("conv-watch")
	rev0, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	update1 := rec
	update1.Title = "update-1"
	rev1, err := s.Update(ctx, state.Key(rec.Name), rev0, update1)
	if err != nil {
		t.Fatalf("Update 1: %v", err)
	}

	update2 := rec
	update2.Title = "update-2"
	if _, err := s.Update(ctx, state.Key(rec.Name), rev1, update2); err != nil {
		t.Fatalf("Update 2: %v", err)
	}

	if err := s.Delete(ctx, state.Key(rec.Name)); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	events, err := s.Watch(ctx, rev1)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	sawUpdate2 := false
	for attempt := 0; attempt < 10; attempt++ {
		ev := recvEvent(t, events)
		switch ev.Type {
		case state.EventPut:
			if ev.Record.Title == update1.Title {
				t.Fatalf("observed update-1's own content after resuming from its own revision: %+v", ev)
			}
			if ev.Record.Title == update2.Title {
				sawUpdate2 = true
			}
		case state.EventDelete:
			if !sawUpdate2 {
				t.Fatal("observed the delete before the second update's content")
			}
			if ev.Key != state.Key(rec.Name) {
				t.Fatalf("delete event key = %q, want %q", ev.Key, rec.Name)
			}
			return
		}
	}
	t.Fatal("never observed the delete event")
}

// TestWatchFromEmptyResumeStartsFromNow: an empty resume must never replay
// a change made before Watch was called, only changes made after.
func TestWatchFromEmptyResumeStartsFromNow(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pre := fixtureRecord("conv-watch-pre")
	if _, err := s.Create(ctx, state.Key(pre.Name), pre); err != nil {
		t.Fatalf("Create pre: %v", err)
	}

	events, err := s.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	post := fixtureRecord("conv-watch-post")
	if _, err := s.Create(ctx, state.Key(post.Name), post); err != nil {
		t.Fatalf("Create post: %v", err)
	}

	sawPost := false
	for attempt := 0; attempt < 10; attempt++ {
		ev := recvEvent(t, events)
		if ev.Key == state.Key(pre.Name) {
			t.Fatalf("observed a change from before Watch was called: %+v", ev)
		}
		if ev.Key == state.Key(post.Name) {
			sawPost = true
			break
		}
	}
	if !sawPost {
		t.Fatal("never observed the post-Watch create")
	}
}

// TestWatchObservesDeleteAfterDisconnectAndResume is the spec's own "no
// gap across a consumer disconnect" scenario: a consumer stops reading,
// changes happen, and resuming from its last kept point observes every
// one of them rather than only the first.
func TestWatchObservesDeleteAfterDisconnectAndResume(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := fixtureRecord("conv-watch-disconnect")
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate "disconnected": nothing reads the feed while this happens.
	if err := s.Delete(ctx, state.Key(rec.Name)); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	events, err := s.Watch(ctx, rev)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	ev := recvEvent(t, events)
	if ev.Type != state.EventDelete || ev.Key != state.Key(rec.Name) {
		t.Fatalf("resumed watch did not observe the delete made while disconnected: %+v", ev)
	}
}
