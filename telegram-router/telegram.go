package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram is the minimal Bot API client this process needs: getUpdates and
// nothing else. Sending lives in channel-telegram — the router never posts to
// a transport.
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

// update is one getUpdates result: the fields the router classifies on, plus
// the VERBATIM JSON. The router forwards Raw untouched — receiving adapters
// parse what they need (chat id, sender, text, thread id), so the wire shape
// stays theirs and the router never becomes a place Telegram semantics accrue.
type update struct {
	UpdateID       int64
	IsTopicMessage bool
	Raw            json.RawMessage
}

// classifyUpdate reads the only field the router cares about. A message in a
// forum topic is a CONTINUATION of a conversation the manager already knows;
// anything else (the general surface) is an ORIGINATION. Telegram puts
// is_topic_message on the message itself, so this needs no manager state —
// which is the whole reason the ingest split is possible.
func classifyUpdate(raw json.RawMessage) (update, error) {
	var parsed struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			IsTopicMessage bool `json:"is_topic_message"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return update{}, err
	}
	u := update{UpdateID: parsed.UpdateID, Raw: raw}
	if parsed.Message != nil {
		u.IsTopicMessage = parsed.Message.IsTopicMessage
	}
	return u, nil
}

// GetUpdates long-polls for message updates from the given offset, preserving
// each update's raw JSON for verbatim forwarding.
func (t *Telegram) GetUpdates(ctx context.Context, offset int64) ([]update, error) {
	res, err := t.API(ctx, "getUpdates", map[string]any{
		"timeout": 20, "offset": offset, "allowed_updates": []string{"message"},
	})
	if err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(res, &raws); err != nil {
		return nil, err
	}
	updates := make([]update, 0, len(raws))
	for _, raw := range raws {
		u, err := classifyUpdate(raw)
		if err != nil {
			return nil, err
		}
		updates = append(updates, u)
	}
	return updates, nil
}
