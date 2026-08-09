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

// Manager is a client for the operator's channel adapter contract
// (/channel/* on the manager's API port, bearer-token auth).
//
// The console reads CONFIGURATION from the Kubernetes API and CONVERSATION
// TRAFFIC from this contract — the same split every other adapter has, just
// with the read side visible. Nothing here writes CRs: thread bindings are
// established by the manager when it completes an ensure-topic op, not by the
// console touching Conversation status.
type Manager struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewManager builds a contract client; the timeout leaves room for the 25s
// ops long-poll.
func NewManager(baseURL, token string) *Manager {
	return &Manager{BaseURL: baseURL, Token: token, HTTP: &http.Client{Timeout: 40 * time.Second}}
}

// Op mirrors the manager's outbound operation shape.
type Op struct {
	ID           string  `json:"id"`
	Channel      string  `json:"channel"`
	Conversation string  `json:"conversation,omitempty"`
	Kind         string  `json:"kind"` // "ensure-topic" | "send" | "close-topic"
	Title        string  `json:"title,omitempty"`
	ThreadID     *string `json:"threadId,omitempty"`
	Text         string  `json:"text,omitempty"`
}

// ChannelInfo is one channel served by this adapter, with its opaque config.
// CredentialEnvPrefix (set when the Channel declares credentialsSecretRef)
// locates the channel's projected credentials in this process's environment:
// Secret key K is available as env <prefix>K.
type ChannelInfo struct {
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

// NextOp long-polls for the next outbound op; nil when none arrived in time.
func (m *Manager) NextOp(ctx context.Context, adapter string, waitSeconds int) (*Op, error) {
	var op Op
	code, err := m.do(ctx, "GET",
		fmt.Sprintf("/channel/ops?adapter=%s&wait=%d", url.QueryEscape(adapter), waitSeconds), nil, &op)
	if err != nil {
		return nil, err
	}
	if code == 204 {
		return nil, nil
	}
	return &op, nil
}

// CompleteOp reports an op result (threadID for ensure-topic; opErr on failure).
func (m *Manager) CompleteOp(ctx context.Context, opID, threadID, opErr string) error {
	body := map[string]string{}
	if threadID != "" {
		body["threadId"] = threadID
	}
	if opErr != "" {
		body["error"] = opErr
	}
	_, err := m.do(ctx, "POST", "/channel/ops/"+url.PathEscape(opID)+"/done", body, nil)
	return err
}

// Inbound pushes one user message into the manager's router. threadId is
// required by the contract: the console continues conversations, it never
// originates one.
func (m *Manager) Inbound(ctx context.Context, channel, threadID, sender, text string) error {
	_, err := m.do(ctx, "POST", "/channel/inbound", map[string]any{
		"channel": channel, "threadId": threadID, "text": text, "sender": sender,
	}, nil)
	return err
}

// Channels lists the channels this adapter serves.
func (m *Manager) Channels(ctx context.Context, adapter string) ([]ChannelInfo, error) {
	var out []ChannelInfo
	_, err := m.do(ctx, "GET", "/channel/channels?adapter="+url.QueryEscape(adapter), nil, &out)
	return out, err
}

// ReportStatus sets the channel's Ready condition.
func (m *Manager) ReportStatus(ctx context.Context, channel string, ready bool, reason, message string) error {
	_, err := m.do(ctx, "POST", "/channel/channels/"+url.PathEscape(channel)+"/status",
		map[string]any{"ready": ready, "reason": reason, "message": message}, nil)
	return err
}
