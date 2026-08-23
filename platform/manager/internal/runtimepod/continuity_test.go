package runtimepod

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Continuity is promised only where it is possible. The distinction this guards
// is between a deployment that CANNOT carry context — a configuration the
// operator chose — and a context that was carried and then lost. Only the second
// is a failure, and collapsing them would make a supported install (no RWX
// provisioner, persistence off) fail every follow-up message.

func TestContinuityNeedsAVolumeWhenTheRuntimeKeepsContextThere(t *testing.T) {
	onVolume := Resolved{ContextStorage: agentopsv1alpha1.ContextOnVolume}

	if onVolume.ContinuityPossible() {
		t.Fatal("no context volume means the pod's context dies with it — continuity must not be promised")
	}

	onVolume.Config.ContextPVC = "agentops-context"
	if !onVolume.ContinuityPossible() {
		t.Fatal("with a context volume the context outlives the pod")
	}
}

func TestExternalContextNeedsNoVolume(t *testing.T) {
	external := Resolved{ContextStorage: agentopsv1alpha1.ContextExternal}

	// Stored somewhere the operator does not provide — a vendor API, a database.
	// Requiring a volume here would deny continuity to a runtime that never
	// needed one.
	if !external.ContinuityPossible() {
		t.Fatal("external context storage does not depend on a context volume")
	}
}

func TestARuntimeThatCannotContinueNeverPromisesTo(t *testing.T) {
	none := Resolved{ContextStorage: agentopsv1alpha1.ContextNone}
	none.Config.ContextPVC = "agentops-context"

	// A volume cannot grant continuity to a backend that has no notion of it.
	if none.ContinuityPossible() {
		t.Fatal("a runtime declaring no continuation must not be promised continuity")
	}
}

// The bootstrap fallback — no AgentRuntime CR at all — is the reference runtime,
// which keeps context on its volume. Treating an unset declaration as "external"
// would promise continuity that the deployment cannot deliver.
func TestUnsetDeclarationIsTreatedAsVolume(t *testing.T) {
	bootstrap := Resolved{}
	if bootstrap.ContinuityPossible() {
		t.Fatal("an unset declaration with no volume must not promise continuity")
	}
	bootstrap.Config.ContextPVC = "agentops-context"
	if !bootstrap.ContinuityPossible() {
		t.Fatal("an unset declaration with a volume behaves like the reference runtime")
	}
}
