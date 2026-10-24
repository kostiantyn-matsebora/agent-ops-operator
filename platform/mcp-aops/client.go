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

// managerClient is a thin wrapper over the manager's /coordinate/* surface
// (httpapi/coordinate.go). It decides nothing: every call carries exactly the
// token and identity the caller presented to this server, and every error it
// returns carries the manager's own status code and body so a tool call can
// report the manager's refusal verbatim rather than inventing its own.
type managerClient struct {
	baseURL string
	http    *http.Client
}

func newManagerClient(baseURL string) *managerClient {
	return &managerClient{baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// managerError is a non-2xx response from the manager, carrying its status
// and body so callers can surface the manager's own refusal reason.
type managerError struct {
	Status int
	Body   string
}

func (e *managerError) Error() string {
	return fmt.Sprintf("manager refused (%d): %s", e.Status, e.Body)
}

// post sends body to path with token as the bearer, decoding a 2xx response
// into out (nil to discard it).
func (c *managerClient) post(ctx context.Context, path, token string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &managerError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
