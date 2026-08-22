// signal-telegram: the chat ORIGINATION adapter. A message on a Telegram
// chat's general surface is a signal like any other — it goes through the same
// claim check, cooldown, grouping and observability as an alert or a cron job,
// instead of creating a Conversation down a private path.
//
//	updates: POST /updates            (from gateway-telegram; LISTEN_ADDR, default :8080)
//	sources: GET /signal/sources?adapter=<ADAPTER_NAME>   (15s poll)
//	push:    POST /signal/inbound     (normalized; chat lane)
//
// It never contacts Telegram and holds NO credentials: the router polls and
// forwards, the channel adapter sends. This process only normalizes.
//
// Chat-id matching and approver filtering happen HERE, against this adapter's
// own source listing — the router carries no channel policy. The same two
// rules live in channel-telegram for the continuation side; they are ~15 lines
// each against the same contract and drift shows up as messages silently
// ignored, so both sides get the same test.
//
// NO adapter-side grouping: cooldown, signature grouping, window reuse and
// recurrence stay manager-side from SignalSource.spec.grouping, and routing
// requires the source's Pipeline claim (an unclaimed chat source drops with
// Wired=False and the reason travels back to the user's surface).
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default
// "telegram"), LISTEN_ADDR (default ":8080" — in-cluster the reconciler
// injects it from SignalAdapter.spec.port).
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Reserved labels every chat adapter sets. agentops.dev/channel is what lets
// the manager answer a command on the surface the message came from without
// creating a Conversation — a chat signal without it is unanswerable, and the
// manager rejects it rather than swallow the reply.
const (
	labelChannel = "agentops.dev/channel"
	labelSender  = "agentops.dev/sender"
	// labelMessage carries Telegram's own handle for the arriving message. The
	// manager treats it as OPAQUE — it stores and returns it unaltered — and it
	// is what lets a reply say which message it answers, so a control offered
	// on somebody's own words can carry those words forward without anything
	// being retained in between.
	labelMessage = "agentops.dev/message"
)

// THE TRANSPORT-LOCAL SPELLING, REVERSED.
//
// Telegram command names admit only [a-z0-9_], so channel-telegram registers a
// hyphenated Pipeline under an underscored spelling — that is the only way
// Telegram will complete it as a person types. What the menu inserts therefore
// arrives here as `/k8s_observe`, and must leave as `k8s-observe`.
//
// The two halves need NO shared state, because the mapping is INJECTIVE BY
// CONSTRUCTION: a Kubernetes object name is a DNS-1123 subdomain and cannot
// contain an underscore, so `_` in a command word is always one we introduced
// and `_` -> `-` reverses it exactly. That is why this is one line in each
// module rather than a map either would have to fetch — and it is load-bearing,
// since this adapter cannot read a channel endpoint at all.
//
// Nothing beyond the COMMAND WORD is touched: the task is the person's text.
func reverseSpelling(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	end := strings.IndexAny(text, " \t\n")
	if end < 0 {
		end = len(text)
	}
	return strings.ReplaceAll(text[:end], "_", "-") + text[end:]
}

// sourceConfig is this adapter's interpretation of SignalSource spec.config.
type sourceConfig struct {
	// ChatID is the Telegram chat whose general surface originates here.
	ChatID string `json:"chatId"`
	// Channel names the Channel this chat is the general surface OF — the
	// conversation's replies and any command answer go back to it.
	Channel string `json:"channel"`
	// Approvers restricts who may originate (empty = anyone in the chat).
	Approvers []int64 `json:"approvers,omitempty"`
}

// servedSource is one validated chat source.
type servedSource struct {
	cfg sourceConfig
}

