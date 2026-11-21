package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

func openBus(t *testing.T, scope string, opts ...Option) *Bus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts = append([]Option{WithInventory(fastInventory()), WithLeaseDuration(testLease), WithSweepInterval(time.Hour)}, opts...)
	b, err := Open(ctx, testConn, scope, opts...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(b.Close)
	return b
}

func sub(t *testing.T, b *Bus, id, key string) commandbus.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task, err := b.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(id), Key: commandbus.Key(key), Payload: []byte(id)})
	if err != nil {
		t.Fatalf("Submit %s: %v", id, err)
	}
	return task
}

// Accepting work reads nothing from a consumer: with nobody consuming, a submit
// is one stored message and one record, and returns at once.
func TestSubmitNeedsNoConsumer(t *testing.T) {
	b := openBus(t, newScope(t))
	start := time.Now()
	for i := 0; i < 20; i++ {
		if task := sub(t, b, fmt.Sprintf("c%d", i), fmt.Sprintf("k%d", i%4)); task.State != commandbus.StateQueued {
			t.Fatalf("state = %s", task.State)
		}
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("20 submits took %s with no consumer", d)
	}
}

// The sequential convoy through the bus: two replicas claim and settle
// concurrently, and the messages of one key are handled one at a time, in the
// order they were submitted, while the other keys proceed.
func TestTwoReplicasHandleAKeyInOrderAndOneAtATime(t *testing.T) {
	scope := newScope(t)
	a, b := openBus(t, scope), openBus(t, scope)
	const keys, per = 3, 4
	for n := 0; n < per; n++ {
		for k := 0; k < keys; k++ {
			sub(t, a, fmt.Sprintf("k%d-%d", k, n), fmt.Sprintf("key%d", k))
		}
	}
	var mu sync.Mutex
	handled := map[commandbus.Key][]string{}
	inflight := map[commandbus.Key]int{}
	var overlap atomic.Bool
	var done atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, bus := range []*Bus{a, b, a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil && done.Load() < keys*per {
				cctx, ccancel := context.WithTimeout(ctx, time.Second)
				task, err := bus.Claim(cctx, "conv", "worker")
				ccancel()
				if err != nil {
					continue
				}
				mu.Lock()
				inflight[task.Key]++
				if inflight[task.Key] > 1 {
					overlap.Store(true)
				}
				handled[task.Key] = append(handled[task.Key], string(task.Payload))
				mu.Unlock()
				time.Sleep(30 * time.Millisecond)
				mu.Lock()
				inflight[task.Key]--
				mu.Unlock()
				if err := bus.Settle(ctx, task.UID, task.Lease.Token); err != nil {
					t.Errorf("Settle: %v", err)
				}
				done.Add(1)
			}
		}()
	}
	wg.Wait()
	if overlap.Load() {
		t.Fatal("two messages of one key were in flight at once")
	}
	for k := 0; k < keys; k++ {
		var want []string
		for n := 0; n < per; n++ {
			want = append(want, fmt.Sprintf("k%d-%d", k, n))
		}
		if got := handled[commandbus.Key(fmt.Sprintf("key%d", k))]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("key%d handled %v, want %v", k, got, want)
		}
	}
}

// A holder that renews keeps its message; one that stops lets it lapse and the
// message is delivered again.
func TestRenewKeepsAMessageAndStoppingLetsItLapse(t *testing.T) {
	b := openBus(t, newScope(t))
	sub(t, b, "slow", "k")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	task, err := b.Claim(ctx, "conv", "w1")
	if err != nil {
		t.Fatal(err)
	}
	// Another consumer claims for as long as the renewals last: it gets nothing.
	rctx, rcancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer rcancel()
	stolen := make(chan commandbus.Task, 1)
	go func() {
		if got, err := b.Claim(rctx, "conv", "w2"); err == nil {
			stolen <- got
		}
	}()
	for i := 0; i < 7; i++ { // 1.4s, well past the 600ms lease
		time.Sleep(200 * time.Millisecond)
		if err := b.Renew(ctx, task.UID, task.Lease.Token); err != nil {
			t.Fatalf("Renew %d: %v", i, err)
		}
	}
	select {
	case got := <-stolen:
		t.Fatalf("a renewed message was handed to another consumer: %+v", got)
	default:
	}
	<-rctx.Done()
	again, err := b.Claim(ctx, "conv", "w2") // renewal stopped: the lease lapses
	if err != nil {
		t.Fatalf("the message never came back after the renewals stopped: %v", err)
	}
	if again.UID != task.UID || again.Attempt != 2 {
		t.Fatalf("redelivered %+v, want attempt 2 of the same task", again)
	}
	if err := b.Renew(ctx, task.UID, task.Lease.Token); !errors.Is(err, commandbus.ErrLeaseLost) {
		t.Fatalf("renewing a lapsed lease = %v, want ErrLeaseLost", err)
	}
}

// Settling through another handle answers the broker: the message is not
// delivered again after the lease would have lapsed.
func TestSettleThroughAnotherHandleStopsRedelivery(t *testing.T) {
	scope := newScope(t)
	a, b := openBus(t, scope), openBus(t, scope)
	sub(t, a, "once", "k")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	task, err := a.Claim(ctx, "conv", "w1")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Settle(ctx, task.UID, task.Lease.Token); err != nil {
		t.Fatal(err)
	}
	short, scancel := context.WithTimeout(ctx, 2*testLease)
	defer scancel()
	if again, err := a.Claim(short, "conv", "w1"); err == nil {
		t.Fatalf("a settled message was delivered again: %+v", again)
	}
}

// An idle convoy queue is removed, and a later message for its key makes it
// again: nothing is lost by the removal.
func TestAnIdleConvoyQueueIsRemovedAndRecreated(t *testing.T) {
	b := openBus(t, newScope(t), WithConvoyIdle(200*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first := sub(t, b, "one", "k")
	task, err := b.Claim(ctx, "conv", "w")
	if err != nil || task.UID != first.UID {
		t.Fatalf("Claim = %+v, %v", task, err)
	}
	if err := b.Settle(ctx, task.UID, task.Lease.Token); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	b.Sweep(ctx)
	if left, _ := b.convoys.List(ctx); len(left) != 0 {
		t.Fatalf("an idle convoy queue survived the sweep: %+v", left)
	}
	second := sub(t, b, "two", "k")
	got, err := b.Claim(ctx, "conv", "w")
	if err != nil || got.UID != second.UID {
		t.Fatalf("the recreated convoy queue delivered %+v, %v", got, err)
	}
}

// A convoy queue with work in it is never swept, however long it has waited.
func TestAConvoyQueueHoldingAMessageIsNotSwept(t *testing.T) {
	b := openBus(t, newScope(t), WithConvoyIdle(100*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first := sub(t, b, "waiting", "k")
	if _, err := b.Claim(ctx, "conv", "w"); err != nil { // creates the consumer, holds the message
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	b.Sweep(ctx)
	if left, _ := b.convoys.List(ctx); len(left) != 1 {
		t.Fatalf("a convoy queue with an unsettled message was swept: %+v", left)
	}
	_ = first
}
