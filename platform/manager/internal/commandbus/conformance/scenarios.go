package conformance

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

const (
	msgSubmit1       = "Submit 1"
	msgSubmit2       = "Submit 2"
	cmdPark          = "cmd-park"
	keyPark          = "key-park"
	fmtClaimedWanted = "claimed %s, want %s"
)

// withTimeout bounds one scenario's whole body — long enough for a slow CI
// runner, short enough that a genuine deadlock fails the test rather than
// hanging the suite.
func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), scenarioTimeout)
}

func req(queue commandbus.Queue, kind commandbus.Kind, cmdID commandbus.CommandID, key commandbus.Key) commandbus.SubmitRequest {
	return commandbus.SubmitRequest{Queue: queue, Kind: kind, CommandID: cmdID, Key: key, Payload: []byte("payload")}
}

func mustNoErr(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// deadTask submits one task and drives it straight to Dead with a
// non-retryable code (design D-6), regardless of how many attempts d's
// policy allows — the shortest deterministic path to the state every
// park/release and dead-letter scenario needs.
func deadTask(t *testing.T, ctx context.Context, bus commandbus.Bus, queue commandbus.Queue, d commandbus.Declaration, cmdID commandbus.CommandID, key commandbus.Key) commandbus.Task {
	t.Helper()
	_, err := bus.Submit(ctx, req(queue, d.Kind, cmdID, key))
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(ctx, queue, "dead-maker")
	mustNoErr(t, err, "Claim")
	if err := bus.Fail(ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeForbidden, "terminal by design"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	got, err := bus.GetTask(ctx, claimed.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateDead {
		t.Fatalf("state = %v, want Dead", got.State)
	}
	return got
}

func testDurableAccept(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-accept", "key-accept"))
	mustNoErr(t, err, "Submit")
	if task.State != commandbus.StateQueued {
		t.Fatalf("state = %v, want Queued", task.State)
	}
	got, err := bus.GetTask(ctx, task.UID)
	mustNoErr(t, err, "GetTask")
	if got.UID != task.UID || got.Queue != queue || got.Kind != d.Kind {
		t.Fatalf("GetTask = %+v, want a durable copy of %+v", got, task)
	}
}

func testIdempotentSubmit(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	request := req(queue, d.Kind, "cmd-idem", "key-idem")

	first, err := bus.Submit(ctx, request)
	mustNoErr(t, err, msgSubmit1)
	second, err := bus.Submit(ctx, request)
	mustNoErr(t, err, msgSubmit2)
	if second.UID != first.UID {
		t.Fatalf("a repeated CommandID minted a second task: %s != %s", second.UID, first.UID)
	}
}

// testDedupAcrossEveryState checks each named state of design D-4 one at a
// time, in its own Backing, rather than trying to walk one task through all
// of them — a task cannot be both Succeeded and Dead.
func testDedupAcrossEveryState(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	env := dedupEnv{ctx: ctx, open: open, d: d, queue: queue}

	cases := []struct {
		name string
		run  func(t *testing.T, e dedupEnv)
	}{
		{"queued", dedupQueued},
		{"running", dedupRunning},
		{"retrying", dedupRetrying},
		{"succeeded within window", dedupSucceededWithinWindow},
		{"succeeded past window", dedupSucceededPastWindow},
		{"dead", dedupDead},
		{"dead past cool-off", dedupDeadPastCoolOff},
		{"expired", dedupExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, env) })
	}
}

// dedupEnv is what each per-state dedup scenario shares.
type dedupEnv struct {
	ctx   context.Context
	open  func() Backing
	d     commandbus.Declaration
	queue commandbus.Queue
}

func dedupQueued(t *testing.T, e dedupEnv) {
	bus := e.open().Bus()
	request := req(e.queue, e.d.Kind, "cmd-q", "key-q")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, msgSubmit1)
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, msgSubmit2)
	if second.UID != first.UID {
		t.Fatalf("a Queued task did not suppress a resubmit")
	}
}

func dedupRunning(t *testing.T, e dedupEnv) {
	bus := e.open().Bus()
	request := req(e.queue, e.d.Kind, "cmd-r", "key-r")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(e.ctx, e.queue, "c1")
	mustNoErr(t, err, "Claim")
	if claimed.UID != first.UID {
		t.Fatalf("claimed the wrong task")
	}
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit while running")
	if second.UID != first.UID {
		t.Fatalf("a Running task did not suppress a resubmit")
	}
}

