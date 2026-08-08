// signal-telegram: the chat ORIGINATION adapter. A message on a Telegram
// chat's general surface is a signal like any other — it goes through the same
// claim check, cooldown, grouping and observability as an alert or a cron job,
// instead of creating a Conversation down a private path.
//
//	updates: POST /updates            (from telegram-router; LISTEN_ADDR, default :8080)
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
)

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
type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		IsTopicMessage bool `json:"is_topic_message"`
	} `json:"message"`
}

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
	m := upd.Message
	if m == nil || m.Chat == nil || m.Text == "" || m.IsTopicMessage {
		return "", Signal{}, false
	}
	chatID := strconv.FormatInt(m.Chat.ID, 10)

	a.mu.Lock()
	names := make([]string, 0, len(a.sources))
	for name := range a.sources {
		names = append(names, name)
	}
	sort.Strings(names) // stable pick when two sources name one chat
	var match string
	var cfg sourceConfig
	for _, name := range names {
		if a.sources[name].cfg.ChatID == chatID {
			match, cfg = name, a.sources[name].cfg
			break
		}
	}
	a.mu.Unlock()
	if match == "" {
		return "", Signal{}, false
	}
	if len(cfg.Approvers) > 0 && (m.From == nil || !containsID(cfg.Approvers, m.From.ID)) {
		return "", Signal{}, false // not an approved user — ignore silently
	}

	labels := map[string]string{labelChannel: cfg.Channel}
	if m.From != nil {
		sender := m.From.Username
		if sender == "" {
			sender = strconv.FormatInt(m.From.ID, 10)
		}
		labels[labelSender] = sender
	}
	return match, Signal{
		// update_id is unique per bot, so identical text twice never
		// collapses on cooldown — repeating yourself is not dedup.
		Fingerprint: "tg-" + strconv.FormatInt(upd.UpdateID, 10),
		Labels:      labels,
		Kind:        "chat",
		Payload:     m.Text,
		Title:       title(m.Text),
	}, true
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
