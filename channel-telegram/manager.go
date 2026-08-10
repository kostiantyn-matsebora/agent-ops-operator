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
// (/channel/* on the manager's API port, bearer-token auth). Adapters need no
// Kubernetes access — channel config, cursor state, and status conditions all
// go through this API.
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

// ContractVersion is the outbound message contract this adapter speaks. The
// manager refuses the ops long-poll without it: version 1 carried rendered
// `text`, and an adapter still reading that field would post empty messages
// forever rather than fail. Bump only alongside the renderer.
const ContractVersion = "2"

// MessageKind names what the manager is saying.
type MessageKind string

const (
	MsgSignal MessageKind = "signal"
	MsgAnswer MessageKind = "answer"
	MsgRelay  MessageKind = "relay"
	MsgNotice MessageKind = "notice"
)

// Message is one SEMANTIC outbound message. The manager composes meaning; this
// adapter composes presentation (see render.go). Prose fields are markdown in
// the contract's subset: **bold**, *italic*, `code`, ```fenced```, [text](url).
type Message struct {
	Kind MessageKind `json:"kind"`
	Body string      `json:"body,omitempty"`

	// signal
	Pipeline string            `json:"pipeline,omitempty"` // may be empty: inferred, blank when ambiguous
	Source   string            `json:"source,omitempty"`
	Title    string            `json:"title,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	InputRef string            `json:"inputRef,omitempty"`

	// relay
	Origin string `json:"origin,omitempty"`
	Sender string `json:"sender,omitempty"`

	// answer
	Status string `json:"status,omitempty"`

	// notice
	Level string `json:"level,omitempty"`
}

// TopicDescriptor describes the thread to create. THIS adapter names it, within
// Telegram's own 128-character limit — the manager sends facts, not a title it
// already cut to a length it guessed at.
type TopicDescriptor struct {
	Conversation string            `json:"conversation"`
	Pipeline     string            `json:"pipeline,omitempty"`
	Source       string            `json:"source,omitempty"`
	Title        string            `json:"title,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Kind         string            `json:"kind,omitempty"`
}

// Op mirrors the manager's outbound operation shape.
type Op struct {
	ID           string  `json:"id"`
	Channel      string  `json:"channel"`
	Conversation string  `json:"conversation,omitempty"`
	Kind         string  `json:"kind"` // "ensure-topic" | "send" | "close-topic"
	ThreadID     *string `json:"threadId,omitempty"`

	Topic   *TopicDescriptor `json:"topic,omitempty"`   // ensure-topic
	Message *Message         `json:"message,omitempty"` // send
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
func (m *Manager) NextOp(ctx context.Context, channelType string, waitSeconds int) (*Op, error) {
	var op Op
	code, err := m.do(ctx, "GET",
		fmt.Sprintf("/channel/ops?adapter=%s&contract=%s&wait=%d",
			url.QueryEscape(channelType), ContractVersion, waitSeconds), nil, &op)
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

// Inbound pushes one user message into the manager's router.
func (m *Manager) Inbound(ctx context.Context, channel string, threadID *string, text string) error {
	_, err := m.do(ctx, "POST", "/channel/inbound", map[string]any{
		"channel": channel, "threadId": threadID, "text": text,
	}, nil)
	return err
}

// Channels lists the channels of this adapter's type.
func (m *Manager) Channels(ctx context.Context, channelType string) ([]ChannelInfo, error) {
	var out []ChannelInfo
	_, err := m.do(ctx, "GET", "/channel/channels?adapter="+url.QueryEscape(channelType), nil, &out)
	return out, err
}

// GetState reads adapter cursor state (persisted by the manager as a Channel
// annotation — survives adapter restarts without any adapter-side storage).
func (m *Manager) GetState(ctx context.Context, channel, key string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	_, err := m.do(ctx, "GET", "/channel/state/"+url.PathEscape(channel)+"/"+url.PathEscape(key), nil, &out)
	return out.Value, err
}

// PutState writes adapter cursor state.
func (m *Manager) PutState(ctx context.Context, channel, key, value string) error {
	_, err := m.do(ctx, "PUT", "/channel/state/"+url.PathEscape(channel)+"/"+url.PathEscape(key),
		map[string]string{"value": value}, nil)
	return err
}

// ReportStatus sets the channel's Ready condition.
func (m *Manager) ReportStatus(ctx context.Context, channel string, ready bool, reason, message string) error {
	_, err := m.do(ctx, "POST", "/channel/channels/"+url.PathEscape(channel)+"/status",
		map[string]any{"ready": ready, "reason": reason, "message": message}, nil)
	return err
}
