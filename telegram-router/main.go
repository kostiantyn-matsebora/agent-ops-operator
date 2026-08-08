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
// rides on the update itself, so the decision is LOCAL. It forwards updates
// verbatim and holds no channel configuration: chat-id matching and approver
// filtering stay in the receiving adapters, each against its own contract
// listing.
//
// This is PLUMBING, not an adapter. It produces no signals, so it is not a
// SignalAdapter and has no served CR: the chart that renders the two real
// adapters also renders this Deployment and injects the two Service URLs it
// forwards to, because that chart is what knows them. It never contacts the
// manager — no contract client, no adapter token, no trust domain to be in.
//
// It persists nothing. The offset value is the router's (it makes the call),
// but storage is delegated downstream to channel-telegram, which already
// speaks the adapter state API. The router therefore needs no ServiceAccount
// token and no RBAC.
//
// Run single-instance (replicas 1 / Recreate). One Deployment per bot token
// makes that structural rather than bookkeeping: two routers on one token is
// the failure that costs the most debugging.
//
// Environment (all required except where noted):
//
//	TELEGRAM_BOT_TOKEN  the bot to poll; or AGENTOPS_CRED_<PREFIX>botToken
//	SIGNAL_TARGET       base URL of signal-telegram  (originations)
//	CHANNEL_TARGET      base URL of channel-telegram (continuations + offset)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// config is everything this process needs, and it all arrives as env. There is
// no CR to read: forwarding targets are in-cluster Service URLs the chart
// renders, and the bot token is projected from the same Secret the Channel
// sends with.
type config struct {
	// SignalTarget receives general-surface updates (originations).
	SignalTarget string
	// ChannelTarget receives topic updates (continuations) and persists the
	// offset on the router's behalf.
	ChannelTarget string
	// Token is the bot whose stream this process owns, exclusively.
	Token string
}

type router struct {
	cfg  config
	down *Downstream
	tg   *Telegram
}

// loadConfig reads the environment and fails loudly. A missing value is a
// misconfiguration that can never fix itself, so the process exits rather than
// idling: a crash-looping pod names the problem in `kubectl describe`, where an
// operator will actually see it.
func loadConfig() config {
	c := config{
		SignalTarget:  strings.TrimSuffix(os.Getenv("SIGNAL_TARGET"), "/"),
		ChannelTarget: strings.TrimSuffix(os.Getenv("CHANNEL_TARGET"), "/"),
		Token:         os.Getenv("TELEGRAM_BOT_TOKEN"),
	}
	// The chart projects the bot Secret with a prefix; accept either the plain
	// name or exactly one projected AGENTOPS_CRED_*botToken.
	if c.Token == "" {
		for _, kv := range os.Environ() {
			k, v, ok := strings.Cut(kv, "=")
			if ok && strings.HasPrefix(k, "AGENTOPS_CRED_") && strings.HasSuffix(k, "botToken") && v != "" {
				c.Token = v
				break
			}
		}
	}
	var missing []string
	if c.Token == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN (or a projected AGENTOPS_CRED_*botToken)")
	}
	if c.SignalTarget == "" {
		missing = append(missing, "SIGNAL_TARGET (base URL of the telegram signal adapter)")
	}
	if c.ChannelTarget == "" {
		missing = append(missing, "CHANNEL_TARGET (base URL of the telegram channel adapter)")
	}
	if len(missing) > 0 {
		log.Fatalf("telegram-router is misconfigured, missing: %s", strings.Join(missing, ", "))
	}
	return c
}

func main() {
	cfg := loadConfig()
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram(cfg.Token)}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("telegram-router starting (signal=%s channel=%s)", cfg.SignalTarget, cfg.ChannelTarget)

	r.poll(ctx)
}

// poll is the single getUpdates consumer for this bot token: read the
// persisted offset once, then poll → classify → forward → report the confirmed
// offset downstream.
func (r *router) poll(ctx context.Context) {
	offset := int64(-1) // -1 = not yet obtained
	for ctx.Err() == nil {
		if offset < 0 {
			raw, err := r.down.GetOffset(ctx, r.cfg.ChannelTarget)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("offset read: %v", err)
					sleepCtx(ctx, 5*time.Second)
				}
				continue
			}
			offset, _ = strconv.ParseInt(raw, 10, 64)
		}
		updates, err := r.tg.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("getUpdates: %v", err)
				sleepCtx(ctx, 5*time.Second)
			}
			continue
		}
		for _, upd := range updates {
			r.route(ctx, upd)
			offset = upd.UpdateID + 1
			// Report AFTER forwarding: a crash in between replays the batch,
			// which is harmless (signals collapse on fingerprint, and a
			// duplicate topic message is the same exposure the single-container
			// adapter had on restart). Losing an update would not be.
			if err := r.down.PutOffset(ctx, r.cfg.ChannelTarget, strconv.FormatInt(offset, 10)); err != nil {
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
func (r *router) route(ctx context.Context, upd update) {
	target := r.cfg.SignalTarget
	if upd.IsTopicMessage {
		target = r.cfg.ChannelTarget
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
