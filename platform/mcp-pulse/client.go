package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// pulseClient is a thin wrapper over Pulse's own REST API. It decides
// nothing: every call carries exactly the bearer token the caller presented
// to this server (requirement "mcp-pulse forwards the caller's own token and
// decides no reach itself"), and every error it returns carries Pulse's own
// status code and body so a tool call can report Pulse's refusal verbatim
// rather than inventing its own.
type pulseClient struct {
	baseURL string
	http    *http.Client
}

func newPulseClient(baseURL string) *pulseClient {
	return &pulseClient{baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// pulseError is a non-2xx response from Pulse, carrying its status and body
// so callers can surface Pulse's own refusal reason — e.g. a 403 for a token
// scoped only to monitoring:read calling a write tool (spec scenario "Two
// callers, two scopes, two outcomes").
type pulseError struct {
	Status int
	Body   string
}

func (e *pulseError) Error() string {
	return fmt.Sprintf("pulse refused (%d): %s", e.Status, e.Body)
}

// get sends a GET to path with query, decoding a 2xx response into out (nil
// to discard it). token is forwarded unchanged as the bearer; empty means no
// Authorization header is set at all, leaving Pulse to decide.
func (c *pulseClient) get(ctx context.Context, path string, query url.Values, token string, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, token, out)
}

// post sends body as a JSON POST to path, decoding a 2xx response into out
// (nil to discard it).
func (c *pulseClient) post(ctx context.Context, path, token string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, token, out)
}

func (c *pulseClient) do(req *http.Request, token string, out any) error {
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
		return &pulseError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
