package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// sweepFixture opens one Bus on its own scope with a controllable clock
// and the given retention caps.
func sweepFixture(t *testing.T, opts ...Option) (*Bus, *clock) {
	t.Helper()
	clk := newClock(time.Now)
	all := append([]Option{withClock(clk), WithSweepInterval(20 * time.Millisecond)}, opts...)
	bus, err := Open(context.Background(), testConn, newScope(t), all...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(bus.Close)
	return bus, clk
}

func makeDead(t *testing.T, bus *Bus, id string) commandbus.Task {
	t.Helper()
	ctx := context.Background()
	if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(id), Key: commandbus.Key(id)}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	claimed, err := bus.Claim(ctx, "conv", "sweep-test")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := bus.Fail(ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeForbidden, "terminal"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	return claimed
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("never became true: %s", what)
}

func TestSweepPurgesSettledTaskPastLedgerRetention(t *testing.T) {
	bus, clk := sweepFixture(t, WithLedgerRetention(time.Minute))
	ctx := context.Background()
	dead := makeDead(t, bus, "purge-me")

	eventually(t, "mirror sees the dead task", func() bool {
		got, err := bus.GetTask(ctx, dead.UID)
		return err == nil && got.State == commandbus.StateDead
	})
	clk.advance(10 * time.Minute) // past the dedup window and the ledger retention
	bus.sweepOnce(ctx)

	eventually(t, "task purged from the ledger", func() bool {
		_, err := bus.tasks.Get(ctx, dead.UID)
		return errors.Is(err, jetstream.ErrKeyNotFound)
	})
}

func TestSweepKeepsSettledTaskInsideRetention(t *testing.T) {
	bus, clk := sweepFixture(t, WithLedgerRetention(time.Hour))
	ctx := context.Background()
	dead := makeDead(t, bus, "keep-me")

	clk.advance(10 * time.Minute) // dedup window passed, retention not
	bus.sweepOnce(ctx)

	got, err := bus.GetTask(ctx, dead.UID)
	if err != nil || got.State != commandbus.StateDead {
		t.Fatalf("GetTask = %v, %v; want the Dead task kept", got.State, err)
	}
}

func TestSweepReapsLapsedLeaseWithoutAClaim(t *testing.T) {
	bus, clk := sweepFixture(t)
	ctx := context.Background()
	if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "lapse", Key: "lapse"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	claimed, err := bus.Claim(ctx, "conv", "crashed")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	clk.advance(bus.leaseDuration + time.Second)
	bus.sweepOnce(ctx)

	eventually(t, "lapsed lease reaped off Running", func() bool {
		got, err := bus.GetTask(ctx, claimed.UID)
		return err == nil && got.State != commandbus.StateRunning
	})
}

func TestSweepEnforcesDeadLetterCountCap(t *testing.T) {
	bus, _ := sweepFixture(t, WithDeadLetterRetention(0, 1))
	ctx := context.Background()
	makeDead(t, bus, "dl-1")
	time.Sleep(5 * time.Millisecond)
	makeDead(t, bus, "dl-2")

	eventually(t, "both dead-lettered", func() bool {
		l, err := bus.DeadLetters(ctx, "conv")
		return err == nil && len(l) == 2
	})
	bus.sweepOnce(ctx)
	eventually(t, "only the newest kept", func() bool {
		l, err := bus.DeadLetters(ctx, "conv")
		return err == nil && len(l) == 1
	})
}

func TestSweepEnforcesDeadLetterAgeCap(t *testing.T) {
	bus, clk := sweepFixture(t, WithDeadLetterRetention(time.Minute, 0))
	ctx := context.Background()
	makeDead(t, bus, "dl-old")
	clk.advance(time.Hour)
	bus.sweepOnce(ctx)

	eventually(t, "aged dead letter purged", func() bool {
		l, err := bus.DeadLetters(ctx, "conv")
		return err == nil && len(l) == 0
	})
}

func TestRunSweepsUntilContextDoneOrClosed(t *testing.T) {
	bus, clk := sweepFixture(t, WithLedgerRetention(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { bus.Run(ctx); close(done) }()

	dead := makeDead(t, bus, "run-purge")
	clk.advance(10 * time.Minute)
	eventually(t, "the background loop purges on its own", func() bool {
		_, err := bus.tasks.Get(context.Background(), dead.UID)
		return errors.Is(err, jetstream.ErrKeyNotFound)
	})
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx was cancelled")
	}

	closing, _ := sweepFixture(t)
	finished := make(chan struct{})
	go func() { closing.Run(context.Background()); close(finished) }()
	closing.Close()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Close")
	}
}
