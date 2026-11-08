package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// MaxReadBatch bounds one read report. The blast radius is one screen of
// conversations — the same bound, for the same reason, as bulk close.
const MaxReadBatch = 50

type readEntry struct {
	ThreadID string `json:"threadId"`
	// ReadAt is an opaque RFC3339(-Nano) timestamp string, never a metav1.Time
	// — see ThreadBinding.ReadAt's comment for why that distinction is load
	// bearing here.
	ReadAt *string `json:"readAt"`
	// Reader is OPTIONAL and OPAQUE: the adapter's own key for whoever read the
	// thread. Absent means the CHANNEL-WIDE mark, which is the only thing a
	// transport with no reader identity can report — and is exactly the
	// behaviour every adapter had before this field existed.
	Reader string `json:"reader,omitempty"`
	// Rewind asks for "mark unread" instead of the ordinary monotonic
	// advance: the named reader's OWN entry is set to ReadAt even where that
	// is earlier than what is stored. It never touches the channel-wide
	// mark, and naming no reader is refused — there would be no per-reader
	// entry to rewind, and rewinding the channel-wide mark would un-read a
	// thread for every OTHER reader too.
	Rewind bool `json:"rewind,omitempty"`
}

type readOutcome struct {
	ThreadID string `json:"threadId"`
	Reader   string `json:"reader,omitempty"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason,omitempty"`
}

const (
	readMarked  = "marked"
	readSkipped = "skipped"
	readFailed  = "failed"
)

// readTarget is one (reader, watermark) pair owed to one conversation, with the
// request positions it answers.
type readTarget struct {
	reader  string
	at      time.Time
	rewind  bool
	results []int
}

// handleChannelRead records how far an adapter's threads have been seen.
//
// The adapter reports; the manager writes. The watermark is MONOTONIC and
// clamped to the manager's own clock: without monotonicity two browsers racing
// would un-read a thread one of them just cleared, and without the clamp a
// client with a skewed clock would mark all future activity read forever.
//
// A mixed batch is a SUCCESS — per-entry outcomes, because "some marked, some
// skipped, one failed" is the ordinary result and one verdict cannot carry the
// reasons.
func (s *Server) handleChannelRead(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in struct {
		Channel string      `json:"channel"`
		Reads   []readEntry `json:"reads"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Channel == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"channel","reads":[{"threadId","readAt","reader"?}]}`})
		return
	}
	if len(in.Reads) == 0 {
		writeJSON(w, 400, map[string]string{"error": "reads is empty: nothing to mark"})
		return
	}
	if len(in.Reads) > MaxReadBatch {
		writeJSON(w, 400, map[string]string{"error": fmt.Sprintf(
			"%d reads exceeds the bound of %d; report in batches", len(in.Reads), MaxReadBatch)})
		return
	}
	// A reader key is an OPAQUE token the adapter derived — never the identity
	// itself. The manager cannot tell a hash from a plaintext address, so it
	// refuses the one shape that is obviously the latter, loudly and for the
	// whole request: an adapter sending addresses is a bug to fix at the
	// author's desk, not a per-entry data condition.
	for _, e := range in.Reads {
		if strings.Contains(e.Reader, "@") {
			writeJSON(w, 400, map[string]string{"error": "reader must be an OPAQUE key derived by the adapter, " +
				"never an identity: the manager stores it verbatim and must not learn who read a conversation"})
			return
		}
	}
	ch := s.stateChannel(r, w, in.Channel)
	if ch == nil {
		return
	}

	ctx := r.Context()
	now := time.Now()

	all, err := s.Conversations.List(ctx, state.ByAll, "")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	byThread := map[string]string{}
	for i := range all {
		c := &all[i]
		if t := c.Thread(ch.Name); t != nil && t.ThreadID != "" {
			byThread[t.ThreadID] = c.Name
		}
	}

	// Resolve first, write second: entries hitting one conversation share a
	// single patch, and within it each reader keeps its own highest watermark.
	results := make([]readOutcome, len(in.Reads))
	groups := map[string][]*readTarget{}
	order := []string{}
	for i, e := range in.Reads {
		results[i] = readOutcome{ThreadID: e.ThreadID, Reader: e.Reader}
		if e.ThreadID == "" || e.ReadAt == nil || *e.ReadAt == "" {
			results[i].Outcome = readFailed
			results[i].Reason = "each read needs a threadId and a readAt"
			continue
		}
		// Full precision matters here: this is the message's own timestamp,
		// and it is compared later against the stored watermark at the same
		// precision it is reported at — see ThreadBinding.ReadAt.
		at, err := time.Parse(time.RFC3339Nano, *e.ReadAt)
		if err != nil {
			results[i].Outcome = readFailed
			results[i].Reason = "readAt must be an RFC3339 timestamp"
			continue
		}
		if e.Rewind && e.Reader == "" {
			// The only watermark a rewind could move with no reader named is
			// the channel-wide one, and a rewind is never allowed to move
			// that — it would un-read the thread for every OTHER reader too.
			results[i].Outcome = readFailed
			results[i].Reason = "a rewind must name a reader"
			continue
		}
		name, ok := byThread[e.ThreadID]
		if !ok {
			results[i].Outcome = readFailed
			results[i].Reason = fmt.Sprintf("no conversation on channel %q holds thread %q", ch.Name, e.ThreadID)
			continue
		}
		if at.After(now) {
			at = now
		}
		if _, seen := groups[name]; !seen {
			order = append(order, name)
		}
		var target *readTarget
		for _, t := range groups[name] {
			if t.reader == e.Reader {
				target = t
				break
			}
		}
		if target == nil {
			target = &readTarget{reader: e.Reader, at: at, rewind: e.Rewind}
			groups[name] = append(groups[name], target)
		} else if at.After(target.at) {
			target.at = at
			target.rewind = target.rewind || e.Rewind
		}
		target.results = append(target.results, i)
	}

	for _, name := range order {
		s.markThreadRead(ctx, name, ch.Name, groups[name], results)
	}
	marked, skipped, failed := 0, 0, 0
	for _, res := range results {
		switch res.Outcome {
		case readMarked:
			marked++
		case readSkipped:
			skipped++
		default:
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{
		"results": results, "marked": marked, "skipped": skipped, "failed": failed,
	})
}

