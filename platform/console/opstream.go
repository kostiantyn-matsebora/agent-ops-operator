package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// The manager's push feed for this adapter's ops, GET /channel/ops/stream
// (async-command-bus design D-16): the same ops the long-poll returns, as SSE
// events `op` whose id is the op's command id, resumable with Last-Event-ID.
// Acknowledgement is unchanged: POST /channel/ops/{id}/done.

// errStreamUnsupported is a manager that predates the stream (404/405). The
// caller falls back to the long-poll.
var errStreamUnsupported = errors.New("the manager does not serve /channel/ops/stream")

// streamIdleTimeout is how long the op stream may go without a byte before it
// is treated as dead: a half-open connection no FIN will ever announce. The
// manager's keep-alive comment arrives every 20s.
var streamIdleTimeout = defaultStreamIdle

// StreamOps opens the feed and calls handle for every `op` event, in order,
// until the connection ends. lastEventID, when non-empty, is sent as
// Last-Event-ID so the manager first replays what was pushed and not
// acknowledged. onVocabulary receives the revision the response header and
// every `vocabulary` event carry.
//
// It returns errStreamUnsupported for a 404/405, handle's own error when handle
// fails (the connection is dropped, and the manager keeps the op leased for the
// resume), and nil on a clean end of stream.
func (m *Manager) StreamOps(ctx context.Context, adapter, lastEventID string,
	onVocabulary func(revision string), handle func(op *Op, id string) error) error {
	path := fmt.Sprintf("/channel/ops/stream?adapter=%s&contract=%s", url.QueryEscape(adapter), ContractVersion)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, "GET", m.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	// The ordinary client's Timeout bounds the WHOLE exchange, body included,
	// and would cut a stream built to stay open. Liveness is the idle watchdog's.
	stream := *m.HTTP
	stream.Timeout = 0
	resp, err := stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		return errStreamUnsupported
	case resp.StatusCode == http.StatusServiceUnavailable:
		m.HTTP.CloseIdleConnections()
		return fmt.Errorf("GET %s: %d", path, resp.StatusCode)
	case resp.StatusCode >= 400:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, bytes.TrimSpace(b))
	}
	if rev := resp.Header.Get(VocabularyRevisionHeader); rev != "" && onVocabulary != nil {
		onVocabulary(rev)
	}

	var idled atomic.Bool
	idle := time.AfterFunc(streamIdleTimeout, func() { idled.Store(true); cancel() })
	defer idle.Stop()
	err = scanSSE(streamCtx, &idleReader{r: resp.Body, timer: idle, limit: streamIdleTimeout}, func(f sseFrame) error {
		return applyOpFrame(f, idle, onVocabulary, handle)
	})
	if idled.Load() && ctx.Err() == nil {
		return errStreamIdle
	}
	return err
}

// applyOpFrame dispatches one event of the op stream.
func applyOpFrame(f sseFrame, idle *time.Timer, onVocabulary func(string), handle func(*Op, string) error) error {
	switch f.event {
	case "vocabulary":
		var v struct {
			Revision string `json:"revision"`
		}
		if json.Unmarshal(f.data, &v) == nil && v.Revision != "" && onVocabulary != nil {
			onVocabulary(v.Revision)
		}
	case "op":
		var op Op
		if err := json.Unmarshal(f.data, &op); err != nil {
			return fmt.Errorf("decode op event %q: %w", f.id, err)
		}
		if f.id == "" {
			return fmt.Errorf("op event %q carries no id", op.ID)
		}
		// Executing and acknowledging is not the manager's silence, so the
		// watchdog sleeps meanwhile.
		idle.Stop()
		defer idle.Reset(streamIdleTimeout)
		return handle(&op, f.id)
	}
	return nil
}

// feedCursor is what the console tells the manager on a reconnect.
//
// The token is a command id (design D-16): the last op acknowledged, which the
// manager finds in the queue and replays every later unacknowledged op after.
// An id says nothing about order, so the console cannot compute "just under the
// lowest op still outstanding". While an op is received and not acknowledged it
// sends no token at all, and the manager starts from the oldest unacknowledged
// op: an acknowledged op is settled and never replayed, so the wider start costs
// nothing, while a token naming a later op could skip one the console holds
// without having acknowledged.
type feedCursor struct {
	lastAcked   string
	outstanding map[string]struct{}
}

func (c *feedCursor) received(id string) {
	if c.outstanding == nil {
		c.outstanding = map[string]struct{}{}
	}
	c.outstanding[id] = struct{}{}
}

func (c *feedCursor) acknowledged(id string) {
	delete(c.outstanding, id)
	c.lastAcked = id
}

// resumeToken is the Last-Event-ID to send, "" before anything was
// acknowledged and while any op is outstanding.
func (c *feedCursor) resumeToken() string {
	if len(c.outstanding) > 0 {
		return ""
	}
	return c.lastAcked
}
