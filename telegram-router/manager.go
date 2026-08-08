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

// Manager is the sliver of the signal adapter contract this process uses:
// ONE listing read, at startup and on refresh, to learn its own source's
// forwarding targets and where its bot token was projected.
//
// Deliberately no more than that. The router pushes no signals (signal-telegram
// does), persists no state (channel-telegram does), and reads no CHANNEL
// configuration ever — chat-id matching and approver filtering belong to the
// receiving adapters. Keeping this client thin is what stops a third container
// from becoming a third place to misconfigure Telegram.
type Manager struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewManager(baseURL, token string) *Manager {
	return &Manager{BaseURL: baseURL, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// SourceInfo is one SignalSource served by this adapter. CredentialEnvPrefix
// (set when the source declares credentialsSecretRef) locates the projected
// bot token in this process's environment: Secret key K is env <prefix>K.
type SourceInfo struct {
	Name                string          `json:"name"`
	Config              json.RawMessage `json:"config,omitempty"`
	CredentialEnvPrefix string          `json:"credentialEnvPrefix,omitempty"`
}

func (m *Manager) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.BaseURL+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}
	if out != nil && resp.StatusCode != 204 {
		return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode, nil
}

// Sources lists the SignalSources this adapter serves.
func (m *Manager) Sources(ctx context.Context, adapter string) ([]SourceInfo, error) {
	var out []SourceInfo
	_, err := m.do(ctx, "GET", "/signal/sources?adapter="+url.QueryEscape(adapter), nil, &out)
	return out, err
}

// ReportStatus sets the source's Ready condition (how a misconfigured router
// tells the operator, since it has no other voice).
func (m *Manager) ReportStatus(ctx context.Context, source string, ready bool, reason, message string) error {
	_, err := m.do(ctx, "POST", "/signal/sources/"+url.PathEscape(source)+"/status",
		map[string]any{"ready": ready, "reason": reason, "message": message}, nil)
	return err
}
