package httpapi

import (
	"context"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
)

func TestNewInvocationIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		id := newInvocationID()
		if id == "" {
			t.Fatal("an invocationId must never be empty")
		}
		if seen[id] {
			t.Fatalf("invocationId %q repeated", id)
		}
		seen[id] = true
	}
}

// acceptAndEnqueue stamps ONE invocationId onto every request in a batch
// (design D-9 step 7/D-7's cascade), and returns a taskId per request, in
// submission order.
func TestAcceptAndEnqueueStampsOneInvocationIDAcrossABatch(t *testing.T) {
	s, _, _ := busServer(t)
	resp, err := s.acceptAndEnqueue(context.Background(),
		commandbus.SubmitRequest{Queue: ConvQueue, CommandID: "c1", Key: "root",
			Payload: convtask.Payload{Verb: convtask.VerbDelete, Conversation: "a"}.Encode()},
		commandbus.SubmitRequest{Queue: ConvQueue, CommandID: "c2", Key: "root",
			Payload: convtask.Payload{Verb: convtask.VerbDelete, Conversation: "root"}.Encode()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resp.InvocationID == "" {
		t.Fatal("want a non-empty invocationId")
	}
	if len(resp.TaskIDs) != 2 || resp.TaskIDs[0] == "" || resp.TaskIDs[1] == "" || resp.TaskIDs[0] == resp.TaskIDs[1] {
		t.Fatalf("want two distinct non-empty taskIds, got %v", resp.TaskIDs)
	}
	// Submitted tasks actually carry that invocationId — verified by reading
	// the first one back.
	task, err := s.Bus.GetTask(context.Background(), resp.TaskIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.InvocationID != resp.InvocationID {
		t.Fatalf("task.InvocationID = %q, want %q", task.InvocationID, resp.InvocationID)
	}
}

// A CommandID missing is a producer bug, not a transient failure — Submit's
// own ErrInvalidRequest surfaces rather than being swallowed.
func TestAcceptAndEnqueuePropagatesASubmitFailure(t *testing.T) {
	s, _, _ := busServer(t)
	if _, err := s.acceptAndEnqueue(context.Background(),
		commandbus.SubmitRequest{Queue: ConvQueue, Key: "root", Payload: []byte("{}")}); err == nil {
		t.Fatal("want an error for a request missing its CommandID")
	}
}
