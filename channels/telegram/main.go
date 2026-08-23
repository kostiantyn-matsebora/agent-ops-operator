// channel-telegram: the reference channel adapter. Serves Channels with
// spec.adapter=telegram against the operator's adapter contract — no Kubernetes
// access, no operator changes needed to run more instances of this pattern
// for other transports (Slack, Teams, …).
//
//	outbound: long-poll GET /channel/ops?adapter=telegram  ->  Bot API calls
//	inbound:  POST /updates (from gateway-telegram)         ->  POST /channel/inbound
//	offset:   GET/PUT /offset (for the router)             ->  adapter state API
//
// This adapter DOES NOT POLL. Telegram serves exactly one update stream per
// bot token, so gateway-telegram owns the single getUpdates loop and forwards
// each update to whichever component it belongs to: a forum-topic message
// CONTINUES a conversation and comes here; a general-surface message
// ORIGINATES one and goes to signal-telegram instead. Conversations are born
// on the signal path only — this adapter carries them, it never starts one.
//
// It still persists the router's offset, because it is the component holding
// a Channel to annotate: the router reports each confirmed offset through
// GET/PUT /offset and this adapter writes it to the contract's state API, so
// restarts never replay updates and the router needs no RBAC of its own.
//
// Credentials are per channel: each served Channel's credentialsSecretRef is
// projected into this pod's environment and located via the channel listing's
// credentialEnvPrefix (key botToken); TELEGRAM_BOT_TOKEN remains the fallback
// for channels without projected credentials (hand-deployed deployments). The
// SAME Secret backs the router, which needs the token to poll.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, TELEGRAM_BOT_TOKEN (optional
// fallback), ADAPTER_NAME (default "telegram"), LISTEN_ADDR (default ":8080" —
// in-cluster the reconciler injects it from ChannelAdapter.spec.port),
// projected AGENTOPS_CRED_* vars.
package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// channelConfig is this adapter's interpretation of Channel spec.config.
//
// There is no pollingEnabled any more: this adapter never polls, so a channel
// cannot opt into a loop that no longer exists here. Ingest is enabled by
// running gateway-telegram against the bot token.
type channelConfig struct {
	ChatID       string  `json:"chatId"`
	FeedThreadID *int64  `json:"feedThreadId,omitempty"`
	Approvers    []int64 `json:"approvers,omitempty"`
	// DeleteTopicOnConversationDelete opts this SURFACE out of keeping the
	// forum topic when its conversation is deleted. Absent = false = the topic
	// survives with a tombstone, which is the default because the transcript
	// above it is what a person scrolls back to.
	//
	// On the CHANNEL rather than the adapter: whether a group's threads should
	// outlive their conversations is a property of the group, and two surfaces
	// served by one adapter may reasonably differ.
	DeleteTopicOnConversationDelete bool `json:"deleteTopicOnConversationDelete,omitempty"`
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
	listen        string

	// pace spreads outbound work across the Bot API's budgets. Telegram
	// REJECTS rather than queues, so without this a burst is lost, not delayed.
	pace *pacer

	// menu tracks the command vocabulary this adapter last registered with
	// Telegram, so a change reaches the composer without re-registering an
	// unchanged list on every poll.
	menu *menu

	mu       sync.Mutex
	channels map[string]servedChannel // validated, by channel name
	reported map[string]string        // last status message per channel (avoid spam)
	clients  map[string]*Telegram     // bot client per token
}

// servedChannelList snapshots the validated channels, deduplicated by chat: one
// chat is one command scope however many Channels name it.
func (a *adapter) servedChannelList() []servedChannel {
	a.mu.Lock()
	defer a.mu.Unlock()
	seen := map[string]bool{}
	out := make([]servedChannel, 0, len(a.channels))
	for _, sc := range a.channels {
		if id := sc.cfg.ChatID; id != "" && !seen[id] {
			seen[id] = true
			out = append(out, sc)
		}
	}
	return out
}

