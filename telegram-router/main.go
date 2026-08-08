// telegram-router: the single getUpdates consumer for a bot token, and the
// only component that knows what a Telegram topic MEANS.
//
//	Telegram getUpdates ─▶ telegram-router ─┬─ no topic ─▶ signal-telegram  ─▶ /signal/inbound
//	                                        └─ topic    ─▶ channel-telegram ─▶ /channel/inbound
//
// Why this process exists: Telegram serves exactly one update stream per bot
// token — a second concurrent getUpdates gets 409, and passing `offset`
// destructively confirms updates for every reader. So origination (general
// surface) and continuation (forum topic) cannot each poll for themselves;
// one process must read the stream and fan it out.
//
// Its whole job is that fan-out. It classifies on is_topic_message, which
// rides on the update itself, so the decision is LOCAL — no manager round-trip
// and no shared state. It forwards updates verbatim and holds no channel
// configuration: chat-id matching and approver filtering stay in the receiving
// adapters, each against its own contract listing.
//
// It reads ONE thing from the manager, at startup and on refresh: its own
// SignalSource, for the forwarding targets and the env prefix its bot token
// was projected under. Adapter CRs carry no configuration by design, so a
// served CR's config is the only channel per-deployment settings can travel.
//
// It persists nothing. The offset value is the router's (it makes the call),
// but storage is delegated downstream to channel-telegram, which already
// speaks the adapter state API. The router therefore needs no ServiceAccount
// token and no RBAC.
//
// Run single-instance (replicas 1 / Recreate — the SignalAdapter reconciler's
// singleton mode does exactly that). Two routers on one token is the failure
// that costs the most debugging.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default
// "telegram-router"), TELEGRAM_BOT_TOKEN (optional fallback), and the
// projected AGENTOPS_CRED_* vars.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// routerConfig is this adapter's interpretation of its SignalSource
// spec.config. Only forwarding targets — no chat id, no approvers, nothing
// about any particular chat surface.
type routerConfig struct {
	// SignalTarget receives general-surface updates (originations).
	SignalTarget string `json:"signalTarget"`
	// ChannelTarget receives topic updates (continuations) and persists the
	// offset on the router's behalf.
	ChannelTarget string `json:"channelTarget"`
}

// servedSource is one validated source with its resolved bot token.
type servedSource struct {
	cfg   routerConfig
	token string
}

