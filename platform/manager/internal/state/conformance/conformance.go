// Package conformance is the one suite every state backend must pass to
// exist (design D-1 of openspec/changes/state-store-generic-contract/): a
// handful of generic engines — Run, RunExpiring, RunAppended, RunBlobs — each
// written once over the capability interfaces of internal/state, and
// RunStores, the entry point that drives all four from the closed inventory,
// state.Stores, so a kind of state cannot be left out of a backend's test.
//
// A kind's own content is a Fixture (fixture.go): what the generic engine
// needs to build, edit and read a record of that kind without the engine
// ever naming one of its fields. fixtures.go holds the one Fixture per field
// of state.Stores, written once here and reused by every backend.
//
// open (the function every Run* takes) MUST return a fresh, empty, isolated
// capability on every call: Run relies on that for scope isolation between
// scenarios, calling it once per leaf scenario that writes. A scenario
// needing continuity across what would be a process restart in production
// (a feed resumed after a disconnect) calls open once and keeps the same
// value for its whole body, since a resumable feed and a fresh backing store
// are different things.
package conformance

import (
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// mustRecvEvent reads the next event off events, failing the test if none
// arrives within timeout.
func mustRecvEvent[T any](t *testing.T, events <-chan state.Event[T], timeout time.Duration) state.Event[T] {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatalf("event channel closed before an event arrived")
		}
		return ev
	case <-time.After(timeout):
		t.Fatalf("no event within %s", timeout)
	}
	return state.Event[T]{}
}

// mustRecvEventMatching drains events, within timeout, until one satisfies
// want, and returns it.
//
// A backend may land one logical Create/Update as more than one underlying
// write — the kube backend's Update does exactly this (one apiserver call for
// spec+metadata, one for status) — and each is its own real, correctly
// ordered notification. A scenario that cares about a write's FINAL state
// looks for it among what the write actually produced rather than assuming
// the very first notification already is it.
func mustRecvEventMatching[T any](t *testing.T, events <-chan state.Event[T], timeout time.Duration, want func(state.Event[T]) bool) state.Event[T] {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed before a matching event arrived")
			}
			if want(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("no matching event within %s", timeout)
		}
	}
}

// drainQuiet collects every event delivered on events until quiet passes with
// nothing new arriving.
func drainQuiet[T any](events <-chan state.Event[T], quiet time.Duration) []state.Event[T] {
	var got []state.Event[T]
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-time.After(quiet):
			return got
		}
	}
}

// sameKeySet reports whether got and want name the same keys, order
// irrelevant, duplicates included.
func sameKeySet(got, want []state.Key) bool {
	if len(got) != len(want) {
		return false
	}
	count := map[state.Key]int{}
	for _, k := range got {
		count[k]++
	}
	for _, k := range want {
		count[k]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	count := map[string]int{}
	for _, s := range got {
		count[s]++
	}
	for _, s := range want {
		count[s]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}
