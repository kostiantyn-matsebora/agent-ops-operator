package main

import (
	"context"
	"testing"
)

// ev builds an Event about one involved object.
func ev(ns, kind, name string) *Event {
	e := &Event{Reason: "FailedScheduling", Type: "Warning"}
	e.Metadata.Namespace = ns
	e.InvolvedObject.Namespace = ns
	e.InvolvedObject.Kind = kind
	e.InvolvedObject.Name = name
	return e
}

// stubLookup answers the owner/label rule for a fixed set of objects.
type stubLookup struct {
	owned map[string]bool // key "ns/kind/name"
	known map[string]bool
}

func (s stubLookup) OwnedByAgentOps(ns, kind, name string) (bool, bool) {
	k := ns + "/" + kind + "/" + name
	return s.owned[k], s.known[k]
}

// The loop this whole capability exists to break: a runtime pod that cannot
// start must not produce a signal, or its conversation creates another pod.
func TestRuntimePodEventIsExcluded(t *testing.T) {
	s := &selfExcluder{ownNamespace: "agentops"}
	excluded, why := s.Excludes(ev("apps", "Pod", "agentops-conv-abc123"), false)
	if !excluded {
		t.Fatalf("runtime pod event must be excluded")
	}
	if why == "" {
		t.Fatalf("exclusion must carry a reason")
	}
}

// Mechanism 1 must not depend on any cache: it is what holds during startup,
// which is exactly when a mass pod-creation failure is most likely in flight.
func TestNamePrefixRuleWorksWithColdCache(t *testing.T) {
	s := &selfExcluder{ownNamespace: "agentops"} // cache is nil
	for _, name := range []string{
		"agentops-conv-xyz",
		"agentops-adapter-console-7f9c8d4-xk2p9",
		"agentops-signal-k8s-events-abc",
	} {
		if excluded, _ := s.Excludes(ev("apps", "Pod", name), false); !excluded {
			t.Fatalf("%s must be excluded with no cache present", name)
		}
	}
}

// Mechanism 2 catches an agent-ops object whose name matches no prefix.
func TestOwnerLabelRuleExcludesUnprefixedObject(t *testing.T) {
	lookup := stubLookup{
		owned: map[string]bool{"apps/Pod/some-other-name": true},
		known: map[string]bool{"apps/Pod/some-other-name": true},
	}
	s := (&selfExcluder{ownNamespace: "agentops"}).withCache(lookup)
	if excluded, _ := s.Excludes(ev("apps", "Pod", "some-other-name"), false); !excluded {
		t.Fatalf("owner/label rule must exclude an agent-ops-owned object")
	}
}

// "Unknown to the cache" must not read as "ours" — that would drop every event
// the cache has not caught up with.
func TestUnknownObjectIsNotExcluded(t *testing.T) {
	lookup := stubLookup{owned: map[string]bool{}, known: map[string]bool{}}
	s := (&selfExcluder{ownNamespace: "agentops"}).withCache(lookup)
	if excluded, _ := s.Excludes(ev("apps", "Pod", "ordinary-pod"), false); excluded {
		t.Fatalf("an object the cache does not know must not be excluded")
	}
}

// Mechanism 3 is the coarse one, and the only configurable one.
func TestOwnNamespaceExclusionAndOverride(t *testing.T) {
	s := &selfExcluder{ownNamespace: "agentops"}
	if excluded, _ := s.Excludes(ev("agentops", "Pod", "my-app-xk2p9"), false); !excluded {
		t.Fatalf("own-namespace events are excluded by default")
	}
	if excluded, _ := s.Excludes(ev("agentops", "Pod", "my-app-xk2p9"), true); excluded {
		t.Fatalf("includeOwnNamespace must re-admit a co-located workload")
	}
}

