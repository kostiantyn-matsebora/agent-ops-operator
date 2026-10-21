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
//
// It holds no Telegram credential of its own. Every call is relayed through
// gateway-telegram's POST /telegram/call/{method} (chat-ops-broker-telegram-gateway,
// see gotchas.md 2026-10-11): that process is now the SOLE holder of the bot
// token, inbound and outbound alike. This client only needs to know where the
// gateway is and the shared secret that authenticates to it.
type Telegram struct {
	// GatewayURL is gateway-telegram's in-cluster base URL, resolved by
	// main.go the same way MANAGER_URL already is: required, and a missing
	// value fails startup naming it.
	GatewayURL string
	// GatewayToken authenticates every call to the gateway. Unlike the
	// /updates push the gateway makes in the other direction (pure
	// network-level trust — see telegram-ingest-router's spec), this leg can
	// SEND with the live bot credential, so it carries a shared secret too.
	GatewayToken string
	HTTP         *http.Client
}

// NewTelegram builds a client that posts every Bot API call through
// gateway-telegram at gatewayURL, authenticating with gatewayToken. The HTTP
// timeout leaves room for the gateway's own 429 retry_after waits.
func NewTelegram(gatewayURL, gatewayToken string) *Telegram {
	return &Telegram{
		GatewayURL:   strings.TrimRight(gatewayURL, "/"),
		GatewayToken: gatewayToken,
		HTTP:         &http.Client{Timeout: 35 * time.Second},
	}
}

