package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// AuthEnabled says whether the CONSOLE authenticates browsers. A POINTER
	// for the same reason WriteEnabled is one, and with a sharper edge: unset
	// must take the process default (on), because a config that lost the field
	// would otherwise disable the only gate. ExternalAuthenticator names what
	// authenticates instead and is required for a false to take effect.
	AuthEnabled           *bool  `json:"authEnabled,omitempty"`
	ExternalAuthenticator string `json:"externalAuthenticator,omitempty"`
	// SignalSource names the SignalSource this console originates from.
	SignalSource string `json:"signalSource,omitempty"`
	// MetricsURL is an optional Prometheus/VictoriaMetrics query endpoint for
	// windows beyond the manager's activity buffer.
	MetricsURL string `json:"metricsUrl,omitempty"`
}

const (
	// channelRefreshInterval is the steady cadence for re-reading the served
	// channels once at least one has resolved.
	channelRefreshInterval = 60 * time.Second
	// channelBootstrapInterval is how fast the console retries while it serves
	// NO channel yet — a startup race, not a refresh.
	channelBootstrapInterval = time.Second
	// credentialEnvPrefix is what the adapter contract advertises for a
	// channel's projected credentials: AGENTOPS_CRED_<CHANNEL>_<key>.
	credentialEnvPrefix = "AGENTOPS_CRED_"
)

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
	// readerSalt turns a resolved identity into the OPAQUE key sent upstream.
	// Projected as a channel credential, so it never leaves this pod — the
	// manager stores hashes it cannot reverse and never learns who read what.
	readerSalt string
	// saltLogged records that the missing-salt degradation has been reported,
	// so a console without one says so once rather than every refresh.
	saltLogged bool
	// authLogged is the last auth mode written to the log, so a mode that has
	// not changed is not restated every refresh.
	authLogged string
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
	a.adoptProjectedCredentials()
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

// refreshLoop re-reads the served channels: FAST while none have resolved,
// then at the steady cadence.
//
// A flat 60s was both, and that is what made a fresh install serve the wrong
// auth mode for a full minute. The console pod is started by the ChannelAdapter
// reconciler at about the moment the Channel CR is created, so the first listing
// legitimately comes back empty — and the retry for that is a startup retry, not
// a refresh interval. A transient listing error had the same cost.
func (a *Adapter) refreshLoop(ctx context.Context) {
	for ctx.Err() == nil {
		wait := channelRefreshInterval
		if !a.hasChannels() {
			wait = channelBootstrapInterval
		}
		sleepCtx(ctx, wait)
		if ctx.Err() == nil {
			a.refreshChannels(ctx)
		}
	}
}

func (a *Adapter) hasChannels() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.channels) > 0
}

// adoptProjectedCredentials reads the browser token and the reader salt out of
// this pod's OWN ENVIRONMENT, without waiting to be told they are there.
//
// The values are projected by the kubelet at pod creation, so they are present
// before the process starts. What the console lacked was never the value — it
// was the variable NAME, because the prefix is derived from the channel name and
// the console waited for the manager's listing to learn it. That is a discovery
// dependency standing in for a data one, and it cost a browser the whole
// bootstrap window.
//
// The prefix shape is fixed by the adapter contract, so it can be recognised
// rather than asked for. First match wins, which is what refreshChannels does
// with several channels anyway.
func (a *Adapter) adoptProjectedCredentials() {
	token, salt := "", ""
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || value == "" || !strings.HasPrefix(name, credentialEnvPrefix) {
			continue
		}
		switch {
		case token == "" && strings.HasSuffix(name, "_uiToken"):
			token = value
		case salt == "" && strings.HasSuffix(name, "_readerSalt"):
			salt = value
		}
	}
	a.mu.Lock()
	if token != "" && a.uiToken == "" {
		a.uiToken = token
	}
	if salt != "" && a.readerSalt == "" {
		a.readerSalt = salt
	}
	a.mu.Unlock()
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
	token, salt := "", ""
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
		if info.CredentialEnvPrefix != "" && salt == "" {
			salt = os.Getenv(info.CredentialEnvPrefix + "readerSalt")
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
	if salt != "" {
		a.readerSalt = salt
	}
	degraded := a.readerSalt == "" && !a.saltLogged
	if degraded {
		a.saltLogged = true
	}
	a.mu.Unlock()
	if degraded {
		// DEGRADE, never crash and never hash unsalted: addresses are
		// low-entropy, so an unsalted digest of a known one is confirmable by
		// anyone holding the CR — which is the disclosure the hash exists to
		// prevent. Without a salt the console falls back to channel-wide marks,
		// which is simply the behaviour it had before per-identity read state.
		log.Printf("console: no reader salt projected (AGENTOPS_CRED_<CHANNEL>_readerSalt); read state " +
			"falls back to channel-wide marks — everyone using this console shares one watermark")
	}
	a.reportAuthMode()
}

// reportAuthMode logs the EFFECTIVE authentication mode, once per change.
//
// The startup line can only report the process default: the served Channel's
// config is where a chart puts this, and it arrives here, minutes of uptime
// later. Without this line a console authenticating nobody logs "authDefault=
// token" and nothing else ever contradicts it — which is precisely the state
// this setting must never be able to hide in.
func (a *Adapter) reportAuthMode() {
	cfg := a.PrimaryConfig()
	enabled := true
	if cfg.AuthEnabled != nil {
		enabled = *cfg.AuthEnabled
	}
	mode := authMode(enabled, cfg.ExternalAuthenticator)
	a.mu.Lock()
	changed := a.authLogged != mode
	a.authLogged = mode
	a.mu.Unlock()
	if changed {
		log.Printf("console auth: %s", mode)
	}
}

