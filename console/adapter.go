package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// The console as a conforming channel adapter.
//
// It is a channel FIRST and a viewer second: conversations on pipelines that
// list the console Channel bind a console thread, and everything the manager
// fans out to that thread — agent results, acks, attributed relays from
// sibling channels — arrives here as `send` ops. That is why the transcript
// view IS the channel rather than a rendering of one.
//
// The rule this file exists to keep: a received op is NEVER posted back
// inbound. Outbound (ops) and inbound (/channel/inbound) meet only in the
// browser, where a human typed something.

// consoleChannelConfig is this adapter's reading of Channel spec.config.
// Everything is optional: a console channel needs no configuration to work,
// so an empty config is valid rather than an error.
//
// This is where the console's own settings live, and it is the ONLY place they
// can: a ChannelAdapter CR is pure implementation and carries no configuration
// or env, so a chart cannot inject them into the pod. `config` is opaque to the
// manager and interpreted by the serving adapter — which is exactly this.
type consoleChannelConfig struct {
	// DisplayName labels the surface in the UI.
	DisplayName string `json:"displayName,omitempty"`
	// WriteEnabled gates BOTH write paths. A POINTER so "unset" is
	// distinguishable from "false": unset takes the process default (on), and
	// only an explicit false makes the console a strict viewer.
	WriteEnabled *bool `json:"writeEnabled,omitempty"`
	// SignalSource names the SignalSource this console originates from.
	SignalSource string `json:"signalSource,omitempty"`
	// MetricsURL is an optional Prometheus/VictoriaMetrics query endpoint for
	// windows beyond the manager's activity buffer.
	MetricsURL string `json:"metricsUrl,omitempty"`
}

// Adapter runs the /channel/* client loop.
type Adapter struct {
	mgr         *Manager
	cache       *Cache
	transcripts *Transcripts
	adapterName string

	mu       sync.Mutex
	channels map[string]consoleChannelConfig
	reported map[string]string
	uiToken  string
}

// NewAdapter builds the channel-side half of the console.
func NewAdapter(mgr *Manager, cache *Cache, transcripts *Transcripts, adapterName string) *Adapter {
	return &Adapter{
		mgr: mgr, cache: cache, transcripts: transcripts, adapterName: adapterName,
		channels: map[string]consoleChannelConfig{}, reported: map[string]string{},
	}
}

// Run refreshes the served channel list, then long-polls for ops until ctx ends.
//
// It waits for the watch cache first: thread ids are derived from conversation
// UIDs, so serving an ensure-topic before the cache is warm would mint an id
// from the fallback path for no reason.
func (a *Adapter) Run(ctx context.Context) {
	a.cache.WaitForSync(ctx)
	a.refreshChannels(ctx)
	go a.refreshLoop(ctx)
	for ctx.Err() == nil {
		op, err := a.mgr.NextOp(ctx, a.adapterName, 25)
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

func (a *Adapter) refreshLoop(ctx context.Context) {
	for ctx.Err() == nil {
		sleepCtx(ctx, 60*time.Second)
		if ctx.Err() == nil {
			a.refreshChannels(ctx)
		}
	}
}

// refreshChannels re-reads the channels this adapter serves, resolves the
// browser token from their projected credentials, and reports Ready.
//
// The UI token is a per-CHANNEL credential like any other: the Channel
// declares credentialsSecretRef, the reconciler projects the Secret into this
// pod with the advertised prefix, and the key is `uiToken`. Nothing reads a
// Secret through the API — not the manager, not the console.
func (a *Adapter) refreshChannels(ctx context.Context) {
	infos, err := a.mgr.Channels(ctx, a.adapterName)
	if err != nil {
		log.Printf("list channels: %v", err)
		return
	}
	next := map[string]consoleChannelConfig{}
	token := ""
	for _, info := range infos {
		var cfg consoleChannelConfig
		problem := ""
		if len(info.Config) > 0 {
			if err := json.Unmarshal(info.Config, &cfg); err != nil {
				problem = "spec.config is not valid JSON for the console adapter: " + err.Error()
			}
		}
		if info.CredentialEnvPrefix != "" && token == "" {
			token = os.Getenv(info.CredentialEnvPrefix + "uiToken")
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
			continue
		}
		if last != "ok" {
			_ = a.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady", "served by the agent-ops console")
			a.mu.Lock()
			a.reported[info.Name] = "ok"
			a.mu.Unlock()
		}
		next[info.Name] = cfg
	}
	a.mu.Lock()
	a.channels = next
	if token != "" {
		a.uiToken = token
	}
	a.mu.Unlock()
}

// ChannelNames lists the served channels, sorted.
func (a *Adapter) ChannelNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.channels))
	for name := range a.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PrimaryChannel is the console Channel conversations bind to. One console
