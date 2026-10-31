package kube

import (
	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
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

// TestKubeSignalStorePassesConformanceSuite runs the SignalStore scenarios
// against the Signal custom resources. Each store gets its own namespace, the
// SignalSources the scenarios record a cooldown on (the window lives on the
// source), and its own expiry sweep.
func TestKubeSignalStorePassesConformanceSuite(t *testing.T) {
	conformance.RunSignals(t, func() state.SignalStore {
		ns, _ := newTestNamespace(t)
		for _, name := range conformance.SignalSources() {
			src := &agentopsv1alpha1.SignalSource{}
			src.Name, src.Namespace = name, ns
			src.Spec.Adapter = "cron"
			if err := wwClient.Create(t.Context(), src); err != nil {
				t.Fatalf("create source %q: %v", name, err)
			}
		}
		s := NewSignalStoreWithSweepInterval(wwClient, ns, conformanceSweepInterval)
		go s.Run(t.Context())
		return s
	})
}
