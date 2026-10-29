package signalroute

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func chatSig(payload string) *agentopsv1alpha1.Signal {
	s := &agentopsv1alpha1.Signal{}
	s.Spec.Kind = "chat"
	s.Spec.Payload = payload
	return s
}

func TestAddressedTargetParsesOnlyAChatTaskForNamedTarget(t *testing.T) {
	cases := []struct {
		kind, payload string
		name, task    string
		ok            bool
	}{
		{"chat", "/k8s-ops check nodes", "k8s-ops", "check nodes", true},
		{"chat", "  /k8s-ops  multi\nline task ", "k8s-ops", "multi\nline task", true},
		{"chat", "/k8s-ops@Bot check", "k8s-ops", "check", true},
		{"chat", "are we up?", "", "", false},
		{"chat", "/k8s-ops", "", "", false},              // bare: answered in place at publish
		{"chat", "/pipelines", "", "", false},            // listing
		{"chat", "/close now", "", "", false},            // reserved
		{"chat", "/exit please", "", "", false},          // reserved
		{"alert", "/k8s-ops check nodes", "", "", false}, // only chat is addressed
	}
	for _, tc := range cases {
		sig := chatSig(tc.payload)
		sig.Spec.Kind = tc.kind
		name, task, ok := AddressedTarget(sig)
		if ok != tc.ok || name != tc.name || task != tc.task {
			t.Errorf("%s %q -> (%q,%q,%v), want (%q,%q,%v)", tc.kind, tc.payload, name, task, ok, tc.name, tc.task, tc.ok)
		}
	}
}

func TestInputNameIsDerivedFromTheSignalAndClaimant(t *testing.T) {
	p := agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: "ops"}
	c := agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindCoordinator, Name: "ops"}
	if InputName("sig-1", p) != InputName("sig-1", p) {
		t.Fatal("must be deterministic")
	}
	if InputName("sig-1", p) == InputName("sig-1", c) {
		t.Fatal("a Pipeline and a Coordinator sharing a name must not share an input")
	}
	if InputName("sig-1", p) == InputName("sig-2", p) {
		t.Fatal("two Signals must not share an input")
	}
}

func TestInputDeliveredReadsTheQueueAndTheRunHistory(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{}
	if inputDelivered(conv, "in-a") {
		t.Fatal("an empty conversation carries nothing")
	}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "1", PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "in-a"}}}
	if !inputDelivered(conv, "in-a") || inputDelivered(conv, "in-b") {
		t.Fatal("queued input")
	}
	conv.Spec.Inputs = nil
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{Inputs: []agentopsv1alpha1.RecordedInput{{PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "in-b"}}}}}
	if !inputDelivered(conv, "in-b") {
		t.Fatal("an input cited by a run is delivered although the queue was pruned")
	}
}

func TestWithChannelAddsOnlyWhenMissing(t *testing.T) {
	refs := []agentopsv1alpha1.ObjectRef{{Name: "a"}}
	if got := withChannel(refs, "a"); len(got) != 1 {
		t.Fatal("already bound")
	}
	if got := withChannel(refs, "b"); len(got) != 2 || got[1].Name != "b" {
		t.Fatal("added")
	}
	if got := withChannel(refs, ""); len(got) != 1 {
		t.Fatal("empty is ignored")
	}
}