func dedupRetrying(t *testing.T, e dedupEnv) {
	bus := e.open().Bus()
	request := req(e.queue, e.d.Kind, "cmd-rt", "key-rt")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(e.ctx, e.queue, "c1")
	mustNoErr(t, err, "Claim")
	mustNoErr(t, bus.Fail(e.ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeConflict, "retry me"), "Fail")
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit while retrying")
	if second.UID != first.UID {
		t.Fatalf("a Retrying task did not suppress a resubmit")
	}
}

func dedupSucceededWithinWindow(t *testing.T, e dedupEnv) {
	if e.d.Policy.DedupWindow <= 0 {
		t.Skip("queue declares no dedup window")
	}
	bus := e.open().Bus()
	request := req(e.queue, e.d.Kind, "cmd-s", "key-s")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(e.ctx, e.queue, "c1")
	mustNoErr(t, err, "Claim")
	mustNoErr(t, bus.Settle(e.ctx, claimed.UID, claimed.Lease.Token), "Settle")
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit right after success")
	if second.UID != first.UID {
		t.Fatalf("a just-Succeeded task did not suppress a resubmit within its window")
	}
}

func dedupSucceededPastWindow(t *testing.T, e dedupEnv) {
	if e.d.Policy.DedupWindow <= 0 {
		t.Skip("queue declares no dedup window")
	}
	b := e.open()
	bus := b.Bus()
	request := req(e.queue, e.d.Kind, "cmd-sp", "key-sp")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(e.ctx, e.queue, "c1")
	mustNoErr(t, err, "Claim")
	mustNoErr(t, bus.Settle(e.ctx, claimed.UID, claimed.Lease.Token), "Settle")
	b.Advance(e.d.Policy.DedupWindow + time.Second)
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit past the window")
	if second.UID == first.UID {
		t.Fatalf("a Succeeded task past its window still suppressed a resubmit")
	}
}

func dedupDead(t *testing.T, e dedupEnv) {
	bus := e.open().Bus()
	first := deadTask(t, e.ctx, bus, e.queue, e.d, "cmd-d", "key-d")
	second, err := bus.Submit(e.ctx, req(e.queue, e.d.Kind, "cmd-d", "key-d"))
	mustNoErr(t, err, "Submit right after dead")
	switch {
	case e.d.Policy.ParkCoolOff > 0 && second.UID != first.UID:
		t.Fatalf("a just-Dead task did not suppress a resubmit during its park cool-off")
	case e.d.Policy.ParkCoolOff <= 0 && second.UID == first.UID:
		t.Fatalf("a Dead task with no park cool-off (D-5: never parked) still suppressed an immediate resubmit")
	}
}

func dedupDeadPastCoolOff(t *testing.T, e dedupEnv) {
	if e.d.Policy.ParkCoolOff <= 0 {
		t.Skip("queue never parks (D-5)")
	}
	b := e.open()
	bus := b.Bus()
	first := deadTask(t, e.ctx, bus, e.queue, e.d, "cmd-dp", "key-dp")
	b.Advance(e.d.Policy.ParkCoolOff + time.Second)
	second, err := bus.Submit(e.ctx, req(e.queue, e.d.Kind, "cmd-dp", "key-dp"))
	mustNoErr(t, err, "Submit past the cool-off")
	if second.UID == first.UID {
		t.Fatalf("a Dead task past its cool-off still suppressed a resubmit")
	}
}

