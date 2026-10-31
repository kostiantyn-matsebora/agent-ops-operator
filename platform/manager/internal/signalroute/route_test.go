package signalroute

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

func chatSig(payload string) *state.SignalRecord {
	return &state.SignalRecord{Kind: "chat", Payload: payload}
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
		sig.Kind = tc.kind
		name, task, ok := AddressedTarget(sig)
		if ok != tc.ok || name != tc.name || task != tc.task {
			t.Errorf("%s %q -> (%q,%q,%v), want (%q,%q,%v)", tc.kind, tc.payload, name, task, ok, tc.name, tc.task, tc.ok)
		}
	}
}

func TestInputNameIsDerivedFromTheSignalAndClaimant(t *testing.T) {
	p := state.SignalClaimant{Kind: state.ClaimantKindPipeline, Name: "ops"}
	c := state.SignalClaimant{Kind: state.ClaimantKindCoordinator, Name: "ops"}
	first, again := InputName("sig-1", p), InputName("sig-1", p)
	if first != again {
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
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&agentopsv1alpha1.Conversation{}).Build()
	store := kube.NewStore(c, "ns")
	conv := &state.ConversationRecord{Name: "conv"}
	if _, err := store.Create(ctx, "conv", *conv); err != nil {
		t.Fatal(err)
	}
	if delivered, err := inputDelivered(ctx, store, conv, "in-a"); err != nil || delivered {
		t.Fatalf("an empty conversation carries nothing: %v, %v", delivered, err)
	}
	conv.Inputs = []state.InputItem{{ID: "1", PayloadRef: "in-a"}}
	if delivered, _ := inputDelivered(ctx, store, conv, "in-a"); !delivered {
		t.Fatal("queued input")
	}
	if delivered, _ := inputDelivered(ctx, store, conv, "in-b"); delivered {
		t.Fatal("queued input")
	}
	conv.Inputs = nil
	if err := store.AppendRun(ctx, "conv", state.Run{RunID: "r1", Inputs: []state.RecordedInput{{PayloadRef: "in-b"}}}); err != nil {
		t.Fatal(err)
	}
	if delivered, err := inputDelivered(ctx, store, conv, "in-b"); err != nil || !delivered {
		t.Fatalf("an input cited by a run is delivered although the queue was pruned: %v, %v", delivered, err)
	}
}

func TestWithChannelAddsOnlyWhenMissing(t *testing.T) {
	refs := []state.Ref{"a"}
	if got := withChannel(refs, "a"); len(got) != 1 {
		t.Fatal("already bound")
	}
	if got := withChannel(refs, "b"); len(got) != 2 || got[1] != "b" {
		t.Fatal("added")
	}
	if got := withChannel(refs, ""); len(got) != 1 {
		t.Fatal("empty is ignored")
	}
}
