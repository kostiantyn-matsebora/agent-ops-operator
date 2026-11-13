package memory_test

import (
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/conformance"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// openBacking adapts memory.New to conformance.Backing: *memory.Backing
// already implements Bus and Advance, so no wrapper type is needed, only
// the narrowing return type.
func openBacking() conformance.Backing { return memory.New() }

func TestConformance(t *testing.T) {
	conformance.Run(t, openBacking)
}

func BenchmarkConformance(b *testing.B) {
	conformance.Bench(b, openBacking)
}
