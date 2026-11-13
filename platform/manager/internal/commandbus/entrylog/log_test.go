package entrylog

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// harness appends planned entries to a Log the way a backend does, numbering
// them itself.
type harness struct {
	t   *testing.T
	log *Log
	seq uint64
	now time.Time
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, log: New(commandbus.DefaultInventory()), now: t0}
}

func (h *harness) apply(ap Append) {
	h.t.Helper()
	if want := h.log.lastSubject(ap.Entry); ap.ExpectLast != want {
		h.t.Fatalf("plan expected the subject to end at %d, the log has it at %d", ap.ExpectLast, want)
	}
	h.seq++
	h.log.Apply(h.seq, ap.Entry)
}

// lastSubject is the sequence the entry's own subject currently ends at.
func (l *Log) lastSubject(e Entry) uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if e.Type == TypeMsg {
		return tail(l.lastMsg[keyRef{e.Queue, e.Key}])
	}
	return tail(l.lastEv[keyRef{e.Queue, e.Key}])
}

func (h *harness) submit(cmd, key string) commandbus.Task {
	h.t.Helper()
	sub, err := h.log.PlanSubmit(commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(cmd), Key: commandbus.Key(key)}, h.now)
	if err != nil {
		h.t.Fatalf("PlanSubmit: %v", err)
	}
	if sub.Existing != nil {
		return *sub.Existing
	}
	h.apply(sub.Append)
	return sub.Task
}

func (h *harness) claim() (commandbus.Task, bool) {
	h.t.Helper()
	plans := h.log.PlanClaim("conv", "h", h.now, 5*time.Minute)
	if len(plans) == 0 {
		return commandbus.Task{}, false
	}
	h.apply(plans[0].Append)
	return plans[0].Task, true
}

func (h *harness) task(uid string) commandbus.Task {
	h.t.Helper()
	got, ok := h.log.Task(uid, h.now)
	if !ok {
		h.t.Fatalf("task %s unknown", uid)
	}
	return got
}

func TestFoldFollowsTheLastEntry(t *testing.T) {
	h := newHarness(t)
	task := h.submit("c1", "k")
	if got := h.task(task.UID); got.State != commandbus.StateQueued || got.SubmittedAt != t0 {
		t.Fatalf("after the message: %+v", got)
	}
	claimed, _ := h.claim()
	if got := h.task(task.UID); got.State != commandbus.StateRunning || got.Attempt != 1 || got.Lease.Token != claimed.Lease.Token {
		t.Fatalf("after a claim: %+v", got)
	}
	f, err := h.log.PlanFail(task.UID, claimed.Lease.Token, commandbus.CodeConflict, "later", h.now)
	if err != nil {
		t.Fatal(err)
	}
	h.apply(f.Append)
	if got := h.task(task.UID); got.State != commandbus.StateRetrying || got.Code != commandbus.CodeConflict || got.Lease != nil || !reflect.DeepEqual(got, f.Task) {
		t.Fatalf("after a retryable failure: %+v, planned %+v", got, f.Task)
	}
	h.now = h.now.Add(time.Hour)
	again, _ := h.claim()
	ap, err := h.log.PlanSettle(task.UID, again.Lease.Token, h.now)
	if err != nil {
		t.Fatal(err)
	}
	h.apply(ap)
	if _, ok := h.log.Task(task.UID, h.now); ok {
		t.Fatalf("a consumed task is still in the read model")
	}
	if again := h.submit("c1", "k"); again.UID != task.UID || again.State != commandbus.StateSucceeded || again.Attempt != 2 {
		t.Fatalf("a resubmit inside the dedup window = %+v, want the settled task", again)
	}
	if seqs := h.log.TakeConsumed(h.now, time.Minute); len(seqs) != 0 {
		t.Fatalf("entries handed over before the grace: %v", seqs)
	}
	seqs := h.log.TakeConsumed(h.now.Add(time.Minute), time.Minute)
	if want := []uint64{1, 2, 3, 4, 5}; !reflect.DeepEqual(seqs, want) {
		t.Fatalf("consumed sequences = %v, want every entry of the task %v", seqs, want)
	}
	if again := h.log.TakeConsumed(h.now.Add(time.Hour), 0); len(again) != 0 {
		t.Fatalf("a removal was offered twice: %v", again)
	}
	h.now = h.now.Add(2 * time.Minute) // past the window
	if again := h.submit("c1", "k"); again.UID == task.UID {
		t.Fatalf("an id past its dedup window must create a new task")
	}
}

