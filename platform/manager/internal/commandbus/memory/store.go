package memory

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	mathrand "math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// idKey scopes dedup exactly as a producer submits: one queue, one kind,
// one CommandID. A producer that retries its own work keeps all three the
// same; nothing in the contract asks a dedup key to mean anything across a
// kind or queue boundary.
type idKey struct {
	queue commandbus.Queue
	kind  commandbus.Kind
	id    commandbus.CommandID
}

// store is the shared engine one or more handle values read and write
// through. Every exported-looking method below is called with mu held by
// its caller unless named ...Locked, in which case the CALLER holds mu —
// the suffix marks the few that assume it rather than take it themselves.
type store struct {
	mu sync.Mutex

	// baseClock defaults to time.Now, so a live local-development process
	// sees ordinary wall-clock time pass. offsetNanos is added on top of
	// it and only ever moved by Advance, which a deterministic test calls
	// to fast-forward past a backoff, a lease or a TTL without sleeping —
	// an atomic because now() is called from methods that already hold mu.
	baseClock     func() time.Time
	offsetNanos   atomic.Int64
	inventory     []commandbus.Declaration
	leaseDuration time.Duration

	tasks map[string]*commandbus.Task // by UID
	ids   map[idKey]string            // latest task UID per dedup key
	order uint64                      // next Task.Order to assign
	seq   commandbus.Sequence         // next TaskEvent.Sequence to assign

	events []commandbus.TaskEvent // the whole gap-free history (see package doc)
	wake   chan struct{}          // closed and replaced on every state change
}

func (s *store) now() time.Time {
	return s.baseClock().Add(time.Duration(s.offsetNanos.Load()))
}

// advance fast-forwards the store's clock by d, waking every Claim and Feed
// call so a lapsed lease or an elapsed backoff is seen at once rather than
// on the next real-time poll.
func (s *store) advance(d time.Duration) {
	s.offsetNanos.Add(int64(d))
	s.mu.Lock()
	s.notifyLocked()
	s.mu.Unlock()
}

// leaseDur reports the lease duration every Claim on s grants — bus-wide,
// not per queue, because D-2 never varies it by queue. Exposed so a
// deterministic test can advance exactly past it rather than guessing.
func (s *store) leaseDur() time.Duration {
	return s.leaseDuration
}

// notifyLocked wakes every Claim and Feed call waiting on s. Called with mu
// held.
func (s *store) notifyLocked() {
	close(s.wake)
	s.wake = make(chan struct{})
}

// appendEventLocked records t's current state as the next feed entry.
// Called with mu held.
func (s *store) appendEventLocked(t *commandbus.Task) {
	s.seq++
	s.events = append(s.events, commandbus.TaskEvent{
		Sequence: s.seq, UID: t.UID, Queue: t.Queue, Kind: t.Kind, Key: t.Key,
		CommandID: t.CommandID, InvocationID: t.InvocationID,
		State: t.State, Attempt: t.Attempt, Code: t.Code,
	})
}

