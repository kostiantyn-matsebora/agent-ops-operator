package main

import "testing"

// syncedCache returns a cache that will answer, so "unknown" in a test means
// the object is genuinely absent rather than the cache being cold.
func syncedCache() *objectCache {
	c := newObjectCache()
	c.markSynced()
	return c
}

func adapterWithCache(c *objectCache) *adapter {
	return &adapter{name: "k8s-events", cache: c, self: newSelfExcluder().withCache(c)}
}

// The owner chain must be followed by REFERENCE for every controller shape.
// Note the pod names: each one is a case where stripping dash-separated
// segments would produce a different (wrong) answer.
func TestWorkloadResolutionByOwnerReference(t *testing.T) {
	cases := []struct {
		name     string
		podName  string
		owner    *ownerRef
		rs       *ownerRef // replicaset's own owner, when the pod is owned by an RS
		rsName   string
		wantWork string
	}{
		{
			name:     "deployment: two hops",
			podName:  "api-7f9c8d4-xk2p9",
			owner:    &ownerRef{Kind: "ReplicaSet", Name: "api-7f9c8d4"},
			rsName:   "api-7f9c8d4",
			rs:       &ownerRef{Kind: "Deployment", Name: "api"},
			wantWork: "Deployment/api",
		},
		{
			// name has ONE trailing segment; segment-stripping would give "api"
			// only by accident and "api-0" for an ordinal-named replica.
			name:     "statefulset: one hop",
			podName:  "api-0",
			owner:    &ownerRef{Kind: "StatefulSet", Name: "api"},
			wantWork: "StatefulSet/api",
		},
		{
			// looks exactly like a deployment pod, is not one
			name:     "daemonset: one hop",
			podName:  "node-exporter-xk2p9",
			owner:    &ownerRef{Kind: "DaemonSet", Name: "node-exporter"},
			wantWork: "DaemonSet/node-exporter",
		},
		{
			name:     "job: one hop",
			podName:  "backup-28f9c-abcde",
			owner:    &ownerRef{Kind: "Job", Name: "backup-28f9c"},
			wantWork: "Job/backup-28f9c",
		},
		{
			// no controller at all: the pod is its own workload
			name:     "bare pod",
			podName:  "debug-shell",
			wantWork: "Pod/debug-shell",
		},
		{
			// a workload whose own name contains a hash-shaped segment
			name:     "deployment with hashy name",
			podName:  "api-7f9c8d4-abc12-9jk2l",
			owner:    &ownerRef{Kind: "ReplicaSet", Name: "api-7f9c8d4-abc12"},
			rsName:   "api-7f9c8d4-abc12",
			rs:       &ownerRef{Kind: "Deployment", Name: "api-7f9c8d4"},
			wantWork: "Deployment/api-7f9c8d4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := syncedCache()
			putPod(t, c, podJSON("prod", tc.podName, "node-a", nil, tc.owner, true, "Running"))
			if tc.rsName != "" {
				putRS(t, c, rsJSON("prod", tc.rsName, tc.rs))
			}
			a := adapterWithCache(c)
			e := evt("Warning", "prod", "Pod", tc.podName, "BackOff")
			got := a.enrich(&e)
			if got.Workload != tc.wantWork {
				t.Fatalf("workload: got %q want %q", got.Workload, tc.wantWork)
			}
		})
	}
}

// Every replica of one Deployment must share a workload, across rollouts —
// this is the property that collapses hundreds of conversations into one.
func TestReplicasAndRolloutsShareOneWorkload(t *testing.T) {
	c := syncedCache()
	putRS(t, c, rsJSON("prod", "api-oldhash", &ownerRef{Kind: "Deployment", Name: "api"}))
	putRS(t, c, rsJSON("prod", "api-newhash", &ownerRef{Kind: "Deployment", Name: "api"}))
	for _, p := range []struct{ name, rs string }{
		{"api-oldhash-aaa", "api-oldhash"},
		{"api-oldhash-bbb", "api-oldhash"},
		{"api-newhash-ccc", "api-newhash"},
	} {
		putPod(t, c, podJSON("prod", p.name, "node-a", nil, &ownerRef{Kind: "ReplicaSet", Name: p.rs}, false, "Running"))
	}
	a := adapterWithCache(c)

	seen := map[string]bool{}
	for _, name := range []string{"api-oldhash-aaa", "api-oldhash-bbb", "api-newhash-ccc"} {
		e := evt("Warning", "prod", "Pod", name, "BackOff")
		seen[a.enrich(&e).Workload] = true
	}
	if len(seen) != 1 || !seen["Deployment/api"] {
		t.Fatalf("all replicas across both replicasets must share one workload: %v", seen)
	}
}

