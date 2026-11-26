//go:build system

package system

// aops-mcp-server's `read` and `get_tree` verbs (httpapi/coordinate.go:
// handleCoordinateRead, descendsFrom, handleCoordinateTree, buildTree) —
// bounded to the caller's OWN subtree, at any depth, never an ancestor or a
// sibling branch. coordinator_test.go's existing full-tier lanes already
// drive `invoke`, `close` and `open-roots` against a real nested member
// tree; `read` and `get_tree` are the two remaining coordinator verbs this
// pack had never called at all. Both are exercised here against a REAL
// two-hop member chain (root -> m1 -> m2), the shape descendsFrom's own walk
// needs to actually loop past one hop, over the LIVE deployed manager and a
// derived coordinator token, exactly as platform/mcp-aops forwards one.

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func TestCoordinatorReadAndGetTreeScopedToCallersOwnSubtree(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const coordName = "e2e-coord-tree"
	const capName = "e2e-coord-tree-domain"
	const srcName = "e2e-coord-tree-src"

	mustCreate(t, e.K, agentCapability(capName, ProfileStub))
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	co := coordinatorObj(coordName, ProfileStub, []string{srcName}, []agentopsv1alpha1.CoordinatorAgentEntry{
		capabilityEntry("domain", capName, "handles the first hop"),
		capabilityEntry("nested", capName, "handles the second hop"),
	})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-coord-tree-root-" + stamp
	e.PostTask(t, srcName, fp, "echo tree-root-"+stamp)
	root := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, root.Name, 1, 3*time.Minute)

	m1 := e.invokeMember(t, coordName, root.Name, "domain", "echo tree-m1-"+stamp)
	e.WaitRun(t, m1, 1, 3*time.Minute)

	// m2 is invoked FROM m1, not from the root — a genuine two-hop member
	// chain (root -> m1 -> m2), the one shape that makes descendsFrom's own
	// walk-up loop past its first hop.
	m2 := e.invokeMember(t, coordName, m1, "nested", "echo tree-m2-"+stamp)
	e.WaitRun(t, m2, 1, 3*time.Minute)

	rootToken := coordinatorToken(e, coordName, root.Name)
	m1Token := coordinatorToken(e, coordName, m1)

	// `read`, two hops deep: the root reading m2, its own grandchild.
	code, out := e.coordinateCall(t, "/coordinate/read", rootToken,
		map[string]any{"conversation": root.Name, "target": m2})
	if code != 200 {
		t.Fatalf("read(root -> m2, two hops deep): %d %v", code, out)
	}
	if name, _ := out["name"].(string); name != m2 {
		t.Fatalf("read must project the TARGET conversation, got name %q want %q", name, m2)
	}

	// `read` refuses the reverse direction: m1 reading its own ANCESTOR
	// (the root) is out of scope — "never an ancestor" (this verb's own doc
	// comment).
	code, out = e.coordinateCall(t, "/coordinate/read", m1Token,
		map[string]any{"conversation": m1, "target": root.Name})
	if code != 403 {
		t.Fatalf("read(m1 -> its own ancestor root) must be refused out of scope, got %d %v", code, out)
	}

	// `get_tree` with no target: the caller's own subtree, nested — the
	// root's tree must contain m1, and m1's own nested members must contain
	// m2 (buildTree's recursion actually recursing, not just a flat list).
	code, out = e.coordinateCall(t, "/coordinate/get_tree", rootToken, map[string]any{"conversation": root.Name})
	if code != 200 {
		t.Fatalf("get_tree(root): %d %v", code, out)
	}
	m1Node := findTreeMember(t, out, m1)
	if m1Node == nil {
		t.Fatalf("get_tree(root) must list m1 among its members, got %v", out)
	}
	if findTreeMember(t, m1Node, m2) == nil {
		t.Fatalf("get_tree(root) must nest m2 under m1, not flatten it, got %v", m1Node)
	}

	// `get_tree` with an explicit TARGET two hops deep: the returned node IS
	// m2 itself (a leaf — exercising handleCoordinateTree's own
	// descendsFrom-gated target-resolution branch, never the no-target
	// default path the call above took).
	code, out = e.coordinateCall(t, "/coordinate/get_tree", rootToken,
		map[string]any{"conversation": root.Name, "target": m2})
	if code != 200 {
		t.Fatalf("get_tree(root, target=m2): %d %v", code, out)
	}
	if name, _ := out["name"].(string); name != m2 {
		t.Fatalf("get_tree(root, target=m2) must root the returned tree at m2, got name %q", name)
	}

	// `get_tree` refuses the same reverse direction `read` does: m1 asking
	// for the tree rooted at its own ancestor is out of scope.
	code, out = e.coordinateCall(t, "/coordinate/get_tree", m1Token,
		map[string]any{"conversation": m1, "target": root.Name})
	if code != 403 {
		t.Fatalf("get_tree(m1 -> its own ancestor root) must be refused out of scope, got %d %v", code, out)
	}
}

// findTreeMember searches one decoded get_tree node's "members" array for a
// child named want, returning the decoded child node (itself searchable the
// same way) or nil.
func findTreeMember(t *testing.T, node map[string]any, want string) map[string]any {
	t.Helper()
	members, _ := node["members"].([]any)
	for _, m := range members {
		child, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := child["name"].(string); name == want {
			return child
		}
	}
	return nil
}
