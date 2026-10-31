package integration

import (
	"context"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// recOf reads a conversation through the state store, the way the code under
// test reaches it. Tests keep reading and seeding the Conversation objects
// directly: that is the stored form the kube backend promises to leave
// unchanged.
func recOf(t *testing.T, name string) *state.ConversationRecord {
	t.Helper()
	rec, _, err := stateStore.Get(context.Background(), state.Key(name))
	if err != nil {
		t.Fatalf("read record %q: %v", name, err)
	}
	return &rec
}
