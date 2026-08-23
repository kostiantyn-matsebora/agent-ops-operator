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
//
// IT DOES NOT MOVE FOR THE BLOCK GRAMMAR. No field was added and none changed
// meaning: a body that was markdown is now markdown plus a grammar, read by the
// component that already read the markdown — this one.
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

	// any kind
	//
	// Choices are the actions this message OFFERS — structured like Labels, not
	// prose. The manager states which actions are on offer and nothing about how
	// they look; rendering them as controls is this adapter's decision.
	Choices []Choice `json:"choices,omitempty"`
	// ExpectsReply marks a message that asks the reader for something. Telegram
	// can open the reply box for them, which is the whole reason it matters
	// here: its command menu SENDS on tap, so a bare command leaves nowhere to
	// type the task.
	ExpectsReply bool `json:"expectsReply,omitempty"`
	// InReplyTo is Telegram's own message id for the message this one answers,
	// handed back exactly as this adapter supplied it. It is what lets a control
	// offered on somebody's own words carry those words forward.
	InReplyTo string `json:"inReplyTo,omitempty"`
}

// Choice is one offered action.
type Choice struct {
	Label   string `json:"label"`
	Command string `json:"command"`
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
	// PreviousThreadID is set when a closed conversation is reopened: the topic
	// this conversation used before it was archived. Telegram CAN un-archive,
	// so this adapter honours it and the conversation continues where it left
	// off. An adapter whose transport cannot would ignore it and open a fresh
	// entity, which is equally valid — the manager holds no opinion.
	PreviousThreadID string `json:"previousThreadId,omitempty"`
}

// Op mirrors the manager's outbound operation shape.
type Op struct {
	ID           string  `json:"id"`
	Channel      string  `json:"channel"`
	Conversation string  `json:"conversation,omitempty"`
	Kind         string  `json:"kind"` // "ensure-topic" | "send" | "close-topic" | "delete-conversation"
	ThreadID     *string `json:"threadId,omitempty"`

	Topic   *TopicDescriptor `json:"topic,omitempty"`   // ensure-topic
	Message *Message         `json:"message,omitempty"` // send, delete-conversation

	// ReclaimAfterSeconds is how long the manager leaves this claim with us
	// before returning the op to its queue. Absent (0) means an older manager.
	ReclaimAfterSeconds int `json:"reclaimAfterSeconds,omitempty"`
}

// RetryBudget is how long this op may spend absorbing transport backpressure.
//
// HALF the advertised claim window, deliberately. The op still has to be sent
// and completed after the last retry, and a budget equal to the window would
// finish exactly as the manager reclaims it — handing a second claimant the
// same message. Half leaves room for the work either side of the waiting.
func (o *Op) RetryBudget() time.Duration {
	if o.ReclaimAfterSeconds <= 0 {
		return defaultRetryBudget
	}
	return time.Duration(o.ReclaimAfterSeconds) * time.Second / 2
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
	code, _, err := m.doH(ctx, method, path, in, out)
	return code, err
}

// doH is do, plus the response headers — the vocabulary revision rides one, so
// a caller that tracks it needs to see them.
func (m *Manager) doH(ctx context.Context, method, path string, in, out any) (int, http.Header, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.BaseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return resp.StatusCode, resp.Header, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}
	if out != nil && resp.StatusCode != 204 {
		return resp.StatusCode, resp.Header, json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode, resp.Header, nil
}

// VocabularyRevisionHeader carries the manager's current vocabulary revision on
// every outbound long-poll response — the 200 that delivers an op and the 204
// that delivers none.
//
// It is how a change reaches this adapter while it is otherwise idle. The
// manager cannot dial us: a ChannelAdapter port is optional and the contract is
// pull-only, so the news rides the connection we are already blocked in.
const VocabularyRevisionHeader = "X-Agentops-Vocabulary-Revision"

// NextOp long-polls for the next outbound op; nil when none arrived in time.
// The second return is the vocabulary revision the manager reported, empty from
// a manager that predates it.
func (m *Manager) NextOp(ctx context.Context, channelType string, waitSeconds int) (*Op, string, error) {
	var op Op
	code, hdr, err := m.doH(ctx, "GET",
		fmt.Sprintf("/channel/ops?adapter=%s&contract=%s&wait=%d",
			url.QueryEscape(channelType), ContractVersion, waitSeconds), nil, &op)
	rev := hdr.Get(VocabularyRevisionHeader)
	if err != nil {
		return nil, rev, err
	}
	if code == 204 {
		return nil, rev, nil
	}
	return &op, rev, nil
}

// Vocabulary fetches what may be typed on a chat surface.
//
// This adapter holds NO Kubernetes access — it cannot read a Pipeline and never
// will — so the manager is the only thing that can tell it what is addressable.
// What comes back is UNFILTERED: deciding which of it Telegram can express is
// this adapter's job, not the manager's.
func (m *Manager) Vocabulary(ctx context.Context) (Vocabulary, error) {
	var v Vocabulary
	_, err := m.do(ctx, "GET", "/channel/vocabulary", nil, &v)
	return v, err
}

// Vocabulary is the manager's list of what a person may type, plus the revision
// identifying it.
type Vocabulary struct {
	Revision string            `json:"revision"`
	Entries  []VocabularyEntry `json:"entries"`
}

// VocabularyEntry is one thing a person may type. Position is `general` (the
// surface a conversation starts from) or `thread` (inside one); Telegram's
// finest command scope is a CHAT, which spans both, so this adapter registers
// the union and the manager's own usage replies correct a command used in the
// wrong place.
type VocabularyEntry struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    string `json:"position"`
	// Icon is how the entry is recognised in a list. Telegram command NAMES
	// take only [a-z0-9_], so it cannot go there — it leads the description,
	// which is the one part of a menu row that accepts an emoji.
	Icon    string `json:"icon,omitempty"`
	Profile string `json:"profile,omitempty"`
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