func (s *store) submit(req commandbus.SubmitRequest) (commandbus.Task, error) {
	if req.CommandID == "" || req.Key == "" {
		return commandbus.Task{}, fmt.Errorf("%w: CommandID and Key are both required", commandbus.ErrInvalidRequest)
	}
	decl, ok := commandbus.Resolve(s.inventory, req.Queue, req.Kind)
	if !ok {
		return commandbus.Task{}, fmt.Errorf("%w: queue=%q kind=%q", commandbus.ErrUnknownQueue, req.Queue, req.Kind)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.reapLocked(now)

	key := idKey{req.Queue, req.Kind, req.CommandID}
	if uid, exists := s.ids[key]; exists {
		if t := s.tasks[uid]; t != nil && t.Suppresses(now, decl.Policy.DedupWindow) {
			return *t, nil
		}
	}

	s.order++
	task := &commandbus.Task{
		UID: newUID(), CommandID: req.CommandID, Queue: req.Queue, Kind: req.Kind, Key: req.Key,
		Payload: req.Payload, InvocationID: req.InvocationID,
		State: commandbus.StateQueued, MaxAttempts: decl.Policy.Attempts,
		Order: s.order, SubmittedAt: now, UpdatedAt: now,
	}
	s.tasks[task.UID] = task
	s.ids[key] = task.UID
	s.appendEventLocked(task)
	s.notifyLocked()
	return *task, nil
}

func (s *store) claim(ctx context.Context, queue commandbus.Queue, consumerID string) (commandbus.Task, error) {
	for {
		s.mu.Lock()
		now := s.now()
		s.reapLocked(now)
		if t, found := s.claimEligibleLocked(queue, consumerID, now); found {
			s.notifyLocked()
			s.mu.Unlock()
			return *t, nil
		}
		wait := s.nextWakeLocked(queue, now)
		waitCh := s.wake
		s.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return commandbus.Task{}, fmt.Errorf("commandbus: claim on %q canceled: %w", queue, ctx.Err())
		case <-waitCh:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// claimEligibleLocked picks the oldest claimable task on queue — Queued, or
// Retrying whose NotBefore has passed — skipping one blocked by an earlier,
// unsettled task sharing its Key or by its invocation's in-flight cap.
// Called with mu held.
func (s *store) claimEligibleLocked(queue commandbus.Queue, consumerID string, now time.Time) (*commandbus.Task, bool) {
	var candidates []*commandbus.Task
	for _, t := range s.tasks {
		if t.Queue != queue {
			continue
		}
		switch {
		case t.State == commandbus.StateQueued:
			candidates = append(candidates, t)
		case t.State == commandbus.StateRetrying && !t.NotBefore.After(now):
			candidates = append(candidates, t)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Order < candidates[j].Order })

	for _, t := range candidates {
		if s.blockedByKeyLocked(queue, t) || s.blockedByInFlightCapLocked(queue, t) {
			continue
		}
		t.State = commandbus.StateRunning
		t.Attempt++
		t.Lease = &commandbus.Lease{Token: newLeaseToken(), Holder: consumerID, Until: now.Add(s.leaseDuration)}
		t.UpdatedAt = now
		s.appendEventLocked(t)
		return t, true
	}
	return nil, false
}

// blockedByKeyLocked reports whether an earlier, still-unsettled task on
// queue shares candidate's Key — the per-key serialization rule, enforced
// at claim time over the whole task set rather than by partitioning.
func (s *store) blockedByKeyLocked(queue commandbus.Queue, candidate *commandbus.Task) bool {
	for _, t := range s.tasks {
		if t.Queue == queue && t.Key == candidate.Key && t.Order < candidate.Order && !t.State.Settled() {
			return true
		}
	}
	return false
}

// blockedByInFlightCapLocked reports whether candidate's invocation already
// has as many tasks Running on queue as its declaration allows.
func (s *store) blockedByInFlightCapLocked(queue commandbus.Queue, candidate *commandbus.Task) bool {
	if candidate.InvocationID == "" {
		return false
	}
	decl, ok := commandbus.Resolve(s.inventory, queue, candidate.Kind)
	if !ok || decl.Policy.InFlightCapPerInvocation <= 0 {
		return false
	}
	running := 0
	for _, t := range s.tasks {
		if t.Queue == queue && t.InvocationID == candidate.InvocationID && t.State == commandbus.StateRunning {
			running++
		}
	}
	return running >= decl.Policy.InFlightCapPerInvocation
}

// nextWakeLocked bounds how long Claim should sleep when nothing is
// eligible yet: the earliest instant a Retrying task's backoff elapses or a
// Running task's lease lapses, capped at defaultPollInterval as a floor for
// "nothing scheduled, just stay responsive to Submit/Settle/Fail".
func (s *store) nextWakeLocked(queue commandbus.Queue, now time.Time) time.Duration {
	wait := defaultPollInterval
	for _, t := range s.tasks {
		if t.Queue != queue {
			continue
		}
		if t.State == commandbus.StateRetrying && t.NotBefore.After(now) {
			if d := t.NotBefore.Sub(now); d < wait {
				wait = d
			}
		}
		if t.State == commandbus.StateRunning && t.Lease != nil && t.Lease.Until.After(now) {
			if d := t.Lease.Until.Sub(now); d < wait {
				wait = d
			}
		}
	}
	if wait <= 0 {
		wait = time.Millisecond
	}
	return wait
}

// reapLocked applies the two transitions that happen without any caller
// asking for them: a Running task whose lease has lapsed fails as
// CodeUnavailable (design D-6), and a Queued or Retrying task past its
// queue's TTL expires. Called with mu held, at the top of every method
// that reads or writes task state, so neither can ever be seen stale.
func (s *store) reapLocked(now time.Time) {
	for _, t := range s.tasks {
		// Sequential, not exclusive: a lease lapsed long enough ago can
		// also already be past the queue's own TTL (counted from the
		// task's original SubmittedAt), and the second check must see the
		// state the first one just produced — a lapse that outlives the
		// TTL expires the task instead of handing it back for redelivery
		// (design D-4/D-5; the system this models is a restart that must
		// not deliver a now-stale notice).
		if t.State == commandbus.StateRunning && t.Lease != nil && now.After(t.Lease.Until) {
			s.failLocked(t, now, commandbus.CodeUnavailable, "lease lapsed")
		}
		if t.State == commandbus.StateQueued || t.State == commandbus.StateRetrying {
			decl, ok := commandbus.Resolve(s.inventory, t.Queue, t.Kind)
			if ok && decl.Policy.TTL > 0 && now.After(t.SubmittedAt.Add(decl.Policy.TTL)) {
				t.State = commandbus.StateExpired
				t.UpdatedAt = now
				s.appendEventLocked(t)
			}
		}
	}
}

func (s *store) settle(taskUID, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.reapLocked(now)
	t := s.tasks[taskUID]
	if t == nil {
		return commandbus.ErrNotFound
	}
	if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != leaseToken {
		return commandbus.ErrLeaseLost
	}
	t.State = commandbus.StateSucceeded
	t.SucceededAt = now
	t.UpdatedAt = now
	t.Lease = nil
	s.appendEventLocked(t)
	s.notifyLocked()
	return nil
}

func (s *store) fail(taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.reapLocked(now)
	t := s.tasks[taskUID]
	if t == nil {
		return commandbus.ErrNotFound
	}
	if t.State != commandbus.StateRunning || t.Lease == nil || t.Lease.Token != leaseToken {
		return commandbus.ErrLeaseLost
	}
	s.failLocked(t, now, code, detail)
	s.notifyLocked()
	return nil
}

// failLocked applies one failed attempt to t: retrying with backoff while
// the code is retryable and attempts remain, dead otherwise (design D-6).
// Shared by Fail and reapLocked's lease-lapse path, which is exactly a
// Fail(CodeUnavailable) the consumer never got to make. Called with mu
// held.
func (s *store) failLocked(t *commandbus.Task, now time.Time, code commandbus.ErrorCode, detail string) {
	code = commandbus.ParseErrorCode(string(code))
	decl, _ := commandbus.Resolve(s.inventory, t.Queue, t.Kind)
	t.Code, t.Detail, t.Lease = code, detail, nil
	if code.Retryable() && t.Attempt < t.MaxAttempts {
		t.State = commandbus.StateRetrying
		t.NotBefore = now.Add(backoffFor(decl.Policy.Backoff, t.Attempt))
	} else {
		t.State = commandbus.StateDead
		t.DeadAt = now
		t.ParkedUntil = now.Add(decl.Policy.ParkCoolOff)
	}
	t.UpdatedAt = now
	s.appendEventLocked(t)
}

// backoffFor computes attempt's delay within [b.Min, b.Max], doubling per
// attempt and clamped, then jittered uniformly down to zero when b.Jitter
// — "full jitter", chosen because it is the one AWS's own backoff writeup
// found to cost the fewest retries under contention, and this package has
// no reason to prefer a narrower spread.
func backoffFor(b commandbus.Backoff, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := b.Min
	for i := 1; i < attempt && d < b.Max; i++ {
		d *= 2
	}
	if d > b.Max {
		d = b.Max
	}
	if d < b.Min {
		d = b.Min
	}
	if b.Jitter && d > 0 {
		return time.Duration(mathrand.Int63n(int64(d))) + 1
	}
	return d
}

func (s *store) getTask(uid string) (commandbus.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked(s.now())
	t := s.tasks[uid]
	if t == nil {
		return commandbus.Task{}, commandbus.ErrNotFound
	}
	return *t, nil
}

func (s *store) deadLetters(queue commandbus.Queue) ([]commandbus.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked(s.now())
	var out []commandbus.Task
	for _, t := range s.tasks {
		if t.State != commandbus.StateDead {
			continue
		}
		if queue != "" && t.Queue != queue {
			continue
		}
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeadAt.Equal(out[j].DeadAt) {
			return out[i].Order < out[j].Order
		}
		return out[i].DeadAt.Before(out[j].DeadAt)
	})
	return out, nil
}

func (s *store) stats(queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	if queue == "" {
		return commandbus.QueueStats{}, fmt.Errorf("%w: Stats needs a queue", commandbus.ErrInvalidRequest)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.reapLocked(now)

	out := commandbus.QueueStats{Queue: queue, Kind: kind}
	var oldestPending time.Time
	markPending := func(t *commandbus.Task) {
		out.Pending++
		if oldestPending.IsZero() || t.SubmittedAt.Before(oldestPending) {
			oldestPending = t.SubmittedAt
		}
	}
	for _, t := range s.tasks {
		if t.Queue != queue || t.Kind != kind {
			continue
		}
		switch t.State {
		case commandbus.StateQueued:
			markPending(t)
		case commandbus.StateRetrying:
			if t.NotBefore.After(now) {
				out.Scheduled++
			} else {
				markPending(t)
			}
		case commandbus.StateRunning:
			out.Running++
		case commandbus.StateDead:
			out.Dead++
			if t.Parked(now) {
				out.Parked++
			}
		}
	}
	if !oldestPending.IsZero() {
		out.OldestPendingAge = now.Sub(oldestPending)
	}
	return out, nil
}

// feed streams s.events from resume (0 meaning "from now") until ctx is done.
// A read of s.events is always copied out under mu before being sent, so a
// slow receiver never holds the store lock.
//
// "From now" is resolved HERE, synchronously, before the streaming goroutine
// is even started — not inside it. A caller that Submits or Claims right
// after Feed returns must see that event, and the goroutine is not
// guaranteed to run before the caller's next line does.
func (s *store) feed(ctx context.Context, resume commandbus.Sequence) <-chan commandbus.TaskEvent {
	s.mu.Lock()
	next := len(s.events)
	if resume != 0 {
		next = int(resume)
	}
	s.mu.Unlock()

	out := make(chan commandbus.TaskEvent)
	go func() {
		defer close(out)
		for {
			s.mu.Lock()
			from := min(next, len(s.events))
			pending := append([]commandbus.TaskEvent(nil), s.events[from:]...)
			waitCh := s.wake
			s.mu.Unlock()

			for _, ev := range pending {
				select {
				case out <- ev:
					next++
				case <-ctx.Done():
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-waitCh:
			}
		}
	}()
	return out
}

func newUID() string        { return randomHex(12) }
func newLeaseToken() string { return randomHex(16) }

func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns a short read without an error, and an
	// error here means the platform's entropy source is broken — not a
	// condition this backend can usefully recover from.
	if _, err := cryptorand.Read(b); err != nil {
		panic(fmt.Errorf("commandbus/memory: reading random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}
