package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// channelStreamPoll is how often the stream looks for a newly claimable op.
// The same cadence as the long-poll's own wait loop: the bus has no push to a
// non-blocking claim.
const channelStreamPoll = time.Second

// lastEventIDHeader is the SSE reconnect header a browser-style client sends.
const lastEventIDHeader = "Last-Event-ID"

// handleChannelOpsStream is GET /channel/ops/stream, the push form of
// /channel/ops (async-command-bus design D-16). It is ADDITIVE: an adapter that
// never calls it sees no change, and /channel/ops and /done keep every
// semantic. Telegram stays on the long-poll.
//
// One SSE event `op` per op, in queue order, `id:` = the op's sequence
// (chat.SequencedOp), `data:` = the same claimedOp JSON the long-poll returns.
// Ops are claimed through OpQueue.ClaimSequenced, the long-poll's own path, so
// leases, at-least-once delivery and idempotency are unchanged: the consumer
// acknowledges with POST /channel/ops/{id}/done, and an op it never
// acknowledges is redelivered when its lease lapses.
//
// RESUME. A reconnect names the last sequence the consumer holds (the
// Last-Event-ID header, or ?since=). Before anything new, the stream re-sends
// every op of the adapter still claimed under a live lease with a sequence
// above it (OpQueue.Resume), reading the claim back from the bus, so the
// reconnect may land on any replica. An acknowledged op is settled and never
// re-sent.
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
	since, ok := resumeSequence(w, r)
	if !ok {
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

	send := func(so chat.SequencedOp) bool {
		return writeOpFrame(w, flusher, so) == nil
	}
	for _, so := range s.Ops.Resume(ctx, adapter, since) {
		if !send(so) {
			return
		}
	}

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

// resumeSequence reads the consumer's resume point: ?since= wins over
// Last-Event-ID, absent means 0 (everything outstanding). Anything that is not
// an unsigned integer is the caller's mistake and refused, never ignored.
func resumeSequence(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	if !onlyParams(w, r, "adapter", "contract", "since") {
		return 0, false
	}
	raw := r.URL.Query().Get("since")
	if raw == "" {
		raw = r.Header.Get(lastEventIDHeader)
	}
	if raw == "" {
		return 0, true
	}
	seq, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": fmt.Sprintf(
			"resume point %q is not a sequence: send the id of the last op event, an unsigned integer", raw)})
		return 0, false
	}
	return seq, true
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
	if _, err := fmt.Fprintf(w, "event: op\nid: %d\ndata: %s\n\n", so.Sequence, data); err != nil {
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
