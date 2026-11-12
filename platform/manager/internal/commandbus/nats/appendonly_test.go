package nats

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Several replicas submitting one command id at the same moment create exactly
// one task and every caller gets its uid: the message append is conditional on
// the key's subject, so the losers re-plan, find the winner and are suppressed.
func TestConcurrentSubmitsOfOneCommandIDCreateOneTask(t *testing.T) {
	b := newBacking(t)
	const replicas = 6
	buses := make([]commandbus.Bus, replicas)
	for i := range buses {
		buses[i] = b.Bus()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	uids := make([]string, replicas)
	errs := make([]error, replicas)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range buses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			task, err := buses[i].Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "same", Key: "k"})
			uids[i], errs[i] = task.UID, err
		}()
	}
	close(start)
	wg.Wait()
	for i := range buses {
		if errs[i] != nil {
			t.Fatalf("replica %d: %v", i, errs[i])
		}
		if uids[i] != uids[0] {
			t.Fatalf("replica %d got task %s, replica 0 got %s: one command id made two tasks", i, uids[i], uids[0])
		}
	}
	if n := len(b.Entries()); n != 1 {
		t.Fatalf("the log holds %d entries, want the one message", n)
	}
}

// Of two claims racing for one key, planned from the same view on different
// replicas, exactly one lands and the other is a rejected append (isWrongLastSeq),
// never an error out of Claim.
func TestRacingClaimsOfOneKeyLandOnce(t *testing.T) {
	b := newBacking(t)
	first, second := b.Bus().(*Bus), b.Bus().(*Bus)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := first.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "c", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	if err := second.sync(ctx); err != nil {
		t.Fatal(err)
	}
	now := first.clock.now()
	planA := first.mirror.log.PlanClaim("conv", "a", now, time.Minute)
	planB := second.mirror.log.PlanClaim("conv", "b", now, time.Minute)
	if len(planA) != 1 || len(planB) != 1 {
		t.Fatalf("plans: %d and %d, want one each", len(planA), len(planB))
	}
	if _, err := first.append(ctx, planA[0].Append); err != nil {
		t.Fatalf("the first claim: %v", err)
	}
	if _, err := second.append(ctx, planB[0].Append); !isWrongLastSeq(err) {
		t.Fatalf("the second claim = %v, want the append rejected as a lost race", err)
	}
	claimed := 0
	for _, e := range b.Entries() {
		if e.Type == "claimed" {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("%d claim entries landed, want exactly one", claimed)
	}
}

// Two submits to one key while a consumer claims what lands: none fails, which
// is the shape of the incident (a claim between a submit's two writes).
func TestSubmitsToOneKeySucceedWhileAConsumerClaims(t *testing.T) {
	b := newBacking(t)
	bus, consumer := b.Bus(), b.Bus()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var settled atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			cctx, ccancel := context.WithTimeout(ctx, 100*time.Millisecond)
			task, err := consumer.Claim(cctx, "conv", "racer")
			ccancel()
			if err == nil && consumer.Settle(ctx, task.UID, task.Lease.Token) == nil {
				settled.Add(1)
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	const n = 60
	var submitters sync.WaitGroup
	var failed atomic.Int64
	for i := range n {
		submitters.Add(1)
		go func() {
			defer submitters.Done()
			if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(fmt.Sprintf("c%d", i)), Key: "shared"}); err != nil {
				failed.Add(1)
				t.Errorf("Submit %d: %v", i, err)
			}
		}()
	}
	submitters.Wait()
	deadline := time.Now().Add(20 * time.Second)
	for settled.Load() < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	if failed.Load() != 0 || settled.Load() != n {
		t.Fatalf("%d submits failed and %d of %d tasks settled", failed.Load(), settled.Load(), n)
	}
}

