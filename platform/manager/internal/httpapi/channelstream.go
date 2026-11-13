package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// channelStreamPoll is how often the stream looks for a message it has not yet
// pushed: a read of the bus, never a write.
const channelStreamPoll = 200 * time.Millisecond

// vocabularyPoll is how often the stream looks for a changed vocabulary revision.
const vocabularyPoll = time.Second

// streamBatch bounds how many unacknowledged messages one read lists.
const streamBatch = 16

// streamWriteTimeout bounds one frame's write: a client that is gone without a
// FIN stops draining its socket, and the write fails instead of blocking the
// handler for as long as TCP takes to notice.
var streamWriteTimeout = 15 * time.Second

// handleChannelOpsStream is GET /channel/ops/stream, the transport a receipt
// adapter (OpQueue.ReceiptAdapters: the console) reads its messages on (design
// D-17). Every other adapter, Telegram among them, reads GET /channel/ops and is
// told so with a 404.
//
// channelStreamPreflight answers every refusal the stream owes before it
// opens, and reports false once it has written one.
func (s *Server) channelStreamPreflight(w http.ResponseWriter, r *http.Request) (string, http.Flusher, bool) {
	adapter, ok := adapterParam(w, r)
	if !ok {
		return "", nil, false
	}
	if !scopeAllows(r, adapter) {
		forbidScope(w)
		return "", nil, false
	}
	if !contractOK(w, r) || !onlyParams(w, r, "adapter", "contract") {
		return "", nil, false
	}
	if !s.Ops.IsReceipt(adapter) {
		writeJSON(w, 404, map[string]string{"error": "adapter " + adapter + " reads its ops from GET /channel/ops, not a stream"})
		return "", nil, false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
		return "", nil, false
	}
	return adapter, flusher, true
}

// One SSE event `op` per message, oldest first, `data:` = the op as JSON. The
// stream only reads: it claims, leases and deletes nothing. A message is the
// adapter's once it POSTs /channel/ops/{id}/ack, at which point the manager
// consumes it. A message pushed and never acknowledged (the adapter died, the
// connection dropped) is still unacknowledged, so the next connection is pushed
// it again; within one connection it is pushed once. The server learns a client
// is gone from a failed write or the closed request, and simply returns.
func (s *Server) handleChannelOpsStream(w http.ResponseWriter, r *http.Request) {
	adapter, flusher, ok := s.channelStreamPreflight(w, r)
	if !ok {
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

	rc := http.NewResponseController(w)
	write := func(f func() error) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
		return f() == nil
	}
	pushed := map[string]bool{}
	poll := time.NewTicker(channelStreamPoll)
	defer poll.Stop()
	vocabulary := time.NewTicker(vocabularyPoll)
	defer vocabulary.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			if !s.pushUnseen(ctx, adapter, pushed, func(op *chat.Op) bool {
				return write(func() error { return writeOpFrame(w, flusher, op) })
			}) {
				return
			}
		case <-keepAlive.C:
			if !write(func() error {
				if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
					return err
				}
				flusher.Flush()
				return nil
			}) {
				return
			}
		case <-vocabulary.C:
			// The long-poll stamps the revision on every response; a stream has
			// one response, so a change after connect is its own event.
			if rev := s.vocabularyRevision(ctx); rev != lastRevision {
				lastRevision = rev
				if !write(func() error { return writeVocabularyFrame(w, flusher, rev) }) {
					return
				}
			}
		}
	}
}

// pushUnseen writes every offered message not yet pushed on this connection, and
// forgets the ones no longer offered (acknowledged, so there is nothing left to
// suppress). False when a write failed and the connection is done.
func (s *Server) pushUnseen(ctx context.Context, adapter string, pushed map[string]bool, push func(*chat.Op) bool) bool {
	offered := s.Ops.Read(ctx, adapter, streamBatch)
	still := make(map[string]bool, len(offered))
	for _, op := range offered {
		still[op.ID] = true
		if pushed[op.ID] {
			continue
		}
		if !push(op) {
			return false
		}
		pushed[op.ID] = true
	}
	for id := range pushed {
		if !still[id] {
			delete(pushed, id)
		}
	}
	return true
}

// writeOpFrame writes one `op` event and flushes it.
func writeOpFrame(w http.ResponseWriter, flusher http.Flusher, op *chat.Op) error {
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: op\nid: %s\ndata: %s\n\n", op.ID, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// writeVocabularyFrame announces a vocabulary revision change.
func writeVocabularyFrame(w http.ResponseWriter, flusher http.Flusher, revision string) error {
	data, _ := json.Marshal(map[string]string{"revision": revision})
	if _, err := fmt.Fprintf(w, "event: vocabulary\ndata: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
