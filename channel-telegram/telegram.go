package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Telegram is a minimal Bot API client for a supergroup with Topics enabled.
type Telegram struct {
	Token string
	HTTP  *http.Client
}

// NewTelegram builds a client; the HTTP timeout leaves room for 20s long-polls.
func NewTelegram(token string) *Telegram {
	return &Telegram{Token: token, HTTP: &http.Client{Timeout: 35 * time.Second}}
}

type tgResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// API calls a Bot API method with a JSON body.
func (t *Telegram) API(ctx context.Context, method string, body any) (json.RawMessage, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+t.Token+"/"+method, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out tgResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram %s: %s", method, out.Description)
	}
	return out.Result, nil
}

// CreateTopic creates a forum topic and returns its thread id.
func (t *Telegram) CreateTopic(ctx context.Context, chatID, title string) (int64, error) {
	if len(title) > 128 {
		title = title[:128]
	}
	res, err := t.API(ctx, "createForumTopic", map[string]any{"chat_id": chatID, "name": title})
	if err != nil {
		return 0, err
	}
	var topic struct {
		MessageThreadID int64 `json:"message_thread_id"`
	}
	if err := json.Unmarshal(res, &topic); err != nil {
		return 0, err
	}
	return topic.MessageThreadID, nil
}

// CloseTopic archives a forum topic. An already-closed topic is NOT an error:
// close-topic ops are at-least-once, so a redelivered one must succeed.
//
// Telegram spells "already closed" as TOPIC_NOT_MODIFIED — confirmed against
// the live Bot API, which is the only place to learn it; TOPIC_CLOSED is what
// it returns for *posting* into a closed topic. Match both, because getting
// this wrong turns every redelivery into a reported failure.
func (t *Telegram) CloseTopic(ctx context.Context, chatID string, threadID int64) error {
	_, err := t.API(ctx, "closeForumTopic",
		map[string]any{"chat_id": chatID, "message_thread_id": threadID})
	if err == nil || alreadyClosed(err) {
		return nil
	}
	return err
}

// alreadyClosed reports whether a closeForumTopic error means the topic was
// already archived.
func alreadyClosed(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "topic_not_modified") || strings.Contains(msg, "topic_closed")
}

// Send posts an HTML message; falls back to General when the topic is gone.
func (t *Telegram) Send(ctx context.Context, chatID string, threadID *int64, html string) error {
	body := map[string]any{"chat_id": chatID, "text": html, "parse_mode": "HTML"}
	if threadID != nil {
		body["message_thread_id"] = *threadID
	}
	if _, err := t.API(ctx, "sendMessage", body); err != nil {
		if threadID == nil {
			return err
		}
		delete(body, "message_thread_id")
		_, err = t.API(ctx, "sendMessage", body)
		return err
	}
	return nil
}

// tgUpdate is the slice of the Telegram update shape this adapter reads from
// updates the ROUTER forwards. There is no GetUpdates here on purpose:
// Telegram serves one update stream per bot token, so telegram-router is the
// only process that may call it. Adding a poll loop back to this file is the
// mistake that produces 409s and stolen updates.
type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		IsTopicMessage  bool  `json:"is_topic_message"`
		MessageThreadID int64 `json:"message_thread_id"`
	} `json:"message"`
}
