package entrylog

import (
	"fmt"
	"sort"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// NextWake is how long a blocked claim on queue may sleep: the earliest instant
// a retrying task's backoff elapses or a running task's lease lapses, or def
// when none is ahead.
func (l *Log) NextWake(queue commandbus.Queue, now time.Time, def time.Duration) time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	wait := def
	for _, rec := range l.live[queue] {
		t := l.fold(rec, now)
		switch {
		case t.State == commandbus.StateRetrying:
			wait = sooner(wait, t.NotBefore, now)
		case t.State == commandbus.StateRunning && t.Lease != nil:
			wait = sooner(wait, t.Lease.Until, now)
		}
	}
	return max(wait, time.Millisecond)
}

func sooner(wait time.Duration, at, now time.Time) time.Duration {
	if d := at.Sub(now); d > 0 && d < wait {
		return d
	}
	return wait
}

// Running lists the tasks of queues that run under a lease still in force, in
// log order: the set a consumer that lost its connection may still hold. When
// after names a command id, only tasks appended after the latest message of
// that id (on any of queues) are listed; an id the log does not know lists
// everything, so a resume from an unknown cursor starts at the oldest
// unacknowledged entry. A lapsed lease is not listed.
func (l *Log) Running(queues []commandbus.Queue, after commandbus.CommandID, now time.Time) []commandbus.Task {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var floor uint64
	if after != "" {
		for _, q := range queues {
			for _, rec := range l.byCmd[cmdRef{q, after}] {
				floor = max(floor, rec.seq)
			}
		}
	}
	var recs []*record
	for _, q := range queues {
		for _, rec := range l.live[q] {
			if rec.seq > floor {
				recs = append(recs, rec)
			}
		}
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].seq < recs[j].seq })
	var out []commandbus.Task
	for _, rec := range recs {
		if t := l.fold(rec, now); t.State == commandbus.StateRunning && t.Lease != nil {
			out = append(out, t)
		}
	}
	return out
}

// DeadTasks lists the dead tasks of queue (every queue when empty), oldest
// death first.
func (l *Log) DeadTasks(queue commandbus.Queue, now time.Time) []commandbus.Task {
	l.mu.RLock()
	defer l.mu.RUnlock()
	type dead struct {
		t   commandbus.Task
		seq uint64
	}
	var found []dead
	for _, rec := range l.records {
		if queue != "" && rec.msg.Queue != queue {
			continue
		}
		if t := l.fold(rec, now); t.State == commandbus.StateDead {
			found = append(found, dead{t, rec.seq})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].t.DeadAt.Equal(found[j].t.DeadAt) {
			return found[i].seq < found[j].seq
		}
		return found[i].t.DeadAt.Before(found[j].t.DeadAt)
	})
	out := make([]commandbus.Task, len(found))
	for i, d := range found {
		out[i] = d.t
	}
	return out
}

// Stats snapshots the counts for one (queue, kind) pair.
func (l *Log) Stats(queue commandbus.Queue, kind commandbus.Kind, now time.Time) (commandbus.QueueStats, error) {
	if queue == "" {
		return commandbus.QueueStats{}, fmt.Errorf("%w: Stats needs a queue", commandbus.ErrInvalidRequest)
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	s := &tally{out: commandbus.QueueStats{Queue: queue, Kind: kind}, now: now}
	for _, rec := range l.records {
		if rec.msg.Queue == queue && rec.msg.Kind == kind {
			s.add(l.fold(rec, now))
		}
	}
	if !s.oldestPending.IsZero() {
		s.out.OldestPendingAge = now.Sub(s.oldestPending)
	}
	if !s.oldestRunning.IsZero() {
		s.out.OldestRunningAge = now.Sub(s.oldestRunning)
	}
	return s.out, nil
}

type tally struct {
	out                          commandbus.QueueStats
	now                          time.Time
	oldestPending, oldestRunning time.Time
}

func (s *tally) add(t commandbus.Task) {
	switch t.State {
	case commandbus.StateQueued:
		s.pending(t)
	case commandbus.StateRetrying:
		if t.NotBefore.After(s.now) {
			s.out.Scheduled++
		} else {
			s.pending(t)
		}
	case commandbus.StateRunning:
		s.out.Running++
		if s.oldestRunning.IsZero() || t.UpdatedAt.Before(s.oldestRunning) {
			s.oldestRunning = t.UpdatedAt
			s.out.OldestRunningID, s.out.OldestRunningKey = t.CommandID, t.Key
		}
	case commandbus.StateDead:
		s.out.Dead++
		if t.Parked(s.now) {
			s.out.Parked++
		}
	}
}

func (s *tally) pending(t commandbus.Task) {
	s.out.Pending++
	if s.oldestPending.IsZero() || t.SubmittedAt.Before(s.oldestPending) {
		s.oldestPending = t.SubmittedAt
		s.out.OldestPendingID, s.out.OldestPendingKey = t.CommandID, t.Key
	}
}