type tgResponse struct {
	OK          bool            `json:"ok"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
	Parameters  struct {
		// RetryAfter is Telegram TELLING US how long to wait. Honour it
		// exactly — never substitute a backoff we computed for an interval the
		// transport stated.
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// retryBudgetKey scopes one OPERATION's total in-process retry allowance.
//
// Per OPERATION rather than per client or per call: the client is shared
// across ops (one per bot token), and a budget reset on every call would let
// an op that makes N transport calls — chunk sends, a fallback, SendDocument —
// spend N times what the manager's claim actually allows. The value stored is
// an ABSOLUTE DEADLINE, minted ONCE when the op is claimed, so every call this
// op goes on to make shares the same clock.
type retryBudgetKey struct{}

// defaultRetryBudget applies when the manager advertised no reclaim interval —
// an older manager. Deliberately well under the 5-minute ReclaimAfter that has
// been the constant for as long as there has been one.
const defaultRetryBudget = 60 * time.Second

// transientBackoff is spent on a call that failed with nothing TELLING us how
// long to wait — a dial error, a timeout, or a bare 5xx with no retry_after.
// Unlike a 429 this interval is ours to pick, not Telegram's: short, fixed,
// and charged against the very same operation deadline as everything else.
const transientBackoff = 2 * time.Second

// WithRetryBudget mints the operation's one shared retry deadline, d from now.
// Every call this op's context reaches — directly or through a child context —
// charges its wait, its retries and its own HTTP round trip against this SAME
// deadline, which is what makes the budget PER OPERATION rather than per call.
//
// THE INEQUALITY THAT MATTERS: d must stay strictly below the manager's
// reclaim interval. Overrun it and the manager hands the op to a second
// claimant while we are still working on it, and the message posts twice.
func WithRetryBudget(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, retryBudgetKey{}, time.Now().Add(d))
}

// retryDeadline reads the operation's shared deadline, minting a fresh
// call-scoped one when nothing upstream set one — a caller outside the
// claimed-op path, such as a direct Send in a test or a hand-deployed script.
func retryDeadline(ctx context.Context) time.Time {
	if dl, ok := ctx.Value(retryBudgetKey{}).(time.Time); ok {
		return dl
	}
	return time.Now().Add(defaultRetryBudget)
}

// withinBudget reports whether spending wait more still lands at or before
// deadline.
func withinBudget(deadline time.Time, wait time.Duration) bool {
	return !time.Now().Add(wait).After(deadline)
}

// API calls a Bot API method through gateway-telegram's /telegram/call
// endpoint, JSON-encoding body.
func (t *Telegram) API(ctx context.Context, method string, body any) (json.RawMessage, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return t.call(ctx, method, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			t.GatewayURL+"/telegram/call/"+method, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+t.GatewayToken)
		return req, nil
	})
}

// call is the ONE retry loop every Bot API call rides, whatever shape its
// request body takes — JSON (API) or multipart (SendDocument). newReq builds
// a FRESH request on every attempt, since a request's body reader is consumed
// by the attempt before it.
//
// Three conditions are absorbed inside the operation's budget, never reported
// as a failure outright:
//
//   - A 429 with retry_after: BACKPRESSURE, not a failure. Telegram states how
//     long to wait, we wait exactly that, and retry the same call.
//   - A dial error or a timeout: nothing posted, so retrying is safe, and
//     nothing told us an interval — transientBackoff is ours to pick.
//   - A bare 5xx, Telegram's own or the gateway's own leg to it: the same
//     transient condition as a dial error, so it rides the same clock instead
//     of failing the operation outright.
//
// Retrying a 429 is safe because a rejected call posted nothing: there is no
// half-sent message to duplicate. This still holds through the gateway: it
// relays Telegram's own response verbatim rather than absorbing the retry
// itself, so a 429 reaches this loop exactly as it would have direct from
// Telegram.
//
// The budget bounds the whole thing, so an operation always completes or
// fails while its claim is still ours. See WithRetryBudget.
func (t *Telegram) call(ctx context.Context, method string, newReq func() (*http.Request, error)) (json.RawMessage, error) {
	deadline := retryDeadline(ctx)
	for {
		req, err := newReq()
		if err != nil {
			return nil, err
		}
		raw, status, netErr := t.do(req)
		if netErr != nil {
			if !withinBudget(deadline, transientBackoff) {
				return nil, fmt.Errorf("telegram %s: %w (would exceed the claim budget)", method, netErr)
			}
			if !sleepCtx(ctx, transientBackoff) {
				return nil, ctx.Err()
			}
			continue
		}
		var out tgResponse
		// A real Bot API response is always JSON, win or lose. A body that
		// fails to parse can only be the gateway's OWN rejection (401 bad
		// token, 400 missing method, 502 upstream dial failure), written as
		// plain text by http.Error.
		if decErr := json.Unmarshal(raw, &out); decErr != nil {
			if status >= 500 {
				// The gateway's own leg is down, not Telegram's answer to us —
				// the same transient condition a dial error is.
				if !withinBudget(deadline, transientBackoff) {
					return nil, fmt.Errorf("telegram %s: gateway returned %d %s: %s (would exceed the claim budget)",
						method, status, http.StatusText(status), strings.TrimSpace(string(raw)))
				}
				if !sleepCtx(ctx, transientBackoff) {
					return nil, ctx.Err()
				}
				continue
			}
			// Name the status so it reads as an auth/gateway problem, not a
			// cryptic JSON error.
			return nil, fmt.Errorf("telegram %s: gateway returned %d %s: %s", method, status, http.StatusText(status), strings.TrimSpace(string(raw)))
		}
		if out.OK {
			return out.Result, nil
		}
		wait := time.Duration(out.Parameters.RetryAfter) * time.Second
		if wait <= 0 && out.ErrorCode >= 500 {
			// Telegram's OWN 5xx, with no stated interval: the same backoff a
			// transport-level failure gets.
			wait = transientBackoff
		}
		if wait <= 0 {
			return nil, fmt.Errorf("telegram %s: %s", method, out.Description)
		}
		if !withinBudget(deadline, wait) {
			// Give the op back while the claim is still valid: the manager
			// re-derives it, paced, rather than a second claimant duplicating it.
			return nil, fmt.Errorf("telegram %s: %s (retry_after %ds would exceed the claim budget)",
				method, out.Description, out.Parameters.RetryAfter)
		}
		if !sleepCtx(ctx, wait) {
			return nil, ctx.Err()
		}
	}
}

// do issues one request and reads its body, reporting the status code
// alongside — the caller tells a transient transport failure (err set) from a
// gateway-level rejection (err nil, status >= 400, body not Telegram's JSON
// shape) from an ordinary Bot API answer.
func (t *Telegram) do(req *http.Request) (raw []byte, status int, err error) {
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

// CreateTopic creates a forum topic and returns its thread id.
func (t *Telegram) CreateTopic(ctx context.Context, chatID, title string) (int64, error) {
	// Rune-safe: a byte slice cuts a multi-byte character in half, and a
	// title carries emoji (🤝, 🔍, 🤖) and whatever alert text an adopter's
	// own alerting sends. This is now the ONLY place that enforces
	// Telegram's topic-name limit — the manager stopped imposing it on
	// every channel (gotchas.md: "a manager-side fix would be one
	// transport's limits imposed on all of them").
	if runes := []rune(title); len(runes) > 128 {
		title = string(runes[:128])
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
// succeed. A REDELIVERED delete additionally meets "message thread not found" —
// there is nothing left to delete the second time — which closeForumTopic never
// answers with, since an archived topic still EXISTS.
func (t *Telegram) DeleteTopic(ctx context.Context, chatID string, threadID int64) error {
	_, err := t.API(ctx, "deleteForumTopic",
		map[string]any{"chat_id": chatID, "message_thread_id": threadID})
	if err == nil || alreadyClosed(err) || threadNotFound(err) {
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

// threadNotFound reports whether a Bot API error means the topic no longer
// exists at all — the shape a redelivered DeleteTopic gets back, since there
// is nothing left to delete a second time.
func threadNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "thread not found")
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
	_, err := t.API(ctx, "sendMessage", body)
	// THE ONE FEATURE PROBE THIS ADAPTER HAS. `<blockquote expandable>` needs
	// Bot API 7.2 and there is no way to ask an endpoint its version — getMe
	// says nothing, and a self-hosted local Bot API server may be any age. So
	// the probe is the send itself: refused for the quote tag, latch it off and
	// re-render degraded. Once per process, not once per message.
	if unsupportedQuote(err) && expandableQuotes.Load() {
		expandableQuotes.Store(false)
		body["text"] = degradeQuotes(html)
		_, err = t.API(ctx, "sendMessage", body)
	}
	if err != nil {
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
// Telegram serves one update stream per bot token, so gateway-telegram is the
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
	// CallbackQuery arrives when a reader taps an inline control this adapter
	// attached to a message (inlineKeyboard) — selecting a choice, not typing
	// one. See dispatchCallback in main.go.
	CallbackQuery *tgCallbackQuery `json:"callback_query,omitempty"`
}

// tgCallbackQuery is the slice of Telegram's callback_query shape this adapter
// reads. Data is this adapter's OWN encoding (choicePrefix + the command,
// built by inlineKeyboard) — never Telegram's to interpret, and never sent
// inbound as-is: dispatchCallback reverses it back to the command it stands
// for before pushing anything to the manager.
type tgCallbackQuery struct {
	ID   string `json:"id"`
	From *struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *struct {
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		IsTopicMessage  bool  `json:"is_topic_message"`
		MessageThreadID int64 `json:"message_thread_id"`
	} `json:"message"`
	Data string `json:"data"`
}

// SendDocument uploads text as a file with a caption, for payloads too large to
// read as chat messages.
//
// The alternative for a 50k alert body is thirteen consecutive messages, which
// buries the thread it is supposed to inform. Splitting still guarantees nothing
// is LOST; this is about the payload staying readable — and it is only possible
// because the op carries the full payload inline rather than a reference the
// adapter cannot resolve (it has no Kubernetes access).
//
// Goes through the SAME call() response path as every other Bot API call, so a
// 429 is honoured, a gateway rejection surfaces with its status and body, and a
// gone thread falls back to the general surface — identically to SendWith,
// never a second, bespoke handling of what the gateway answers.
func (t *Telegram) SendDocument(ctx context.Context, chatID string, threadID *int64, filename, caption, content string) error {
	newReq := func(withThread bool) func() (*http.Request, error) {
		return func() (*http.Request, error) {
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			_ = w.WriteField("chat_id", chatID)
			_ = w.WriteField("caption", caption)
			_ = w.WriteField("parse_mode", "HTML")
			if withThread && threadID != nil {
				_ = w.WriteField("message_thread_id", strconv.FormatInt(*threadID, 10))
			}
			part, err := w.CreateFormFile("document", filename)
			if err != nil {
				return nil, err
			}
			if _, err := io.WriteString(part, content); err != nil {
				return nil, err
			}
			if err := w.Close(); err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				t.GatewayURL+"/telegram/call/sendDocument", &buf)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", w.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+t.GatewayToken)
			return req, nil
		}
	}
	_, err := t.call(ctx, "sendDocument", newReq(true))
	if err != nil && threadID != nil {
		// The topic this document was aimed at may be gone — the general
		// surface still wants the attachment, same fallback as SendWith.
		_, err = t.call(ctx, "sendDocument", newReq(false))
	}
	return err
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