func dedupExpired(t *testing.T, e dedupEnv) {
	if e.d.Policy.TTL <= 0 {
		t.Skip("queue declares no TTL")
	}
	b := e.open()
	bus := b.Bus()
	request := req(e.queue, e.d.Kind, "cmd-e", "key-e")
	first, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit")
	b.Advance(e.d.Policy.TTL + time.Second)
	got, err := bus.GetTask(e.ctx, first.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateExpired {
		t.Fatalf("state = %v, want Expired", got.State)
	}
	second, err := bus.Submit(e.ctx, request)
	mustNoErr(t, err, "Submit after expiry")
	if second.UID == first.UID {
		t.Fatalf("an Expired task still suppressed a resubmit")
	}
}

func testParkThenRelease(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	first := deadTask(t, ctx, bus, queue, d, cmdPark, keyPark)

	suppressed, err := bus.Submit(ctx, req(queue, d.Kind, cmdPark, keyPark))
	mustNoErr(t, err, "Submit immediately after dead")
	if d.Policy.ParkCoolOff > 0 && suppressed.UID != first.UID {
		t.Fatalf("a parked id was not suppressed immediately after going dead")
	}

	b.Advance(d.Policy.ParkCoolOff + time.Second)
	released, err := bus.Submit(ctx, req(queue, d.Kind, cmdPark, keyPark))
	mustNoErr(t, err, "Submit after the cool-off")
	if released.UID == first.UID {
		t.Fatalf("the id was not released once its park cool-off elapsed")
	}
	if _, err := bus.GetTask(ctx, first.UID); err != nil {
		t.Fatalf("the original dead task's record should still read back: %v", err)
	}
}

func testFIFOWithRetryKeepingPlace(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	a, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-a", "key-a"))
	mustNoErr(t, err, "Submit A")
	claimedA, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim A")
	if claimedA.UID != a.UID {
		t.Fatalf("claimed %s, want A (%s)", claimedA.UID, a.UID)
	}
	mustNoErr(t, bus.Fail(ctx, claimedA.UID, claimedA.Lease.Token, commandbus.CodeConflict, "retry"), "Fail A")

	bTask, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-b", "key-b"))
	mustNoErr(t, err, "Submit B")
	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim (expect B, A still backing off)")
	if claimed.UID != bTask.UID {
		t.Fatalf("claimed %s while A is retrying, want B (%s)", claimed.UID, bTask.UID)
	}
	mustNoErr(t, bus.Settle(ctx, claimed.UID, claimed.Lease.Token), "Settle B")

	// A's backoff elapses, and a brand-new C arrives after it — A must
	// still come first: its Order was assigned before C's, and a retry
	// never moves a task to the back of the queue.
	b.Advance(d.Policy.Backoff.Max + time.Second)
	cTask, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-c", "key-c"))
	mustNoErr(t, err, "Submit C")
	claimed, err = bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim (expect A back)")
	if claimed.UID != a.UID {
		t.Fatalf("claimed %s, want A (%s) to keep its place ahead of C (%s)", claimed.UID, a.UID, cTask.UID)
	}
}

func testSerializationPerKeyUnderParallelConsumers(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()

	first, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-serial-1", "key-serial"))
	mustNoErr(t, err, msgSubmit1)
	second, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-serial-2", "key-serial"))
	mustNoErr(t, err, msgSubmit2)

	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim first")
	if claimed.UID != first.UID {
		t.Fatalf("claimed %s first, want %s", claimed.UID, first.UID)
	}

	// Several consumers race the same queue while the key's first task is
	// still Running. None may be handed the second.
	const consumers = 8
	results := make(chan commandbus.Task, consumers)
	raceCtx, raceCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer raceCancel()
	var wg sync.WaitGroup
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if task, err := bus.Claim(raceCtx, queue, fmt.Sprintf("racer-%d", n)); err == nil {
				results <- task
			}
		}(i)
	}
	wg.Wait()
	close(results)
	for got := range results {
		if got.UID == second.UID {
			t.Fatalf("the second task of a shared key was claimed while the first was still running")
		}
	}

	mustNoErr(t, bus.Settle(ctx, claimed.UID, claimed.Lease.Token), "Settle first")
	freed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim after settling the first")
	if freed.UID != second.UID {
		t.Fatalf("claimed %s after settling the first, want %s", freed.UID, second.UID)
	}
}