// authMode names how browsers are authenticated. Half a declaration — off with
// no authenticator named — is reported as what it actually does: nothing.
func authMode(enabled bool, external string) string {
	switch {
	case enabled:
		return "token"
	case external != "":
		return "external:" + external
	default:
		return "token (authentication disabled with no external authenticator named — ignored)"
	}
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
		tid := a.threadID(op.Conversation)
		// A REOPEN arrives as an ordinary ensure-topic, and this console's
		// thread id is deterministic, so the reopened conversation lands back
		// in the thread it already had. Clearing the archived flag is what
		// makes it typeable again — the UI derives "you cannot reply here"
		// from it — and it puts a line in the transcript saying the thread is
		// live, so the reopen is visible rather than inferred from a composer
		// quietly reappearing. Silent for a thread that was never archived.
		a.transcripts.Reopen(tid)
		return tid, ""
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
		a.transcripts.AppendOp(op.ID, thread, op.Message, op.Channel)
		return "", ""
	case "delete-conversation":
		// The CR is about to vanish from the watch cache. The transcript is
		// NOT: it stays for this console session with the tombstone at the end,
		// so whoever was reading can still see how it ended — and archived, so
		// there is no composer offering to reply to something that is gone.
		if op.ThreadID != nil {
			if op.Message != nil {
				a.transcripts.AppendOp(op.ID, *op.ThreadID, op.Message, op.Channel)
			}
			a.transcripts.Archive(*op.ThreadID)
		}
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

// Reopen and Delete reach a conversation through its BINDING rather than
// through a thread it holds, because a closed conversation holds none. The
// manager enforces the reach; this side supplies the channel it serves and
// nothing else.
//
// PrimaryChannel rather than ThreadFor: a closed conversation's thread is
// archived and may be gone, so asking for one would refuse every conversation
// these two verbs exist for.
func (a *Adapter) Reopen(ctx context.Context, conversation string) error {
	channel := a.PrimaryChannel()
	if channel == "" {
		return errNotJoined
	}
	return a.mgr.Reopen(ctx, channel, conversation)
}

// ReaderKey turns a resolved identity into the opaque key sent upstream.
//
// Salted, because addresses are low-entropy: a bare sha256 of a known address
// is confirmable by anyone holding the CR and a list of colleagues, which is
// exactly the disclosure hashing exists to prevent. With no salt projected this
// returns "" and every read falls back to the channel-wide mark — degraded, not
// silently unsalted.
//
// An empty identity also returns "": under static-token auth the console
// resolves everyone to "token", so all holders share one key. That is not a
// special case to write, it is what per-IDENTITY means when the credential
// proves possession rather than personhood.
func (a *Adapter) ReaderKey(identity string) string {
	if identity == "" {
		return ""
	}
	a.mu.Lock()
	salt := a.readerSalt
	a.mu.Unlock()
	if salt == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + "\x00" + identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ReadReport is one conversation the browser has seen, with the watermark it
// read off that conversation's own state. The console never invents a "now" —
// the manager clamps anyway, but a client reporting a timestamp it never
// rendered would be marking activity nobody looked at.
type ReadReport struct {
	Conversation string
	ReadAt       string
}

// ReadResult is one conversation's outcome, named for the CONVERSATION rather
// than the thread: names are what the browser sent and what it can act on.
type ReadResult struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// ReportRead marks this console's threads read up to the reported watermarks.
//
// Conversations the console merely OBSERVES are skipped here rather than sent:
// with no console thread there is nothing to report against, and it is the same
// reach boundary bulk close draws.
func (a *Adapter) ReportRead(ctx context.Context, reader string, reports []ReadReport) ([]ReadResult, error) {
	channel := a.PrimaryChannel()
	results := make([]ReadResult, 0, len(reports))
	entries := make([]ReadEntry, 0, len(reports))
	byThread := map[string]string{}
	for _, rep := range reports {
		_, thread, ok := a.ThreadFor(rep.Conversation)
		if !ok || channel == "" {
			results = append(results, ReadResult{Name: rep.Conversation,
				Outcome: closeOutcomeSkipped, Reason: notJoinedReason})
			continue
		}
		if rep.ReadAt == "" {
			results = append(results, ReadResult{Name: rep.Conversation,
				Outcome: closeOutcomeSkipped, Reason: "this conversation has no activity to have been read"})
			continue
		}
		byThread[thread] = rep.Conversation
		entries = append(entries, ReadEntry{ThreadID: thread, ReadAt: rep.ReadAt, Reader: reader})
	}
	if len(entries) == 0 {
		return results, nil
	}
	outcomes, err := a.mgr.ReportRead(ctx, channel, entries)
	if err != nil {
		return results, err
	}
	for _, o := range outcomes {
		results = append(results, ReadResult{Name: byThread[o.ThreadID], Outcome: o.Outcome, Reason: o.Reason})
	}
	return results, nil
}

func (a *Adapter) Delete(ctx context.Context, conversation string) error {
	channel := a.PrimaryChannel()
	if channel == "" {
		return errNotJoined
	}
	return a.mgr.Delete(ctx, channel, conversation)
}

// errNotJoined: the conversation has no console thread, so there is nowhere to
// post. Observed conversations are read-only by construction, not by a UI rule.
var errNotJoined = &consoleError{"conversation is not joined to the console channel"}

type consoleError struct{ msg string }

func (e *consoleError) Error() string { return e.msg }
