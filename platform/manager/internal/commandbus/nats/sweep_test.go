package nats

import (
	"context"
	"testing"
	"time"

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

// logMsgs is how many entries the queue log's stream holds — what the bus
// itself must never reduce.
func logMsgs(t *testing.T, bus *Bus) uint64 {
	t.Helper()
	info, err := bus.logInfo(context.Background())
	if err != nil {
		t.Fatalf("log info: %v", err)
	}
	return info.State.Msgs
}

func TestSweepForgetsSettledTaskPastLogRetentionWithoutTouchingTheLog(t *testing.T) {
	bus, clk := sweepFixture(t, WithLogRetention(time.Minute, 0))
	ctx := context.Background()
	dead := makeDead(t, bus, "forget-me")
	before := logMsgs(t, bus)

	eventually(t, "mirror sees the dead task", func() bool {
		got, err := bus.GetTask(ctx, dead.UID)
		return err == nil && got.State == commandbus.StateDead
	})
	clk.advance(10 * time.Minute) // past the dedup window and the log's retention
	bus.sweepOnce(ctx)

	if _, ok := bus.mirror.log.Task(dead.UID, clk.now()); ok {
		t.Fatalf("the mirror still holds a task settled past the log's retention")
	}
	if after := logMsgs(t, bus); after != before {
		t.Fatalf("the sweep changed the log: %d entries before, %d after (the bus removes none)", before, after)
	}
	if got, err := bus.GetTask(ctx, dead.UID); err != nil || got.State != commandbus.StateDead {
		t.Fatalf("GetTask = %v, %v; want the dead task still read from the dead log", got.State, err)
	}
}

func TestSweepKeepsSettledTaskInsideRetention(t *testing.T) {
	bus, clk := sweepFixture(t, WithLogRetention(time.Hour, 0))
	ctx := context.Background()
	dead := makeDead(t, bus, "keep-me")

	clk.advance(10 * time.Minute) // dedup window passed, retention not
	bus.sweepOnce(ctx)

	if _, ok := bus.mirror.log.Task(dead.UID, clk.now()); !ok {
		t.Fatalf("the mirror forgot a task settled inside the log's retention")
	}
}

func TestSweepAppendsNothingForALapseWithAttemptsLeft(t *testing.T) {
	bus, clk := sweepFixture(t)
	ctx := context.Background()
	if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "lapse", Key: "lapse"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	claimed, err := bus.Claim(ctx, "conv", "crashed")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	before := logMsgs(t, bus)
	clk.advance(bus.leaseDuration + time.Second)
	bus.sweepOnce(ctx)

	if after := logMsgs(t, bus); after != before {
		t.Fatalf("the sweep appended %d entries for a lapse that needs none", after-before)
	}
	got, err := bus.GetTask(ctx, claimed.UID)
	if err != nil || got.State != commandbus.StateRetrying {
		t.Fatalf("GetTask = %v, %v; want the lapsed task reading as retrying", got.State, err)
	}
}

// A lapse that spends the task's last attempt reads as dead with no write, and
// the sweep makes it a fact of the log and the dead log.
func TestSweepClosesALapsedTaskWhoseAttemptsAreSpent(t *testing.T) {
	inv := []commandbus.Declaration{{Queue: "conv", Policy: commandbus.Policy{Attempts: 1, DedupWindow: time.Minute, ParkCoolOff: time.Minute}}}
	bus, clk := sweepFixture(t, WithInventory(inv))
	ctx := context.Background()
	if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "last", Key: "last"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	claimed, err := bus.Claim(ctx, "conv", "crashed")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	clk.advance(bus.leaseDuration + time.Second)
	if got, err := bus.GetTask(ctx, claimed.UID); err != nil || got.State != commandbus.StateDead {
		t.Fatalf("before the sweep: %v, %v; want dead derived from the lapse alone", got.State, err)
	}
	before := logMsgs(t, bus)
	bus.sweepOnce(ctx)

	if after := logMsgs(t, bus); after != before+1 {
		t.Fatalf("the sweep appended %d entries, want the one dead entry", after-before)
	}
	eventually(t, "the dead log holds the task", func() bool { return bus.mirror.hasDead(claimed.UID) })
	if l, err := bus.DeadLetters(ctx, "conv"); err != nil || len(l) != 1 {
		t.Fatalf("DeadLetters = %v, %v; want the one task", l, err)
	}
}

func TestDeadLogEnforcesItsCountCap(t *testing.T) {
	bus, _ := sweepFixture(t, WithDeadLetterRetention(0, 1))
	ctx := context.Background()
	makeDead(t, bus, "dl-1")
	time.Sleep(5 * time.Millisecond)
	newest := makeDead(t, bus, "dl-2")

	eventually(t, "only the newest listed", func() bool {
		l, err := bus.DeadLetters(ctx, "conv")
		return err == nil && len(l) == 1 && l[0].UID == newest.UID
	})
	eventually(t, "the dead stream's own limit dropped the older entry", func() bool {
		s, err := bus.js.Stream(ctx, bus.dead)
		if err != nil {
			return false
		}
		return s.CachedInfo().State.Msgs == 1
	})
}

func TestSweepEnforcesDeadLetterAgeCap(t *testing.T) {
	bus, clk := sweepFixture(t, WithDeadLetterRetention(time.Minute, 0))
	ctx := context.Background()
	makeDead(t, bus, "dl-old")
	clk.advance(time.Hour)
	bus.sweepOnce(ctx)

	eventually(t, "aged dead letter no longer listed", func() bool {
		l, err := bus.DeadLetters(ctx, "conv")
		return err == nil && len(l) == 0
	})
}

func TestRunSweepsUntilContextDoneOrClosed(t *testing.T) {
	bus, clk := sweepFixture(t, WithLogRetention(time.Minute, 0))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { bus.Run(ctx); close(done) }()

	dead := makeDead(t, bus, "run-prune")
	clk.advance(10 * time.Minute)
	eventually(t, "the background loop prunes the mirror on its own", func() bool {
		_, ok := bus.mirror.log.Task(dead.UID, clk.now())
		return !ok
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
