package conformance

// Bench reports the numbers task 2.5 asks for — allocations per
// Submit/Claim/Settle cycle, and bytes per task (a JSON-encoded size proxy,
// mirroring internal/state/conformance.Bench) — walked once per queue in
// commandbus.DefaultInventory, so `go test -bench=. -benchmem` prints every
// one without a backend writing its own loop.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Bench runs the submit/claim/settle cycle for every declared queue against
// a Backing open constructs once per sub-benchmark.
func Bench(b *testing.B, open func() Backing) {
	b.Helper()
	for _, d := range commandbus.DefaultInventory() {
		d := d
		name := string(d.Queue)
		if d.Kind != commandbus.KindDefault {
			name += "/" + string(d.Kind)
		}
		b.Run(name, func(b *testing.B) {
			benchDeclaration(b, open, d)
		})
	}
}

func benchDeclaration(b *testing.B, open func() Backing, d commandbus.Declaration) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), benchCycleTimeout)
	defer cancel()
	queue := sampleBenchQueue(d)
	bus := open().Bus()

	sample := commandbus.Task{Queue: queue, Kind: d.Kind, Key: "bench-key", Payload: []byte("payload")}
	taskJSON, err := json.Marshal(sample)
	if err != nil {
		b.Fatalf("marshal sample task: %v", err)
	}

	// A fresh tag every call: benchCycle's CommandID becomes the dedup key,
	// and a repeated one would hand AllocsPerRun's second call back the
	// first's already-Succeeded task — nothing left to Claim, so it would
	// block for the rest of ctx's timeout instead of measuring anything.
	var tag int
	allocs := testing.AllocsPerRun(1, func() {
		tag++
		benchCycle(b, ctx, bus, queue, d.Kind, fmt.Sprintf("warmup-%d", tag))
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCycle(b, ctx, bus, queue, d.Kind, fmt.Sprintf("bench-%d", i))
	}

	b.ReportMetric(float64(len(taskJSON)), "bytes/task(json-proxy)")
	b.ReportMetric(allocs, "allocs/submit+claim+settle")
}

// benchCycleTimeout bounds every Submit/Claim/Settle in one benchmark
// iteration. memory is declared uncapped — every task it has ever seen
// stays resident for as long as the Backing lives (package doc) — so a
// large -benchtime grows every scan this backend does from O(1) towards
// O(b.N) by the run's end. A context deadline turns that into a failed
// benchmark naming the slow call instead of an indefinite hang; running
// with a bounded -benchtime (e.g. -benchtime=200x) avoids tripping it.
const benchCycleTimeout = 30 * time.Second

// benchCycle submits, claims and settles one task under a fresh CommandID
// and Key, so neither dedup nor per-key serialization shapes the timing.
func benchCycle(b *testing.B, ctx context.Context, bus commandbus.Bus, queue commandbus.Queue, kind commandbus.Kind, tag string) {
	b.Helper()
	task, err := bus.Submit(ctx, commandbus.SubmitRequest{
		Queue: queue, Kind: kind,
		CommandID: commandbus.CommandID("cmd-" + tag), Key: commandbus.Key("key-" + tag),
		Payload: []byte("payload"),
	})
	if err != nil {
		b.Fatalf("Submit: %v", err)
	}
	claimed, err := bus.Claim(ctx, queue, "bench-consumer")
	if err != nil {
		b.Fatalf("Claim: %v", err)
	}
	if err := bus.Settle(ctx, claimed.UID, claimed.Lease.Token); err != nil {
		b.Fatalf("Settle: %v", err)
	}
	_ = task
}

// sampleBenchQueue mirrors sampleQueue without a *testing.T to fail
// through — a benchmark walking an inventory entry this package does not
// know how to drive is a programming error caught by the conformance suite
// already, not something a benchmark run needs to re-report.
func sampleBenchQueue(d commandbus.Declaration) commandbus.Queue {
	if d.Queue == commandbus.OutFamily {
		return commandbus.OutQueue("bench")
	}
	return d.Queue
}