// tgUpdate is the slice of the Telegram update shape this adapter reads. The
// router forwards updates verbatim, so this parses the wire format directly.
type tgFrom struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	IsBot    bool   `json:"is_bot"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgMessage struct {
	MessageID      int64      `json:"message_id"`
	Text           string     `json:"text"`
	From           *tgFrom    `json:"from"`
	Chat           *tgChat    `json:"chat"`
	IsTopicMessage bool       `json:"is_topic_message"`
	ReplyToMessage *tgMessage `json:"reply_to_message"`
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
	// CallbackQuery is a person SELECTING a control the manager offered. The
	// message it is attached to is the manager's own reply, and the message
	// THAT answered is the person's original — which is how a selection carries
	// their words forward with nothing held in between.
	CallbackQuery *struct {
		ID      string     `json:"id"`
		Data    string     `json:"data"`
		From    *tgFrom    `json:"from"`
		Message *tgMessage `json:"message"`
	} `json:"callback_query"`
}

// ChoicePrefix marks callback data this adapter owns. The payload after it is
// the Pipeline's REAL name — callback data is never displayed, so there is
// nothing to gain from spelling it for the transport.
const ChoicePrefix = "p:"

type adapter struct {
	mgr        *Manager
	sourceType string
	listen     string

	mu       sync.Mutex
	sources  map[string]servedSource
	reported map[string]string
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func main() {
	sourceType := os.Getenv("ADAPTER_NAME")
	if sourceType == "" {
		sourceType = "telegram"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		listen:     listen,
		sources:    map[string]servedSource{},
		reported:   map[string]string{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-telegram adapter starting (adapter=%s, listen=%s)", sourceType, listen)

	go a.registryLoop(ctx)

	srv := &http.Server{Addr: listen, Handler: a.handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("update server: %v", err)
	}
}

func (a *adapter) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /updates", a.handleUpdate)
	return mux
}

// registryLoop keeps the served-source map fresh and reports Ready state.
func (a *adapter) registryLoop(ctx context.Context) {
	for ctx.Err() == nil {
		a.refreshSources(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
		}
	}
}

func (a *adapter) refreshSources(ctx context.Context) {
	infos, err := a.mgr.Sources(ctx, a.sourceType)
	if err != nil {
		log.Printf("list sources: %v", err)
		return
	}
	next := map[string]servedSource{}
	for _, info := range infos {
		var cfg sourceConfig
		problem := ""
		if len(info.Config) == 0 {
			problem = "spec.config is missing"
		} else if err := json.Unmarshal(info.Config, &cfg); err != nil {
			problem = "spec.config is not valid JSON for the telegram signal adapter: " + err.Error()
		} else if cfg.ChatID == "" {
			problem = "spec.config.chatId is required"
		} else if cfg.Channel == "" {
			problem = "spec.config.channel is required — it names the Channel this chat is the general surface of, and replies have nowhere to go without it"
		}
		a.mu.Lock()
		last := a.reported[info.Name]
		a.mu.Unlock()
		if problem != "" {
			if last != problem {
				_ = a.mgr.ReportStatus(ctx, info.Name, false, "InvalidConfig", problem)
				a.mu.Lock()
				a.reported[info.Name] = problem
				a.mu.Unlock()
			}
			continue // keep serving the other sources
		}
		if last != "ok" {
			_ = a.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady",
				"served by signal-telegram — general-surface messages originate conversations")
			a.mu.Lock()
			a.reported[info.Name] = "ok"
			a.mu.Unlock()
		}
		next[info.Name] = servedSource{cfg: cfg}
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// handleUpdate takes one raw update forwarded by the router.
//
// Always 204, even when the update is filtered: the router is a dumb pipe and
// a non-2xx would only make it log. Whether a message is POLICY-eligible is
// this adapter's business, not the router's.
func (a *adapter) handleUpdate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var upd tgUpdate
	if err := json.Unmarshal(body, &upd); err != nil {
		http.Error(w, "not a telegram update", http.StatusBadRequest)
		return
	}
	source, sig, ok := a.normalize(upd)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	res, err := a.mgr.Inbound(r.Context(), source, []Signal{sig})
	if err != nil {
		log.Printf("inbound %s: %v", source, err)
		http.Error(w, "push failed", http.StatusBadGateway)
		return
	}
	if res.Reason != "" {
		log.Printf("source %s: %s", source, res.Reason)
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalize applies chat-id matching and approver filtering, then renders the
// chat signal. Returns ok=false when the update is not ours to originate.
//
// A topic message must never arrive here — that is the router's continuation
// branch — but if one does, it is dropped rather than turned into a second
// conversation alongside the one it belongs to.
func (a *adapter) normalize(upd tgUpdate) (string, Signal, bool) {
	if upd.CallbackQuery != nil {
		return a.normalizeSelection(upd)
	}
	m := upd.Message
	if m == nil || m.Chat == nil || m.Text == "" || m.IsTopicMessage {
		return "", Signal{}, false
	}
	match, cfg, ok := a.sourceFor(m.Chat.ID)
	if !ok {
		return "", Signal{}, false
	}
	if !approved(cfg, m.From) {
		return "", Signal{}, false // not an approved user — ignore silently
	}

	// The menu inserts the transport-local spelling; the manager only knows the
	// real one. Reversed HERE, so nothing outside this adapter ever sees it.
	text := reverseSpelling(m.Text)

	// ANSWERING A PROMPT. A command menu sends on tap, so `/k8s-ops` arrives
	// with no task and the manager asks for one. This is that answer, and the
	// Pipeline it belongs to is two links back up the reply chain:
	//
	//	this reply  ->  the manager's question  ->  the bare command
	//
	// Read from Telegram's own linkage rather than from anything remembered, so
	// a restart between the question and the answer changes nothing — the same
	// technique a tapped control already uses.
	if cmd := promptedCommand(m); cmd != "" {
		text = cmd + " " + text
	}

	return match, Signal{
		// update_id is unique per bot, so identical text twice never
		// collapses on cooldown — repeating yourself is not dedup.
		Fingerprint: "tg-" + strconv.FormatInt(upd.UpdateID, 10),
		Labels:      a.chatLabels(cfg, m.From, m.MessageID),
		Kind:        "chat",
		Payload:     text,
		Title:       title(text),
	}, true
}

// normalizeSelection turns a tap on an offered control into the addressed
// command the person meant.
//
// STATELESS. The manager's offer was posted as a REPLY to the person's own
// message, so Telegram itself still holds that text — this adapter can restart
// between the offer and the tap and the selection still works. Nothing is kept
// on either side.
//
// An original that cannot be recovered is NOT an error to report: the addressed
// command goes out with no task, and the manager answers with its own usage
// reply on the surface the person is looking at. This adapter holds no Telegram
// credential and could not tell them itself.
func (a *adapter) normalizeSelection(upd tgUpdate) (string, Signal, bool) {
	cb := upd.CallbackQuery
	if cb.Message == nil || cb.Message.Chat == nil || cb.Message.IsTopicMessage {
		return "", Signal{}, false
	}
	name, found := strings.CutPrefix(cb.Data, ChoicePrefix)
	if !found || name == "" {
		return "", Signal{}, false // not a control this adapter offered
	}
	match, cfg, ok := a.sourceFor(cb.Message.Chat.ID)
	if !ok {
		return "", Signal{}, false
	}
	if !approved(cfg, cb.From) {
		return "", Signal{}, false
	}

	payload := "/" + name
	var handle int64
	if orig := cb.Message.ReplyToMessage; orig != nil {
		handle = orig.MessageID
		if orig.Text != "" {
			payload += " " + orig.Text
		}
	}
	return match, Signal{
		// The callback id is unique per tap, so tapping twice is two requests
		// rather than one collapsed by cooldown.
		Fingerprint: "tg-cb-" + cb.ID,
		Labels:      a.chatLabels(cfg, cb.From, handle),
		Kind:        "chat",
		Payload:     payload,
		Title:       title(payload),
	}, true
}

// promptedCommand returns the addressed command this message is answering, or
// "" when it is not answering one.
//
// THE CHAIN IS ONE LINK LONG, and that is Telegram's rule rather than a choice:
// a reply carries the message it answers, but that message never carries its
// own reply. So "two links up to the original command" is not available, and an
// earlier version of this that assumed otherwise recovered nothing — every
// prompted answer fell through to the manager's ambiguity refusal.
//
// What survives is the QUESTION'S OWN TEXT, and the manager names the addressed
// form in it for exactly this reason. The first slash-token is that form.
//
// Guarded on the question coming FROM A BOT: a person quoting `/ha-ops` at
// somebody is not answering a prompt, and their reply must stay their words.
func promptedCommand(m *tgMessage) string {
	q := m.ReplyToMessage
	if q == nil || q.From == nil || !q.From.IsBot {
		return ""
	}
	for _, field := range strings.Fields(q.Text) {
		token := strings.Trim(field, "`*_.,:;!?()[]")
		if !strings.HasPrefix(token, "/") || len(token) < 2 {
			continue
		}
		name := reverseSpelling(token)
		if at := strings.IndexByte(name, '@'); at > 0 {
			name = name[:at]
		}
		return name
	}
	return ""
}

