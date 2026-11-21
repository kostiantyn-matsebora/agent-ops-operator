package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// mirrorRetryDelay is the first pause before a mirror whose watch dropped
// (server restart, network blip) tries again, and mirrorRetryMax the most it
// grows to — mirrors internal/state/nats's own pair, restated rather than
// imported (design D-1). Each further failure in a row doubles the pause, and
// a watch that delivered an entry starts it over: a watch is a consumer
// created on the server, and a fixed retry against a server that is slow to
// create them only adds attempts.
var (
	mirrorRetryDelay = time.Second
	mirrorRetryMax   = 30 * time.Second
)

// caughtUpPollInterval bounds how often waitCaughtUpTo re-checks once it has
// to wait at all — mirrors internal/state/nats's own constant of the same
// name.
const caughtUpPollInterval = 2 * time.Millisecond

// mirror is the watch-fed, in-memory read model: one ordered consumer over the
// queue log applies every entry, in sequence, to an entrylog.Log, so Claim's
// candidate search, its per-key ordering check and Stats never read the
// server (design D-2's "every consumer folds the log into a local mirror").
// A second watch keeps the dead log's snapshots. Settle, Fail, Submit and
// GetTask fence on the server's last sequence before deciding (Bus.sync), so
// a decision is never staler than the call.
type mirror struct {
	js       jetstream.JetStream
	logName  string
	deadName string
	log      *entrylog.Log
	// onChange is called after every applied entry — the nats Bus wires it to
	// its clock's notify, so a Claim blocked on nothing eligible wakes the
	// instant the mirror sees why.
	onChange func()

	mu          sync.RWMutex
	lastErr     error
	dead        map[string]commandbus.Task
	deadApplied uint64

	logReady  chan struct{}
	deadReady chan struct{}
	closed    chan struct{}
}

func newMirror(js jetstream.JetStream, logName, deadName string, log *entrylog.Log, onChange func()) *mirror {
	m := &mirror{
		js: js, logName: logName, deadName: deadName, log: log, onChange: onChange,
		dead:     map[string]commandbus.Task{},
		logReady: make(chan struct{}), deadReady: make(chan struct{}), closed: make(chan struct{}),
	}
	go m.watch(logName, m.log.LastSeq, m.applyLog, m.logReady)
	go m.watch(deadName, m.deadSeq, m.applyDead, m.deadReady)
	return m
}

// waitReady blocks until both watches' first full replay completes, however
// many reconnect attempts that takes.
func (m *mirror) waitReady(ctx context.Context) error {
	for _, ch := range []chan struct{}{m.logReady, m.deadReady} {
		select {
		case <-ch: // checked alone first: a ready mirror must win over an expired ctx
			continue
		default:
		}
		select {
		case <-ch:
		case <-ctx.Done():
			m.mu.RLock()
			last := m.lastErr
			m.mu.RUnlock()
			if last != nil {
				return fmt.Errorf("%w (the watch last failed: %v)", ctx.Err(), last)
			}
			return ctx.Err()
		}
	}
	return nil
}

// waitCaughtUpTo blocks until the mirror has applied a sequence at least as
// high as target.
func (m *mirror) waitCaughtUpTo(ctx context.Context, target uint64) error {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	for m.log.LastSeq() < target {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(caughtUpPollInterval):
		}
	}
	return nil
}

// watch runs one ordered consumer over stream until stop, resuming after the
// last applied sequence whenever it drops. ready closes once the replay
// reaches the end of the stream.
func (m *mirror) watch(stream string, applied func() uint64, apply func(seq uint64, data []byte), ready chan struct{}) {
	var once sync.Once
	caughtUp := func() { once.Do(func() { close(ready) }) }
	pause := mirrorRetryDelay
	for {
		progressed, err := m.watchOnce(stream, applied, apply, caughtUp)
		if err != nil {
			m.mu.Lock()
			m.lastErr = err
			m.mu.Unlock()
		}
		if progressed {
			pause = mirrorRetryDelay // a healthy connection earns a fast first retry
		}
		select {
		case <-m.closed:
			return
		case <-time.After(pause):
		}
		pause = min(pause*2, max(mirrorRetryMax, mirrorRetryDelay))
	}
}

// watchOnce reports whether the watch delivered at least one entry before it
// ended, which is what resets the retry pause.
func (m *mirror) watchOnce(stream string, applied func() uint64, apply func(uint64, []byte), caughtUp func()) (progressed bool, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-m.closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	s, err := m.js.Stream(ctx, stream)
	if err != nil {
		return false, fmt.Errorf("stream info %q: %w", stream, err)
	}
	if s.CachedInfo().State.Msgs == 0 {
		caughtUp()
	}
	cfg := jetstream.OrderedConsumerConfig{DeliverPolicy: jetstream.DeliverAllPolicy}
	if from := applied(); from > 0 {
		cfg.DeliverPolicy, cfg.OptStartSeq = jetstream.DeliverByStartSequencePolicy, from+1
	}
	consumer, err := m.js.OrderedConsumer(ctx, stream, cfg)
	if err != nil {
		return false, fmt.Errorf("open ordered consumer on %q: %w", stream, err)
	}
	msgs, err := consumer.Messages()
	if err != nil {
		return false, fmt.Errorf("open message iterator on %q: %w", stream, err)
	}
	defer msgs.Stop()
	for {
		msg, err := msgs.Next()
		if err != nil {
			select {
			case <-m.closed:
				return progressed, nil
			default:
				return progressed, fmt.Errorf("%w: %v", errWatchEnded, err)
			}
		}
		meta, err := msg.Metadata()
		if err != nil {
			continue
		}
		apply(meta.Sequence.Stream, msg.Data())
		progressed = true
		if meta.NumPending == 0 {
			caughtUp()
		}
		if m.onChange != nil {
			m.onChange()
		}
	}
}

// applyLog adds one queue-log entry. An entry this process cannot decode (one
// it never wrote) still advances the applied sequence, so no fence waits on it.
func (m *mirror) applyLog(seq uint64, data []byte) {
	var e entrylog.Entry
	if err := json.Unmarshal(data, &e); err != nil {
		e = entrylog.Entry{}
	}
	m.log.Apply(seq, e)
}

func (m *mirror) applyDead(seq uint64, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deadApplied = max(m.deadApplied, seq)
	var t commandbus.Task
	if err := json.Unmarshal(data, &t); err != nil || t.UID == "" {
		return
	}
	m.dead[t.UID] = t
}

func (m *mirror) deadSeq() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.deadApplied
}

// hasDead reports whether the dead log holds a snapshot of uid.
func (m *mirror) hasDead(uid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.dead[uid]
	return ok
}

func (m *mirror) deadTask(uid string) (commandbus.Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.dead[uid]
	return t, ok
}

// deadSnapshots lists the dead log's snapshots, oldest death first.
func (m *mirror) deadSnapshots() []commandbus.Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]commandbus.Task, 0, len(m.dead))
	for _, t := range m.dead {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeadAt.Equal(out[j].DeadAt) {
			return out[i].UID < out[j].UID
		}
		return out[i].DeadAt.Before(out[j].DeadAt)
	})
	return out
}

// forgetDead drops the snapshots the dead log's own limits no longer hold: the
// stream has removed them, and an ordered consumer is never told.
func (m *mirror) forgetDead(keep func(commandbus.Task) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for uid, t := range m.dead {
		if !keep(t) {
			delete(m.dead, uid)
		}
	}
}

func (m *mirror) stop() {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
}
