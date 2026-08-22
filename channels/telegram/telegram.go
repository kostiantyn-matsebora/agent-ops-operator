package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Telegram is a minimal Bot API client for a supergroup with Topics enabled.
type Telegram struct {
	Token string
	HTTP  *http.Client
	// BaseURL is the Bot API root. Empty means the real one; tests point it at
	// a local server, because a Bot API call with no seam to exercise it is one
	// that ships unverified.
	BaseURL string
}

// telegramAPIBase is the real Bot API root.
const telegramAPIBase = "https://api.telegram.org"

// apiBase returns the root this client posts to.
func (t *Telegram) apiBase() string {
	if t.BaseURL != "" {
		return t.BaseURL
	}
	return telegramAPIBase
}

// NewTelegram builds a client; the HTTP timeout leaves room for 20s long-polls.
func NewTelegram(token string) *Telegram {
	return &Telegram{Token: token, HTTP: &http.Client{Timeout: 35 * time.Second}}
}

type tgResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
	Parameters  struct {
		// RetryAfter is Telegram TELLING US how long to wait. Honour it
		// exactly — never substitute a backoff we computed for an interval the
		// transport stated.
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// retryBudgetKey scopes one operation's total in-process retry allowance.
//
// Per OPERATION rather than per client: the client is shared across ops (one
// per bot token) and the budget comes from the claim the manager handed us, so
// a field on Telegram would race between concurrent ops and outlive the claim
// it belongs to.
type retryBudgetKey struct{}

// defaultRetryBudget applies when the manager advertised no reclaim interval —
// an older manager. Deliberately well under the 5-minute ReclaimAfter that has
// been the constant for as long as there has been one.
const defaultRetryBudget = 60 * time.Second

// WithRetryBudget bounds how long API may spend sleeping out `retry_after`
// before it gives up and reports the operation failed.
//
// THE INEQUALITY THAT MATTERS: this must stay strictly below the manager's
// reclaim interval. Overrun it and the manager hands the op to a second
// claimant while we are still working on it, and the message posts twice.
func WithRetryBudget(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, retryBudgetKey{}, d)
}

func retryBudget(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(retryBudgetKey{}).(time.Duration); ok && d > 0 {
		return d
	}
	return defaultRetryBudget
}

// API calls a Bot API method with a JSON body, absorbing rate limiting.
//
// A 429 is BACKPRESSURE, not a failure: Telegram states how long to wait, we
// wait exactly that, and we retry the same call. Reporting it to the manager
// instead makes the manager's recovery path load-bearing for a condition we
// could have ridden out — and the re-derivation arrives into the same limit.
//
// Retrying is safe because a rejected call posted nothing: there is no
// half-sent message to duplicate.
//
// The budget bounds the whole thing, so an operation always completes or fails
// while its claim is still ours. See WithRetryBudget.
func (t *Telegram) API(ctx context.Context, method string, body any) (json.RawMessage, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	spent := time.Duration(0)
	budget := retryBudget(ctx)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			t.apiBase()+"/bot"+t.Token+"/"+method, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := t.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		var out tgResponse
		decErr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if decErr != nil {
			return nil, decErr
		}
		if out.OK {
			return out.Result, nil
		}
		wait := time.Duration(out.Parameters.RetryAfter) * time.Second
		if wait <= 0 {
			return nil, fmt.Errorf("telegram %s: %s", method, out.Description)
		}
		if spent+wait > budget {
			// Give the op back while the claim is still valid: the manager
			// re-derives it, paced, rather than a second claimant duplicating it.
			return nil, fmt.Errorf("telegram %s: %s (retry_after %ds would exceed the %s claim budget)",
				method, out.Description, out.Parameters.RetryAfter, budget)
		}
		spent += wait
		if !sleepCtx(ctx, wait) {
			return nil, ctx.Err()
		}
	}
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

// ReopenTopic un-archives a forum topic so a reopened conversation continues in
// the thread it already had. An already-open topic is NOT an error, for the
// same reason CloseTopic tolerates an already-closed one: ops are at-least-once.
func (t *Telegram) ReopenTopic(ctx context.Context, chatID string, threadID int64) error {
	_, err := t.API(ctx, "reopenForumTopic",
		map[string]any{"chat_id": chatID, "message_thread_id": threadID})
	if err == nil || alreadyClosed(err) {
		return nil
	}
	return err
}

