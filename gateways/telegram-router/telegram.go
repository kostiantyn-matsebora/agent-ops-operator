package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram is the minimal Bot API client this process needs: getUpdates, and
// the content-free acknowledgement a selection requires. Sending lives in
// channel-telegram — the router composes and posts no message.
type Telegram struct {
	Token string
	HTTP  *http.Client
	// BaseURL is the Bot API root. Empty means the real one; tests point it at
	// a local server, because a Bot API call with no seam to exercise it is one
	// that ships unverified — which is how a wrong env-var name once ran for a
	// whole implementation pass.
	BaseURL string
}

// telegramAPIBase is the real Bot API root.
const telegramAPIBase = "https://api.telegram.org"

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
	base := t.BaseURL
	if base == "" {
		base = telegramAPIBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/bot"+t.Token+"/"+method, bytes.NewReader(b))
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
	// CallbackID is set when this update is a SELECTION on a control. Telegram
	// spins the tapper's client until the bot answers, so the router
	// acknowledges it — see Acknowledge.
	CallbackID string
	Raw        json.RawMessage
}

// classifyUpdate reads the only field the router cares about. A message in a
// forum topic is a CONTINUATION of a conversation the manager already knows;
// anything else (the general surface) is an ORIGINATION. Telegram puts
// is_topic_message on the message itself, so this needs no manager state —
// which is the whole reason the ingest split is possible.
//
// A SELECTION on a control is classified by the SAME rule, read from the
// message the control was attached to. There is no second rule: a tap on the
// general surface is an origination and a tap inside a topic is a continuation,
// exactly as the message would have been.
func classifyUpdate(raw json.RawMessage) (update, error) {
	type msg struct {
		IsTopicMessage bool `json:"is_topic_message"`
	}
	var parsed struct {
		UpdateID      int64 `json:"update_id"`
		Message       *msg  `json:"message"`
		CallbackQuery *struct {
			ID      string `json:"id"`
			Message *msg   `json:"message"`
		} `json:"callback_query"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return update{}, err
	}
	u := update{UpdateID: parsed.UpdateID, Raw: raw}
	switch {
	case parsed.CallbackQuery != nil:
		u.CallbackID = parsed.CallbackQuery.ID
		if parsed.CallbackQuery.Message != nil {
			u.IsTopicMessage = parsed.CallbackQuery.Message.IsTopicMessage
		}
	case parsed.Message != nil:
		u.IsTopicMessage = parsed.Message.IsTopicMessage
	}
	return u, nil
}

// Acknowledge answers a selection so the tapper's client stops waiting.
//
// UNCONDITIONAL AND CONTENT-FREE — it says nothing, decides nothing and reads no
// configuration, which is what keeps it stream hygiene rather than a policy the
// router has no business holding. It lives here because the router is the one
// component that always holds the token: signal-telegram deliberately holds no
// credential, and handing it one to stop a spinner would undo the reason it has
// none.
//
// A failure is logged and dropped by the caller: the selection has already been
// forwarded, and the visible result is the message that follows it.
func (t *Telegram) Acknowledge(ctx context.Context, callbackID string) error {
	_, err := t.API(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID})
	return err
}

// GetUpdates long-polls for message updates from the given offset, preserving
// each update's raw JSON for verbatim forwarding.
func (t *Telegram) GetUpdates(ctx context.Context, offset int64) ([]update, error) {
	res, err := t.API(ctx, "getUpdates", map[string]any{
		// Selections join messages: a control the manager offered is answered by
		// tapping it, and an update kind we do not ask for is one Telegram never
		// sends.
		"timeout": 20, "offset": offset,
		"allowed_updates": []string{"message", "callback_query"},
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
