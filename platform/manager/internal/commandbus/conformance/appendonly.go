package conformance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// Scenarios of the append-only contract (design D-2, spec "Every queue is
// append-only"). Each is walked per declared queue, for both backends, against
// the entries the backing reports.

// entriesEventually waits for the backing to report at least n entries. A
// backend that writes through a stream may report an entry a moment after the
// call that appended it returned.
func entriesEventually(t *testing.T, b Backing, n int) []entrylog.Entry {
	t.Helper()
	deadline := time.Now().Add(scenarioTimeout)
	for {
		got := b.Entries()
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func entryTypes(es []entrylog.Entry) []entrylog.Type {
	out := make([]entrylog.Type, len(es))
	for i, e := range es {
		out[i] = e.Type
	}
	return out
}

// testNothingIsRewritten walks one task through submit, claim, a retryable
// failure, a second claim and completion. Every step appends one entry, and
// each earlier snapshot of the log is an unchanged prefix of every later one.
// Completing the task, which consumes it, removes its entries from the log, like
// a regular queue's: the only removal the bus makes, and never a rewrite.
func testNothingIsRewritten(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	var snapshots [][]entrylog.Entry
	step := func(wantTotal int) {
		t.Helper()
		snapshots = append(snapshots, entriesEventually(t, b, wantTotal))
		if got := len(snapshots[len(snapshots)-1]); got != wantTotal {
			t.Fatalf("log holds %d entries, want %d: %v", got, wantTotal, entryTypes(snapshots[len(snapshots)-1]))
		}
	}

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-append", "key-append"))
	mustNoErr(t, err, "Submit")
	step(1)
	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	step(2)
	mustNoErr(t, bus.Fail(ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeConflict, "retry"), "Fail")
	step(3)
	b.Advance(d.Policy.Backoff.Max + time.Second)
	again, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim again")
	if again.Attempt != 2 {
		t.Fatalf("Attempt = %d, want 2", again.Attempt)
	}
	step(4)

	final := snapshots[len(snapshots)-1]
	want := []entrylog.Type{entrylog.TypeMsg, entrylog.TypeClaimed, entrylog.TypeFailed, entrylog.TypeClaimed}
	if got := entryTypes(final); !reflect.DeepEqual(got, want) {
		t.Fatalf("entry types = %v, want %v", got, want)
	}
	for _, e := range final {
		if e.UID != task.UID {
			t.Fatalf("an entry names %s, want every entry of the log to name %s", e.UID, task.UID)
		}
	}
	for i, snap := range snapshots {
		if !reflect.DeepEqual(snap, final[:len(snap)]) {
			t.Fatalf("snapshot %d changed after it was read: an entry was rewritten", i)
		}
	}

	mustNoErr(t, bus.Settle(ctx, again.UID, again.Lease.Token), "Settle")
	b.Compact()
	if left := b.Entries(); len(left) != 0 {
		t.Fatalf("a consumed task left %d entries in the log: %v", len(left), entryTypes(left))
	}
}

// testSubmitIsOneWriteUnderARacingConsumer is the regression for the incident
// that motivated the append-only log: a consumer claiming a task between a
// submit's create and its second write failed the submit with NATS 10071. A
// submit now makes no write after its append, so every one succeeds while
// consumers claim and settle as fast as tasks appear.
func testSubmitIsOneWriteUnderARacingConsumer(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	const submits, keys, consumers = 200, 20, 4
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	raceCtx, stop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var settled atomic.Int64
	for c := 0; c < consumers; c++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			consumer := b.Bus()
			for raceCtx.Err() == nil {
				task, err := consumer.Claim(raceCtx, queue, fmt.Sprintf("racer-%d", n))
				if err != nil {
					return
				}
				if err := consumer.Settle(ctx, task.UID, task.Lease.Token); err == nil {
					settled.Add(1)
				}
			}
		}(c)
	}

	uids := map[string]bool{}
	for i := 0; i < submits; i++ {
		task, err := bus.Submit(ctx, req(queue, d.Kind,
			commandbus.CommandID(fmt.Sprintf("cmd-race-%d", i)), commandbus.Key(fmt.Sprintf("key-race-%d", i%keys))))
		if err != nil {
			stop()
			wg.Wait()
			t.Fatalf("Submit %d of %d failed while a consumer was claiming: %v", i, submits, err)
		}
		uids[task.UID] = true
	}
	deadline := time.Now().Add(scenarioTimeout)
	for settled.Load() < submits && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	stop()
	wg.Wait()

	if len(uids) != submits {
		t.Fatalf("%d submits made %d distinct tasks", submits, len(uids))
	}
	// Every task was consumed, so the log holds none of their entries; what a
	// racing consumer could never do is append one twice or rewrite it.
	seen := map[string]int{}
	for _, e := range b.Entries() {
		seen[fmt.Sprintf("%s/%s/%d/%s", e.UID, e.Type, e.Attempt, e.Token)]++
	}
	for entry, n := range seen {
		if n != 1 {
			t.Fatalf("entry %s was appended %d times, want once", entry, n)
		}
	}
}