// A kind the cache does not track is its own workload — a Node is a workload,
// not a degraded pod.
func TestUntrackedKindIsItsOwnWorkload(t *testing.T) {
	a := adapterWithCache(syncedCache())
	e := evt("Warning", "", "Node", "node-1", "NodeNotReady")
	if got := a.enrich(&e).Workload; got != "Node/node-1" {
		t.Fatalf("workload: got %q want Node/node-1", got)
	}
}

// A tracked kind the cache has no entry for yields NO workload. Falling back to
// the pod's own name would silently reinstate per-pod grouping, which is the
// bug this whole change removes.
func TestUnknownPodYieldsNoWorkloadRatherThanItsOwnName(t *testing.T) {
	a := adapterWithCache(syncedCache())
	e := evt("Warning", "prod", "Pod", "api-7f9c8d4-xk2p9", "BackOff")
	got := a.enrich(&e)
	if got.Workload != "" {
		t.Fatalf("an unresolvable pod must carry no workload, got %q", got.Workload)
	}
	// and the signal must still be emitted, un-enriched
	s := normalize("cluster-events", &e, got)
	if s.Labels["name"] != "api-7f9c8d4-xk2p9" || s.Labels["alertname"] != "BackOff" {
		t.Fatalf("enrichment failure must not block the signal: %+v", s.Labels)
	}
	if _, ok := s.Labels["workload"]; ok {
		t.Fatalf("no workload label may be written when it could not be resolved")
	}
}

func TestEnrichmentAddsNodeAndPodLabels(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("prod", "api-1", "node-a",
		map[string]string{"app.kubernetes.io/part-of": "infra", "team": "platform"},
		nil, true, "Running"))
	a := adapterWithCache(c)
	e := evt("Warning", "prod", "Pod", "api-1", "Unhealthy")
	s := normalize("cluster-events", &e, a.enrich(&e))

	if s.Labels["node"] != "node-a" {
		t.Fatalf("node label: %q", s.Labels["node"])
	}
	if s.Labels["app.kubernetes.io/part-of"] != "infra" || s.Labels["team"] != "platform" {
		t.Fatalf("pod labels must be copied in: %+v", s.Labels)
	}
}

// A user label may never rewrite the signal's own identity — through
// signatureLabels that would silently change grouping.
func TestPodLabelsCannotOverwriteReservedKeys(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("prod", "api-1", "node-a",
		map[string]string{"name": "not-the-pod", "namespace": "elsewhere", "workload": "fake"},
		nil, true, "Running"))
	a := adapterWithCache(c)
	e := evt("Warning", "prod", "Pod", "api-1", "Unhealthy")
	s := normalize("cluster-events", &e, a.enrich(&e))

	if s.Labels["name"] != "api-1" {
		t.Fatalf("reserved label `name` was overwritten: %q", s.Labels["name"])
	}
	if s.Labels["namespace"] != "prod" {
		t.Fatalf("reserved label `namespace` was overwritten: %q", s.Labels["namespace"])
	}
	if s.Labels["workload"] != "Pod/api-1" {
		t.Fatalf("reserved label `workload` was overwritten: %q", s.Labels["workload"])
	}
}

// A conversation grouped by workload must not be titled after one of its pods.
func TestTitleNamesTheWorkload(t *testing.T) {
	c := syncedCache()
	putRS(t, c, rsJSON("prod", "api-7f9c8d4", &ownerRef{Kind: "Deployment", Name: "api"}))
	putPod(t, c, podJSON("prod", "api-7f9c8d4-xk2p9", "node-a", nil,
		&ownerRef{Kind: "ReplicaSet", Name: "api-7f9c8d4"}, false, "Running"))
	a := adapterWithCache(c)
	e := evt("Warning", "prod", "Pod", "api-7f9c8d4-xk2p9", "BackOff")
	s := normalize("cluster-events", &e, a.enrich(&e))

	if s.Title != "BackOff: Deployment/api" {
		t.Fatalf("title must name the workload: %q", s.Title)
	}
}

// Self-exclusion mechanism 2 through the real cache: a runtime pod is owned by
// a Conversation, which is not a kind the cache tracks — so the check must be
// on the owner REFERENCE, needing no lookup.
func TestOwnerRuleFindsConversationOwnedPod(t *testing.T) {
	c := syncedCache()
	putPod(t, c, podJSON("agentops", "some-runtime-pod", "node-a",
		map[string]string{"app.kubernetes.io/name": "agentops-runtime"},
		&ownerRef{Kind: "Conversation", Name: "conv-1"}, false, "Pending"))
	s := (&selfExcluder{ownNamespace: "other"}).withCache(c)
	e := evt("Warning", "agentops", "Pod", "some-runtime-pod", "FailedScheduling")
	if excluded, _ := s.Excludes(&e, false); !excluded {
		t.Fatalf("a Conversation-owned pod must be excluded even with an unmatched name")
	}
}
