package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A relist pass that reaches its batch bound inside a run of events sharing
// one timestamp must still, within a bounded number of later passes, emit
// every event of the run. Kubernetes stamps events to the whole second, so a
// tie at the clip point is the ordinary case, not an edge.
//
// Two ways to get this wrong, one row each:
//
//   - a read position left AT the shared second makes the next pass skip the
//     tied remainder as already seen — a permanent, unreported loss;
//   - a read position held BEFORE a run longer than the bound makes every pass
//     re-examine the same bound's worth and never reach the rest.
//
// The contract is pinned, not the mechanism: where a pass clips and how many
// it posts are the adapter's choice. That nothing is lost and that the
// remainder is reached in a bounded number of passes are not.
func TestRelistBatchBoundNeverLosesATiedRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		// distinct events one second apart, then a run sharing the last second
		distinct, run int
	}{
		{"one event ties the clip point", maxBatchPerSrc - 1, 2},
		{"a tied run longer than the bound", 0, 2*maxBatchPerSrc + 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertTiedRunNeverLost(t, tc.distinct, tc.run)
		})
	}
}

// assertTiedRunNeverLost is TestRelistBatchBoundNeverLosesATiedRun's body,
// extracted to a named top-level function — a t.Run closure does not lower
// Sonar's go:S3776 cognitive-complexity count, which charges the enclosing
// function instead (.claude/rules/gotchas.md).
func assertTiedRunNeverLost(t *testing.T, distinct, run int) {
	t.Helper()
	const passes = 3
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))
	// the emit cap is not under test here
	a.cap = newEmitCap(passes * (distinct + run))
	base := mustTime(t, "2026-08-08T12:00:00Z")
	a.sources["src"].cursor = base

	var events []Event
	name := func(k int) string { return fmt.Sprintf("pod-%03d", k) }
	for k := 1; k <= distinct; k++ {
		e := evt("Warning", "prod", "Pod", name(k), "BackOff")
		e.LastTimestamp = base.Add(time.Duration(k) * time.Second).Format(time.RFC3339)
		events = append(events, e)
	}
	tied := base.Add(time.Duration(distinct+1) * time.Second).Format(time.RFC3339)
	for k := distinct + 1; k <= distinct+run; k++ {
		e := evt("Warning", "prod", "Pod", name(k), "BackOff")
		e.LastTimestamp = tied
		events = append(events, e)
	}

	for i := 0; i < passes; i++ {
		a.deliver(context.Background(), events, pass{list: true})
	}

	seen := map[string]bool{}
	for _, s := range mgr.signalsFor("src") {
		seen[s.Labels["name"]] = true
	}
	for k := 1; k <= len(events); k++ {
		if !seen[name(k)] {
			t.Fatalf("%s was never emitted in %d relist passes — the bound lost it (%d of %d emitted)",
				name(k), passes, len(seen), len(events))
		}
	}
}
