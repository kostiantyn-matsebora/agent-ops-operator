// channel-telegram: the reference channel adapter. Serves Channels with
// spec.type=telegram against the operator's adapter contract — no Kubernetes
// access, no operator changes needed to run more instances of this pattern
// for other transports (Slack, Teams, …).
//
//	outbound: long-poll GET /channel/ops?type=telegram  ->  Bot API calls
//	inbound:  one getUpdates loop per DISTINCT bot token -> POST /channel/inbound
//
// Credentials are per channel: each served Channel's credentialsSecretRef is
// projected into this pod's environment and located via the channel listing's
// credentialEnvPrefix (key botToken); TELEGRAM_BOT_TOKEN remains the fallback
// for channels without projected credentials (hand-deployed deployments).
// Channels sharing a token share one getUpdates loop — the single-consumer
// rule holds per TOKEN. Run single-instance (replicas 1 / Recreate; the
// ChannelAdapter reconciler's singleton mode does exactly that).
//
// The getUpdates offset persists through the contract's state API (a Channel
// annotation on the manager side), so restarts never replay updates.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, TELEGRAM_BOT_TOKEN (optional
// fallback), CHANNEL_TYPE (default "telegram"), projected AGENTOPS_CRED_* vars.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// channelConfig is this adapter's interpretation of Channel spec.config.
type channelConfig struct {
	ChatID         string  `json:"chatId"`
	FeedThreadID   *int64  `json:"feedThreadId,omitempty"`
	Approvers      []int64 `json:"approvers,omitempty"`
	PollingEnabled bool    `json:"pollingEnabled,omitempty"`
}

// servedChannel is one validated channel with its resolved credentials.
type servedChannel struct {
	cfg   channelConfig
	token string
}

type adapter struct {
	mgr           *Manager
	channelType   string
	fallbackToken string

	mu       sync.Mutex
	channels map[string]servedChannel // validated, by channel name
	reported map[string]string        // last status message per channel (avoid spam)
	clients  map[string]*Telegram     // bot client per token
	loops    map[string]context.CancelFunc
	loopWG   sync.WaitGroup
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func main() {
	channelType := os.Getenv("CHANNEL_TYPE")
	if channelType == "" {
		channelType = "telegram"
	}
	a := &adapter{
		mgr:           NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		channelType:   channelType,
		fallbackToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		channels:      map[string]servedChannel{},
		reported:      map[string]string{},
		clients:       map[string]*Telegram{},
		loops:         map[string]context.CancelFunc{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("channel-telegram adapter starting (type=%s)", channelType)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.opsLoop(ctx) }()
	go func() { defer wg.Done(); a.pollManager(ctx) }()
	wg.Wait()
	a.loopWG.Wait()
}

// refreshChannels re-reads served channels, validates config, resolves each
// channel's bot token (projected credentials via credentialEnvPrefix, key
// botToken; TELEGRAM_BOT_TOKEN fallback), and reports validity changes as the
// channel's Ready condition.
func (a *adapter) refreshChannels(ctx context.Context) {
	infos, err := a.mgr.Channels(ctx, a.channelType)
	if err != nil {
		log.Printf("list channels: %v", err)
		return
	}
	next := map[string]servedChannel{}
	for _, info := range infos {
		var cfg channelConfig
		problem := ""
		token := ""
		if info.CredentialEnvPrefix != "" {
			token = os.Getenv(info.CredentialEnvPrefix + "botToken")
		}
		if token == "" {
			token = a.fallbackToken
		}
		if len(info.Config) == 0 {
			problem = "spec.config is missing"
		} else if err := json.Unmarshal(info.Config, &cfg); err != nil {
			problem = "spec.config is not valid JSON for the telegram adapter: " + err.Error()
		} else if cfg.ChatID == "" {
			problem = "spec.config.chatId is required"
		} else if token == "" {
			problem = "no bot token: set Channel.credentialsSecretRef (Secret key botToken) or provide the adapter a TELEGRAM_BOT_TOKEN fallback"
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
			continue // keep serving the other channels
		}
		if last != "ok" {
			_ = a.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady", "served by channel-telegram")
			a.mu.Lock()
			a.reported[info.Name] = "ok"
			a.mu.Unlock()
		}
		next[info.Name] = servedChannel{cfg: cfg, token: token}
	}
	a.mu.Lock()
	a.channels = next
	a.mu.Unlock()
}

func (a *adapter) channel(name string) (servedChannel, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sc, ok := a.channels[name]
	return sc, ok
}

// client returns (caching) the Bot API client for a token.
func (a *adapter) client(token string) *Telegram {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.clients[token]; c != nil {
		return c
	}
	c := NewTelegram(token)
	a.clients[token] = c
	return c
}

// ---- outbound: ops long-poll ------------------------------------------------

func (a *adapter) opsLoop(ctx context.Context) {
	a.refreshChannels(ctx)
	for ctx.Err() == nil {
		op, err := a.mgr.NextOp(ctx, a.channelType, 25)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("ops poll: %v", err)
				sleepCtx(ctx, 5*time.Second)
			}
			continue
		}
		if op == nil {
			continue
		}
		threadID, opErr := a.execute(ctx, op)
		if err := a.mgr.CompleteOp(ctx, op.ID, threadID, opErr); err != nil && ctx.Err() == nil {
			log.Printf("complete op %s: %v", op.ID, err)
		}
	}
}

