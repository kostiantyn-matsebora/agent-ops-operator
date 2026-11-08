// createForList is shared by the layout-specific tests that still build
// several conversations to check against (cachedlists_test.go). The index
// behavior itself — every declared index returns exactly its matching set,
// an undeclared one is refused — is now the generic suite's
// (internal/state/conformance), driven by conversationFixture's own pool.
package kube

import (
	"context"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func createForList(t *testing.T, ctx context.Context, s state.Stores, name, conversationID, pipelineRef, phase string) {
	t.Helper()
	rec := fixtureRecord(name)
	rec.ConversationID = conversationID
	rec.PipelineRef = pipelineRef
	rec.Phase = phase
	if _, err := s.Conversations.Create(ctx, state.Key(name), rec); err != nil {
		t.Fatalf("Create %q: %v", name, err)
	}
}
