// Package conformance is the one suite every commandbus.Bus backend must
// pass to exist (design D-14 of openspec/changes/async-command-bus/,
// mirroring internal/state/conformance's own role for the state store): a
// handful of scenarios, each written once against the contract, walked
// once per entry of commandbus.DefaultInventory so a declared queue cannot
// be left untested — and, if the inventory ever grows in a shape this
// package does not know how to drive, Run fails loudly naming the queue
// rather than silently skipping it (sampleQueue below).
package conformance

import (
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// Backing is a fresh, isolated instance of a backend's durable state, kept
// alive for one scenario's whole body. Bus returns an independent handle
// onto it — calling it more than once is how the suite proves a claim
// completes through a handle other than the one that claimed it (design
// D-2), without assuming how many connections or goroutines a real backend
// needs to do that. Advance fast-forwards the backing's clock, so a
// scenario can skip past a backoff, a lease or a TTL deterministically.
type Backing interface {
	Bus() commandbus.Bus
	Advance(d time.Duration)
	// LeaseDuration reports how long this backing's Claim leases run before
	// a lapse — bus-wide, not per queue (design D-2 never varies it by
	// queue). The lease-lapse scenarios advance exactly past it rather than
	// guessing a number that might also blow through an unrelated queue's
	// TTL.
	LeaseDuration() time.Duration
	// Entries reads back every entry the backing's log holds, in append
	// order, as stored. It is how the append-only scenarios see that nothing
	// was written twice or rewritten (design D-2): a backend that updated an
	// entry in place would show it here as a changed or repeated one.
	Entries() []entrylog.Entry
	// Compact runs whatever removal of consumed entries the backend defers: the
	// entries of a task that succeeded are removed from the log, like a regular
	// queue's, and a backend may wait a short grace before doing so. A backend
	// that removes them inside Settle does nothing here.
	Compact()
}

// scenarioTimeout bounds every blocking call (chiefly Claim) a scenario
// makes — long enough for a slow CI runner, short enough that a genuine
// deadlock fails the test instead of hanging the suite.
const scenarioTimeout = 5 * time.Second

// Run drives every scenario in this package against open, once per entry
// of commandbus.DefaultInventory(). open must return a fresh, independent
// Backing on every call — scenarios rely on that for isolation between
// each other, and open a second one themselves where a scenario needs a
// second, unrelated queue.
func Run(t *testing.T, open func() Backing) {
	t.Helper()
	for _, d := range commandbus.DefaultInventory() {
		d := d
		name := string(d.Queue)
		if d.Kind != commandbus.KindDefault {
			name += "/" + string(d.Kind)
		}
		t.Run(name, func(t *testing.T) {
			queue := sampleQueue(t, d)
			for _, sc := range declarationScenarios {
				sc := sc
				t.Run(sc.name, func(t *testing.T) {
					sc.run(t, open, d, queue)
				})
			}
		})
	}
	t.Run("gap-free resumable feed", func(t *testing.T) { testGapFreeResumableFeed(t, open) })
}

// sampleQueue names the concrete runtime Queue the suite submits to for
// Declaration d. "out" is commandbus's own sentinel for the parametrized
// out.<channel> family, one queue per Channel resource (see commandbus.OutQueue); every other queue
// name is used literally. A Declaration this switch does not recognize is
// a queue genuinely left unwired — named in the failure, not skipped.
func sampleQueue(t *testing.T, d commandbus.Declaration) commandbus.Queue {
	t.Helper()
	switch d.Queue {
	case commandbus.OutFamily:
		return commandbus.OutQueue("conformance")
	case "conv", "in.signal":
		return d.Queue
	default:
		t.Fatalf("conformance: DefaultInventory declares queue %q kind %q, which sampleQueue does not know how to drive — add it here", d.Queue, d.Kind)
		return ""
	}
}

// declarationScenario is one scenario the suite runs once per declared
// queue.
type declarationScenario struct {
	name string
	run  func(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue)
}

var declarationScenarios = []declarationScenario{
	{"durable accept", testDurableAccept},
	{"idempotent submit", testIdempotentSubmit},
	{"dedup across every state", testDedupAcrossEveryState},
	{"park then release", testParkThenRelease},
	{"FIFO with a retry keeping its place", testFIFOWithRetryKeepingPlace},
	{"serialization per key under parallel consumers", testSerializationPerKeyUnderParallelConsumers},
	{"lease completion through a second handle", testLeaseCompletionThroughASecondHandle},
	{"running lists held leases through a second handle", testRunningListsHeldLeases},
	{"lease-lapse redelivery", testLeaseLapseRedelivery},
	{"lease-lapse expires under a short TTL", testLeaseLapseExpiresUnderAShortTTL},
	{"bounded retry into dead", testBoundedRetryIntoDead},
	{"TTL expiry counts as settled", testTTLExpiryCountsAsSettled},
	{"dead-letter listing", testDeadLetterListing},
	{"per-queue statistics", testPerQueueStatistics},
	{"per-invocation in-flight cap", testPerInvocationInFlightCap},
	{"running resumes after a command id", testRunningResumesAfterACommandID},
	{"nothing is rewritten", testNothingIsRewritten},
	{"a submit is one write under a racing consumer", testSubmitIsOneWriteUnderARacingConsumer},
	{"state folds from the entries", testStateFoldsFromEntries},
	{"a lapsed lease redelivers with no write", testLapsedLeaseRedeliversWithNoWrite},
}