// The override relaxes mechanism 3 ONLY. If it could relax 1 or 2 the loop
// would be re-openable by configuration, which is the whole thing this design
// refuses.
func TestOverrideCannotReadmitAgentOpsObjects(t *testing.T) {
	lookup := stubLookup{
		owned: map[string]bool{"agentops/Pod/labelled-one": true},
		known: map[string]bool{"agentops/Pod/labelled-one": true},
	}
	s := (&selfExcluder{ownNamespace: "agentops"}).withCache(lookup)
	if excluded, _ := s.Excludes(ev("agentops", "Pod", "agentops-conv-abc"), true); !excluded {
		t.Fatalf("includeOwnNamespace must not re-admit a runtime pod (mechanism 1)")
	}
	if excluded, _ := s.Excludes(ev("agentops", "Pod", "labelled-one"), true); !excluded {
		t.Fatalf("includeOwnNamespace must not re-admit a labelled object (mechanism 2)")
	}
}

// An ordinary workload in an ordinary namespace stays visible.
func TestOrdinaryEventIsNotExcluded(t *testing.T) {
	s := &selfExcluder{ownNamespace: "agentops"}
	if excluded, _ := s.Excludes(ev("prod", "Pod", "api-7f9c8d4-xk2p9"), false); excluded {
		t.Fatalf("an ordinary pod event must not be excluded")
	}
}

// With no POD_NAMESPACE injected, mechanism 3 must be inert rather than
// matching every event whose namespace happens to be empty.
func TestEmptyOwnNamespaceDisablesMechanismThree(t *testing.T) {
	s := &selfExcluder{ownNamespace: ""}
	if excluded, _ := s.Excludes(ev("", "Node", "node-1"), false); excluded {
		t.Fatalf("cluster-scoped event must not be excluded when POD_NAMESPACE is unset")
	}
}

// End to end through deliver(): the excluded event must produce no signal AND
// must not advance the cursor, or a restart would treat it as considered.
func TestDeliverEmitsNothingForRuntimePodEvents(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))

	own := evt("Warning", "agentops", "Pod", "agentops-conv-abc123", "FailedScheduling")
	own.LastTimestamp = "2026-08-08T13:00:00Z"
	ordinary := evt("Warning", "prod", "Pod", "api-xk2p9", "BackOff")
	ordinary.LastTimestamp = "2026-08-08T13:00:00Z"

	a.deliver(context.Background(), []Event{own, ordinary}, false)

	got := mgr.signalsFor("src")
	if len(got) != 1 {
		t.Fatalf("exactly one signal expected (the ordinary pod): %+v", got)
	}
	if got[0].Labels["name"] != "api-xk2p9" {
		t.Fatalf("the runtime pod event must not be emitted: %+v", got)
	}
}

// The failing-pod loop, exercised as the sequence that produces it: successive
// pod names, each of which would otherwise be a fresh fingerprint AND a fresh
// signature, so nothing downstream could collapse them.
func TestSuccessiveRuntimePodNamesAllExcluded(t *testing.T) {
	mgr := newFakeManager()
	a := newTestAdapter(mgr.start(t), nil, "src", mustFilter(t, `{}`))

	var events []Event
	for _, name := range []string{"agentops-conv-a1", "agentops-conv-b2", "agentops-conv-c3"} {
		e := evt("Warning", "agentops", "Pod", name, "FailedScheduling")
		e.LastTimestamp = "2026-08-08T13:00:00Z"
		events = append(events, e)
	}
	a.deliver(context.Background(), events, false)

	if got := mgr.signalsFor("src"); len(got) != 0 {
		t.Fatalf("no runtime pod event may produce a signal, whatever its name: %+v", got)
	}
}

func TestIsOwnedLabels(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"runtime pod", map[string]string{"app.kubernetes.io/name": "agentops-runtime"}, true},
		{"manager", map[string]string{"app.kubernetes.io/name": "agentops-manager"}, true},
		{"channel adapter", map[string]string{"app.kubernetes.io/name": "agentops-adapter"}, true},
		{"signal adapter", map[string]string{"app.kubernetes.io/name": "agentops-signal-adapter"}, true},
		{"conversation label", map[string]string{"agentops.dev/conversation": "abc"}, true},
		{"unrelated", map[string]string{"app.kubernetes.io/name": "postgres"}, false},
		{"empty", map[string]string{}, false},
	}
	for _, tc := range cases {
		if got := isOwnedLabels(tc.labels); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
