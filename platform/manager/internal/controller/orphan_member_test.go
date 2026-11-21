package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A member whose parent record is gone is closed, never left Idle at the top
// of the list; one whose parent exists is untouched.
func TestReconcileClosesAMemberWhoseParentIsGone(t *testing.T) {
	orphan := claimTestConv("orphan")
	orphan.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "gone", Entry: "worker"}
	kept := claimTestConv("kept")
	kept.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "parent", Entry: "worker"}
	c := fakeClaimClient(t, orphan, kept, claimTestConv("parent")).Build()
	store := claimStore(c)
	r := &ConversationReconciler{
		Conversations: store, Runs: claimRunsCollection(c), Client: c, Router: routerOver(c, store),
	}
	for _, n := range []string{"orphan", "kept"} {
		_, _ = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: claimTestNS, Name: n}})
	}
	if got := claimRec(t, store, "orphan"); got.Phase != state.PhaseClosed {
		t.Fatalf("orphaned member phase = %q, want Closed", got.Phase)
	}
	if got := claimRec(t, store, "kept"); got.Phase == state.PhaseClosed {
		t.Fatal("a member with a live parent must not be closed")
	}
}