func TestNonRetryableCodeEndsDeadAndParks(t *testing.T) {
	h := newHarness(t)
	task := h.submit("c1", "k")
	claimed, _ := h.claim()
	f, err := h.log.PlanFail(task.UID, claimed.Lease.Token, commandbus.CodeForbidden, "no", h.now)
	if err != nil {
		t.Fatal(err)
	}
	h.apply(f.Append)
	got := h.task(task.UID)
	if got.State != commandbus.StateDead || !got.Parked(h.now) || got.DeadAt != h.now {
		t.Fatalf("after a terminal failure: %+v", got)
	}
	if again := h.submit("c1", "k"); again.UID != task.UID {
		t.Fatalf("a parked id must suppress a resubmit")
	}
	h.now = h.now.Add(time.Hour)
	if again := h.submit("c1", "k"); again.UID == task.UID {
		t.Fatalf("a released id must create a new task")
	}
}

func TestALapsedLeaseReadsAsRetryableWithNoEntry(t *testing.T) {
	h := newHarness(t)
	task := h.submit("c1", "k")
	claimed, _ := h.claim()
	before := h.log.LastSeq()

	h.now = claimed.Lease.Until.Add(time.Second)
	got := h.task(task.UID)
	if got.State != commandbus.StateRetrying || got.Code != commandbus.CodeUnavailable || got.Lease != nil || got.Attempt != 1 {
		t.Fatalf("a lapsed lease: %+v, want retrying with code unavailable", got)
	}
	if h.log.LastSeq() != before {
		t.Fatalf("reading a lapse appended an entry")
	}
	if _, err := h.log.PlanSettle(task.UID, claimed.Lease.Token, h.now); !errors.Is(err, commandbus.ErrLeaseLost) {
		t.Fatalf("a stale token settled: %v", err)
	}
	if _, err := h.log.PlanFail(task.UID, claimed.Lease.Token, commandbus.CodeInternal, "", h.now); !errors.Is(err, commandbus.ErrLeaseLost) {
		t.Fatalf("a stale token failed the task: %v", err)
	}
	again, ok := h.claim()
	if !ok || again.Attempt != 2 || again.Code != commandbus.CodeUnavailable {
		t.Fatalf("redelivery: %+v, %v", again, ok)
	}
}

func TestALapseWithSpentAttemptsReadsAsDeadAndIsClosedByAnEntry(t *testing.T) {
	inv := []commandbus.Declaration{{Queue: "conv", Policy: commandbus.Policy{Attempts: 1, ParkCoolOff: time.Minute}}}
	h := &harness{t: t, log: New(inv), now: t0}
	task := h.submit("c1", "k")
	claimed, _ := h.claim()
	h.now = claimed.Lease.Until.Add(time.Second)
	if got := h.task(task.UID); got.State != commandbus.StateDead || got.DeadAt != claimed.Lease.Until {
		t.Fatalf("a lapse on the last attempt: %+v, want dead at the lapse", got)
	}
	plans := h.log.PlanLapsedDeaths(h.now)
	if len(plans) != 1 || !plans[0].Append.Entry.Dead || plans[0].Task.State != commandbus.StateDead {
		t.Fatalf("PlanLapsedDeaths = %+v, want one dead entry", plans)
	}
	h.apply(plans[0].Append)
	if again := h.log.PlanLapsedDeaths(h.now); len(again) != 0 {
		t.Fatalf("a task closed by an entry was planned again: %+v", again)
	}
	if got := h.task(task.UID); got.State != commandbus.StateDead {
		t.Fatalf("after the dead entry: %+v", got)
	}
}

