package runtimepod

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The six rows of the proposal's resolution table, in order. Each names WHO
// creates the claim as well as what the conversation gets, and the second
// column is what this function answers.
func TestResolveClaimCoversEveryRowOfTheResolutionTable(t *testing.T) {
	tests := []struct {
		row      string
		pipeline string
		binding  *agentopsv1alpha1.PersistenceBinding
		release  string
		want     string
	}{{
		row:      "Pipeline, claimName — nobody creates it, the conversation gets that claim",
		pipeline: "k8s-ops",
		binding:  &agentopsv1alpha1.PersistenceBinding{ClaimName: "team-context"},
		release:  "agentops-context",
		want:     "team-context",
	}, {
		row:      "Pipeline, volumeName — the MANAGER creates it, the conversation gets what it created",
		pipeline: "k8s-ops",
		binding:  &agentopsv1alpha1.PersistenceBinding{VolumeName: "pv-ops-context"},
		release:  "agentops-context",
		want:     "agentops-k8s-ops-context",
	}, {
		row:      "Chart, existingClaim — nobody creates it; reaches here as the release default",
		pipeline: "k8s-ops",
		binding:  nil,
		release:  "operator-made-claim",
		want:     "operator-made-claim",
	}, {
		row:      "Chart, volumeName — the CHART creates it; reaches here as the release default",
		pipeline: "k8s-ops",
		binding:  nil,
		release:  "agentops-context",
		want:     "agentops-context",
	}, {
		row:      "Chart, neither, persistence on — the chart's release default",
		pipeline: "k8s-ops",
		binding:  &agentopsv1alpha1.PersistenceBinding{},
		release:  "agentops-context",
		want:     "agentops-context",
	}, {
		row:      "persistence off — EPHEMERAL, and a route binding nothing cannot rescue it",
		pipeline: "k8s-ops",
		binding:  nil,
		release:  "",
		want:     "",
	}}

	for _, tc := range tests {
		t.Run(tc.row, func(t *testing.T) {
			if got := ResolveClaim(tc.pipeline, VolumeContext, tc.binding, tc.release); got != tc.want {
				t.Fatalf("ResolveClaim = %q, want %q", got, tc.want)
			}
		})
	}
}

// The exception to the last row: a route that bound its own volume keeps it
// even where release-wide persistence is off, because that operator has said
// where this route's state goes.
func TestARouteKeepsItsOwnVolumeWhereTheReleaseHasNone(t *testing.T) {
	got := ResolveClaim("ha-ops", VolumeContext,
		&agentopsv1alpha1.PersistenceBinding{ClaimName: "ha-context"}, "")
	if got != "ha-context" {
		t.Fatalf("ResolveClaim = %q, want the route's own claim", got)
	}
}

// The requirement the whole move exists for: one runtime, two routes, two
// volumes — and the runtime is in no part of the answer, so it cannot appear.
func TestTwoRoutesOnOneRuntimeResolveDifferently(t *testing.T) {
	defaults := Config{ContextPVC: "agentops-context"}
	observe := &agentopsv1alpha1.Pipeline{}
	observe.Name = "observe"
	observe.Spec.Persistence = &agentopsv1alpha1.PipelinePersistence{
		Context: &agentopsv1alpha1.PersistenceBinding{ClaimName: "observe-context"},
	}
	act := &agentopsv1alpha1.Pipeline{}
	act.Name = "act"
	act.Spec.Persistence = &agentopsv1alpha1.PipelinePersistence{
		Context: &agentopsv1alpha1.PersistenceBinding{VolumeName: "pv-act"},
	}

	oCtx, _ := ResolvePersistence(observe, defaults)
	aCtx, _ := ResolvePersistence(act, defaults)
	if oCtx != "observe-context" {
		t.Fatalf("observe context = %q", oCtx)
	}
	if aCtx != "agentops-act-context" {
		t.Fatalf("act context = %q", aCtx)
	}
}

// Both volumes are independent: binding one leaves the other on the release
// default rather than dragging it along.
func TestTheVolumesResolveIndependently(t *testing.T) {
	defaults := Config{ContextPVC: "agentops-context", WorkspacePVC: "agentops-workspace"}
	p := &agentopsv1alpha1.Pipeline{}
	p.Name = "route"
	p.Spec.Persistence = &agentopsv1alpha1.PipelinePersistence{
		Workspace: &agentopsv1alpha1.PersistenceBinding{ClaimName: "route-workspace"},
	}
	ctxClaim, wsClaim := ResolvePersistence(p, defaults)
	if ctxClaim != "agentops-context" {
		t.Fatalf("context = %q, want the release default", ctxClaim)
	}
	if wsClaim != "route-workspace" {
		t.Fatalf("workspace = %q, want the route's own", wsClaim)
	}
}

// A conversation with no Pipeline behind it takes the release defaults, which
// is exactly what it did before this field existed.
func TestNoPipelineTakesTheReleaseDefaults(t *testing.T) {
	ctxClaim, wsClaim := ResolvePersistence(nil,
		Config{ContextPVC: "agentops-context", WorkspacePVC: "agentops-workspace"})
	if ctxClaim != "agentops-context" || wsClaim != "agentops-workspace" {
		t.Fatalf("ResolvePersistence(nil) = %q, %q", ctxClaim, wsClaim)
	}
}
