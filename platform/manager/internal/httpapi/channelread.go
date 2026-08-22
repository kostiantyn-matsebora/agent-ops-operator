package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// MaxReadBatch bounds one read report. The blast radius is one screen of
// conversations — the same bound, for the same reason, as bulk close.
const MaxReadBatch = 50

// readPatchAttempts retries a status patch losing to a concurrent writer. The
// reconciler patches conversation status on every run, so a conflict here is
// ordinary rather than exceptional.
const readPatchAttempts = 3

type readEntry struct {
	ThreadID string       `json:"threadId"`
	ReadAt   *metav1.Time `json:"readAt"`
	// Reader is OPTIONAL and OPAQUE: the adapter's own key for whoever read the
	// thread. Absent means the CHANNEL-WIDE mark, which is the only thing a
	// transport with no reader identity can report — and is exactly the
	// behaviour every adapter had before this field existed.
	Reader string `json:"reader,omitempty"`
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
	at      metav1.Time
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
	now := metav1.NewTime(time.Now())

	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	byThread := map[string]string{}
	for i := range list.Items {
		c := &list.Items[i]
		if t := c.Status.Thread(ch.Name); t != nil && t.ThreadID != "" {
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
		if e.ThreadID == "" || e.ReadAt == nil || e.ReadAt.IsZero() {
			results[i].Outcome = readFailed
			results[i].Reason = "each read needs a threadId and a readAt"
			continue
		}
		name, ok := byThread[e.ThreadID]
		if !ok {
			results[i].Outcome = readFailed
			results[i].Reason = fmt.Sprintf("no conversation on channel %q holds thread %q", ch.Name, e.ThreadID)
			continue
		}
		at := *e.ReadAt
		if at.Time.After(now.Time) {
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
			target = &readTarget{reader: e.Reader, at: at}
			groups[name] = append(groups[name], target)
		} else if at.Time.After(target.at.Time) {
			target.at = at
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
	for attempt := 0; attempt < readPatchAttempts; attempt++ {
		var conv agentopsv1alpha1.Conversation
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, &conv); err != nil {
			if apierrors.IsNotFound(err) {
				fail(fmt.Sprintf("conversation %q no longer exists", name))
				return
			}
			fail(err.Error())
			return
		}
		t := conv.Status.Thread(channel)
		if t == nil {
			fail(fmt.Sprintf("conversation %q holds no thread on channel %q", name, channel))
			return
		}
		patch := client.MergeFrom(conv.DeepCopy())
		advanced := false
		pending := map[*readTarget]bool{}
		for _, target := range targets {
			// A reader with no entry inherits the channel-wide mark, so a report
			// that would not pass THAT is not an advance for them either.
			if cur := t.Watermark(target.reader); cur != nil && !target.at.Time.After(cur.Time) {
				for _, i := range target.results {
					results[i].Outcome, results[i].Reason = readSkipped, "the watermark would not advance"
				}
				continue
			}
			setWatermark(t, target.reader, target.at)
			pending[target] = true
			advanced = true
		}
		if !advanced {
			return
		}
		// A binding written before this mechanism existed is being read NOW, so
		// it is tracked from here on: leaving it untracked would keep it
		// permanently "read" whatever the watermark said.
		t.ReadTracked = true
		err := s.Client.Status().Patch(ctx, &conv, patch)
		if err == nil {
			for target := range pending {
				for _, i := range target.results {
					results[i].Outcome, results[i].Reason = readMarked, ""
				}
			}
			return
		}
		if !apierrors.IsConflict(err) {
			fail(err.Error())
			return
		}
	}
	fail(fmt.Sprintf("conflict marking thread on %q read", name))
}

// setWatermark records one reader's mark, or the channel-wide one when no
// reader is named.
//
// A reader report NEVER advances the channel-wide mark: that is the whole point
// of the overlay — one person reading must not clear the badge for colleagues
// who have not.
func setWatermark(t *agentopsv1alpha1.ThreadBinding, reader string, at metav1.Time) {
	if reader == "" {
		t.ReadAt = at.DeepCopy()
		return
	}
	for i := range t.Readers {
		if t.Readers[i].Key == reader {
			t.Readers[i].ReadAt = at.DeepCopy()
			return
		}
	}
	t.Readers = append(t.Readers, agentopsv1alpha1.ReaderMark{Key: reader, ReadAt: at.DeepCopy()})
	evictReaders(t, reader)
}

// evictReaders keeps the overlay bounded, dropping the oldest watermark first
// and never the entry just written — evicting the new mark would silently
// discard the report that caused the eviction.
func evictReaders(t *agentopsv1alpha1.ThreadBinding, keep string) {
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

func readerBefore(a, b agentopsv1alpha1.ReaderMark) bool {
	if a.ReadAt == nil {
		return true
	}
	if b.ReadAt == nil {
		return false
	}
	return a.ReadAt.Time.Before(b.ReadAt.Time)
}
