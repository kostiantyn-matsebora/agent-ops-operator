// channel-telegram: the reference channel adapter. Serves Channels with
// spec.type=telegram against the operator's adapter contract — no Kubernetes
// access, no operator changes needed to run more instances of this pattern
// for other transports (Slack, Teams, …).
//
//	outbound: long-poll GET /channel/ops?type=telegram  ->  Bot API calls
//	inbound:  getUpdates per pollingEnabled channel     ->  POST /channel/inbound
//
// The getUpdates offset persists through the contract's state API (a Channel
// annotation on the manager side), so restarts never replay updates. Exactly
// ONE instance may run per bot token (getUpdates single-consumer) — deploy
// replicas: 1 / strategy: Recreate.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, TELEGRAM_BOT_TOKEN,
// CHANNEL_TYPE (default "telegram").
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
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

type adapter struct {
	mgr         *Manager
	tg          *Telegram
	channelType string

	mu       sync.Mutex
	channels map[string]channelConfig // validated, by channel name
	reported map[string]string        // last status message per channel (avoid spam)
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
		mgr:         NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		tg:          NewTelegram(mustEnv("TELEGRAM_BOT_TOKEN")),
		channelType: channelType,
		channels:    map[string]channelConfig{},
		reported:    map[string]string{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("channel-telegram adapter starting (type=%s)", channelType)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.opsLoop(ctx) }()
	go func() { defer wg.Done(); a.pollLoop(ctx) }()
	wg.Wait()
}

// refreshChannels re-reads served channels and validates their config,
// reporting validity changes as the channel's Ready condition.
func (a *adapter) refreshChannels(ctx context.Context) {
	infos, err := a.mgr.Channels(ctx, a.channelType)
	if err != nil {
		log.Printf("list channels: %v", err)
		return
	}
	next := map[string]channelConfig{}
	for _, info := range infos {
		var cfg channelConfig
		problem := ""
		if len(info.Config) == 0 {
			problem = "spec.config is missing"
		} else if err := json.Unmarshal(info.Config, &cfg); err != nil {
			problem = "spec.config is not valid JSON for the telegram adapter: " + err.Error()
		} else if cfg.ChatID == "" {
			problem = "spec.config.chatId is required"
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
		next[info.Name] = cfg
	}
	a.mu.Lock()
	a.channels = next
	a.mu.Unlock()
}

func (a *adapter) config(name string) (channelConfig, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg, ok := a.channels[name]
	return cfg, ok
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
	cfg, ok := a.config(op.Channel)
	if !ok {
		a.refreshChannels(ctx)
		if cfg, ok = a.config(op.Channel); !ok {
			return "", fmt.Sprintf("channel %s is not served (missing or invalid config)", op.Channel)
		}
	}
	switch op.Kind {
	case "ensure-topic":
		id, err := a.tg.CreateTopic(ctx, cfg.ChatID, op.Title)
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
		if err := a.tg.Send(ctx, cfg.ChatID, tid, op.Text); err != nil {
			return "", err.Error()
		}
		return "", ""
	}
	return "", "unknown op kind " + op.Kind
}

// ---- inbound: getUpdates ----------------------------------------------------

func (a *adapter) pollLoop(ctx context.Context) {
	lastRefresh := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastRefresh) > 30*time.Second {
			a.refreshChannels(ctx)
			lastRefresh = time.Now()
		}
		a.mu.Lock()
		names := make([]string, 0, len(a.channels))
		for name, cfg := range a.channels {
			if cfg.PollingEnabled {
				names = append(names, name)
			}
		}
		a.mu.Unlock()
		if len(names) == 0 {
			sleepCtx(ctx, 10*time.Second) // nothing enabled — idle re-list
			continue
		}
		for _, name := range names {
			if err := a.pollChannel(ctx, name); err != nil && ctx.Err() == nil {
				log.Printf("poll %s: %v", name, err)
				sleepCtx(ctx, 5*time.Second)
			}
		}
	}
}

func (a *adapter) pollChannel(ctx context.Context, name string) error {
	cfg, ok := a.config(name)
	if !ok {
		return nil
	}
	raw, err := a.mgr.GetState(ctx, name, "telegram-offset")
	if err != nil {
		return err
	}
	offset, _ := strconv.ParseInt(raw, 10, 64)
	updates, err := a.tg.GetUpdates(ctx, offset)
	if err != nil {
		return err
	}
	for _, upd := range updates {
		offset = upd.UpdateID + 1
		// persist BEFORE handling, like the old poller: a poison message must
		// not be replayed forever
		if err := a.mgr.PutState(ctx, name, "telegram-offset", strconv.FormatInt(offset, 10)); err != nil {
			return err
		}
		m := upd.Message
		if m == nil || m.Chat == nil || strconv.FormatInt(m.Chat.ID, 10) != cfg.ChatID || m.Text == "" {
			continue
		}
		if len(cfg.Approvers) > 0 && (m.From == nil || !containsID(cfg.Approvers, m.From.ID)) {
			continue // not an approved user — ignore silently
		}
		var threadID *string
		if m.IsTopicMessage {
			t := strconv.FormatInt(m.MessageThreadID, 10)
			threadID = &t
		}
		if err := a.mgr.Inbound(ctx, name, threadID, m.Text); err != nil {
			log.Printf("inbound %s: %v", name, err)
		}
	}
	return nil
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