// testStateFoldsFromEntries checks state is a function of the entries alone: a
// handle opened after every step, with nothing but the log to go on, reads the
// state the step left.
func testStateFoldsFromEntries(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	read := func(uid string) commandbus.Task {
		t.Helper()
		got, err := b.Bus().GetTask(ctx, uid)
		mustNoErr(t, err, "GetTask through a fresh handle")
		return got
	}

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-fold", "key-fold"))
	mustNoErr(t, err, "Submit")
	if got := read(task.UID); got.State != commandbus.StateQueued || got.Attempt != 0 {
		t.Fatalf("after submit: state %v attempt %d, want queued 0", got.State, got.Attempt)
	}

	claimed, err := bus.Claim(ctx, queue, "folder")
	mustNoErr(t, err, "Claim")
	got := read(task.UID)
	if got.State != commandbus.StateRunning || got.Attempt != 1 || got.Lease == nil ||
		got.Lease.Token != claimed.Lease.Token || got.Lease.Holder != "folder" {
		t.Fatalf("after claim: %+v, want running attempt 1 under the claim's lease", got)
	}

	mustNoErr(t, bus.Fail(ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeConflict, "fold me"), "Fail")
	got = read(task.UID)
	if got.State != commandbus.StateRetrying || got.Code != commandbus.CodeConflict || got.Detail != "fold me" || got.Lease != nil {
		t.Fatalf("after a retryable failure: %+v, want retrying with the failure recorded", got)
	}

	b.Advance(d.Policy.Backoff.Max + time.Second)
	again, err := bus.Claim(ctx, queue, "folder")
	mustNoErr(t, err, "Claim again")
	if again.Attempt != 2 {
		t.Fatalf("Attempt = %d, want 2", again.Attempt)
	}
	mustNoErr(t, bus.Settle(ctx, again.UID, again.Lease.Token), "Settle")
	// A consumed task has no entries left to fold: it is absent, through any
	// handle, and its id stays suppressed for the queue's dedup window only.
	if _, err := b.Bus().GetTask(ctx, task.UID); !errors.Is(err, commandbus.ErrNotFound) {
		t.Fatalf("GetTask of a consumed task = %v, want ErrNotFound", err)
	}
}

