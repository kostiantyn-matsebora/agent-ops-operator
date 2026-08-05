package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram implements Provider for a supergroup with Topics enabled.
type Telegram struct {
	Token  string
	ChatID string
	HTTP   *http.Client
}

// NewTelegram builds a Telegram provider.
func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{Token: token, ChatID: chatID, HTTP: &http.Client{Timeout: 35 * time.Second}}
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

// EnsureTopic implements Provider.
func (t *Telegram) EnsureTopic(ctx context.Context, title string) (int64, error) {
	if len(title) > 128 {
		title = title[:128]
	}
	res, err := t.API(ctx, "createForumTopic", map[string]any{"chat_id": t.ChatID, "name": title})
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

// Send implements Provider; falls back to General when the topic is gone.
func (t *Telegram) Send(ctx context.Context, threadID *int64, html string) error {
	body := map[string]any{"chat_id": t.ChatID, "text": html, "parse_mode": "HTML"}
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
