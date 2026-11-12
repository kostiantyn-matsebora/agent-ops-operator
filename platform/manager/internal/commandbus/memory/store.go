package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// store is the shared engine one or more handle values read and write
// through. Methods named ...Locked assume the caller holds mu.
type store struct {
	mu sync.Mutex

	baseClock func() time.Time
	// offsetNanos is how far Backing.Advance has moved the clock past the
	// base clock — atomic so now() never needs mu.
	offsetNanos   atomic.Int64
	inventory     []commandbus.Declaration
	leaseDuration time.Duration

	log *entrylog.Log
	seq uint64 // the last sequence given to an entry

	entries []entrylog.Entry       // every entry, as appended, for Backing.Entries
	events  []commandbus.TaskEvent // one per entry, the gap-free history Feed replays
	wake    chan struct{}          // closed and replaced on every append
}

func (s *store) now() time.Time {
	return s.baseClock().Add(time.Duration(s.offsetNanos.Load()))
}

func (s *store) advance(d time.Duration) {
	s.offsetNanos.Add(int64(d))
	s.mu.Lock()
	s.notifyLocked()
	s.mu.Unlock()
}

func (s *store) leaseDur() time.Duration { return s.leaseDuration }

func (s *store) notifyLocked() {
	close(s.wake)
	s.wake = make(chan struct{})
}

// appendLocked gives the entry the next sequence and applies it. Under mu the
// condition a planned append carries (the subject still ends where the plan
// saw it) always holds, so it is not re-checked: there is no second writer.
func (s *store) appendLocked(ap entrylog.Append) {
	s.seq++
	s.log.Apply(s.seq, ap.Entry)
	s.entries = append(s.entries, ap.Entry)
	s.events = append(s.events, ap.Entry.Event(s.seq))
	if ap.Entry.Type == entrylog.TypeSucceeded {
		s.removeConsumedLocked(ap.Entry.UID)
	}
	s.notifyLocked()
}

// removeConsumedLocked drops every entry of a task that succeeded: a consumed
// message leaves the log, like a regular queue's. It is the only removal the
// store ever makes, and it only ever removes the entries of a settled task. The
// Feed's history is a separate record of transitions and is not touched.
func (s *store) removeConsumedLocked(uid string) {
	s.log.TakeConsumed(s.now(), 0)
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.UID != uid {
			kept = append(kept, e)
		}
	}
	s.entries = kept
}

func (s *store) submit(req commandbus.SubmitRequest) (commandbus.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, err := s.log.PlanSubmit(req, s.now())
	if err != nil {
		return commandbus.Task{}, err
	}
	if sub.Existing != nil {
		return *sub.Existing, nil
	}
	s.appendLocked(sub.Append)
	return sub.Task, nil
}

func (s *store) claim(ctx context.Context, queue commandbus.Queue, consumerID string) (commandbus.Task, error) {
	for {
		s.mu.Lock()
		now := s.now()
		if plans := s.log.PlanClaim(queue, consumerID, now, s.leaseDuration); len(plans) > 0 {
			s.appendLocked(plans[0].Append)
			s.mu.Unlock()
			return plans[0].Task, nil
		}
		wait := s.log.NextWake(queue, now, defaultPollInterval)
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

func (s *store) settle(taskUID, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ap, err := s.log.PlanSettle(taskUID, leaseToken, s.now())
	if err != nil {
		return err
	}
	s.appendLocked(ap)
	return nil
}

func (s *store) fail(taskUID, leaseToken string, code commandbus.ErrorCode, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.log.PlanFail(taskUID, leaseToken, code, detail, s.now())
	if err != nil {
		return err
	}
	s.appendLocked(f.Append)
	return nil
}

func (s *store) getTask(uid string) (commandbus.Task, error) {
	t, ok := s.log.Task(uid, s.now())
	if !ok {
		return commandbus.Task{}, commandbus.ErrNotFound
	}
	return t, nil
}

func (s *store) deadLetters(queue commandbus.Queue) ([]commandbus.Task, error) {
	return s.log.DeadTasks(queue, s.now()), nil
}

func (s *store) running(queues []commandbus.Queue, after commandbus.CommandID) []commandbus.Task {
	return s.log.Running(queues, after, s.now())
}

func (s *store) stats(queue commandbus.Queue, kind commandbus.Kind) (commandbus.QueueStats, error) {
	return s.log.Stats(queue, kind, s.now())
}

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

// entriesSnapshot copies every entry appended so far, in order.
func (s *store) entriesSnapshot() []entrylog.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]entrylog.Entry(nil), s.entries...)
}