// surface is the normal case; when several exist the first by name wins, which
// only decides which one the UI treats as "the console channel".
func (a *Adapter) PrimaryChannel() string {
	names := a.ChannelNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// UITokenFromChannel returns the browser token projected via the console
// Channel's credentials, or "" when no channel supplied one.
func (a *Adapter) UITokenFromChannel() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.uiToken
}

// PrimaryConfig returns the served config of the primary console Channel. The
// zero value is valid — a console with no config still works.
func (a *Adapter) PrimaryConfig() consoleChannelConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.channels))
	for name := range a.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return consoleChannelConfig{}
	}
	return a.channels[names[0]]
}

// ThreadFor resolves a conversation's console thread id from the watch cache.
func (a *Adapter) ThreadFor(conversation string) (channel, thread string, ok bool) {
	ch := a.PrimaryChannel()
	if ch == "" {
		return "", "", false
	}
	obj := a.cache.Get("conversations", conversation)
	if obj == nil {
		return "", "", false
	}
	for _, t := range conversationView(obj).Status.Threads {
		if t.Channel == ch {
			return ch, t.ThreadID, true
		}
	}
	return "", "", false
}

// execute performs one op.
func (a *Adapter) execute(ctx context.Context, op *Op) (threadID, opErr string) {
	switch op.Kind {
	case "ensure-topic":
		return a.threadID(op.Conversation), ""
	case "send":
		if op.Message == nil {
			return "", "send without a message"
		}
		thread := ""
		if op.ThreadID != nil {
			thread = *op.ThreadID
		}
		if thread == "" {
			// a channel-level notice with no thread (e.g. an /agents reply):
			// park it on the channel's own pseudo-thread so it is still visible
			thread = "channel:" + op.Channel
		}
		a.transcripts.AppendOp(op.ID, thread, op.Message)
		return "", ""
	case "close-topic":
		// The Conversation is being deleted and its CR is about to vanish from
		// the watch cache. The transcript is NOT: it stays for this console
		// session, marked archived, so whoever was reading the thread when it
		// ended can still see how it ended.
		if op.ThreadID != nil {
			a.transcripts.Archive(*op.ThreadID)
		}
		return "", ""
	}
	return "", "unknown op kind " + op.Kind
}

// threadID derives a conversation's console thread id.
//
// Deterministic from the conversation's UID, so the same conversation maps to
// the same thread across console restarts and a redelivered ensure-topic
// completes identically. Falls back to the conversation NAME when the cache
// has not seen the object — also deterministic, and the binding the manager
// persists is whichever id it received first, so the two can never disagree
// about a live conversation.
func (a *Adapter) threadID(conversation string) string {
	if obj := a.cache.Get("conversations", conversation); obj != nil && obj.Metadata.UID != "" {
		return "console-" + obj.Metadata.UID
	}
	return "console-name-" + conversation
}

// Send posts a UI-typed message into the manager's router and records it as
// pending locally.
//
// This is the ONLY caller of /channel/inbound in the console. Ops arriving on
// the outbound path never reach it, so a relay of a console user's own message
// cannot loop back out — it can only confirm the pending bubble.
func (a *Adapter) Send(ctx context.Context, conversation, sender, text string) (Message, error) {
	channel, thread, ok := a.ThreadFor(conversation)
	if !ok {
		return Message{}, errNotJoined
	}
	if err := a.mgr.Inbound(ctx, channel, thread, sender, text); err != nil {
		return Message{}, err
	}
	return a.transcripts.AppendLocal("local:"+nowRFC3339(), thread, sender, strings.TrimSpace(text)), nil
}

// errNotJoined: the conversation has no console thread, so there is nowhere to
// post. Observed conversations are read-only by construction, not by a UI rule.
var errNotJoined = &consoleError{"conversation is not joined to the console channel"}

type consoleError struct{ msg string }

func (e *consoleError) Error() string { return e.msg }