// markThreadRead moves one binding's watermarks forward, or reports why they did
// not, writing the outcome into results at each target's request positions.
//
// The monotonic check is made against a STRONG read inside the retry loop, not
// against the listing: a batch resolved a moment ago must not overwrite a
// watermark another reporter advanced in between. It compares against the
// READER's own watermark, so one reader's report is never skipped because
// another reader is further ahead.
func (s *Server) markThreadRead(ctx context.Context, name, channel string, targets []*readTarget, results []readOutcome) {
	fail := func(reason string) {
		for _, t := range targets {
			for _, i := range t.results {
				results[i].Outcome, results[i].Reason = readFailed, reason
			}
		}
	}
	var pending map[*readTarget]bool
	_, err := state.Mutate(ctx, s.Conversations, state.Key(name), func(conv *state.ConversationRecord) error {
		t := conv.Thread(channel)
		if t == nil {
			return fmt.Errorf("conversation %q holds no thread on channel %q", name, channel)
		}
		pending = map[*readTarget]bool{}
		for _, target := range targets {
			// A REWIND is exempt from the monotonic check by design — "mark
			// unread" exists precisely to set a reader's own entry EARLIER
			// than what is stored. It still names a reader (refused above
			// otherwise), so it can never reach the channel-wide mark.
			//
			// A reader with no entry inherits the channel-wide mark, so an
			// ORDINARY report that would not pass THAT is not an advance for
			// them either.
			if !target.rewind {
				if cur := t.Watermark(target.reader); cur != nil {
					if curTime, ok := state.ParseWatermark(*cur); ok && !target.at.After(curTime) {
						for _, i := range target.results {
							results[i].Outcome, results[i].Reason = readSkipped, "the watermark would not advance"
						}
						continue
					}
				}
			}
			setWatermark(t, target.reader, target.at)
			pending[target] = true
		}
		if len(pending) == 0 {
			return nil
		}
		// A binding written before this mechanism existed is being read NOW, so
		// it is tracked from here on: leaving it untracked would keep it
		// permanently "read" whatever the watermark said.
		t.ReadTracked = true
		return nil
	})
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			fail(fmt.Sprintf("conversation %q no longer exists", name))
			return
		}
		if errors.Is(err, state.ErrConflict) {
			fail(fmt.Sprintf("conflict marking thread on %q read: %v", name, err))
			return
		}
		fail(err.Error())
		return
	}
	// The monotonic check ran against the record the winning write was built
	// from, so a batch resolved a moment ago never overwrites a watermark
	// another reporter advanced in between.
	for target := range pending {
		for _, i := range target.results {
			results[i].Outcome, results[i].Reason = readMarked, ""
		}
	}
}

// setWatermark records one reader's mark, or the channel-wide one when no
// reader is named.
//
// A reader report NEVER advances the channel-wide mark: that is the whole point
// of the overlay — one person reading must not clear the badge for colleagues
// who have not.
func setWatermark(t *state.ThreadBinding, reader string, at time.Time) {
	// Full nanosecond precision, preserved as an opaque string rather than run
	// through metav1.Time — see ThreadBinding.ReadAt's comment for why that
	// distinction is the whole fix here.
	formatted := at.UTC().Format(time.RFC3339Nano)
	if reader == "" {
		t.ReadAt = &formatted
		return
	}
	for i := range t.Readers {
		if t.Readers[i].Key == reader {
			t.Readers[i].ReadAt = &formatted
			return
		}
	}
	t.Readers = append(t.Readers, state.ReaderMark{Key: reader, ReadAt: &formatted})
	evictReaders(t, reader)
}

// evictReaders keeps the overlay bounded, dropping the oldest watermark first
// and never the entry just written — evicting the new mark would silently
// discard the report that caused the eviction.
func evictReaders(t *state.ThreadBinding, keep string) {
	for len(t.Readers) > agentopsv1alpha1.MaxReadersPerThread {
		oldest := -1
		for i := range t.Readers {
			if t.Readers[i].Key == keep {
				continue
			}
			if oldest < 0 || readerBefore(t.Readers[i], t.Readers[oldest]) {
				oldest = i
			}
		}
		if oldest < 0 {
			return
		}
		t.Readers = append(t.Readers[:oldest], t.Readers[oldest+1:]...)
	}
}

func readerBefore(a, b state.ReaderMark) bool {
	if a.ReadAt == nil {
		return true
	}
	if b.ReadAt == nil {
		return false
	}
	at, aok := state.ParseWatermark(*a.ReadAt)
	bt, bok := state.ParseWatermark(*b.ReadAt)
	if !aok {
		return true
	}
	if !bok {
		return false
	}
	return at.Before(bt)
}
