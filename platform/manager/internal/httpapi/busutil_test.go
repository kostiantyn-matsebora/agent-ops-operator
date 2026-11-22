package httpapi

import (
	"context"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// assignUIDs gives every pre-seeded fixture a UID derived from its own Name
// when it carries none — the fake client, unlike a real apiserver, never
// mints one for an object seeded through WithObjects. conversation-identity
// moved CausedBy.Parent to the parent's ID, and most fixtures in this
// package still write it as the parent's Name — the trivial, legitimate
// case the design permits, where a test does not care whether the two
// differ. A test that DOES care sets a distinct UID itself, which this
// never overrides.
func assignUIDs(objs []client.Object) {
	for _, o := range objs {
		if o.GetUID() == "" {
			o.SetUID(types.UID(o.GetName()))
		}
	}
}

// withAutoUID extends an interceptor.Funcs' own Create (if any) with the
// same convention assignUIDs applies to a pre-seeded fixture, for a test
// that creates its CausedBy subtree through the client rather than seeding
// it — a Create through the fake client never mints a UID either.
func withAutoUID(funcs interceptor.Funcs) interceptor.Funcs {
	inner := funcs.Create
	funcs.Create = func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if obj.GetUID() == "" {
			obj.SetUID(types.UID(obj.GetName()))
		}
		if inner != nil {
			return inner(ctx, c, obj, opts...)
		}
		return c.Create(ctx, obj, opts...)
	}
	return funcs
}

// busServer builds a Server wired for the "conv" queue's accept-then-
// process split: a fake client, the real Router over it, and a fresh
// commandbus/memory Bus — the shape the hand-back report's own test
// guidance names ("run the registered consumer pool or call the handler
// function directly against a memory bus").
func busServer(t *testing.T, objs ...client.Object) (*Server, client.Client, commandbus.Bus) {
	t.Helper()
	assignUIDs(objs)
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(objs...).Build()
	store, runs, blobs := testConversations(t, c, "agent-ops"), testRuns(t, c, "agent-ops"), testBlobs(c, "agent-ops")
	bus := memory.Open()
	// async-command-bus task 6: OpQueue shares the SAME Bus as the Server's
	// own "conv" queue side, exactly as cmd/manager wires one Bus value
	// into both.
	q := &chat.OpQueue{Conversations: store, Runs: runs, Registry: chat.NewRegistry(), Bus: bus}
	router := &chat.Router{Client: c, Reader: c, Conversations: store, Runs: runs, Blobs: blobs, Namespaces: namespace.Namespaces{Control: "agent-ops", Conversations: "agent-ops"}, Ops: q}
	s := &Server{
		Reader: c, Client: c, Conversations: store, Runs: runs, Blobs: blobs,
		Namespaces: namespace.Namespaces{Control: "agent-ops", Conversations: "agent-ops"}, AdapterToken: "bus-test-master", Ops: q, Router: router, Bus: bus,
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

// claimOp is what an adapter does in two calls: read the oldest message, then
// acknowledge it (design D-17). Nil when nothing is offered.
func claimOp(q *chat.OpQueue, adapter string) *chat.Op {
	ctx := context.Background()
	for _, op := range q.Read(ctx, adapter, 1) {
		if _, err := q.Acknowledge(ctx, adapter, op.ID); err == nil {
			return op
		}
	}
	return nil
}
