package consumer_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

func noop(context.Context, commandbus.Task) error { return nil }

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	bus := memory.Open()
	for _, tc := range []struct {
		name string
		cfg  consumer.Config
	}{
		{"nil bus", consumer.Config{Queue: "conv", Handler: noop, Concurrency: 1}},
		{"empty queue", consumer.Config{Bus: bus, Handler: noop, Concurrency: 1}},
		{"nil handler", consumer.Config{Bus: bus, Queue: "conv", Concurrency: 1}},
		{"zero concurrency", consumer.Config{Bus: bus, Queue: "conv", Handler: noop}},
		{"negative concurrency", consumer.Config{Bus: bus, Queue: "conv", Handler: noop, Concurrency: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := consumer.New(tc.cfg); err == nil {
				t.Fatal("want an error, got none")
			}
		})
	}
}

// runPool starts p.Run in the background and returns a channel closed once
// Run returns, so a test can assert it stops within a bound instead of
// hanging the suite if it never does.
func runPool(ctx context.Context, p *consumer.Pool) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Run(ctx)
	}()
	return done
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestPoolSettlesATaskOnHandlerSuccess(t *testing.T) {
	bus := memory.Open()
	var holder string
	var mu sync.Mutex
	handler := func(_ context.Context, task commandbus.Task) error {
		mu.Lock()
		if task.Lease != nil {
			holder = task.Lease.Holder
		}
		mu.Unlock()
		return nil
	}
	p, err := consumer.New(consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runPool(ctx, p)

	task, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "cmd-1", Key: "key-1"})
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool {
		got, err := bus.GetTask(ctx, task.UID)
		return err == nil && got.State == commandbus.StateSucceeded
	})

	// A pool whose Config.ConsumerID was left empty names its claim after
	// the queue, with an appended per-goroutine index — never a
	// caller-supplied value, and never empty.
	mu.Lock()
	got := holder
	mu.Unlock()
	if got != "conv-0" {
		t.Fatalf("holder = %q, want %q", got, "conv-0")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after its context was cancelled")
	}
}

func TestPoolFailsATaskWithTheHandlersExplicitCode(t *testing.T) {
	bus := memory.Open()
	handler := func(context.Context, commandbus.Task) error {
		return consumer.WithCode(commandbus.CodeNotFound, errors.New("no such conversation"))
	}
	p, err := consumer.New(consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runPool(ctx, p)

	task, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "cmd-1", Key: "key-1"})
	if err != nil {
		t.Fatal(err)
	}

	var got commandbus.Task
	waitFor(t, 2*time.Second, func() bool {
		got, err = bus.GetTask(ctx, task.UID)
		return err == nil && got.State == commandbus.StateDead
	})
	// CodeNotFound is not retryable (commandbus.ErrorCode.Retryable), so one
	// Fail call is enough to move the task straight to Dead — proving the
	// pool recorded the handler's OWN code rather than re-deriving one from
	// the error's text, which would have normalized to CodeInternal (a
	// retryable code) and left the task Retrying instead.
	if got.Code != commandbus.CodeNotFound {
		t.Fatalf("Code = %q, want %q", got.Code, commandbus.CodeNotFound)
	}

	cancel()
	<-done
}

func TestPoolFailsATaskWithAPlainErrorAsInternal(t *testing.T) {
	bus := memory.Open()
	handler := func(context.Context, commandbus.Task) error {
		return errors.New("boom")
	}
	p, err := consumer.New(consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runPool(ctx, p)

	task, err := bus.Submit(ctx, commandbus.SubmitRequest{Queue: "conv", CommandID: "cmd-1", Key: "key-1"})
	if err != nil {
		t.Fatal(err)
	}

	var got commandbus.Task
	waitFor(t, 2*time.Second, func() bool {
		got, err = bus.GetTask(ctx, task.UID)
		// CodeInternal is retryable, so the ledger moves to Retrying on the
		// very first Fail rather than Dead.
		return err == nil && got.State == commandbus.StateRetrying
	})
	if got.Code != commandbus.CodeInternal {
		t.Fatalf("Code = %q, want %q", got.Code, commandbus.CodeInternal)
	}

	cancel()
	<-done
}

// Concurrency is honored: Config.Concurrency goroutines run their handler
// calls genuinely at once, not one at a time behind a shared lock.
func TestPoolHonorsItsConfiguredConcurrency(t *testing.T) {
	const n = 3
	bus := memory.Open()
	var inFlight, maxSeen atomic.Int32
	release := make(chan struct{})
	handler := func(context.Context, commandbus.Task) error {
		cur := inFlight.Add(1)
		for {
			m := maxSeen.Load()
			if cur <= m || maxSeen.CompareAndSwap(m, cur) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		return nil
	}
	p, err := consumer.New(consumer.Config{Bus: bus, Queue: "conv", Handler: handler, Concurrency: n})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runPool(ctx, p)

	for i := 0; i < n; i++ {
		if _, err := bus.Submit(ctx, commandbus.SubmitRequest{
			Queue: "conv", CommandID: commandbus.CommandID(fmt.Sprintf("cmd-%d", i)), Key: commandbus.Key(fmt.Sprintf("key-%d", i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, 2*time.Second, func() bool { return maxSeen.Load() >= int32(n) })

	close(release)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after its context was cancelled")
	}
}

// Run returns once its context ends, even with nothing ever submitted —
// the pool must not hang in Claim's own internal wait forever.
func TestPoolStopsOnContextCancelWithNoWorkEverClaimed(t *testing.T) {
	bus := memory.Open()
	p, err := consumer.New(consumer.Config{Bus: bus, Queue: "conv", Handler: noop, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := runPool(ctx, p)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after its context was cancelled with no tasks ever submitted")
	}
}
