package conformance

import "testing"

// TestMemStorePassesConformanceSuite proves this package's own scenarios
// are internally consistent — no scenario here is impossible to satisfy,
// and no two scenarios contradict each other — against a real, if minimal,
// Store double, before the kube backend exists (tasks.md 3.1/3.2).
func TestMemStorePassesConformanceSuite(t *testing.T) {
	Run(t, newMemStore)
}

// TestBenchReportsTierOneMetrics runs Bench under go test (not -bench), so
// its code is exercised and the three metrics are asserted present.
func TestBenchReportsTierOneMetrics(t *testing.T) {
	res := testing.Benchmark(func(b *testing.B) { Bench(b, newMemStore) })
	if res.N == 0 {
		t.Fatal("Bench failed or did not run")
	}
	for _, unit := range []string{"bytes/run(json-proxy)", "allocs/write", "resident_records(phase=Working)"} {
		if _, ok := res.Extra[unit]; !ok {
			t.Errorf("Bench did not report %q", unit)
		}
	}
}

// BenchmarkMemStore proves Bench runs against any backend and prints the
// three tier-1 numbers design.md names (tasks.md 3.2's verify line).
func BenchmarkMemStore(b *testing.B) {
	Bench(b, newMemStore)
}
