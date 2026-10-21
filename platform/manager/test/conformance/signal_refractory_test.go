//go:build conformance

package conformance

import (
	"fmt"
	"testing"
	"time"
)

// The post-emit refractory window — the signal adapter contract's one named
// allowance beside the no-dedup rule (2.2.0). ha and k8s-events declare it on
// their dwell queues. Its bound the fake manager can observe: the window
// suppresses only re-posts of the id that emitted, never a distinct id.
//
// Each test dwells two conditions under a 2s rule, so the window is the
// one-minute floor. Condition A emits and opens its window. Inside it, A
// recurs and a distinct condition B arises. B must be posted and A's chatter
// must not be.

// refractoryFloor is the shortest window either adapter may declare: the
// contract's one-minute floor. Everything below must land inside it.
const refractoryFloor = time.Minute

// assertRefractoryNeverSuppressesADistinctID drives the sequence above.
// recurA and raiseB each deliver one condition's upstream evidence, enough
// for the adapter's dwell to verify it as still recurring at the close.
func assertRefractoryNeverSuppressesADistinctID(t *testing.T, mgr *FakeManager, source string, recurA, raiseB func()) {
	t.Helper()
	recurA()
	waitFor(t, "condition A to dwell and emit", 60*time.Second, func() bool { return len(accepted(mgr, source)) >= 1 })
	windowOpened := time.Now()
	first := accepted(mgr, source)[0]
	assertNormalized(t, first, source)
	idA, _ := first.Signals[0]["initiationId"].(string)

	recurA() // chatter inside A's window
	raiseB()

	var idB string
	waitFor(t, "distinct condition B to be posted while A's window is open", 45*time.Second, func() bool {
		for _, p := range accepted(mgr, source)[1:] {
			for _, s := range p.Signals {
				if id, _ := s["initiationId"].(string); id != idA {
					idB = id
					return true
				}
			}
		}
		return false
	})
	if elapsed := time.Since(windowOpened); elapsed >= refractoryFloor {
		t.Fatalf("B arrived %s after A emitted, past the window floor — this run proves nothing about the window", elapsed)
	}
	// A's chatter dwelled no longer than B and was delivered first, so had
	// the window not held it back it would have been decided in the same
	// flush as B, or an earlier one.
	for _, p := range accepted(mgr, source)[1:] {
		for _, s := range p.Signals {
			if s["initiationId"] == idA {
				t.Fatalf("A's chatter inside its own window was re-posted under %q (B is %q)", idA, idB)
			}
		}
	}
}

// ---- signal-ha -------------------------------------------------------------

func TestSignalHARefractoryWindowNeverSuppressesADistinctID(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	ha := NewFakeHA(t, "ha-long-lived-token")
	cfg := `{"endpoint":"` + ha.Endpoint() + `","backfill":false,"rules":[{"for":"2s"}]}`
	haStartAdapterAndAssertAuthenticatedSubscription(t, mgr, ha, cfg)

	ts := float64(time.Now().Unix())
	// recur pushes two occurrences of one record a moment apart, each with a
	// later timestamp, so the dwell's recurrence rung sees it still live.
	recur := func(logger, file string, line int) func() {
		return func() {
			for i := 0; i < 2; i++ {
				ts++
				rec := map[string]any{
					"name": logger, "level": "ERROR",
					"message":   []any{"Error communicating with " + logger},
					"source":    []any{file, line},
					"timestamp": ts, "first_occurred": ts, "count": 1,
				}
				waitFor(t, "the log event to reach a session", 10*time.Second, func() bool { return ha.PushLog(rec) >= 1 })
				time.Sleep(300 * time.Millisecond)
			}
		}
	}
	assertRefractoryNeverSuppressesADistinctID(t, mgr, "ha",
		recur("homeassistant.components.zwave_js", "components/zwave_js/__init__.py", 212),
		recur("homeassistant.components.mqtt", "components/mqtt/client.py", 88))
}

// ---- signal-k8s-events -----------------------------------------------------

func TestSignalK8sEventsRefractoryWindowNeverSuppressesADistinctID(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	api := NewFakeAPIServer(t)
	k8sEventsStartAdapterAndAssertWatch(t, mgr, api, `{"namespaces":["apps"],"rules":[{"for":"2s"}]}`)

	// A PersistentVolumeClaim carries no health predicate, so the dwell
	// verifies it by recurrence alone — no object cache involved.
	seq := 0
	recur := func(claim string) func() {
		return func() {
			for i := 0; i < 2; i++ {
				seq++
				now := time.Now().UTC().Format(time.RFC3339)
				ev := map[string]any{
					"apiVersion": "v1", "kind": "Event",
					"metadata":       map[string]any{"name": fmt.Sprintf("%s.%d", claim, seq), "namespace": "apps", "creationTimestamp": now},
					"involvedObject": map[string]any{"kind": "PersistentVolumeClaim", "name": claim, "namespace": "apps"},
					"reason":         "ProvisioningFailed", "type": "Warning", "count": seq,
					"message":        "failed to provision volume for claim " + claim,
					"firstTimestamp": now, "lastTimestamp": now,
				}
				api.Push("/api/v1/namespaces/apps/events", "ADDED", ev)
				time.Sleep(300 * time.Millisecond)
			}
		}
	}
	assertRefractoryNeverSuppressesADistinctID(t, mgr, "events", recur("data-a"), recur("data-b"))
}
