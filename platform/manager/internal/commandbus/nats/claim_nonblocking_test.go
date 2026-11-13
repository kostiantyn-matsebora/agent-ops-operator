package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// async-command-bus task 6 (internal/chat.OpQueue.Claim): a channel-adapter-
// facing Claim must be NON-BLOCKING, unlike Bus.Claim's own block-until-
// available contract (design D-2) — handleChannelOps already owns the long
// poll, calling Claim once a second, and a Claim that itself blocked for any
// real duration would stack on top of that wait instead of standing in for
// it.
//
// The mechanism OpQueue.Claim uses is a short, non-zero deadline (not
// literally zero — see ops.go's own claimPollTimeout doc comment: this
// backend's own Claim opens with a select between "the mirror is ready"
// and "ctx is already done", and with BOTH already satisfied Go's select
// picks between them at random, so a zero deadline would spuriously report
// "nothing claimable" on roughly half of calls even with a task sitting
// right there). This test proves the mechanism holds for THIS backend —
// commandbus/memory's own twin lives in internal/chat's test suite.
func TestClaimOnAnEmptyQueueReturnsPromptly(t *testing.T) {
	bus := newBacking(t).Bus()
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

// The same short deadline must still find a task that IS already sitting
// there — the steady-state case OpQueue.Claim depends on just as much as
// the empty one above.
func TestClaimWithAShortDeadlineStillFindsAnEligibleTask(t *testing.T) {
	bus := newBacking(t).Bus()
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
