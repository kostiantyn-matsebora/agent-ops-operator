package chat

import "context"

// Claim is what an adapter does in two calls, for the tests that only care about
// the message it ends up holding: read the oldest message, then acknowledge it.
// Nil when nothing is offered.
func (q *OpQueue) Claim(adapter string) *Op {
	ctx := context.Background()
	for _, op := range q.Read(ctx, adapter, 1) {
		if _, err := q.Acknowledge(ctx, adapter, op.ID); err == nil {
			return op
		}
	}
	return nil
}
