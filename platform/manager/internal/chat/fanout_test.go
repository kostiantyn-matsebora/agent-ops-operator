package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// fanOutFixture is a conversation bound to the named channels, each served by
// its own adapter named after it, every thread already open.
func fanOutFixture(t *testing.T, channels ...string) (Env, *OpQueue, *state.ConversationRecord) {
	t.Helper()
	b := fake.NewClientBuilder().WithScheme(closeTestScheme(t))
	rec := &state.ConversationRecord{Name: "conv-1", ID: "uid-conv-1", Title: "t"}
	for _, ch := range channels {
		b = b.WithObjects(nsChannel(ch, "adapter-"+ch))
		rec.ChannelRefs = append(rec.ChannelRefs, ch)
		rec.Threads = append(rec.Threads, state.ThreadBinding{Channel: ch, ThreadID: "thread-" + ch})
	}
	c := b.Build()
	env := Env{Reader: c, Blobs: newTestBlobs(c), Namespace: testNS}
	return env, &OpQueue{Registry: NewRegistry(), Bus: testBus()}, rec
}

func input(id string, at time.Time, origin *state.InputOrigin) state.InputItem {
	return state.InputItem{ID: id, Type: state.InputReply, Payload: id, ReceivedAt: at, Origin: origin}
}

// Every entry reaches each bound channel exactly once, less the surface that
// already displayed it, and a second pass adds nothing.
func TestFanOutThreadReachesEachBoundChannelOnceLessTheDisplayingSurface(t *testing.T) {
	ctx := context.Background()
	env, q, rec := fanOutFixture(t, "c1", "c2", "c3")
	now := time.Now()
	esc := now.Add(-time.Hour)
	fin := now.Add(-time.Minute)
	rec.Reopens = 1
	rec.EscalatedAt, rec.EscalationMessage = &esc, "needs a human"
	// typed on c1, which shows its own messages (no ChannelAdapter says otherwise)
	rec.Inputs = []state.InputItem{input("typed", now, &state.InputOrigin{Kind: state.OriginChannel, Name: "c1", Sender: "pat"})}
	runs := []state.Run{{RunID: "run-1", Status: "succeeded", Result: "done", DeliveryTracked: true, FinishedAt: &fin}}

	FanOutThread(ctx, env, q, rec, runs)
	FanOutThread(ctx, env, q, rec, runs) // the backstop pass: nothing new

	for _, ch := range []string{"c1", "c2", "c3"} {
		ids := map[string]int{}
		for _, op := range drainCompleting(q, "adapter-"+ch) {
			ids[strings.SplitN(op.ID, ":", 2)[0]]++
		}
		want := map[string]int{"reopened": 1, "escalate": 1, "send": 1, "input": 1}
		if ch == "c1" {
			delete(want, "input") // the surface the person typed on already showed it
		}
		if len(ids) != len(want) {
			t.Fatalf("%s: ops by kind = %v, want %v", ch, ids, want)
		}
		for k, n := range want {
			if ids[k] != n {
				t.Fatalf("%s: ops by kind = %v, want %v", ch, ids, want)
			}
		}
	}
}

// A queue keeps enqueue order and nothing more, so the one fan-out point must
// enqueue in the order things happened: a run's reply before the corrective
// input its own turn caused, an input before a later run's reply, whatever
// order the record holds them in.
func TestFanOutThreadEnqueuesInCausalOrder(t *testing.T) {
	ctx := context.Background()
	env, q, rec := fanOutFixture(t, "c1")
	t0 := time.Now().Add(-time.Hour)
	finished1 := t0.Add(time.Minute)
	finished2 := t0.Add(10 * time.Minute)
	mgr := &state.InputOrigin{Kind: state.OriginManager, Name: "move-per-turn"}
	rec.Inputs = []state.InputItem{
		input("nudge", finished1.Add(time.Second), mgr), // caused by run-1's turn
		input("first", t0, mgr),
	}
	runs := []state.Run{
		{RunID: "run-2", Status: "succeeded", Result: "two", DeliveryTracked: true, FinishedAt: &finished2},
		{RunID: "run-1", Status: "succeeded", Result: "one", DeliveryTracked: true, FinishedAt: &finished1},
	}

	FanOutThread(ctx, env, q, rec, runs)

	var got []string
	for _, op := range drainCompleting(q, "adapter-c1") {
		got = append(got, op.ID)
	}
	want := []string{
		"input:conv-1:first:c1", "send:conv-1:c1:run-1", "input:conv-1:nudge:c1", "send:conv-1:c1:run-2",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("claim order\n got  %v\n want %v", got, want)
	}
}

// At one instant a run's reply precedes the input, and a run with no recorded
// finish is delivered after everything dated.
func TestFanOutThreadTiebreaksARunReplyBeforeAnInputAndUndatedLast(t *testing.T) {
	ctx := context.Background()
	env, q, rec := fanOutFixture(t, "c1")
	at := time.Now().Add(-time.Minute)
	rec.Inputs = []state.InputItem{input("same", at, &state.InputOrigin{Kind: state.OriginManager, Name: "m"})}
	runs := []state.Run{
		{RunID: "undated", Status: "succeeded", DeliveryTracked: true},
		{RunID: "tied", Status: "succeeded", DeliveryTracked: true, FinishedAt: &at},
	}

	FanOutThread(ctx, env, q, rec, runs)

	var got []string
	for _, op := range drainCompleting(q, "adapter-c1") {
		got = append(got, op.ID)
	}
	want := "send:conv-1:c1:tied input:conv-1:same:c1 send:conv-1:c1:undated"
	if strings.Join(got, " ") != want {
		t.Fatalf("claim order\n got  %v\n want %s", got, want)
	}
}

// A binding with no thread yet is skipped but left owed, so the reconcile its
// topic triggers comes back for the reply; a delivered one owes nothing; a run
// that never tracked delivery is the caller's to backfill, not sent.
func TestFanOutThreadReportsWhatIsStillOwed(t *testing.T) {
	ctx := context.Background()
	env, q, rec := fanOutFixture(t, "c1", "c2")
	rec.Threads[1].ThreadID = "" // c2's topic is not created yet
	fin := time.Now()
	runs := []state.Run{
		{RunID: "run-1", Status: "succeeded", Result: "x", DeliveryTracked: true, FinishedAt: &fin, Delivered: []string{"c1"}},
		{RunID: "legacy", Status: "succeeded", Result: "y"},
	}

	owed := FanOutThread(ctx, env, q, rec, runs)

	if len(owed) != 1 || !owed["c2"] {
		t.Fatalf("owed = %v, want only c2", owed)
	}
	if ops := drain(q, "adapter-c1"); len(ops) != 0 {
		t.Fatalf("a delivered reply and an untracked run must send nothing on c1: %+v", ops)
	}
	if ops := drain(q, "adapter-c2"); len(ops) != 0 {
		t.Fatalf("a claim-only binding must not be delivered into: %+v", ops)
	}
}

// An unbound conversation, or no queue, delivers nothing and owes nothing.
func TestFanOutThreadWithNothingBoundDeliversNothing(t *testing.T) {
	ctx := context.Background()
	env, q, rec := fanOutFixture(t)
	rec.Inputs = []state.InputItem{input("i", time.Now(), &state.InputOrigin{Kind: state.OriginManager, Name: "m"})}
	if owed := FanOutThread(ctx, env, q, rec, nil); len(owed) != 0 {
		t.Fatalf("owed = %v", owed)
	}
	if owed := FanOutThread(ctx, env, nil, rec, nil); len(owed) != 0 {
		t.Fatalf("a nil queue must deliver nothing, owed = %v", owed)
	}
}
