package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// channelStreamPoll is how often the stream looks for a newly claimable op.
// The same cadence as the long-poll's own wait loop: the bus has no push to a
// non-blocking claim.
const channelStreamPoll = time.Second

// channelStreamSweep is how often a stream re-sends the adapter's ops that are
// leased but were never pushed on THIS connection: a claim made by a handler
// whose client is gone without a FIN keeps writing into a dead socket for as
// long as TCP takes to notice, and its op would otherwise sit leased (with
// everything behind it on its key) until the lease lapsed. A variable so a test
// can shorten it.
var channelStreamSweep = 5 * time.Second

// lastEventIDHeader is the SSE reconnect header a browser-style client sends.
const lastEventIDHeader = "Last-Event-ID"

// handleChannelOpsStream is GET /channel/ops/stream, the push form of
// /channel/ops (async-command-bus design D-16). It is ADDITIVE: an adapter that
// never calls it sees no change, and /channel/ops and /done keep every
// semantic. Telegram stays on the long-poll.
//
// One SSE event `op` per op, in queue order, `id:` = the op's command id (the
// cursor, never an order), `data:` = the same claimedOp JSON the long-poll
// returns.
// Ops are claimed through OpQueue.ClaimSequenced, the long-poll's own path, so
// leases, at-least-once delivery and idempotency are unchanged: the consumer
// acknowledges with POST /channel/ops/{id}/done, and an op it never
// acknowledges is redelivered when its lease lapses.
//
// RESUME AND SWEEP. Before anything new, the stream re-sends every op of the
// adapter still claimed under a live lease (OpQueue.Resume), reading the claim
// back from the bus so the reconnect may land on any replica, and repeats that
// every channelStreamSweep for ops not pushed on this connection (a claim made
// by a dead connection's handler). The cursor (Last-Event-ID or ?since=) is
// validated but advisory: an unacknowledged op can sit before it in the log. An
// acknowledged op is settled and never re-sent.
func (s *Server) handleChannelOpsStream(w http.ResponseWriter, r *http.Request) {
	adapter, ok := adapterParam(w, r)
	if !ok {
		return
	}
	if !scopeAllows(r, adapter) {
		forbidScope(w)
		return
	}
	if !contractOK(w, r) {
		return
	}
	if _, ok := resumeCursor(w, r); !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
		return
	}
	ctx := r.Context()
	s.setVocabularyRevision(ctx, w)
	lastRevision := s.vocabularyRevision(ctx)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	// Flushed at once: the caller's Do() blocks on the headers, and nothing may
	// follow until the next op exists.
	flusher.Flush()

	// pushed holds the ops this connection has written, so the sweep re-sends
	// only what some other (dead) claimant holds.
	pushed := map[string]struct{}{}
	send := func(so chat.SequencedOp) bool {
		if writeOpFrame(w, flusher, so) != nil {
			return false
		}
		pushed[so.Op.ID] = struct{}{}
		return true
	}
	if !s.resendOutstanding(ctx, adapter, pushed, send) {
		return
	}
	sweep := time.NewTicker(channelStreamSweep)
	defer sweep.Stop()

	poll := time.NewTicker(channelStreamPoll)
	defer poll.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		// Drain what is claimable before waiting, so a backlog is not paced at
		// one op a second.
		for {
			so, ok := s.Ops.ClaimSequenced(adapter)
			if !ok {
				break
			}
			if !send(so) {
				return // the op stays leased; the reconnect resumes it
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			if !s.resendOutstanding(ctx, adapter, pushed, send) {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			// The long-poll stamps the revision on every response; a stream has
			// one response, so a change after connect is its own event.
			if rev := s.vocabularyRevision(ctx); rev != lastRevision {
				lastRevision = rev
				if writeVocabularyFrame(w, flusher, rev) != nil {
					return
				}
			}
		}
	}
}

// resendOutstanding sends every op of adapter that is leased and not yet pushed
// on this connection, and forgets pushed ids that are no longer outstanding
// (acknowledged). False when a write failed.
func (s *Server) resendOutstanding(ctx context.Context, adapter string, pushed map[string]struct{}, send func(chat.SequencedOp) bool) bool {
	outstanding := s.Ops.Resume(ctx, adapter)
	live := make(map[string]struct{}, len(outstanding))
	for _, so := range outstanding {
		live[so.Op.ID] = struct{}{}
	}
	for id := range pushed {
		if _, ok := live[id]; !ok {
			delete(pushed, id)
		}
	}
	for _, so := range outstanding {
		if _, done := pushed[so.Op.ID]; done {
			continue
		}
		if !send(so) {
			return false
		}
	}
	return true
}

// maxCursorLen bounds a resume cursor: a command id is a short derived string,
// and the value is client input.
const maxCursorLen = 512

// resumeCursor reads the consumer's resume point: ?since= wins over
// Last-Event-ID, absent means "" (everything outstanding). The cursor is a
// command id and so opaque, but it is client input: a value that is too long or
// holds a control character cannot be an id the stream ever sent, and is
// refused rather than looked up.
func resumeCursor(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !onlyParams(w, r, "adapter", "contract", "since") {
		return "", false
	}
	raw := r.URL.Query().Get("since")
	if raw == "" {
		raw = r.Header.Get(lastEventIDHeader)
	}
	if !validCursor(raw) {
		writeJSON(w, 400, map[string]string{"error": "resume point is not a command id: send the id of the last op event, unchanged"})
		return "", false
	}
	return raw, true
}

func validCursor(raw string) bool {
	if len(raw) > maxCursorLen {
		return false
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// writeOpFrame writes one `op` event and flushes it.
func writeOpFrame(w http.ResponseWriter, flusher http.Flusher, so chat.SequencedOp) error {
	reclaim := int(so.ReclaimAfter.Seconds())
	if reclaim < 1 {
		reclaim = 1
	}
	data, err := json.Marshal(claimedOp{Op: so.Op, ReclaimAfterSeconds: reclaim})
	if err != nil {
		return err
	}
	if !validCursor(so.Op.ID) {
		return fmt.Errorf("op id %q cannot be an SSE event id", so.Op.ID)
	}
	if _, err := fmt.Fprintf(w, "event: op\nid: %s\ndata: %s\n\n", so.Op.ID, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// writeVocabularyFrame announces a vocabulary revision change. It carries no
// id, so it never moves the consumer's resume point.
func writeVocabularyFrame(w http.ResponseWriter, flusher http.Flusher, revision string) error {
	data, _ := json.Marshal(map[string]string{"revision": revision})
	if _, err := fmt.Fprintf(w, "event: vocabulary\ndata: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
