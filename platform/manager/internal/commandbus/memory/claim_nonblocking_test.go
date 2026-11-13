package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// async-command-bus task 6 (internal/chat.OpQueue.Claim): the memory
// backend's own twin of commandbus/nats's identically-named test — see
// that file's doc comment for the full rationale (why a short, non-zero
// deadline, not literally zero).
func TestClaimOnAnEmptyQueueReturnsPromptly(t *testing.T) {
	bus := memory.Open()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := bus.Claim(ctx, "conv", "test")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("claim on an empty queue: err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("claim on an empty queue took %v, want well under a second", elapsed)
	}
}

func TestClaimWithAShortDeadlineStillFindsAnEligibleTask(t *testing.T) {
	bus := memory.Open()
	submitted, err := bus.Submit(context.Background(), commandbus.SubmitRequest{
		Queue: "conv", Kind: commandbus.KindDefault,
		CommandID: "cmd-nonblocking-claim", Key: "key-nonblocking-claim",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	claimed, err := bus.Claim(ctx, "conv", "test")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("claim on a non-empty queue: %v", err)
	}
	if claimed.UID != submitted.UID {
		t.Fatalf("claimed %s, want %s", claimed.UID, submitted.UID)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("claim on a non-empty queue took %v, want well under a second", elapsed)
	}
}