// Consumed entries are removed once the grace passes, and nothing else is: an
// unconsumed message and a dead task's entries stay, retained by the log's own
// limits alone.
func TestOnlyConsumedEntriesAreRemovedAfterTheGrace(t *testing.T) {
	bus, clk := sweepFixture(t)
	ctx := context.Background()
	submit := func(id string) commandbus.Task {
		task, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(id), Key: commandbus.Key(id)})
		if err != nil {
			t.Fatalf("Submit %s: %v", id, err)
		}
		return task
	}
	submit("consumed")
	claimed, err := bus.Claim(ctx, "conv", "c")
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Settle(ctx, claimed.UID, claimed.Lease.Token); err != nil {
		t.Fatal(err)
	}
	makeDead(t, bus, "dead")
	waiting := submit("waiting")
	before := logMsgs(t, bus) // consumed: 3 entries, dead: 3, waiting: 1

	bus.sweepOnce(ctx)
	if got := logMsgs(t, bus); got != before {
		t.Fatalf("entries were removed inside the grace: %d before, %d after", before, got)
	}
	clk.advance(DefaultSettledGrace + time.Second)
	bus.sweepOnce(ctx)
	if got := logMsgs(t, bus); got != before-3 {
		t.Fatalf("log holds %d entries, want %d: the consumed task's three gone and nothing else", got, before-3)
	}
	if got, err := bus.GetTask(ctx, waiting.UID); err != nil || got.State != commandbus.StateQueued {
		t.Fatalf("the unconsumed message: %v, %v; want it still queued", got.State, err)
	}
	if l, err := bus.DeadLetters(ctx, "conv"); err != nil || len(l) != 1 {
		t.Fatalf("DeadLetters = %v, %v; want the dead task kept", l, err)
	}
}

// A key whose earlier entries were removed (consumed) is submitted to and
// claimed again: the mirror's belief about the key's last entry is corrected by
// the rejected append, never left standing.
func TestAKeyIsReusableAfterItsEntriesWereRemoved(t *testing.T) {
	bus, _ := sweepFixture(t, WithSettledGrace(0))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for i := range 3 {
		if _, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(fmt.Sprintf("r%d", i)), Key: "reused"}); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
		task, err := bus.Claim(ctx, "conv", "c")
		if err != nil {
			t.Fatalf("Claim %d: %v", i, err)
		}
		if err := bus.Settle(ctx, task.UID, task.Lease.Token); err != nil {
			t.Fatalf("Settle %d: %v", i, err)
		}
	}
	if got := logMsgs(t, bus); got != 0 {
		t.Fatalf("log holds %d entries after every task was consumed, want none", got)
	}
}

// A claim made on one replica is acknowledged on another, over one key whose
// earlier entries the first replica already removed: the second replica's
// belief about the key's last entry is stale, and the acknowledgement must
// still land (a rejected append re-learns the truth) rather than leave the task
// leased until it lapses.
func TestSettleOnAnotherReplicaAfterRemovalsOnTheKey(t *testing.T) {
	scope := newScope(t)
	open := func() *Bus {
		b, err := Open(context.Background(), testConn, scope, WithSettledGrace(0))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(b.Close)
		return b
	}
	a, b := open(), open()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := range 4 {
		if _, err := a.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(fmt.Sprintf("op%d", i)), Key: "chat-k"}); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
		task, err := a.Claim(ctx, "conv", "stream")
		if err != nil {
			t.Fatalf("Claim %d: %v", i, err)
		}
		running, err := b.Running(ctx, []commandbus.Queue{"conv"}, "")
		if err != nil || len(running) != 1 || running[0].UID != task.UID {
			t.Fatalf("Running on the other replica = %v, %v; want the claim", running, err)
		}
		if err := b.Settle(ctx, task.UID, running[0].Lease.Token); err != nil {
			t.Fatalf("Settle %d on the other replica: %v", i, err)
		}
	}
	if left, _ := a.Running(ctx, []commandbus.Queue{"conv"}, ""); len(left) != 0 {
		t.Fatalf("%d tasks still leased after every acknowledgement", len(left))
	}
}
