package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// unreadableConversations is a Conversations store that cannot be listed, as a
// replica's is for the moments after it restarts and before its view is ready.
type unreadableConversations struct {
	state.Collection[state.ConversationRecord]
}

var errNotReady = errors.New("state store not ready")

func (unreadableConversations) List(context.Context, state.Index[state.ConversationRecord], string) ([]state.ConversationRecord, error) {
	return nil, errNotReady
}

// A message that cannot be matched to a conversation because the store could
// not be read is a FAILURE, not "no conversation in this thread". Answering it
// as a no-op made /channel/inbound say 202 for a /close that was dropped, and
// the console reported a close that never happened.
func TestAMessageIsRefusedWhenTheStoreCannotBeRead(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	r, _, _ := closeFixture(t, nsChannel("c1", "slack"), conv)
	r.Conversations = unreadableConversations{r.Conversations}

	thread := "thread-c1"
	for _, text := range []string{"/close", "/exit", "hello"} {
		err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
			InboundMessage{ThreadID: &thread, Text: text})
		if !errors.Is(err, errNotReady) {
			t.Fatalf("%q with an unreadable store = %v, want the read failure so the caller retries", text, err)
		}
	}
}

// And the ordinary case stays what it was: a thread no conversation owns is a
// quiet no-op.
func TestAMessageInAThreadNoConversationOwnsIsStillANoOp(t *testing.T) {
	r, _, _ := closeFixture(t, nsChannel("c1", "slack"))
	thread := "nobody-has-this"
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/close"}); err != nil {
		t.Fatalf("an unowned thread must stay a no-op, got %v", err)
	}
}