// testLapsedLeaseRedeliversWithNoWrite pins that a lapse needs no entry: the
// task reads as retryable once its lease has passed, the next claim appends
// the one entry that matters, and a completion under the stale token is
// refused and appends nothing.
func testLapsedLeaseRedeliversWithNoWrite(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	b := open()
	lease := b.LeaseDuration()
	if d.Policy.TTL > 0 && d.Policy.TTL <= lease {
		t.Skip("queue's TTL does not outlive a lapsed lease")
	}
	ctx, cancel := withTimeout()
	defer cancel()
	bus := b.Bus()

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-lapse-write", "key-lapse-write"))
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(ctx, queue, "crashed")
	mustNoErr(t, err, "Claim")
	before := entriesEventually(t, b, 2)

	b.Advance(lease + time.Second)
	got, err := bus.GetTask(ctx, task.UID)
	mustNoErr(t, err, "GetTask after the lapse")
	if got.State != commandbus.StateRetrying || got.Code != commandbus.CodeUnavailable || got.Lease != nil {
		t.Fatalf("after the lapse: %+v, want retrying with code unavailable and no lease", got)
	}
	if err := bus.Settle(ctx, claimed.UID, claimed.Lease.Token); err != commandbus.ErrLeaseLost {
		t.Fatalf("Settle under the lapsed lease = %v, want ErrLeaseLost", err)
	}
	if after := b.Entries(); !reflect.DeepEqual(after, before) {
		t.Fatalf("a lapse and a refused completion wrote entries: %d before, %d after", len(before), len(after))
	}

	redelivered, err := bus.Claim(ctx, queue, "rescuer")
	mustNoErr(t, err, "Claim after the lapse")
	if redelivered.UID != task.UID || redelivered.Attempt != 2 {
		t.Fatalf("redelivered %s attempt %d, want %s attempt 2", redelivered.UID, redelivered.Attempt, task.UID)
	}
	final := entriesEventually(t, b, 3)
	if len(final) != 3 || !reflect.DeepEqual(final[:2], before) || final[2].Type != entrylog.TypeClaimed || final[2].Attempt != 2 {
		t.Fatalf("log after the redelivery = %v, want the original two entries plus one claim of attempt 2", entryTypes(final))
	}
}

// testRunningResumesAfterACommandID pins the cursor of design D-16: Running
// after a command id lists the later running tasks in log order, and an id the
// log does not know (never seen, or consumed and so removed) lists everything.
func testRunningResumesAfterACommandID(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	ids := []commandbus.CommandID{"cmd-after-a", "cmd-after-b", "cmd-after-c", "cmd-after-d"}
	for _, id := range ids {
		_, err := bus.Submit(ctx, req(queue, d.Kind, id, commandbus.Key("key-"+id)))
		mustNoErr(t, err, "Submit "+string(id))
	}
	claimed := map[commandbus.CommandID]commandbus.Task{}
	for range ids {
		task, err := bus.Claim(ctx, queue, "c1")
		mustNoErr(t, err, "Claim")
		claimed[task.CommandID] = task
	}
	mustNoErr(t, bus.Settle(ctx, claimed["cmd-after-b"].UID, claimed["cmd-after-b"].Lease.Token), "Settle b")

	queues := []commandbus.Queue{queue}
	ofIDs := func(ts []commandbus.Task) []commandbus.CommandID {
		out := make([]commandbus.CommandID, len(ts))
		for i, task := range ts {
			out[i] = task.CommandID
		}
		return out
	}
	for _, c := range []struct {
		name  string
		after commandbus.CommandID
		want  []commandbus.CommandID
	}{
		{"no cursor", "", []commandbus.CommandID{"cmd-after-a", "cmd-after-c", "cmd-after-d"}},
		{"a running entry", "cmd-after-a", []commandbus.CommandID{"cmd-after-c", "cmd-after-d"}},
		// A consumed entry is removed from the log, so its id is unknown: the
		// resume starts from the oldest unacknowledged entry, which is exactly
		// what is still running.
		{"an acknowledged entry", "cmd-after-b", []commandbus.CommandID{"cmd-after-a", "cmd-after-c", "cmd-after-d"}},
		{"the last entry", "cmd-after-d", nil},
		{"an unknown id", "cmd-never-seen", []commandbus.CommandID{"cmd-after-a", "cmd-after-c", "cmd-after-d"}},
	} {
		got, err := bus.Running(ctx, queues, c.after)
		mustNoErr(t, err, "Running after "+c.name)
		if g := ofIDs(got); !reflect.DeepEqual(g, c.want) && (len(g) != 0 || len(c.want) != 0) {
			t.Fatalf("Running after %s = %v, want %v", c.name, g, c.want)
		}
	}
}
