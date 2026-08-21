package integration

import (
	"strings"
	"testing"
)

// DRAIN AWARENESS is the manager's only cluster-scoped grant, so the render
// must prove two things: that it is absent by default, and that the behaviour
// and the permission it needs ship TOGETHER.
//
// The pairing is the part worth pinning. Enabling the behaviour without the
// ClusterRole produces a manager that cannot read nodes — which does not fail
// loudly, it just fills the log with forbidden errors while quietly never
// releasing anything.
func TestDrainAwareIsOffByDefault(t *testing.T) {
	out := helmTemplate(t)
	if strings.Contains(out, "kind: ClusterRole") {
		t.Fatal("the default install must stay namespace-scoped: no ClusterRole may be rendered")
	}
	if !strings.Contains(out, `name: DRAIN_AWARE`) {
		t.Fatal("DRAIN_AWARE should still be rendered, explicitly false")
	}
	if !strings.Contains(out, "value: \"false\"") {
		t.Fatal("drain awareness must default to off")
	}
}

func TestDrainAwareShipsItsPermissionWithIt(t *testing.T) {
	out := helmTemplate(t, "--set", "rbac.drainAware=true")

	if !strings.Contains(out, "kind: ClusterRole") ||
		!strings.Contains(out, "kind: ClusterRoleBinding") {
		t.Fatal("enabling drain awareness must render both the ClusterRole and its binding")
	}
	if !strings.Contains(out, `resources: ["nodes"]`) {
		t.Fatal("the grant must be on nodes")
	}
	// READ-ONLY, and pinned: this is the one place the manager reaches outside
	// its namespace, so a widened verb list must fail a test rather than a review.
	if !strings.Contains(out, `verbs: ["get", "list", "watch"]`) {
		t.Fatal("the node grant must be read-only")
	}
	for _, forbidden := range []string{`"create"`, `"update"`, `"patch"`, `"delete"`} {
		idx := strings.Index(out, `resources: ["nodes"]`)
		if idx < 0 {
			t.Fatal("nodes rule not found")
		}
		window := out[idx:min(idx+200, len(out))]
		if strings.Contains(window, forbidden) {
			t.Fatalf("the node grant must never include %s", forbidden)
		}
	}
	if !strings.Contains(out, `value: "true"`) {
		t.Fatal("DRAIN_AWARE must be true when the permission is granted")
	}
}
