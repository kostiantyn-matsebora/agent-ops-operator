package runtimepod

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// ResolveActingCoordinator's own three cases (design D-A), the same shape
// coordinator-owner-reach's spec pins against chat.Router's version — see
// TestResolveActingCoordinatorMatchesChatPackage in internal/controller for
// the cross-package parity check.
func TestResolveActingCoordinatorOwnCases(t *testing.T) {
	root := &state.ConversationRecord{Name: "root-1", CoordinatorRef: "co-a"}
	member := &state.ConversationRecord{Name: "member-1", CausedBy: &state.Provenance{Parent: "root-1", Entry: "reaper"}}
	plain := &state.ConversationRecord{Name: "plain-1"}

	store := seededStore(t, root, member, plain)

	cases := []struct {
		name string
		conv *state.ConversationRecord
		want string
	}{
		{"a plain member resolves via its ancestor root", member, "co-a"},
		{"a Coordinator's own root resolves via its own field", root, "co-a"},
		{"a Pipeline-addressed conversation resolves to nothing", plain, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveActingCoordinator(context.Background(), store, tc.conv)
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
	member := &state.ConversationRecord{Name: "member-1", CausedBy: &state.Provenance{Parent: "missing-parent", Entry: "reaper"}}
	store := seededStore(t, member)

	if _, err := ResolveActingCoordinator(context.Background(), store, member); err == nil {
		t.Fatal("want an error when the ancestry chain is broken")
	}
	if got := resolveActingCoordinatorBestEffort(context.Background(), store, member); got != "" {
		t.Fatalf("the best-effort wrapper must swallow the error, got %q", got)
	}
}

// ResolveFor itself threads the resolved name through to Resolved — the
// field Build reads for a member's AOPS_MCP_TOKEN.
func TestResolveForResolvesActingCoordinatorForAMember(t *testing.T) {
	root := &state.ConversationRecord{Name: "root-1", CoordinatorRef: "co-a"}
	member := &state.ConversationRecord{Name: "member-1", ProfileRef: "plain",
		CausedBy: &state.Provenance{Parent: "root-1", Entry: "reaper"}}

	store := seededStore(t, root, member)
	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).Build()
	got, err := ResolveFor(context.Background(), c, store, testNS, member, Config{Image: "bootstrap-image"})
	if err != nil {
		t.Fatalf("ResolveFor: %v", err)
	}
	if got.ActingCoordinator != "co-a" {
		t.Fatalf("ActingCoordinator = %q, want %q", got.ActingCoordinator, "co-a")
	}
}
