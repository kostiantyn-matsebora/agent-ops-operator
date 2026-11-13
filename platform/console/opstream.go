package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// The manager's push stream for this adapter's ops, GET /channel/ops/stream
// (async-command-bus design D-17): SSE events `op`, oldest first. The stream only
// reads. The console applies each event to its own state and then ACKNOWLEDGES it
// with a plain call, POST /channel/ops/{id}/ack, which is its receipt of the
// message: the manager consumes the message there and never sends it again. A
// message pushed and not acknowledged (the console died, the connection
// dropped) is offered again on the next connection, so a reconnect needs no
// cursor.

// streamIdleTimeout is how long the op stream may go without a byte before it
// is treated as dead: a half-open connection no FIN will ever announce. The
// manager's keep-alive comment arrives every 20s, so three missed ones.
var streamIdleTimeout = 60 * time.Second

// StreamOps opens the stream and calls handle for every `op` event, in order,
// until the connection ends. onVocabulary receives the revision the response
// header and every `vocabulary` event carry.
//
// It returns handle's own error when handle fails (the connection is dropped, and
// the reconnect is offered whatever was not acknowledged), errStreamIdle when the
// connection went silent, and nil on a clean end of stream.
func (m *Manager) StreamOps(ctx context.Context, adapter string,
	onVocabulary func(revision string), handle func(op *Op) error) error {
	path := fmt.Sprintf("/channel/ops/stream?adapter=%s&contract=%s", url.QueryEscape(adapter), ContractVersion)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, "GET", m.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	req.Header.Set("Accept", "text/event-stream")
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
		return applyOpFrame(f, onVocabulary, handle)
	})
	if idled.Load() && ctx.Err() == nil {
		return errStreamIdle
	}
	return err
}

// applyOpFrame dispatches one event of the op stream.
func applyOpFrame(f sseFrame, onVocabulary func(string), handle func(*Op) error) error {
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
		if op.ID == "" {
			return fmt.Errorf("op event %q carries no id", f.id)
		}
		return handle(&op)
	}
	return nil
}

// AckOp acknowledges one op: the console's receipt of it. The manager consumes
// the message at this call. A message already gone (404: consumed, or expired
// out of the log) is a harmless duplicate and is not an error.
func (m *Manager) AckOp(ctx context.Context, adapter, id string) error {
	path := fmt.Sprintf("/channel/ops/%s/ack?adapter=%s", url.PathEscape(id), url.QueryEscape(adapter))
	code, err := m.do(ctx, "POST", path, nil, nil)
	if code == http.StatusNotFound {
		return nil
	}
	return err
}
