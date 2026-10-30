package conformance

import "testing"

// TestMemStorePassesConformanceSuite proves this package's own scenarios
// are internally consistent — no scenario here is impossible to satisfy,
// and no two scenarios contradict each other — against a real, if minimal,
// Store double, before the kube backend exists (tasks.md 3.1/3.2).
func TestMemStorePassesConformanceSuite(t *testing.T) {
	Run(t, newMemStore)
}

// BenchmarkMemStore proves Bench runs against any backend and prints the
// three tier-1 numbers design.md names (tasks.md 3.2's verify line).
func BenchmarkMemStore(b *testing.B) {
	Bench(b, newMemStore)
}