// chatIDs lists the chats this adapter currently serves.
func (a *adapter) chatIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	seen := map[string]bool{}
	out := make([]string, 0, len(a.channels))
	for _, sc := range a.channels {
		if id := sc.cfg.ChatID; id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func main() {
	channelType := os.Getenv("ADAPTER_NAME")
	if channelType == "" {
		channelType = "telegram"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	a := &adapter{
		mgr:           NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		channelType:   channelType,
		fallbackToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		listen:        listen,
		pace:          newPacer(),
		menu:          newMenu(),
		channels:      map[string]servedChannel{},
		reported:      map[string]string{},
		clients:       map[string]*Telegram{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("channel-telegram adapter starting (adapter=%s, listen=%s)", channelType, listen)

	go a.opsLoop(ctx)
	go a.refreshLoop(ctx)

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
	mux.HandleFunc("GET /offset", a.handleOffsetGet)
	mux.HandleFunc("PUT /offset", a.handleOffsetPut)
	return mux
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
		// PACE BEFORE CLAIMING, not before sending. Work this adapter cannot
		// yet deliver stays queued in the manager, where it is still derivable
		// from CR state and survives an adapter restart. Gating the send
		// instead would hold a claim while waiting, and a crash mid-wait would
		// strand the op until ReclaimAfter.
		if !a.pace.wait(ctx, a.chatIDs()) {
			continue
		}
		op, revision, err := a.mgr.NextOp(ctx, a.channelType, 25)
		// The revision rides EVERY poll response, delivered op or not, so a
		// vocabulary change reaches this adapter while it is otherwise idle —
		// the manager cannot dial us. Acted on before the error check: a poll
		// that failed may still have carried it.
		a.syncCommands(ctx, revision)
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
		// The claim window is the manager's to state and ours to respect: every
		// Bot API call this op makes shares one retry budget, bounded well
		// inside it.
		threadID, opErr := a.execute(WithRetryBudget(ctx, op.RetryBudget()), op)
		if err := a.mgr.CompleteOp(ctx, op.ID, threadID, opErr); err != nil && ctx.Err() == nil {
			log.Printf("complete op %s: %v", op.ID, err)
		}
	}
}

// refreshLoop re-reads served channels on a timer.
//
// Without it a Channel's spec.config is read ONCE at startup and again only
// when an op names a channel this adapter has never seen — so an edit to an
// EXISTING channel's config never reached the adapter until its pod restarted.
// That made every config change silently require a restart, which is not how
// any other part of this system behaves; the console adapter has had this loop
// all along. Found when a newly enabled deleteTopicOnConversationDelete did
// nothing on a live surface.
func (a *adapter) refreshLoop(ctx context.Context) {
	for ctx.Err() == nil {
		sleepCtx(ctx, 60*time.Second)
		if ctx.Err() == nil {
			a.refreshChannels(ctx)
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
		if op.Topic == nil {
			return "", "ensure-topic without a topic descriptor"
		}
		// A REOPEN carries the archived topic as a hint. Telegram can
		// un-archive, so honour it: the conversation continues in the thread it
		// already had, with its history above the new messages. Returning the
		// SAME id is what makes that continuity visible to the manager.
		//
		// Falling through to a fresh topic when un-archiving fails is
		// deliberate — the topic may have been deleted by hand, and a reopen
		// that failed outright would strand a conversation the manager has
		// already moved back to Idle.
		if prev := op.Topic.PreviousThreadID; prev != "" {
			if n, err := strconv.ParseInt(prev, 10, 64); err == nil {
				if err := tg.ReopenTopic(ctx, sc.cfg.ChatID, n); err == nil {
					return prev, ""
				}
			}
		}
		id, err := tg.CreateTopic(ctx, sc.cfg.ChatID, renderTopicName(*op.Topic))
		if err != nil {
			return "", err.Error()
		}
		// POINT AT THE NEW TOPIC, for a conversation a PERSON started.
		//
		// Telegram cannot move somebody's client, and the person who typed the
		// command is standing in the general surface with no sign anything
		// happened — the answer is in a topic they have to go and find. A link
		// is the closest a transport gets to taking them there.
		//
		// Only for the lanes a person initiated. An alert or a cron tick opens
		// a topic nobody is waiting in front of, and a pointer for each would
		// turn the general surface into a log of links.
		//
		// Best-effort: the topic EXISTS, and failing the op over a signpost
		// would make the manager create a second one.
		if op.Topic.Kind == "chat" || op.Topic.Kind == "task" {
			if link := topicLink(sc.cfg.ChatID, id); link != "" {
				name := renderTopicName(*op.Topic)
				if err := tg.Send(ctx, sc.cfg.ChatID, nil,
					`💬 <a href="`+escape(link)+`">`+escape(name)+`</a>`); err != nil {
					log.Printf("point at topic %d: %v", id, err)
				}
			}
		}
		return strconv.FormatInt(id, 10), ""
	case "send":
		if op.Message == nil {
			return "", "send without a message"
		}
		var tid *int64
		if op.ThreadID != nil {
			if n, err := strconv.ParseInt(*op.ThreadID, 10, 64); err == nil {
				tid = &n
			}
		}
		// A very large EVENT payload becomes an attachment rather than a wall of
		// consecutive messages: a 50k alert body would otherwise arrive as a
		// dozen chunks and bury the thread it exists to inform. Only possible
		// because the op carries the payload inline — the adapter has no
		// Kubernetes access to fetch it with.
		if caption, doc, ok := asDocument(*op.Message); ok {
			if err := tg.SendDocument(ctx, sc.cfg.ChatID, tid,
				documentName(*op.Message), caption, doc); err != nil {
				return "", err.Error()
			}
			return "", ""
		}
		// Render here, split here: the Bot API caps a message at 4096 and used
		// to FAIL the whole op past it, which lost exactly the long payloads
		// worth reading. Every chunk must land for the op to succeed.
		//
		// The controls hang off the LAST chunk, where the reader ends up, and
		// the reply linkage off the first, which is the one that answers.
		chunks := renderChunks(a.menu, *op.Message)
		for i, chunk := range chunks {
			extras := SendExtras{}
			if i == 0 {
				extras.ReplyTo = op.Message.InReplyTo
			}
			if i == len(chunks)-1 {
				extras.Keyboard = inlineKeyboard(op.Message.Choices)
				// The ASK goes on the last chunk too — that is where the reader
				// ends up, and it is what they are answering.
				extras.ForceReply = op.Message.ExpectsReply
			}
			if err := tg.SendWith(ctx, sc.cfg.ChatID, tid, chunk, extras); err != nil {
				return "", err.Error()
			}
		}
		return "", ""
	case "delete-conversation":
		// Three calls where one would do, and each is required by the
		// transport: a CLOSED forum topic refuses sendMessage, so the tombstone
		// cannot be posted without un-archiving first — and leaving the topic
		// open afterwards would invite replies into a conversation that no
		// longer exists, which the manager drops because the thread maps to
		// nothing.
		//
		// deleteForumTopic is deliberately NOT used: the transcript above the
		// tombstone is what a person scrolls back to after an incident, and an
		// archived topic already refuses replies without destroying it.
		if op.ThreadID == nil {
			return "", "delete-conversation without a thread id"
		}
		tid, err := strconv.ParseInt(*op.ThreadID, 10, 64)
		if err != nil {
			return "", "delete-conversation: thread id " + *op.ThreadID + " is not a topic id"
		}
		// This SURFACE may have opted out of keeping the thread. Deleting
		// REPLACES the tombstone rather than following it: a topic about to
		// disappear has nobody to tell, and posting first would write into a
		// topic the next call removes.
		//
		// A failure here is REPORTED, never softened into mark-and-archive.
		// deleteForumTopic needs can_delete_messages, and falling back would
		// make the setting mean "delete the topic, or maybe not" — leaving an
		// operator who enabled it for tidiness with archived topics piling up
		// and no signal that anything was wrong.
		if sc.cfg.DeleteTopicOnConversationDelete {
			if err := tg.DeleteTopic(ctx, sc.cfg.ChatID, tid); err != nil {
				return "", err.Error()
			}
			// Leave a trace on the GENERAL surface. Deleting the topic destroys
			// the only place this conversation was visible, and its CR is gone
			// too — so without this line a thread simply vanishes and nothing
			// anywhere says agent-ops did it. A person would reasonably assume
			// somebody deleted it by hand, or that Telegram lost it.
			//
			// Posted AFTER the delete, never before: announcing a removal that
			// then failed would be worse than saying nothing. One line, naming
			// the conversation, because the topic that carried the name is what
			// just went away.
			if err := tg.Send(ctx, sc.cfg.ChatID, nil,
				"🗑 Conversation <code>"+escape(op.Conversation)+
					"</code> was deleted, and its topic removed with it."); err != nil {
				// The topic is already gone; failing the op would ask the
				// manager to retry a deletion that succeeded.
				log.Printf("delete-conversation %s: topic deleted but the general-surface note failed: %v",
					op.Conversation, err)
			}
			return "", ""
		}
		if err := tg.ReopenTopic(ctx, sc.cfg.ChatID, tid); err != nil {
			return "", err.Error()
		}
		if op.Message != nil {
			for _, chunk := range renderChunks(a.menu, *op.Message) {
				if err := tg.Send(ctx, sc.cfg.ChatID, &tid, chunk); err != nil {
					// Close it again even so: a topic left open after a failed
					// tombstone is worse than a missing tombstone.
					_ = tg.CloseTopic(ctx, sc.cfg.ChatID, tid)
					return "", err.Error()
				}
			}
		}
		if err := tg.CloseTopic(ctx, sc.cfg.ChatID, tid); err != nil {
			return "", err.Error()
		}
		return "", ""
	case "close-topic":
		// The conversation is gone; archive its topic. A failure is REPORTED,
		// never retried here — the manager treats a failed close-topic as
		// terminal, so retrying would only delay a deletion that proceeds anyway.
		if op.ThreadID == nil {
			return "", "close-topic without a threadId"
		}
		tid, err := strconv.ParseInt(*op.ThreadID, 10, 64)
		if err != nil {
			return "", "close-topic threadId is not a telegram topic id: " + *op.ThreadID
		}
		if err := tg.CloseTopic(ctx, sc.cfg.ChatID, tid); err != nil {
			return "", err.Error()
		}
		return "", ""
	}
	return "", "unknown op kind " + op.Kind
}

// ---- inbound: topic updates forwarded by the router -------------------------

// handleUpdate takes one raw update the router classified as a CONTINUATION.
//
// Always 204, even when filtered: the router is a dumb pipe and a non-2xx
// would only make it log. Whether a message is policy-eligible is this
// adapter's business, not the router's.
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
	a.dispatch(r.Context(), upd)
	w.WriteHeader(http.StatusNoContent)
}

// dispatch routes one forwarded update to the channel whose chatId matches.
//
// Chat-id matching and approver filtering live here, and the SAME two rules
// live in signal-telegram for the origination side. The duplication is
// deliberate — it is what lets the router stay configuration-free — so both
// sides carry the same test table.
func (a *adapter) dispatch(ctx context.Context, upd tgUpdate) {
	m := upd.Message
	if m == nil || m.Chat == nil || m.Text == "" {
		return
	}
	// A general-surface message must never arrive here — that is the router's
	// origination branch, and adopting it would recreate the very path this
	// design removed: a conversation born on a channel, answered by whichever
	// pipeline happened to be oldest.
	if !m.IsTopicMessage {
		return
	}
	chatID := strconv.FormatInt(m.Chat.ID, 10)
	for _, name := range a.channelNames() {
		sc, ok := a.channel(name)
		if !ok || sc.cfg.ChatID != chatID {
			continue
		}
		if len(sc.cfg.Approvers) > 0 && (m.From == nil || !containsID(sc.cfg.Approvers, m.From.ID)) {
			return // not an approved user — ignore silently
		}
		threadID := strconv.FormatInt(m.MessageThreadID, 10)
		if err := a.mgr.Inbound(ctx, name, &threadID, m.Text); err != nil {
			log.Printf("inbound %s: %v", name, err)
		}
		return // first matching channel wins
	}
}

// channelNames lists served channels sorted by name, so the pick among
// channels sharing a chat id is stable.
func (a *adapter) channelNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.channels))
	for name := range a.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ---- offset: persistence delegated FROM the router --------------------------

// offsetChannel picks the channel whose annotation carries the cursor: the
// first served channel by name. Stable across restarts, and safe if it
// changes — getUpdates never replays updates already confirmed by a
// higher-offset request.
func (a *adapter) offsetChannel() (string, bool) {
	names := a.channelNames()
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

func (a *adapter) handleOffsetGet(w http.ResponseWriter, r *http.Request) {
	name, ok := a.offsetChannel()
	if !ok {
		a.refreshChannels(r.Context())
		if name, ok = a.offsetChannel(); !ok {
			http.Error(w, "no channel served yet — cannot hold an offset", http.StatusServiceUnavailable)
			return
		}
	}
	value, err := a.mgr.GetState(r.Context(), name, "telegram-offset")
	if err != nil {
		log.Printf("offset read %s: %v", name, err)
		http.Error(w, "offset read failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"value": value})
}

func (a *adapter) handleOffsetPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		http.Error(w, "need {\"value\":\"<offset>\"}", http.StatusBadRequest)
		return
	}
	name, ok := a.offsetChannel()
	if !ok {
		http.Error(w, "no channel served yet — cannot hold an offset", http.StatusServiceUnavailable)
		return
	}
	if err := a.mgr.PutState(r.Context(), name, "telegram-offset", in.Value); err != nil {
		log.Printf("offset write %s: %v", name, err)
		http.Error(w, "offset write failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func containsID(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// sleepCtx waits, reporting false when the context ended first — which is how
// a retry loop tells "the interval elapsed" from "we are shutting down".
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
