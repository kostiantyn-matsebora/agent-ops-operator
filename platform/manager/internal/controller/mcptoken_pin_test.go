package controller

import (
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// TestDeriveCoordinatorTokenMatchesChatPackage pins runtimepod's DUPLICATED
// derivation against chat's own — chat imports runtimepod, so runtimepod
// cannot import chat back, and this package is the one place that can hold
// both and prove they never drift apart (the same shape
// labelSignatureHash is pinned by, one package over).
func TestDeriveCoordinatorTokenMatchesChatPackage(t *testing.T) {
	got := runtimepod.DeriveCoordinatorToken("master-key", "co-a", "root-1")
	want := chat.DeriveCoordinatorToken("master-key", "co-a", "root-1")
	if got != want {
		t.Fatalf("runtimepod.DeriveCoordinatorToken diverged from chat.DeriveCoordinatorToken: got %q, want %q", got, want)
	}
}
