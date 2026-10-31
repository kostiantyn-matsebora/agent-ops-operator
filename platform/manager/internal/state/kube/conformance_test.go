package kube

import (
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/conformance"
)

// conformanceSweepInterval is short enough that the suite's expiry scenario
// (an expired record becomes a delete on the feed, "eventually" per the
// contract) resolves well inside a single test's timeout, rather than
// DefaultSweepInterval's production-scaled 30s.
const conformanceSweepInterval = 50 * time.Millisecond

// TestKubeBackendPassesConformanceSuite is task 3.3: the kube backend runs
// the one suite every state.Store backend must pass to exist (design D-10).
// Every call to newStore gets its own fresh namespace — this package's own
// "one install per namespace" scope boundary, reused here for the isolation
// the suite requires between scenarios (conformance.go's own doc on Run).
//
// Each store also gets its own expiry sweep started (Store.Run, expiry.go),
// tied to this test's own context — nothing else in this package starts it
// unsolicited (NewStore's own doc), and the generic suite has no reason to
// know kube carries a sweep goroutine at all, so starting it is this
// wiring's job alone.
func TestKubeBackendPassesConformanceSuite(t *testing.T) {
	conformance.Run(t, func() state.Store {
		_, s := newTestNamespaceWithSweepInterval(t, conformanceSweepInterval)
		go s.Run(t.Context())
		return s
	})
}
