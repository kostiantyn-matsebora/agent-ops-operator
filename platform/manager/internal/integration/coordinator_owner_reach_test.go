// The Coordinator-owner reach class (coordinator-owner-reach, design D-A/D-B)
// against a REAL API server: the coordinatorRef-resolution walk and the
// widened close bound, exercised over an actual causedBy chain the schema
// round-trips — the behaviour `internal/chat`'s fake-client tests cannot
// prove on their own.
package integration

import (
	"context"
	"encoding/json"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// mkMember creates a plain capabilityRef-shaped member: causedBy set, no
// coordinatorRef of its own — the self-heal reaper's exact shape.
func mkMember(t *testing.T, name, profile, parent, entry string) *agentopsv1alpha1.Conversation {
	t.Helper()
	m := &agentopsv1alpha1.Conversation{}
	m.Name, m.Namespace = name, ns
	m.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	m.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: entry}
	m.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	if err := k8sClient.Create(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestCoordinateOpenRootsAuthenticatesAPlainMemberOverTheRealAPI is the
// reaper's own case end to end: a plain member with no coordinatorRef of its
// own authenticates against a token derived from the Coordinator its
// ANCESTOR ROOT names, resolved by walking a REAL causedBy chain.
func TestCoordinateOpenRootsAuthenticatesAPlainMemberOverTheRealAPI(t *testing.T) {
	mkProfile(t, "prof-oproots-co")
	mkCoordinator(t, "co-oproots", nil, nil, "prof-oproots-co", nil)
	reconcileCoordinator(t, "co-oproots")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-oproots-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-oproots-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-oproots"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	reaper := mkMember(t, "co-oproots-reaper", "prof-oproots-co", root.Name, "reaper")

	sibling := &agentopsv1alpha1.Conversation{}
	sibling.Name, sibling.Namespace = "co-oproots-incident", ns
	sibling.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-oproots-co"}
	sibling.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-oproots"}
	if err := k8sClient.Create(context.Background(), sibling); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-oproots", reaper.Name)
	rec := postCoordinateReq(t, srv, "/coordinate/open-roots", token, map[string]any{"conversation": reaper.Name})
	if rec.Code != 200 {
		t.Fatalf("open-roots: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Roots []map[string]any `json:"roots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Roots) != 1 || out.Roots[0]["name"] != sibling.Name {
		t.Fatalf("want only the sibling root (the caller's own ancestor root excluded), got %+v", out.Roots)
	}
}

// TestCoordinateCloseWidenedBoundOverTheRealAPI: the reaper closes a sibling
// root it did not directly cause, through the widened close bound, and is
// refused when it tries to reach its own ancestor root or a member.
func TestCoordinateCloseWidenedBoundOverTheRealAPI(t *testing.T) {
	mkProfile(t, "prof-wclose-co")
	mkCoordinator(t, "co-wclose", nil, nil, "prof-wclose-co", nil)
	reconcileCoordinator(t, "co-wclose")

	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "co-wclose-root", ns
	root.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-wclose-co"}
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-wclose"}
	if err := k8sClient.Create(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	reaper := mkMember(t, "co-wclose-reaper", "prof-wclose-co", root.Name, "reaper")

	sibling := &agentopsv1alpha1.Conversation{}
	sibling.Name, sibling.Namespace = "co-wclose-incident", ns
	sibling.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-wclose-co"}
	sibling.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-wclose"}
	// An alert, not a person's own request — isHumanInitiated must report
	// false for the widened bound to close it.
	sibling.Spec.Signal = &agentopsv1alpha1.SignalProvenance{
		SourceRef: &agentopsv1alpha1.ObjectRef{Name: "alerts"},
		Labels:    map[string]string{"alertname": "KubeJobFailed"},
	}
	if err := k8sClient.Create(context.Background(), sibling); err != nil {
		t.Fatal(err)
	}
	// A SEPARATE sibling root, left open, whose own member is the target of
	// the "never reach a member" assertion below — closing `sibling` itself
	// cascades to ITS OWN members, which would otherwise close this one as a
	// side effect and invalidate the assertion.
	secondSibling := &agentopsv1alpha1.Conversation{}
	secondSibling.Name, secondSibling.Namespace = "co-wclose-incident-2", ns
	secondSibling.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-wclose-co"}
	secondSibling.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-wclose"}
	if err := k8sClient.Create(context.Background(), secondSibling); err != nil {
		t.Fatal(err)
	}
	siblingMember := mkMember(t, "co-wclose-incident-member", "prof-wclose-co", secondSibling.Name, "endpoint-check")

	srv := apiServer()
	token := chat.DeriveCoordinatorToken(srv.AdapterToken, "co-wclose", reaper.Name)

	// The reaper closes the sibling root it did not directly cause.
	rec := postCoordinateReq(t, srv, "/coordinate/close", token,
		map[string]any{"conversation": reaper.Name, "target": sibling.Name, "reason": "resolved by the reaper"})
	if rec.Code != 200 {
		t.Fatalf("close sibling root: %d %s", rec.Code, rec.Body.String())
	}
	if got := getConv(t, sibling.Name); got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("the sibling root must close, got %s", got.Status.Phase)
	}

	// Refused: the reaper's own ancestor root, even named directly.
	recOwn := postCoordinateReq(t, srv, "/coordinate/close", token,
		map[string]any{"conversation": reaper.Name, "target": root.Name, "reason": "trying to close my own root"})
	if recOwn.Code != 403 {
		t.Fatalf("want 403 closing the reaper's own ancestor root, got %d %s", recOwn.Code, recOwn.Body.String())
	}
	if got := getConv(t, root.Name); got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("the reaper must never close its own ancestor root")
	}

	// Refused: a member, even one within the caller's own Coordinator's tree.
	recMember := postCoordinateReq(t, srv, "/coordinate/close", token,
		map[string]any{"conversation": reaper.Name, "target": siblingMember.Name, "reason": "trying to reach a member"})
	if recMember.Code != 403 {
		t.Fatalf("want 403 closing a member through the widened bound, got %d %s", recMember.Code, recMember.Body.String())
	}
	if got := getConv(t, siblingMember.Name); got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a member must never be reached by the widened bound")
	}
}
