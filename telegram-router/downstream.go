package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Downstream is the in-cluster client for the two receiving adapters. The wire
// contract is deliberately tiny — it exists between components that ship
// together, not as a public surface:
//
//	POST <target>/updates   raw getUpdates update, verbatim
//	GET  <channel>/offset   {"value":"<offset>"}   (delegated persistence)
//	PUT  <channel>/offset   {"value":"<offset>"}
//
// The router owns the offset VALUE (it is the process calling getUpdates) but
// not its STORAGE: it holds no ServiceAccount token and no CR to annotate, so
// channel-telegram persists on its behalf through the adapter state API it
// already uses.
type Downstream struct {
	HTTP *http.Client
}

func NewDownstream() *Downstream {
	return &Downstream{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (d *Downstream) do(ctx context.Context, method, endpoint string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("%s %s: %d %s", method, endpoint, resp.StatusCode, bytes.TrimSpace(b))
	}
	if out != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Forward posts one raw update to a receiving adapter.
func (d *Downstream) Forward(ctx context.Context, target string, raw json.RawMessage) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/updates", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("forward to %s: %d %s", target, resp.StatusCode, bytes.TrimSpace(b))
	}
	return nil
}

// GetOffset reads the persisted getUpdates offset through the channel adapter.
func (d *Downstream) GetOffset(ctx context.Context, channelTarget string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := d.do(ctx, http.MethodGet, channelTarget+"/offset", nil, &out)
	return out.Value, err
}

// PutOffset reports a confirmed offset for persistence.
func (d *Downstream) PutOffset(ctx context.Context, channelTarget, value string) error {
	return d.do(ctx, http.MethodPut, channelTarget+"/offset", map[string]string{"value": value}, nil)
}