func TestExpiryIsDerivedFromTheTTL(t *testing.T) {
	h := newHarness(t)
	sub, err := h.log.PlanSubmit(commandbus.SubmitRequest{Queue: "out.x", Kind: commandbus.KindOutEphemeral, CommandID: "e", Key: "k"}, h.now)
	if err != nil {
		t.Fatal(err)
	}
	h.apply(sub.Append)
	h.now = h.now.Add(3 * time.Minute)
	got := h.task(sub.Task.UID)
	if got.State != commandbus.StateExpired || !got.State.Settled() {
		t.Fatalf("a queued task past its TTL: %+v, want expired", got)
	}
	if plans := h.log.PlanClaim("out.x", "h", h.now, time.Minute); len(plans) != 0 {
		t.Fatalf("an expired task was offered for a claim")
	}
}

func TestClaimIsFIFOAndARetryKeepsItsPlace(t *testing.T) {
	h := newHarness(t)
	a, b := h.submit("a", "ka"), h.submit("b", "kb")
	first, _ := h.claim()
	if first.UID != a.UID {
		t.Fatalf("claimed %s first, want the oldest %s", first.UID, a.UID)
	}
	f, _ := h.log.PlanFail(a.UID, first.Lease.Token, commandbus.CodeConflict, "", h.now)
	h.apply(f.Append)
	if next, _ := h.claim(); next.UID != b.UID {
		t.Fatalf("claimed %s while a backs off, want %s", next.UID, b.UID)
	}
	c := h.submit("c", "kc")
	h.now = h.now.Add(time.Hour)
	if again, _ := h.claim(); again.UID != a.UID {
		t.Fatalf("claimed %s, want the retry %s ahead of the newer %s", again.UID, a.UID, c.UID)
	}
}

func TestKeyBlocksUntilTheEarlierTaskSettles(t *testing.T) {
	h := newHarness(t)
	first, second := h.submit("a", "k"), h.submit("b", "k")
	claimed, _ := h.claim()
	if claimed.UID != first.UID {
		t.Fatalf("claimed %s, want %s", claimed.UID, first.UID)
	}
	if _, ok := h.claim(); ok {
		t.Fatalf("the second task of a key was offered while the first runs")
	}
	ap, _ := h.log.PlanSettle(first.UID, claimed.Lease.Token, h.now)
	h.apply(ap)
	if next, ok := h.claim(); !ok || next.UID != second.UID {
		t.Fatalf("after the first settled: %+v, %v", next, ok)
	}
}

func TestTheClaimIsConditionalOnTheKeysLastEntry(t *testing.T) {
	h := newHarness(t)
	h.submit("a", "k")
	plans := h.log.PlanClaim("conv", "x", h.now, time.Minute)
	rival := h.log.PlanClaim("conv", "y", h.now, time.Minute)
	if plans[0].Append.ExpectLast != rival[0].Append.ExpectLast || plans[0].Append.ExpectLast != 0 {
		t.Fatalf("two claims planned from one view must carry the same condition: %d, %d", plans[0].Append.ExpectLast, rival[0].Append.ExpectLast)
	}
	h.apply(plans[0].Append) // the first lands; the rival's condition (0) no longer holds
	if h.log.lastSubject(rival[0].Append.Entry) == rival[0].Append.ExpectLast {
		t.Fatalf("the key's event subject did not move, so a rival claim would also land")
	}
}

func TestApplyIgnoresAReplayedSequence(t *testing.T) {
	h := newHarness(t)
	task := h.submit("a", "k")
	claimed, _ := h.claim()
	plans := h.log.PlanClaim("conv", "h", h.now, time.Minute)
	if len(plans) != 0 {
		t.Fatalf("a running task was offered for a claim: %+v", plans)
	}
	h.log.Apply(2, Entry{Type: TypeClaimed, UID: task.UID, Queue: "conv", Key: "k", Attempt: 9, Token: "forged"}) // seq 2 was already applied
	if got := h.task(task.UID); got.Lease.Token != claimed.Lease.Token || got.Attempt != 1 {
		t.Fatalf("a replayed sequence changed the task: %+v", got)
	}
}

