package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const benchInputID = "input-1"

// Bench reports the tier-1 numbers design.md's D-8/D-10 name for comparing
// backends: bytes written per run, allocations per write, and records
// resident after a listing — at design.md's own measured scale (161
// conversations, 108 closed, rounded here to 200). It runs against any
// backend newStore constructs: nothing here imports internal/state/kube or
// assumes a particular storage engine.
//
// The bytes figure is a PROXY, not a wire-accurate count. The contract
// gives Store no byte-count callback (design D-2's provenance table has no
// such row), so this reports the JSON-encoded size of a representative Run
// struct — a lower bound true of every backend, never the authoritative
// figure for what actually crosses the wire for all of them.
func Bench(b *testing.B, newStore func() state.Store) {
	b.Helper()
	ctx := context.Background()
	store := newStore()

	key := state.Key("bench-conversation")
	if _, err := store.Create(ctx, key, benchRecord()); err != nil {
		b.Fatalf("Create: %v", err)
	}

	// bytes written per run (proxy).
	run := benchRun()
	runJSON, err := json.Marshal(run)
	if err != nil {
		b.Fatalf("marshal Run: %v", err)
	}

	// allocations per write: one isolated Update call, kept apart from the
	// timed AppendRun loop below so the number reflects a single CAS write
	// and nothing else.
	_, rev, err := store.Get(ctx, key)
	if err != nil {
		b.Fatalf("Get: %v", err)
	}
	updated := benchRecord()
	// AllocsPerRun(1, f) calls f twice (one untimed warm-up, one measured —
	// see its doc), so rev is advanced after each call rather than reused
	// stale, or the second call would lose its own CAS race against the
	// first.
	allocs := testing.AllocsPerRun(1, func() {
		newRev, err := store.Update(ctx, key, rev, updated)
		if err != nil {
			b.Fatalf("Update: %v", err)
		}
		rev = newRev
	})

	// records resident after a listing, at design.md's measured scale.
	const residentScale = 200
	const residentClosedFraction = 108.0 / 161.0 // design.md's own ratio
	for i := 0; i < residentScale; i++ {
		rk := state.Key(fmt.Sprintf("bench-resident-%d", i))
		rr := benchRecord()
		rr.Name = string(rk)
		if float64(i)/float64(residentScale) < residentClosedFraction {
			rr.Phase = "Closed"
		} else {
			rr.Phase = "Working"
		}
		if _, err := store.Create(ctx, rk, rr); err != nil {
			b.Fatalf("Create resident %d: %v", i, err)
		}
	}
	activeListed, err := store.List(ctx, state.ByPhase, state.ListKey{Phase: "Working"})
	if err != nil {
		b.Fatalf("List: %v", err)
	}

	// the timed loop: AppendRun's own write cost per run, with the
	// b.N-amortised allocs/op the harness already knows how to print
	// beside ns/op layered on top via b.ReportAllocs.
	//
	// ResetTimer clears testing.B's own "extra" metrics map (its own
	// source: ResetTimer calls clear(b.extra)), so every ReportMetric call
	// above would be wiped by it — they are made AFTER ResetTimer and the
	// loop instead, once per full invocation of this function. The
	// benchmark harness calls Bench several times with a growing b.N to
	// calibrate; only the last invocation's numbers are the ones printed,
	// and each invocation's are self-consistent since store is freshly
	// built at the top of this function every time.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.AppendRun(ctx, key, run); err != nil {
			b.Fatalf("AppendRun: %v", err)
		}
	}

	b.ReportMetric(float64(len(runJSON)), "bytes/run(json-proxy)")
	b.ReportMetric(allocs, "allocs/write")
	b.ReportMetric(float64(len(activeListed)), "resident_records(phase=Working)")
}

func benchRecord() state.ConversationRecord {
	now := time.Now()
	return state.ConversationRecord{
		Name:        "bench-conversation",
		ProfileRef:  "profile-default",
		PipelineRef: "bench-pipeline",
		Phase:       "Working",
		Title:       "a representative conversation title, not empty",
		Inputs: []state.InputItem{
			{ID: benchInputID, Type: "task", Payload: "a representative queued payload", ReceivedAt: now},
		},
		LastActivity: &now,
	}
}

func benchRun() state.Run {
	now := time.Now()
	return state.Run{
		RunID:      "bench-run-1",
		JobKind:    "agent",
		Status:     "Succeeded",
		StartedAt:  &now,
		FinishedAt: &now,
		InputIDs:   []string{benchInputID},
		Inputs: []state.RecordedInput{
			{ID: benchInputID, Type: "task", Text: "a representative recorded input body", Surface: "telegram"},
		},
	}
}
