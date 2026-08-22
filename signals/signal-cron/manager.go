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

// Manager is a client for the operator's signal adapter contract (/signal/*
// on the manager's API port, bearer-token auth). Adapters need no Kubernetes
// access — source config, cursor state, and status conditions all go through
// this API; normalized signals go in through /signal/inbound and the manager
// applies grouping/cooldown/recurrence itself.
type Manager struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewManager builds a contract client.
func NewManager(baseURL, token string) *Manager {
	return &Manager{BaseURL: baseURL, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// SourceInfo is one signal source served by this adapter, with its opaque
// config. CredentialEnvPrefix (set when the source declares
// credentialsSecretRef) locates projected credentials in this process's
// environment: Secret key K is env <prefix>K.
type SourceInfo struct {
	Name                string          `json:"name"`
	Config              json.RawMessage `json:"config,omitempty"`
	CredentialEnvPrefix string          `json:"credentialEnvPrefix,omitempty"`
}

// Signal is the contract's normalized signal shape.
type Signal struct {
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels,omitempty"`
	Title       string            `json:"title,omitempty"`
	Payload     string            `json:"payload,omitempty"`
	Kind        string            `json:"kind,omitempty"` // "alert" | "job"
}

func (m *Manager) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}
	if out != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Sources lists the signal sources of this adapter's type.
func (m *Manager) Sources(ctx context.Context, sourceType string) ([]SourceInfo, error) {
	var out []SourceInfo
	err := m.do(ctx, "GET", "/signal/sources?adapter="+url.QueryEscape(sourceType), nil, &out)
	return out, err
}

// Inbound pushes normalized signals for a source.
func (m *Manager) Inbound(ctx context.Context, source string, signals []Signal) error {
	return m.do(ctx, "POST", "/signal/inbound", map[string]any{
		"source": source, "signals": signals,
	}, nil)
}

// GetState reads adapter cursor state (persisted by the manager as a
// SignalSource annotation — survives adapter restarts without any adapter-side
// storage).
func (m *Manager) GetState(ctx context.Context, source, key string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := m.do(ctx, "GET", "/signal/state/"+url.PathEscape(source)+"/"+url.PathEscape(key), nil, &out)
	return out.Value, err
}

// PutState writes adapter cursor state.
func (m *Manager) PutState(ctx context.Context, source, key, value string) error {
	return m.do(ctx, "PUT", "/signal/state/"+url.PathEscape(source)+"/"+url.PathEscape(key),
		map[string]string{"value": value}, nil)
}

// ReportStatus sets the source's Ready condition.
func (m *Manager) ReportStatus(ctx context.Context, source string, ready bool, reason, message string) error {
	return m.do(ctx, "POST", "/signal/sources/"+url.PathEscape(source)+"/status",
		map[string]any{"ready": ready, "reason": reason, "message": message}, nil)
}