// sourceFor resolves the served source whose chat this is.
func (a *adapter) sourceFor(chatID int64) (string, sourceConfig, bool) {
	id := strconv.FormatInt(chatID, 10)
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.sources))
	for name := range a.sources {
		names = append(names, name)
	}
	sort.Strings(names) // stable pick when two sources name one chat
	for _, name := range names {
		if a.sources[name].cfg.ChatID == id {
			return name, a.sources[name].cfg, true
		}
	}
	return "", sourceConfig{}, false
}

// approved applies the source's approver list (empty = anyone in the chat).
func approved(cfg sourceConfig, from *tgFrom) bool {
	if len(cfg.Approvers) == 0 {
		return true
	}
	return from != nil && containsID(cfg.Approvers, from.ID)
}

// chatLabels renders the reserved labels every chat signal carries. The message
// handle is omitted when there is none — it is optional, and an adapter that
// cannot supply one loses the reply linkage and nothing else.
func (a *adapter) chatLabels(cfg sourceConfig, from *tgFrom, messageID int64) map[string]string {
	labels := map[string]string{labelChannel: cfg.Channel}
	if from != nil {
		sender := from.Username
		if sender == "" {
			sender = strconv.FormatInt(from.ID, 10)
		}
		labels[labelSender] = sender
	}
	if messageID != 0 {
		labels[labelMessage] = strconv.FormatInt(messageID, 10)
	}
	return labels
}

// title renders a short conversation title from the message text.
func title(text string) string {
	const max = 60
	for i, r := range text {
		if r == '\n' {
			text = text[:i]
			break
		}
	}
	if len(text) > max {
		return text[:max] + "…"
	}
	return text
}

func containsID(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}
