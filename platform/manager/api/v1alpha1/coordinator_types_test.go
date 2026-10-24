package v1alpha1

import "testing"

// Coordinator satisfies dispatch.CapabilityHolder ahead of the phase that
// wires it in (design D-B onward) — nothing yet calls these through that
// interface, so they are pinned directly.

func TestCoordinatorCapabilityRefReturnsTheSpecRef(t *testing.T) {
	co := &Coordinator{}
	if got := co.CapabilityRef(); got != nil {
		t.Fatalf("CapabilityRef() = %v, want nil when unset", got)
	}
	co.Spec.AgentRef = &ObjectRef{Name: "cap-a"}
	if got := co.CapabilityRef(); got == nil || got.Name != "cap-a" {
		t.Fatalf("CapabilityRef() = %v, want the spec's AgentRef", got)
	}
}

func TestCoordinatorInlineCapabilityReturnsTheEmbeddedSpec(t *testing.T) {
	co := &Coordinator{}
	co.Spec.ProfileRef = &ObjectRef{Name: "prof-a"}
	if got := co.InlineCapability(); got.ProfileRef == nil || got.ProfileRef.Name != "prof-a" {
		t.Fatalf("InlineCapability() = %+v, want the embedded AgentCapabilitySpec", got)
	}
}