func (a *adapter) execute(ctx context.Context, op *Op) (threadID, opErr string) {
	sc, ok := a.channel(op.Channel)
	if !ok {
		a.refreshChannels(ctx)
		if sc, ok = a.channel(op.Channel); !ok {
			return "", fmt.Sprintf("channel %s is not served (missing/invalid config or credentials)", op.Channel)
		}
	}
	tg := a.client(sc.token)
	switch op.Kind {
	case "ensure-topic":
		id, err := tg.CreateTopic(ctx, sc.cfg.ChatID, op.Title)
		if err != nil {
			return "", err.Error()
		}
		return strconv.FormatInt(id, 10), ""
	case "send":
		var tid *int64
		if op.ThreadID != nil {
			if n, err := strconv.ParseInt(*op.ThreadID, 10, 64); err == nil {
				tid = &n
			}
		}
		if err := tg.Send(ctx, sc.cfg.ChatID, tid, op.Text); err != nil {
			return "", err.Error()
		}
		return "", ""
	}
	return "", "unknown op kind " + op.Kind
}

// ---- inbound: one getUpdates loop per distinct token ------------------------

// pollManager keeps exactly one polling goroutine alive per distinct bot token
// among polling-enabled channels (channels sharing a token share a loop — the
// getUpdates single-consumer rule holds per token).
func (a *adapter) pollManager(ctx context.Context) {
	for ctx.Err() == nil {
		a.refreshChannels(ctx)

		want := map[string]bool{}
		a.mu.Lock()
		for _, sc := range a.channels {
			if sc.cfg.PollingEnabled {
				want[sc.token] = true
			}
		}
		// stop loops for gone tokens, start loops for new ones
		for token, cancel := range a.loops {
			if !want[token] {
				cancel()
				delete(a.loops, token)
			}
		}
		for token := range want {
			if _, running := a.loops[token]; running {
				continue
			}
			loopCtx, cancel := context.WithCancel(ctx)
			a.loops[token] = cancel
			a.loopWG.Add(1)
			go func(token string) {
				defer a.loopWG.Done()
				a.pollToken(loopCtx, token)
			}(token)
		}
		a.mu.Unlock()

		sleepCtx(ctx, 30*time.Second)
	}
}

// tokenGroup returns the polling-enabled channels using a token, sorted by
// name (the first is the group leader holding the shared offset cursor).
func (a *adapter) tokenGroup(token string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var names []string
	for name, sc := range a.channels {
		if sc.token == token && sc.cfg.PollingEnabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// pollToken is the single getUpdates consumer for one bot token. The offset
// cursor persists per channel through the state API: written to the group
// leader, read as the max across the group (safe on leader change — getUpdates
// never replays updates already confirmed by a higher-offset request).
func (a *adapter) pollToken(ctx context.Context, token string) {
	tg := a.client(token)
	for ctx.Err() == nil {
		group := a.tokenGroup(token)
		if len(group) == 0 {
			return // pollManager cancels us; be safe anyway
		}
		offset := int64(0)
		for _, name := range group {
			raw, err := a.mgr.GetState(ctx, name, "telegram-offset")
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("offset read %s: %v", name, err)
					sleepCtx(ctx, 5*time.Second)
				}
				offset = -1
				break
			}
			if n, _ := strconv.ParseInt(raw, 10, 64); n > offset {
				offset = n
			}
		}
		if offset < 0 {
			continue
		}
		updates, err := tg.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("getUpdates (%d channel(s)): %v", len(group), err)
				sleepCtx(ctx, 5*time.Second)
			}
			continue
		}
		leader := group[0]
		for _, upd := range updates {
			offset = upd.UpdateID + 1
			// persist BEFORE handling: a poison message must not replay forever
			if err := a.mgr.PutState(ctx, leader, "telegram-offset", strconv.FormatInt(offset, 10)); err != nil {
				if ctx.Err() == nil {
					log.Printf("offset write %s: %v", leader, err)
				}
				break
			}
			a.dispatch(ctx, group, upd)
		}
	}
}

// dispatch routes one update to the group channel whose chatId matches.
func (a *adapter) dispatch(ctx context.Context, group []string, upd tgUpdate) {
	m := upd.Message
	if m == nil || m.Chat == nil || m.Text == "" {
		return
	}
	chatID := strconv.FormatInt(m.Chat.ID, 10)
	for _, name := range group {
		sc, ok := a.channel(name)
		if !ok || sc.cfg.ChatID != chatID {
			continue
		}
		if len(sc.cfg.Approvers) > 0 && (m.From == nil || !containsID(sc.cfg.Approvers, m.From.ID)) {
			return // not an approved user — ignore silently
		}
		var threadID *string
		if m.IsTopicMessage {
			t := strconv.FormatInt(m.MessageThreadID, 10)
			threadID = &t
		}
		if err := a.mgr.Inbound(ctx, name, threadID, m.Text); err != nil {
			log.Printf("inbound %s: %v", name, err)
		}
		return // first matching channel wins
	}
}

func containsID(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
