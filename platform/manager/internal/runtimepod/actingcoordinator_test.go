package runtimepod

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// ResolveActingCoordinator's own three cases (design D-A), the same shape
// coordinator-owner-reach's spec pins against chat.Router's version — see
// TestResolveActingCoordinatorMatchesChatPackage in internal/controller for
// the cross-package parity check.
func TestResolveActingCoordinatorOwnCases(t *testing.T) {
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "root-1", testNS
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}

	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}

	plain := &agentopsv1alpha1.Conversation{}
	plain.Name, plain.Namespace = "plain-1", testNS

	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).WithObjects(root, member, plain).Build()

	cases := []struct {
		name string
		conv *agentopsv1alpha1.Conversation
		want string
	}{
		{"a plain member resolves via its ancestor root", member, "co-a"},
		{"a Coordinator's own root resolves via its own field", root, "co-a"},
		{"a Pipeline-addressed conversation resolves to nothing", plain, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveActingCoordinator(context.Background(), c, testNS, tc.conv)
			if err != nil {
				t.Fatalf("ResolveActingCoordinator: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A broken ancestry chain surfaces as an error from ResolveActingCoordinator
// itself, but ResolveFor's best-effort wrapper swallows it rather than
// failing an otherwise-ordinary pod build.
func TestResolveActingCoordinatorPropagatesABrokenAncestryChain(t *testing.T) {
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "missing-parent", Entry: "reaper"}
	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).WithObjects(member).Build()

	if _, err := ResolveActingCoordinator(context.Background(), c, testNS, member); err == nil {
		t.Fatal("want an error when the ancestry chain is broken")
	}
	if got := resolveActingCoordinatorBestEffort(context.Background(), c, testNS, member); got != "" {
		t.Fatalf("the best-effort wrapper must swallow the error, got %q", got)
	}
}

// ResolveFor itself threads the resolved name through to Resolved — the
// field Build reads for a member's AOPS_MCP_TOKEN.
func TestResolveForResolvesActingCoordinatorForAMember(t *testing.T) {
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "root-1", testNS
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}

	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	member.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "plain"}

	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).WithObjects(root, member).Build()
	got, err := ResolveFor(context.Background(), c, testNS, member, Config{Image: "bootstrap-image"})
	if err != nil {
		t.Fatalf("ResolveFor: %v", err)
	}
	if got.ActingCoordinator != "co-a" {
		t.Fatalf("ActingCoordinator = %q, want %q", got.ActingCoordinator, "co-a")
	}
}