func testLeaseCompletionThroughASecondHandle(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	first, second := b.Bus(), b.Bus()

	task, err := first.Submit(ctx, req(queue, d.Kind, "cmd-handle", "key-handle"))
	mustNoErr(t, err, "Submit")
	claimed, err := first.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	if claimed.UID != task.UID {
		t.Fatalf("claimed the wrong task")
	}

	if err := second.Settle(ctx, claimed.UID, claimed.Lease.Token); err != nil {
		t.Fatalf("Settle through a second handle on the same store: %v", err)
	}
	got, err := first.GetTask(ctx, claimed.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateSucceeded {
		t.Fatalf("state = %v, want Succeeded", got.State)
	}
}

func testLeaseLapseRedelivery(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	b := open()
	lease := b.LeaseDuration()
	if d.Policy.TTL > 0 && d.Policy.TTL <= lease {
		// A lapse this long also outlives the queue's own TTL, counted from
		// the ORIGINAL SubmittedAt. Redelivery cannot be the right outcome
		// there — see testLeaseLapseExpiresUnderAShortTTL, which covers
		// exactly this queue instead.
		t.Skip("queue's TTL does not outlive a lapsed lease; see testLeaseLapseExpiresUnderAShortTTL")
	}
	ctx, cancel := withTimeout()
	defer cancel()
	bus := b.Bus()

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-lapse", "key-lapse"))
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	if claimed.Attempt != 1 {
		t.Fatalf("Attempt = %d, want 1", claimed.Attempt)
	}

	b.Advance(lease + time.Second)
	redelivered, err := bus.Claim(ctx, queue, "c2")
	mustNoErr(t, err, "Claim after lapse")
	if redelivered.UID != task.UID {
		t.Fatalf("claimed %s, want the lapsed task %s redelivered", redelivered.UID, task.UID)
	}
	if redelivered.Attempt != 2 {
		t.Fatalf("Attempt = %d, want 2 (the lapse counted as a failed attempt)", redelivered.Attempt)
	}
	if redelivered.Code != commandbus.CodeUnavailable {
		t.Fatalf("Code = %q, want %q", redelivered.Code, commandbus.CodeUnavailable)
	}
	if err := bus.Settle(ctx, claimed.UID, claimed.Lease.Token); err == nil {
		t.Fatalf("Settle through the lapsed lease unexpectedly succeeded")
	}
}

// testLeaseLapseExpiresUnderAShortTTL covers the queues
// testLeaseLapseRedelivery skips: a lease long enough to outlive the
// queue's own TTL means a lapsed, retried task is immediately past its
// TTL (counted from its original SubmittedAt, design D-4) and expires
// rather than coming back for redelivery. A restart must not deliver a
// notice this stale — the exact reason D-5 gives ephemeral sends a TTL.
func testLeaseLapseExpiresUnderAShortTTL(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	b := open()
	lease := b.LeaseDuration()
	if d.Policy.TTL <= 0 || d.Policy.TTL > lease {
		t.Skip("queue's TTL outlives the bus's lease; see testLeaseLapseRedelivery")
	}
	ctx, cancel := withTimeout()
	defer cancel()
	bus := b.Bus()

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-lapse-ttl", "key-lapse-ttl"))
	mustNoErr(t, err, "Submit")
	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	if claimed.UID != task.UID {
		t.Fatalf(fmtClaimedWanted, claimed.UID, task.UID)
	}

	b.Advance(lease + time.Second)
	got, err := bus.GetTask(ctx, task.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateExpired {
		t.Fatalf("state = %v, want Expired (a lapse this long also outlives the queue's TTL)", got.State)
	}

	// A fresh arrival sharing the same key, submitted only now, must not be
	// blocked by the one that just expired.
	next, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-lapse-ttl-2", "key-lapse-ttl"))
	mustNoErr(t, err, "Submit (same key, after the expiry)")
	freed, err := bus.Claim(ctx, queue, "c2")
	mustNoErr(t, err, "Claim")
	if freed.UID != next.UID {
		t.Fatalf(fmtClaimedWanted, freed.UID, next.UID)
	}
}

func testBoundedRetryIntoDead(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	task, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-bounded", "key-bounded"))
	mustNoErr(t, err, "Submit")

	for attempt := 1; attempt <= d.Policy.Attempts; attempt++ {
		claimed, err := bus.Claim(ctx, queue, "c1")
		mustNoErr(t, err, fmt.Sprintf("Claim attempt %d", attempt))
		if claimed.UID != task.UID {
			t.Fatalf(fmtClaimedWanted, claimed.UID, task.UID)
		}
		if claimed.Attempt != attempt {
			t.Fatalf("Attempt = %d, want %d", claimed.Attempt, attempt)
		}
		mustNoErr(t, bus.Fail(ctx, claimed.UID, claimed.Lease.Token, commandbus.CodeConflict, "keep failing"),
			fmt.Sprintf("Fail attempt %d", attempt))
		if attempt < d.Policy.Attempts {
			b.Advance(d.Policy.Backoff.Max + time.Second)
		}
	}

	got, err := bus.GetTask(ctx, task.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateDead {
		t.Fatalf("state = %v, want Dead after exhausting %d attempts", got.State, d.Policy.Attempts)
	}

	emptyCtx, emptyCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer emptyCancel()
	if _, err := bus.Claim(emptyCtx, queue, "c1"); err == nil {
		t.Fatalf("Claim returned a task after the only one went Dead")
	}
}

func testTTLExpiryCountsAsSettled(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	if d.Policy.TTL <= 0 {
		t.Skip("queue declares no TTL")
	}
	ctx, cancel := withTimeout()
	defer cancel()
	b := open()
	bus := b.Bus()

	first, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-ttl-1", "key-ttl"))
	mustNoErr(t, err, "Submit first")

	b.Advance(d.Policy.TTL + time.Second)

	got, err := bus.GetTask(ctx, first.UID)
	mustNoErr(t, err, "GetTask")
	if got.State != commandbus.StateExpired {
		t.Fatalf("state = %v, want Expired", got.State)
	}

	// Submitted only now, sharing first's key: a second task minted BEFORE
	// the advance would itself have aged past the same TTL, so this is the
	// one way to test "an Expired task must not block its key" without
	// the fresh arrival expiring right along with it.
	second, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-ttl-2", "key-ttl"))
	mustNoErr(t, err, "Submit second (same key, after the expiry)")

	// The expired task must not block the second, even though it was
	// never claimed and never explicitly settled (design D-4: "expired
	// counts as settled for ordering").
	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	if claimed.UID != second.UID {
		t.Fatalf("claimed %s, want %s — an Expired task must release its key", claimed.UID, second.UID)
	}
}

func testDeadLetterListing(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()

	// deadTask's own Claim takes the oldest eligible task in the queue, so
	// it must run before anything else is queued, or it claims that other
	// task instead of its own.
	dead := deadTask(t, ctx, bus, queue, d, "cmd-dl-dead", "key-dl-dead")
	alive, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-dl-alive", "key-dl-alive"))
	mustNoErr(t, err, "Submit alive")

	byQueue, err := bus.DeadLetters(ctx, queue)
	mustNoErr(t, err, "DeadLetters(queue)")
	if !containsUID(byQueue, dead.UID) {
		t.Fatalf("DeadLetters(%q) missing %s", queue, dead.UID)
	}
	if containsUID(byQueue, alive.UID) {
		t.Fatalf("DeadLetters(%q) listed a task that is not dead", queue)
	}

	all, err := bus.DeadLetters(ctx, "")
	mustNoErr(t, err, `DeadLetters("")`)
	if !containsUID(all, dead.UID) {
		t.Fatalf(`DeadLetters("") missing %s`, dead.UID)
	}
}

func containsUID(tasks []commandbus.Task, uid string) bool {
	for _, task := range tasks {
		if task.UID == uid {
			return true
		}
	}
	return false
}

func testPerQueueStatistics(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()

	deadTask(t, ctx, bus, queue, d, "cmd-stat-dead", "key-stat-dead")

	running, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-stat-running", "key-stat-running"))
	mustNoErr(t, err, "Submit running")
	claimedRunning, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim running")
	if claimedRunning.UID != running.UID {
		t.Fatalf(fmtClaimedWanted, claimedRunning.UID, running.UID)
	}

	pending, err := bus.Submit(ctx, req(queue, d.Kind, "cmd-stat-pending", "key-stat-pending"))
	mustNoErr(t, err, "Submit pending")

	stats, err := bus.Stats(ctx, queue, d.Kind)
	mustNoErr(t, err, "Stats")
	if stats.Queue != queue || stats.Kind != d.Kind {
		t.Fatalf("Stats identifies %q/%q, want %q/%q", stats.Queue, stats.Kind, queue, d.Kind)
	}
	if stats.Running != 1 {
		t.Fatalf("Running = %d, want 1", stats.Running)
	}
	if stats.Pending < 1 {
		t.Fatalf("Pending = %d, want at least 1 (%s still queued)", stats.Pending, pending.UID)
	}
	if stats.Dead != 1 {
		t.Fatalf("Dead = %d, want 1", stats.Dead)
	}
	if stats.OldestPendingAge <= 0 {
		t.Fatalf("OldestPendingAge = %v, want > 0", stats.OldestPendingAge)
	}
	// async-command-bus task 6: the identities OpQueue.Stats() needs beside
	// the counts — an op id (CommandID) and an ordering key alone cannot be
	// recovered from a count.
	if stats.OldestPendingID != pending.CommandID || stats.OldestPendingKey != pending.Key {
		t.Fatalf("OldestPendingID/Key = %q/%q, want %q/%q",
			stats.OldestPendingID, stats.OldestPendingKey, pending.CommandID, pending.Key)
	}
	if stats.OldestRunningAge <= 0 {
		t.Fatalf("OldestRunningAge = %v, want > 0", stats.OldestRunningAge)
	}
	if stats.OldestRunningID != running.CommandID || stats.OldestRunningKey != running.Key {
		t.Fatalf("OldestRunningID/Key = %q/%q, want %q/%q",
			stats.OldestRunningID, stats.OldestRunningKey, running.CommandID, running.Key)
	}
}

// testPerInvocationInFlightCap is not one of design D-14/2.6's named
// scenarios — those are silent on the in-flight cap — but D-5 declares it
// as part of every queue's policy, and test-coverage.md owes it the same
// coverage as any other declared behaviour. Skipped on a queue whose
// policy declares no cap (every one except "conv" today).
func testPerInvocationInFlightCap(t *testing.T, open func() Backing, d commandbus.Declaration, queue commandbus.Queue) {
	invCap := d.Policy.InFlightCapPerInvocation
	if invCap <= 0 {
		t.Skip("queue declares no per-invocation in-flight cap")
	}
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()

	for i := 0; i < invCap+1; i++ {
		_, err := bus.Submit(ctx, commandbus.SubmitRequest{
			Queue: queue, Kind: d.Kind,
			CommandID:    commandbus.CommandID(fmt.Sprintf("cmd-cap-%d", i)),
			Key:          commandbus.Key(fmt.Sprintf("key-cap-%d", i)),
			InvocationID: "invocation-cap",
		})
		mustNoErr(t, err, "Submit")
	}

	var claimed []commandbus.Task
	for i := 0; i < invCap; i++ {
		task, err := bus.Claim(ctx, queue, fmt.Sprintf("c%d", i))
		mustNoErr(t, err, "Claim")
		claimed = append(claimed, task)
	}

	overCapCtx, overCapCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer overCapCancel()
	if _, err := bus.Claim(overCapCtx, queue, "over-cap"); err == nil {
		t.Fatalf("claimed a task past the invocation's in-flight cap of %d", invCap)
	}

	mustNoErr(t, bus.Settle(ctx, claimed[0].UID, claimed[0].Lease.Token), "Settle")
	if _, err := bus.Claim(ctx, queue, "after-settle"); err != nil {
		t.Fatalf("Claim after settling freed a cap slot: %v", err)
	}
}

func testGapFreeResumableFeed(t *testing.T, open func() Backing) {
	ctx, cancel := withTimeout()
	defer cancel()
	bus := open().Bus()
	const queue, kind = commandbus.Queue("conv"), commandbus.KindDefault

	for i := 0; i < 3; i++ {
		_, err := bus.Submit(ctx, req(queue, kind,
			commandbus.CommandID(fmt.Sprintf("cmd-feed-%d", i)), commandbus.Key(fmt.Sprintf("key-feed-%d", i))))
		mustNoErr(t, err, "Submit")
	}

	// resume 0 means "from now" (mirroring internal/state.Watch), so the
	// three submits above are deliberately NOT what this feed observes —
	// only the claim and settle that follow are.
	events, err := bus.Feed(ctx, 0)
	mustNoErr(t, err, "Feed")

	claimed, err := bus.Claim(ctx, queue, "c1")
	mustNoErr(t, err, "Claim")
	mustNoErr(t, bus.Settle(ctx, claimed.UID, claimed.Lease.Token), "Settle")

	var got []commandbus.TaskEvent
	for len(got) < 2 {
		select {
		case ev := <-events:
			got = append(got, ev)
		case <-time.After(scenarioTimeout):
			t.Fatalf("feed delivered %d events, want at least 2 (claim, settle)", len(got))
		}
	}
	var last commandbus.Sequence
	for _, ev := range got {
		if last != 0 && ev.Sequence != last+1 {
			t.Fatalf("feed sequence jumped from %d to %d", last, ev.Sequence)
		}
		last = ev.Sequence
	}
	if got[0].UID != claimed.UID || got[0].State != commandbus.StateRunning {
		t.Fatalf("first event = %+v, want the claim (Running) on %s", got[0], claimed.UID)
	}
	if got[1].State != commandbus.StateSucceeded {
		t.Fatalf("second event state = %v, want Succeeded", got[1].State)
	}

	resumed, err := bus.Feed(ctx, got[0].Sequence)
	mustNoErr(t, err, "Feed resumed")
	select {
	case ev := <-resumed:
		if ev.Sequence != got[1].Sequence {
			t.Fatalf("resumed feed's first event has sequence %d, want %d (strictly after %d)",
				ev.Sequence, got[1].Sequence, got[0].Sequence)
		}
	case <-time.After(scenarioTimeout):
		t.Fatalf("resumed feed delivered nothing")
	}
}