type router struct {
	mgr        *Manager
	down       *Downstream
	sourceType string
	fallback   string

	mu       sync.Mutex
	sources  map[string]servedSource
	reported map[string]string
	clients  map[string]*Telegram
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
	sourceType := os.Getenv("ADAPTER_NAME")
	if sourceType == "" {
		sourceType = "telegram-router"
	}
	r := &router{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		down:       NewDownstream(),
		sourceType: sourceType,
		fallback:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		sources:    map[string]servedSource{},
		reported:   map[string]string{},
		clients:    map[string]*Telegram{},
		loops:      map[string]context.CancelFunc{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("telegram-router starting (adapter=%s)", sourceType)

	r.pollManager(ctx)
	r.loopWG.Wait()
}

// refreshSources re-reads served sources, validates config, resolves each
// bot token (projected credentials via credentialEnvPrefix, key botToken;
// TELEGRAM_BOT_TOKEN fallback), and reports validity changes as the source's
// Ready condition.
func (r *router) refreshSources(ctx context.Context) {
	infos, err := r.mgr.Sources(ctx, r.sourceType)
	if err != nil {
		log.Printf("list sources: %v", err)
		return
	}
	next := map[string]servedSource{}
	for _, info := range infos {
		var cfg routerConfig
		problem := ""
		token := ""
		if info.CredentialEnvPrefix != "" {
			token = os.Getenv(info.CredentialEnvPrefix + "botToken")
		}
		if token == "" {
			token = r.fallback
		}
		if len(info.Config) == 0 {
			problem = "spec.config is missing"
		} else if err := json.Unmarshal(info.Config, &cfg); err != nil {
			problem = "spec.config is not valid JSON for the telegram router: " + err.Error()
		} else if cfg.SignalTarget == "" {
			problem = "spec.config.signalTarget is required (base URL of the telegram signal adapter)"
		} else if cfg.ChannelTarget == "" {
			problem = "spec.config.channelTarget is required (base URL of the telegram channel adapter)"
		} else if token == "" {
			problem = "no bot token: set SignalSource.credentialsSecretRef (Secret key botToken) — the same Secret the Channel uses — or provide a TELEGRAM_BOT_TOKEN fallback"
		}
		r.mu.Lock()
		last := r.reported[info.Name]
		r.mu.Unlock()
		if problem != "" {
			if last != problem {
				_ = r.mgr.ReportStatus(ctx, info.Name, false, "InvalidConfig", problem)
				r.mu.Lock()
				r.reported[info.Name] = problem
				r.mu.Unlock()
			}
			continue // keep serving the other sources
		}
		if last != "ok" {
			_ = r.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady", "polled by telegram-router")
			r.mu.Lock()
			r.reported[info.Name] = "ok"
			r.mu.Unlock()
		}
		next[info.Name] = servedSource{cfg: cfg, token: token}
	}
	r.mu.Lock()
	r.sources = next
	r.mu.Unlock()
}

// client returns (caching) the Bot API client for a token.
func (r *router) client(token string) *Telegram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.clients[token]; c != nil {
		return c
	}
	c := NewTelegram(token)
	r.clients[token] = c
	return c
}

// pollManager keeps exactly one polling goroutine alive per distinct bot token
// among served sources. Sources sharing a token share a loop — the
// single-consumer rule holds per TOKEN, not per CR.
func (r *router) pollManager(ctx context.Context) {
	for ctx.Err() == nil {
		r.refreshSources(ctx)

		want := map[string]bool{}
		r.mu.Lock()
		for _, ss := range r.sources {
			want[ss.token] = true
		}
		for token, cancel := range r.loops {
			if !want[token] {
				cancel()
				delete(r.loops, token)
			}
		}
		for token := range want {
			if _, running := r.loops[token]; running {
				continue
			}
			loopCtx, cancel := context.WithCancel(ctx)
			r.loops[token] = cancel
			r.loopWG.Add(1)
			go func(token string) {
				defer r.loopWG.Done()
				r.pollToken(loopCtx, token)
			}(token)
		}
		r.mu.Unlock()

		sleepCtx(ctx, 30*time.Second)
	}
}

// leader returns the served source that owns a token's cursor and targets:
// the lexicographically first, so the choice is stable across restarts.
func (r *router) leader(token string) (servedSource, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var names []string
	for name, ss := range r.sources {
		if ss.token == token {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return servedSource{}, false
	}
	sort.Strings(names)
	return r.sources[names[0]], true
}

// pollToken is the single getUpdates consumer for one bot token: read the
// persisted offset once, then poll → classify → forward → report the confirmed
// offset downstream.
func (r *router) pollToken(ctx context.Context, token string) {
	tg := r.client(token)
	offset := int64(-1) // -1 = not yet obtained
	for ctx.Err() == nil {
		lead, ok := r.leader(token)
		if !ok {
			return // pollManager cancels us; be safe anyway
		}
		if offset < 0 {
			raw, err := r.down.GetOffset(ctx, lead.cfg.ChannelTarget)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("offset read: %v", err)
					sleepCtx(ctx, 5*time.Second)
				}
				continue
			}
			offset, _ = strconv.ParseInt(raw, 10, 64)
		}
		updates, err := tg.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("getUpdates: %v", err)
				sleepCtx(ctx, 5*time.Second)
			}
			continue
		}
		for _, upd := range updates {
			r.route(ctx, lead.cfg, upd)
			offset = upd.UpdateID + 1
			// Report AFTER forwarding: a crash in between replays the batch,
			// which is harmless (signals collapse on fingerprint, and a
			// duplicate topic message is the same exposure the single-container
			// adapter had on restart). Losing an update would not be.
			if err := r.down.PutOffset(ctx, lead.cfg.ChannelTarget, strconv.FormatInt(offset, 10)); err != nil {
				if ctx.Err() == nil {
					log.Printf("offset report: %v", err)
				}
				break
			}
		}
	}
}

// route is the entire routing rule: topic → continuation → channel adapter;
// general surface → origination → signal adapter. The update goes on verbatim.
func (r *router) route(ctx context.Context, cfg routerConfig, upd update) {
	target := cfg.SignalTarget
	if upd.IsTopicMessage {
		target = cfg.ChannelTarget
	}
	if err := r.down.Forward(ctx, target, upd.Raw); err != nil {
		log.Printf("forward update %d: %v", upd.UpdateID, err)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
