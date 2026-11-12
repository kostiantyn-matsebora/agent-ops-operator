package httpapi

import (
	"context"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// busServer builds a Server wired for the "conv" queue's accept-then-
// process split: a fake client, the real Router over it, and a fresh
// commandbus/memory Bus — the shape the hand-back report's own test
// guidance names ("run the registered consumer pool or call the handler
// function directly against a memory bus").
func busServer(t *testing.T, objs ...client.Object) (*Server, client.Client, commandbus.Bus) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(objs...).Build()
	store, runs, blobs := testConversations(t, c, "agent-ops"), testRuns(t, c, "agent-ops"), testBlobs(c, "agent-ops")
	bus := memory.Open()
	// async-command-bus task 6: OpQueue shares the SAME Bus as the Server's
	// own "conv" queue side, exactly as cmd/manager wires one Bus value
	// into both.
	q := &chat.OpQueue{Conversations: store, Runs: runs, Registry: chat.NewRegistry(), Bus: bus}
	router := &chat.Router{Client: c, Reader: c, Conversations: store, Runs: runs, Blobs: blobs, Namespace: "agent-ops", Ops: q}
	s := &Server{
		Reader: c, Client: c, Conversations: store, Runs: runs, Blobs: blobs,
		Namespace: "agent-ops", AdapterToken: "bus-test-master", Ops: q, Router: router, Bus: bus,
	}
	return s, c, bus
}

// drainOneConvTask claims exactly one task off ConvQueue and runs
// HandleConvTask against it DIRECTLY, settling or failing it on the bus by
// hand — the "call the handler function directly against a memory bus"
// half of the test guidance, for a test that only wants the handler's own
// returned error, not the bus's derived ErrorCode (see
// runConvTaskViaPoolToTerminal for that).
func drainOneConvTask(t *testing.T, s *Server, bus commandbus.Bus) (commandbus.Task, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task, err := bus.Claim(ctx, ConvQueue, "test")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	handleErr := s.HandleConvTask(context.Background(), task)
	leaseToken := ""
	if task.Lease != nil {
		leaseToken = task.Lease.Token
	}
	if handleErr == nil {
		if err := bus.Settle(context.Background(), task.UID, leaseToken); err != nil {
			t.Fatalf("settle: %v", err)
		}
	} else if err := bus.Fail(context.Background(), task.UID, leaseToken,
		commandbus.ParseErrorCode(handleErr.Error()), handleErr.Error()); err != nil {
		t.Fatalf("fail: %v", err)
	}
	return task, handleErr
}

// runConvTaskViaPoolToTerminal runs a REAL consumer.Pool against bus until
// taskUID reaches a terminal State (commandbus.TaskState.Settled) — the
// "run the registered consumer pool" half of the test guidance, needed
// whenever a test cares about the ErrorCode the bus itself recorded
// (consumer.Pool's own codeFor/WithCode logic is unexported, so hand-rolling
// Fail's arguments in a test, as drainOneConvTask above does, cannot
// reproduce it).
func runConvTaskViaPoolToTerminal(t *testing.T, s *Server, bus commandbus.Bus, taskUID string) commandbus.Task {
	t.Helper()
	pool, err := consumer.New(consumer.Config{Bus: bus, Queue: ConvQueue, Handler: s.HandleConvTask, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = pool.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := bus.GetTask(context.Background(), taskUID)
		if err != nil {
			cancel()
			<-done
			t.Fatal(err)
		}
		if task.State.Settled() {
			cancel()
			<-done
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("task %s never reached a terminal state", taskUID)
	return commandbus.Task{}
}