func TestRunningAfterACommandIDListsLaterTasksInLogOrder(t *testing.T) {
	h := newHarness(t)
	for _, id := range []string{"a", "b", "c"} {
		h.submit(id, "k-"+id)
	}
	for range 3 {
		h.claim()
	}
	ids := func(after commandbus.CommandID) []commandbus.CommandID {
		var out []commandbus.CommandID
		for _, task := range h.log.Running([]commandbus.Queue{"conv"}, after, h.now) {
			out = append(out, task.CommandID)
		}
		return out
	}
	for after, want := range map[commandbus.CommandID][]commandbus.CommandID{
		"":        {"a", "b", "c"},
		"a":       {"b", "c"},
		"c":       nil,
		"unknown": {"a", "b", "c"},
	} {
		if got := ids(after); !reflect.DeepEqual(got, want) {
			t.Errorf("after %q: %v, want %v", after, got, want)
		}
	}
}

func TestPruneForgetsOnlySettledTasksOutsideTheirWindow(t *testing.T) {
	h := newHarness(t)
	dead := h.submit("dead", "k1")
	live := h.submit("live", "k2")
	claimed, _ := h.claim()
	f, _ := h.log.PlanFail(dead.UID, claimed.Lease.Token, commandbus.CodeForbidden, "", h.now)
	h.apply(f.Append)

	if n := h.log.Prune(h.now, h.now.Add(time.Hour)); n != 0 {
		t.Fatalf("pruned %d tasks inside the park cool-off", n)
	}
	h.now = h.now.Add(10 * time.Minute)
	if n := h.log.Prune(h.now, h.now.Add(-time.Minute)); n != 1 {
		t.Fatalf("pruned %d tasks, want the one dead past its cool-off", n)
	}
	if _, ok := h.log.Task(dead.UID, h.now); ok {
		t.Fatalf("a pruned task is still readable")
	}
	if _, ok := h.log.Task(live.UID, h.now); !ok {
		t.Fatalf("an unsettled task was pruned")
	}
}

func TestPlanSubmitRefusesWhatTheContractRefuses(t *testing.T) {
	l := New(commandbus.DefaultInventory())
	for name, req := range map[string]commandbus.SubmitRequest{
		"no command id": {Queue: "conv", Key: "k"},
		"no key":        {Queue: "conv", CommandID: "c"},
	} {
		if _, err := l.PlanSubmit(req, t0); !errors.Is(err, commandbus.ErrInvalidRequest) {
			t.Errorf("%s: %v, want ErrInvalidRequest", name, err)
		}
	}
	if _, err := l.PlanSubmit(commandbus.SubmitRequest{Queue: "nope", CommandID: "c", Key: "k"}, t0); !errors.Is(err, commandbus.ErrUnknownQueue) {
		t.Errorf("unknown queue: %v, want ErrUnknownQueue", err)
	}
}

func TestEntryEventDescribesTheStateItLeaves(t *testing.T) {
	for _, c := range []struct {
		e    Entry
		want commandbus.TaskState
	}{
		{Entry{Type: TypeMsg}, commandbus.StateQueued},
		{Entry{Type: TypeClaimed}, commandbus.StateRunning},
		{Entry{Type: TypeFailed}, commandbus.StateRetrying},
		{Entry{Type: TypeFailed, Dead: true}, commandbus.StateDead},
		{Entry{Type: TypeSucceeded}, commandbus.StateSucceeded},
	} {
		if got := c.e.Event(7); got.State != c.want || got.Sequence != 7 {
			t.Errorf("%s dead=%v: event %+v, want state %s at sequence 7", c.e.Type, c.e.Dead, got, c.want)
		}
	}
}
