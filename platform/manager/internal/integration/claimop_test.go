package integration

import (
	"context"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// claimOp is what an adapter does in two calls: read the oldest message, then
// acknowledge it (design D-17). Nil when nothing is offered.
func claimOp(q *chat.OpQueue, adapter string) *chat.Op {
	ctx := context.Background()
	for _, op := range q.Read(ctx, adapter, 1) {
		if _, err := q.Acknowledge(ctx, adapter, op.ID); err == nil {
			return op
		}
	}
	return nil
}
