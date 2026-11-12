package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// channelStreamPoll is how often the stream looks for a changed vocabulary
// revision.
const channelStreamPoll = time.Second

// streamWriteTimeout bounds one frame's write: a client that is gone without a
// FIN stops draining its socket, and the write fails instead of blocking the
// handler for as long as TCP takes to notice.
var streamWriteTimeout = 15 * time.Second

// deliveredTimeout bounds the side effects a written frame triggers. They run
// detached from the request, so a client that drops right after the write does
// not cancel the bookkeeping that frame earned.
const deliveredTimeout = 15 * time.Second

// lastEventIDHeader is the SSE reconnect header a browser-style client sends.
const lastEventIDHeader = "Last-Event-ID"

// handleChannelOpsStream is GET /channel/ops/stream, an adapter's ops as a
// plain SSE stream (async-command-bus design D-16). It serves an adapter the
// manager keeps a retained feed for (OpQueue.FeedAdapters: the console). Every
// other adapter, Telegram among them, keeps /channel/ops with its claim and
// acknowledgement, and is told so with a 404.
//
// One SSE event `op` per op, in log order, `id:` = the op's command id, `data:`
// = the op as JSON. The cursor is the id of the last event the client received,
// sent back as Last-Event-ID or ?since=: the stream replays every later entry,
// an unknown id replays from the start of the retained log, and then it pushes
// new ones. The connection claims, leases and deletes nothing, so a dead or a
// second connection strands nothing. The server learns a client is gone from a
// failed write or the closed request, and simply returns.
//
// An op is DELIVERED when its frame was written: the manager then runs the side
// effects an acknowledgement used to trigger (OpQueue.Delivered), once per op id.
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
	cursor, ok := resumeCursor(w, r)
	if !ok {
		return
	}
	if !s.Ops.IsFeed(adapter) {
		writeJSON(w, 404, map[string]string{"error": "adapter " + adapter + " reads its ops from GET /channel/ops, not a stream"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	ops, err := s.Ops.OpenFeed(ctx, adapter, cursor)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "the ops feed is unavailable: " + err.Error()})
		return
	}
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
	poll := time.NewTicker(channelStreamPoll)
	defer poll.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-ops:
			if !ok {
				return
			}
			if !validCursor(op.ID) {
				log.FromContext(ctx).Error(fmt.Errorf("op id %q cannot be an SSE event id", op.ID), "skip a feed entry", "adapter", adapter)
				continue
			}
			if !write(func() error { return writeOpFrame(w, flusher, op) }) {
				return
			}
			dctx, stop := context.WithTimeout(context.WithoutCancel(ctx), deliveredTimeout)
			s.Ops.Delivered(dctx, op)
			stop()
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
		case <-poll.C:
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

// maxCursorLen bounds a resume cursor: a command id is a short derived string,
// and the value is client input.
const maxCursorLen = 512

// resumeCursor reads the consumer's resume point: ?since= wins over
// Last-Event-ID, absent means "" (the whole retained log). The cursor is a
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