// DeleteTopic removes a forum topic and everything in it. Needs the bot to hold
// can_delete_messages; without it the Bot API refuses and the caller reports the
// operation failed rather than quietly doing something else.
//
// A topic that is already gone is NOT an error, for the same reason CloseTopic
// tolerates an already-closed one: ops are at-least-once, so a redelivery must
// succeed.
func (t *Telegram) DeleteTopic(ctx context.Context, chatID string, threadID int64) error {
	_, err := t.API(ctx, "deleteForumTopic",
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
	return t.SendWith(ctx, chatID, threadID, html, SendExtras{})
}

// SendExtras are the optional parts of a send that come from the OP rather than
// from the prose: which message this answers, and which controls hang off it.
//
// Both are presentation decisions made HERE from meaning stated by the manager —
// the op carries an opaque reply handle and a list of offered actions, and what
// a "control" is remains Telegram's business.
type SendExtras struct {
	// ReplyTo is Telegram's own message id, as the manager handed it back
	// unaltered. Empty when the message answers nothing in particular.
	ReplyTo string
	// Keyboard is the inline keyboard markup, already built. Nil for none.
	Keyboard any
	// ForceReply opens the reply box on the reader's behalf, pre-aimed at this
	// message. Telegram allows ONE reply_markup, so a message offering controls
	// keeps its keyboard — a reader who has buttons to press is not being asked
	// to type.
	ForceReply bool
}

// SendWith posts a message with its optional reply linkage and controls.
//
// The controls are INLINE — attached to this message — never a reply keyboard.
// A reply keyboard is shown to every member of a group and replaces their own
// composer, which is not acceptable on an operations chat several people read.
func (t *Telegram) SendWith(ctx context.Context, chatID string, threadID *int64, html string, extras SendExtras) error {
	body := map[string]any{"chat_id": chatID, "text": html, "parse_mode": "HTML"}
	if threadID != nil {
		body["message_thread_id"] = *threadID
	}
	switch {
	case extras.Keyboard != nil:
		body["reply_markup"] = extras.Keyboard
	case extras.ForceReply:
		// `selective` aims it at the person being asked rather than opening a
		// reply box for everyone in the group.
		body["reply_markup"] = map[string]any{"force_reply": true, "selective": true}
	}
	if extras.ReplyTo != "" {
		if id, err := strconv.ParseInt(extras.ReplyTo, 10, 64); err == nil {
			// allow_sending_without_reply: the original may have been deleted,
			// and losing the linkage is better than losing the message.
			body["reply_parameters"] = map[string]any{
				"message_id":                  id,
				"allow_sending_without_reply": true,
			}
		}
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

// SetCommands registers this bot's command vocabulary for one chat, so Telegram
// renders its own control in the composer and completes what a person types.
//
// Scoped to the CHAT rather than the bot's default, so the bot does not claim a
// command vocabulary in chats it does not serve.
func (t *Telegram) SetCommands(ctx context.Context, chatID string, commands []BotCommand) error {
	_, err := t.API(ctx, "setMyCommands", map[string]any{
		"commands": commands,
		"scope":    map[string]any{"type": "chat", "chat_id": chatID},
	})
	return err
}

// BotCommand is one entry in Telegram's own command menu.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
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

// SendDocument uploads text as a file with a caption, for payloads too large to
// read as chat messages.
//
// The alternative for a 50k alert body is thirteen consecutive messages, which
// buries the thread it is supposed to inform. Splitting still guarantees nothing
// is LOST; this is about the payload staying readable — and it is only possible
// because the op carries the full payload inline rather than a reference the
// adapter cannot resolve (it has no Kubernetes access).
func (t *Telegram) SendDocument(ctx context.Context, chatID string, threadID *int64, filename, caption, content string) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", chatID)
	_ = w.WriteField("caption", caption)
	_ = w.WriteField("parse_mode", "HTML")
	if threadID != nil {
		_ = w.WriteField("message_thread_id", strconv.FormatInt(*threadID, 10))
	}
	part, err := w.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		t.apiBase()+"/bot"+t.Token+"/sendDocument", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out tgResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("sendDocument: %s", out.Description)
	}
	return nil
}

// topicLink builds a deep link to a forum topic, or "" when it cannot.
//
// Telegram's private-group links use the chat id with its `-100` supergroup
// prefix removed — `-1001234567890` becomes `1234567890` — and a topic is a
// second path segment. There is no API for this and no field that carries it,
// so the shape is reproduced here, where being wrong costs one dead link rather
// than a failed operation.
func topicLink(chatID string, threadID int64) string {
	id := strings.TrimPrefix(chatID, "-100")
	// A chat that is not a supergroup has no such link, and a public one is
	// addressed by username rather than id. Neither is worth guessing at.
	if id == chatID || id == "" {
		return ""
	}
	return "https://t.me/c/" + id + "/" + strconv.FormatInt(threadID, 10)
}
