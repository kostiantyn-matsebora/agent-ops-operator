package memory

import (
	"context"
	"fmt"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// feedEntry is one retained entry with the position the log gave it.
type feedEntry struct {
	seq uint64
	commandbus.FeedEntry
}

// feedLog is one feed: its entries ascending by seq, trimmed from the front.
type feedLog struct {
	nextSeq uint64
	entries []feedEntry
	last    map[commandbus.CommandID]uint64 // latest seq per id, for dedup and cursors
}

func (s *store) feedLogLocked(feed commandbus.Queue) *feedLog {
	if s.feeds == nil {
		s.feeds = map[commandbus.Queue]*feedLog{}
	}
	f := s.feeds[feed]
	if f == nil {
		f = &feedLog{nextSeq: 1, last: map[commandbus.CommandID]uint64{}}
		s.feeds[feed] = f
	}
	return f
}

// trimLocked drops what is past the policy's age or count: the only way an
// entry leaves a feed.
func (s *store) trimLocked(f *feedLog, p commandbus.FeedPolicy) {
	drop := 0
	if p.MaxAge > 0 {
		cutoff := s.now().Add(-p.MaxAge)
		for drop < len(f.entries) && f.entries[drop].At.Before(cutoff) {
			drop++
		}
	}
	if p.MaxEntries > 0 && len(f.entries)-drop > p.MaxEntries {
		drop = len(f.entries) - p.MaxEntries
	}
	for _, e := range f.entries[:drop] {
		if f.last[e.ID] == e.seq {
			delete(f.last, e.ID)
		}
	}
	f.entries = append([]feedEntry(nil), f.entries[drop:]...)
}

func (s *store) publish(feed commandbus.Queue, id commandbus.CommandID, payload []byte) (commandbus.FeedEntry, bool, error) {
	if id == "" || len(id) > commandbus.MaxFeedIDLen {
		return commandbus.FeedEntry{}, false, fmt.Errorf("%w: a feed entry needs a command id of 1 to %d bytes", commandbus.ErrInvalidRequest, commandbus.MaxFeedIDLen)
	}
	decl, ok := commandbus.ResolveFeed(s.feedInv, feed)
	if !ok {
		return commandbus.FeedEntry{}, false, fmt.Errorf("%w: feed=%q", commandbus.ErrUnknownQueue, feed)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.feedLogLocked(feed)
	s.trimLocked(f, decl.Policy)
	now := s.now()
	if seq, ok := f.last[id]; ok {
		for i := len(f.entries) - 1; i >= 0; i-- {
			if f.entries[i].seq == seq {
				if now.Sub(f.entries[i].At) < decl.Policy.DedupWindow {
					return f.entries[i].FeedEntry, false, nil
				}
				break
			}
		}
	}
	e := feedEntry{seq: f.nextSeq, FeedEntry: commandbus.FeedEntry{ID: id, At: now, Payload: append([]byte(nil), payload...)}}
	f.nextSeq++
	f.entries = append(f.entries, e)
	f.last[id] = e.seq
	s.trimLocked(f, decl.Policy)
	s.notifyLocked()
	return e.FeedEntry, true, nil
}

func (s *store) follow(ctx context.Context, feed commandbus.Queue, after commandbus.CommandID) (<-chan commandbus.FeedEntry, error) {
	decl, ok := commandbus.ResolveFeed(s.feedInv, feed)
	if !ok {
		return nil, fmt.Errorf("%w: feed=%q", commandbus.ErrUnknownQueue, feed)
	}
	s.mu.Lock()
	f := s.feedLogLocked(feed)
	next := uint64(0) // 0: from the oldest retained
	if seq, ok := f.last[after]; ok && after != "" {
		next = seq + 1
	}
	s.mu.Unlock()

	out := make(chan commandbus.FeedEntry)
	go func() {
		defer close(out)
		for {
			s.mu.Lock()
			s.trimLocked(f, decl.Policy)
			var pending []feedEntry
			for _, e := range f.entries {
				if e.seq >= next {
					pending = append(pending, e)
				}
			}
			waitCh := s.wake
			s.mu.Unlock()
			for _, e := range pending {
				select {
				case out <- e.FeedEntry:
					next = e.seq + 1
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
	return out, nil
}
