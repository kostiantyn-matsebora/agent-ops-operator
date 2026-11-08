// Task 3.4: the listing with items and the put event carrying them, free from
// the object under kube.
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A listing with items equals the combined read: every record comes back with
// exactly the runs GetWithItems returns for it, in append order.
func TestListingWithItemsEqualsTheCombinedRead(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	for i, runs := range []int{0, 1, 3} {
		name := fmt.Sprintf("conv-items-%d", i)
		if _, err := s.Conversations.Create(ctx, state.Key(name), fixtureRecord(name)); err != nil {
			t.Fatal(err)
		}
		for r := 0; r < runs; r++ {
			if err := s.Runs.Append(ctx, state.Key(name), state.Run{RunID: fmt.Sprintf("%s-run-%d", name, r), Status: "succeeded"}); err != nil {
				t.Fatal(err)
			}
		}
	}

	listed, err := s.Runs.ListWithItems(ctx, state.ByAll, "")
	if err != nil {
		t.Fatalf("ListWithItems: %v", err)
	}
	all, err := s.Conversations.List(ctx, state.ByAll, "")
	if err != nil || len(listed) != len(all) || len(all) != 3 {
		t.Fatalf("listing holds %d records, the whole set %d (%v); want 3", len(listed), len(all), err)
	}
	for _, w := range listed {
		rec, rev, items, err := s.Runs.GetWithItems(ctx, state.Key(w.Record.Name))
		if err != nil {
			t.Fatal(err)
		}
		if w.Revision != rev || mustJSON(t, w.Record) != mustJSON(t, rec) || mustJSON(t, w.Items) != mustJSON(t, items) {
			t.Fatalf("%s: listing %+v differs from the combined read %+v / %v", w.Record.Name, w, rec, items)
		}
	}

	byPhase, err := s.Runs.ListWithItems(ctx, state.ByPhase, "no-such-phase")
	if err != nil || len(byPhase) != 0 {
		t.Fatalf("a lookup nothing matches = %v, %v; want empty", byPhase, err)
	}
	undeclared := state.NewIndex("undeclared", func(*state.ConversationRecord) (string, bool) { return "", true })
	if _, err := s.Runs.ListWithItems(ctx, undeclared, ""); err == nil {
		t.Fatal("an undeclared index must be refused")
	}
}

// A run appended and the record then updated: the put event the update
// produces carries the run.
func TestAPutAfterAnAppendCarriesTheRun(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := fixtureRecord("conv-put-items")
	rev, err := s.Conversations.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.Runs.WatchWithItems(ctx, rev)
	if err != nil {
		t.Fatalf("WatchWithItems: %v", err)
	}
	if err := s.Runs.Append(ctx, state.Key(rec.Name), state.Run{RunID: "run-1", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	_, cur, _ := s.Conversations.Get(ctx, state.Key(rec.Name))
	rec.Title = "after the append"
	if _, err := s.Conversations.Update(ctx, state.Key(rec.Name), cur, rec); err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 12; attempt++ {
		ev := recvEvent(t, events)
		if ev.Type != state.EventPut || ev.Record.Record.Title != rec.Title {
			continue
		}
		if len(ev.Record.Items) != 1 || ev.Record.Items[0].RunID != "run-1" || ev.Record.Revision != ev.Revision {
			t.Fatalf("the put after the update carries %+v, want the appended run at its revision", ev.Record)
		}
		return
	}
	t.Fatal("never observed the put the update produced")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
