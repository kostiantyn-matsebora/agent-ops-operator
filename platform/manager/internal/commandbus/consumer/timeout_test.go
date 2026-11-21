package consumer_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

func startPool(t *testing.T, cfg consumer.Config) context.Context {
	t.Helper()
	p, err := consumer.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runPool(ctx, p)
	return ctx
}

func submit(t *testing.T, ctx context.Context, bus commandbus.Bus, id string) commandbus.Task {
	t.Helper()
	task, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(id), Key: commandbus.Key(id)})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// A handler that honours its context is cancelled at the deadline and the
// attempt is failed with a retryable code, so the message is delivered again
// instead of being processed for ever.
func TestAMessageIsNotProcessedBeyondTheHandlerTimeout(t *testing.T) {
	bus := memory.Open()
	var starts atomic.Int32
	handler := func(ctx context.Context, _ commandbus.Task) error {
		starts.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx := startPool(t, consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1, HandlerTimeout: 100 * time.Millisecond})
	task := submit(t, ctx, bus, "slow")
	waitFor(t, 3*time.Second, func() bool {
		got, err := bus.GetTask(ctx, task.UID)
		return err == nil && got.Code == commandbus.CodeUnavailable && got.Attempt >= 1
	})
	waitFor(t, 5*time.Second, func() bool { return starts.Load() >= 2 }) // retried after its backoff
}

// A handler that ignores its context cannot be stopped, but it does not hold
// the message: the attempt is failed at the deadline.
func TestAHandlerThatIgnoresItsContextDoesNotHoldItsMessage(t *testing.T) {
	bus := memory.Open()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	handler := func(context.Context, commandbus.Task) error { <-release; return nil }
	ctx := startPool(t, consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1, HandlerTimeout: 100 * time.Millisecond})
	task := submit(t, ctx, bus, "stuck")
	waitFor(t, 3*time.Second, func() bool {
		got, err := bus.GetTask(ctx, task.UID)
		return err == nil && got.Code == commandbus.CodeUnavailable
	})
}

// renewingBus is a bus whose leases a holder must extend.
type renewingBus struct {
	commandbus.Bus
	renewed atomic.Int32
	lost    atomic.Bool
}

func (b *renewingBus) Renew(context.Context, string, string) error {
	b.renewed.Add(1)
	if b.lost.Load() {
		return commandbus.ErrLeaseLost
	}
	return nil
}

// While a handler works, the pool keeps its lease alive; the handler itself
// knows nothing of it.
func TestTheLeaseIsRenewedWhileAHandlerRuns(t *testing.T) {
	bus := &renewingBus{Bus: memory.Open(memory.WithLeaseDuration(600 * time.Millisecond))}
	handler := func(context.Context, commandbus.Task) error { time.Sleep(900 * time.Millisecond); return nil }
	ctx := startPool(t, consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1, HandlerTimeout: 5 * time.Second})
	submit(t, ctx, bus, "long")
	waitFor(t, 5*time.Second, func() bool { return bus.renewed.Load() >= 2 })
}

// A holder that learns its lease was taken stops working: another consumer may
// hold the message now.
func TestAHandlerIsCancelledWhenItsLeaseIsLost(t *testing.T) {
	bus := &renewingBus{Bus: memory.Open(memory.WithLeaseDuration(600 * time.Millisecond))}
	bus.lost.Store(true)
	cancelled := make(chan struct{})
	handler := func(ctx context.Context, _ commandbus.Task) error { <-ctx.Done(); close(cancelled); return ctx.Err() }
	ctx := startPool(t, consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1, HandlerTimeout: 10 * time.Second})
	submit(t, ctx, bus, "lost")
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("the handler kept running after it lost its lease")
	}
}
