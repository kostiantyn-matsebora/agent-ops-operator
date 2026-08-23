package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Manager is a client for the operator's channel adapter contract
// (/channel/* on the manager's API port, bearer-token auth).
//
// The console reads CONFIGURATION from the Kubernetes API and CONVERSATION
// TRAFFIC from this contract — the same split every other adapter has, just
// with the read side visible. Nothing here writes CRs: thread bindings are
// established by the manager when it completes an ensure-topic op, not by the
// console touching Conversation status.
type Manager struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewManager builds a contract client; the timeout leaves room for the 25s
// ops long-poll.
func NewManager(baseURL, token string) *Manager {
	return &Manager{BaseURL: baseURL, Token: token, HTTP: &http.Client{Timeout: 40 * time.Second}}
}

// Op mirrors the manager's outbound operation shape.
type Op struct {
	ID           string  `json:"id"`
	Channel      string  `json:"channel"`
	Conversation string  `json:"conversation,omitempty"`
	Kind         string  `json:"kind"` // "ensure-topic" | "send" | "close-topic" | "delete-conversation"
	ThreadID     *string `json:"threadId,omitempty"`

	Topic   *TopicDescriptor `json:"topic,omitempty"`   // ensure-topic
	Message *OpMessage       `json:"message,omitempty"` // send, delete-conversation
}

// ContractVersion is the outbound message contract this console speaks. The
// manager refuses the ops long-poll without it — an adapter reading the retired
// `text` field would render empty transcript entries and look healthy doing it.
//
// IT DOES NOT MOVE FOR THE BLOCK GRAMMAR. The body is markdown plus a grammar,
// and the SPA parses both — see ui/src/api/blocks.ts. Nothing was added to the
// wire, so there is nothing to version.
const ContractVersion = "2"

// OpMessage is one SEMANTIC outbound message: the manager composes meaning, each
// adapter composes presentation. The console renders in the browser, so it
// keeps the fields STRUCTURED all the way to the transcript rather than
// flattening them to a string here.
type OpMessage struct {
	Kind string `json:"kind"` // signal | answer | relay | notice
	Body string `json:"body,omitempty"`

	Pipeline string            `json:"pipeline,omitempty"`
	Source   string            `json:"source,omitempty"`
	Title    string            `json:"title,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	InputRef string            `json:"inputRef,omitempty"`

	Origin string `json:"origin,omitempty"`
	Sender string `json:"sender,omitempty"`

	Status string `json:"status,omitempty"`
	Level  string `json:"level,omitempty"`

	// Choices are the actions this message OFFERS — structured like Labels, not
	// prose. The console is a browser surface, so it renders them as controls
	// and keeps them structured all the way to the transcript.
	Choices []OpChoice `json:"choices,omitempty"`
	// InReplyTo is the transport handle for the message this one answers. The
	// console has no use for it — its transcript is already ordered and every
	// entry is visible — so it is carried and ignored rather than dropped.
	InReplyTo string `json:"inReplyTo,omitempty"`
}

// OpChoice is one offered action.
type OpChoice struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

// TopicDescriptor describes a thread to create. The console has no naming limit
// worth enforcing, so it uses the title as given.
type TopicDescriptor struct {
	Conversation string            `json:"conversation"`
	Pipeline     string            `json:"pipeline,omitempty"`
	Source       string            `json:"source,omitempty"`
	Title        string            `json:"title,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Kind         string            `json:"kind,omitempty"`
}

// Render composes the plain-text form of a message for a transcript entry.
// Markdown passes through untouched — the SPA renders it — but the structured
// fields are laid out here, because the console is the surface that can show
// them most fully.
func (m *OpMessage) Render() string {
	if m == nil {
		return ""
	}
	switch m.Kind {
	case "relay":
		who := m.Origin
		if m.Sender != "" {
			who = m.Origin + "/" + m.Sender
		}
		return "💬 **" + who + "**: " + m.Body
	case "signal":
		// A CARD, NOT A SENTENCE. Every part is its own block, and the two
		// questions a reader has first — what fired, and where it came from —
		// are answered before anything else.
		//
		// This ran title, attribution, labels and the payload together with
		// single newlines and no fence. Markdown reflows that into ONE line,
		// so the source was buried mid-sentence among a dozen chips and the
		// raw JSON trailed off the end of it.
		var parts []string
		shown := map[string]bool{}
		if m.Title != "" {
			parts = append(parts, "📣 **"+m.Title+"**")
			for _, w := range strings.Fields(strings.ReplaceAll(m.Title, ":", " ")) {
				shown[w] = true
			}
		}

		// WHERE IT CAME FROM, on its own line and labelled. It is the first
		// thing asked of any alert and it was previously a scrap of italic text
		// between the title and a wall of chips.
		var from []string
		if m.Source != "" {
			from = append(from, "**Source** `"+m.Source+"`")
			shown[m.Source] = true
		}
		// Omitted when blank rather than filled in: the pipeline is inferred and
		// an empty value means "not determinable", not "none".
		if m.Pipeline != "" {
			from = append(from, "**Pipeline** `"+m.Pipeline+"`")
			shown[m.Pipeline] = true
		}
		if len(from) > 0 {
			parts = append(parts, strings.Join(from, " · "))
		}

		// LABELS AS A TABLE. A row per label is scannable — a reader looks down
		// one column for the key they want. Joined into a line they are a run
		// of `k=v` nobody reads, which is what shipped.
		if rows := labelRows(m, shown); len(rows) > 0 {
			parts = append(parts, "| label | value |\n|---|---|\n"+strings.Join(rows, "\n"))
		}

		// The payload is NOT here: it travels separately so the browser can
		// collapse it. See SignalPayload.
		if m.InputRef != "" {
			parts = append(parts, "_full event: `conversationinput/"+m.InputRef+"`_")
		}
		return strings.Join(parts, "\n\n")
	default:
		return m.Body
	}
}

// ChannelInfo is one channel served by this adapter, with its opaque config.
// CredentialEnvPrefix (set when the Channel declares credentialsSecretRef)
// locates the channel's projected credentials in this process's environment:
// Secret key K is available as env <prefix>K.
type ChannelInfo struct {
	Name                string          `json:"name"`
	Config              json.RawMessage `json:"config,omitempty"`
	CredentialEnvPrefix string          `json:"credentialEnvPrefix,omitempty"`
}

func (m *Manager) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.BaseURL+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}
	if out != nil && resp.StatusCode != 204 {
		return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode, nil
}

// NextOp long-polls for the next outbound op; nil when none arrived in time.
func (m *Manager) NextOp(ctx context.Context, adapter string, waitSeconds int) (*Op, error) {
	var op Op
	code, err := m.do(ctx, "GET",
		fmt.Sprintf("/channel/ops?adapter=%s&contract=%s&wait=%d",
			url.QueryEscape(adapter), ContractVersion, waitSeconds), nil, &op)
	if err != nil {
		return nil, err
	}
	if code == 204 {
		return nil, nil
	}
	return &op, nil
}

// CompleteOp reports an op result (threadID for ensure-topic; opErr on failure).
// Vocabulary fetches what may be typed on a chat surface.
//
// The console CAN see Pipelines — it watches them for topology and
// configuration — and fetches this anyway. The typeahead and a Telegram command
// menu answer the same question, and this module cannot import the manager's
// derivation of it: they are separate Go modules by design. Two independent
// derivations of one fact is exactly the drift this endpoint removes.
func (m *Manager) Vocabulary(ctx context.Context) (Vocabulary, error) {
	var v Vocabulary
	_, err := m.do(ctx, "GET", "/channel/vocabulary", nil, &v)
	return v, err
}

// Vocabulary is the manager's list of what a person may type.
type Vocabulary struct {
	Revision string            `json:"revision"`
	Entries  []VocabularyEntry `json:"entries"`
}

// VocabularyEntry is one thing a person may type.
//
// Position is what the console can honour that a chat transport cannot: it has
// TWO composers — one that starts a conversation and one attached to an
// existing one — and they take disjoint sets.
type VocabularyEntry struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    string `json:"position"`
	// Icon is how the entry is recognised in a list — an emoji, or nothing.
	Icon    string `json:"icon,omitempty"`
	Profile string `json:"profile,omitempty"`
}

func (m *Manager) CompleteOp(ctx context.Context, opID, threadID, opErr string) error {
	body := map[string]string{}
	if threadID != "" {
		body["threadId"] = threadID
	}
	if opErr != "" {
		body["error"] = opErr
	}
	_, err := m.do(ctx, "POST", "/channel/ops/"+url.PathEscape(opID)+"/done", body, nil)
	return err
}

// Inbound pushes one user message into the manager's router. threadId is
// required by the contract: the console continues conversations, it never
// originates one.
func (m *Manager) Inbound(ctx context.Context, channel, threadID, sender, text string) error {
	_, err := m.do(ctx, "POST", "/channel/inbound", map[string]any{
		"channel": channel, "threadId": threadID, "text": text, "sender": sender,
	}, nil)
	return err
}

// Reopen brings a closed conversation back to Idle. The manager decides
// whether this surface may: reach is the conversation's own channelRefs, read
// there and never taken from this request.
//
// This and Delete below are the only calls the console makes that are not
// /channel/inbound, and they exist because a CLOSED conversation has no thread
// to post a command on. The console still performs no Kubernetes write — both
// are manager verbs over the same authenticated adapter path as everything else
// here.
func (m *Manager) Reopen(ctx context.Context, channel, conversation string) error {
	_, err := m.do(ctx, "POST", "/channel/conversations/"+url.PathEscape(conversation)+"/reopen",
		map[string]any{"channel": channel}, nil)
	return err
}

// Delete reclaims a conversation the manager has already closed. It refuses
// anything not already Closed, which is what keeps the irreversible step
// something an operator ordered twice.
func (m *Manager) Delete(ctx context.Context, channel, conversation string) error {
	_, err := m.do(ctx, "POST", "/channel/conversations/"+url.PathEscape(conversation)+"/delete",
		map[string]any{"channel": channel}, nil)
	return err
}

// ReadEntry reports one thread as seen up to a point in its activity.
type ReadEntry struct {
	ThreadID string `json:"threadId"`
	ReadAt   string `json:"readAt"`
	// Reader is the OPAQUE key of whoever read it — a salted hash, never an
	// identity. Empty reports the channel-wide mark, which is what a console
	// with no salt projected falls back to.
	Reader string `json:"reader,omitempty"`
}

// ReadOutcome is the manager's per-thread verdict on a read report.
type ReadOutcome struct {
	ThreadID string `json:"threadId"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason,omitempty"`
}

// ReportRead records how far this console's threads have been seen. The manager
// writes the watermark; the console performs no Kubernetes write, exactly as
// with every other verb here.
//
// The watermark is monotonic and clamped manager-side, so a report that would
// not advance comes back skipped rather than as an error.
func (m *Manager) ReportRead(ctx context.Context, channel string, reads []ReadEntry) ([]ReadOutcome, error) {
	var out struct {
		Results []ReadOutcome `json:"results"`
	}
	_, err := m.do(ctx, "POST", "/channel/read",
		map[string]any{"channel": channel, "reads": reads}, &out)
	return out.Results, err
}

// Channels lists the channels this adapter serves.
func (m *Manager) Channels(ctx context.Context, adapter string) ([]ChannelInfo, error) {
	var out []ChannelInfo
	_, err := m.do(ctx, "GET", "/channel/channels?adapter="+url.QueryEscape(adapter), nil, &out)
	return out, err
}

// ReportStatus sets the channel's Ready condition.
func (m *Manager) ReportStatus(ctx context.Context, channel string, ready bool, reason, message string) error {
	_, err := m.do(ctx, "POST", "/channel/channels/"+url.PathEscape(channel)+"/status",
		map[string]any{"ready": ready, "reason": reason, "message": message}, nil)
	return err
}

// ---- manager introspection --------------------------------------------------
//
// THE BOUNDARY: the manager exposes only what only the manager knows. Everything
// below exists in NO Kubernetes object — op queues are in-memory by design,
// runtime slots are counted from live pods, cooldowns are per-source memory, and
// capability resolution is manager logic. CR state is read from the API server
// and is never fetched through here.

// QueueStat is one adapter's outstanding-op picture. The IDENTITIES live here
// rather than in a metric label: metrics answer "how deep, how old", this
// answers "which one".
type QueueStat struct {
	Adapter                 string  `json:"adapter"`
	Queued                  int     `json:"queued"`
	Claimed                 int     `json:"claimed"`
	OldestQueuedOpID        string  `json:"oldestQueuedOpId,omitempty"`
	OldestQueuedAgeSeconds  float64 `json:"oldestQueuedAgeSeconds,omitempty"`
	OldestQueuedConv        string  `json:"oldestQueuedConversation,omitempty"`
	OldestClaimedOpID       string  `json:"oldestClaimedOpId,omitempty"`
	OldestClaimedAgeSeconds float64 `json:"oldestClaimedAgeSeconds,omitempty"`
	OldestClaimedConv       string  `json:"oldestClaimedConversation,omitempty"`
}

// CooldownStat reports a suppressed signal lane. A suppressed lane looks exactly
// like an idle one on a graph, which is the whole reason it is reported.
type CooldownStat struct {
	Source        string  `json:"source"`
	Suppressed    int     `json:"suppressed"`
	WindowSeconds float64 `json:"windowSeconds"`
}

// ManagerStatus is GET /status.
type ManagerStatus struct {
	Version      string `json:"version,omitempty"`
	Leader       string `json:"leader,omitempty"`
	Now          string `json:"now,omitempty"`
	RuntimeSlots struct {
		InUse   int `json:"inUse"`
		Max     int `json:"max"`
		Waiting int `json:"waiting"`
	} `json:"runtimeSlots"`
	Queues    []QueueStat    `json:"queues"`
	Cooldowns []CooldownStat `json:"cooldowns"`
	// StorageOutage is present while context storage is being treated as
	// unavailable install-wide. It is the answer to "why is nothing running"
	// that RuntimeSlots alone cannot give: a queue that has stopped moving is
	// either full or storage-blocked, and those demand opposite responses.
	StorageOutage *StorageOutage `json:"storageOutage,omitempty"`
}

// StorageOutage reports an install-wide context-storage outage.
type StorageOutage struct {
	Since      string  `json:"since,omitempty"`
	ForSeconds float64 `json:"forSeconds"`
}

// Status reads the manager's own runtime state.
func (m *Manager) Status(ctx context.Context) (*ManagerStatus, error) {
	var out ManagerStatus
	if _, err := m.do(ctx, "GET", "/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolvedCapabilities is GET /pipelines/{name}/resolved — what an agent routed
// by this pipeline may actually reach. Rendered VERBATIM: the console must not
// recompute composition, because a second implementation would eventually
// disagree with the one that runs, and the console's entire claim is that it
// cannot disagree with the system.
type ResolvedCapabilities struct {
	Pipeline     string   `json:"pipeline"`
	Profile      string   `json:"profile"`
	Runtime      string   `json:"runtime,omitempty"`
	AllowedTools []string `json:"allowedTools"`
	ToolsMode    string   `json:"toolsMode"`
	Toolsets     []string `json:"toolsets"`
	MCPConfigs   []string `json:"mcpConfigs"`
	MCPServers   []string `json:"mcpServers"`
	Unresolved   []string `json:"unresolved,omitempty"`
}

// Resolved reads a pipeline's authoritative capability resolution.
func (m *Manager) Resolved(ctx context.Context, pipeline string) (*ResolvedCapabilities, error) {
	var out ResolvedCapabilities
	if _, err := m.do(ctx, "GET", "/pipelines/"+url.PathEscape(pipeline)+"/resolved", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// fence wraps a signal payload in a code block, tagged json when it looks like
// one so the console highlights it.
//
// A payload is a MACHINE DOCUMENT. Unfenced it is reflowed as prose — every
// newline collapsed, every quote and brace run together with the sentence
// before it, which is what made an event card unreadable.
func fence(body string) string {
	t := strings.TrimSpace(body)
	lang := ""
	switch {
	case strings.HasPrefix(t, "{"), strings.HasPrefix(t, "["):
		lang = "json"
	case !strings.Contains(t, "\n"):
		// A ONE-LINE PAYLOAD IS NOT A DOCUMENT. A posted task is a sentence
		// somebody wrote, and putting it in a code block renders prose in a
		// monospaced box that scrolls sideways — the machine-document treatment
		// applied to something that is not one.
		return t
	}
	// A payload containing its own fence would close ours early. Longer fences
	// nest, which is the markdown-defined way out.
	ticks := "```"
	for strings.Contains(body, ticks) {
		ticks += "`"
	}
	return ticks + lang + "\n" + body + "\n" + ticks
}

// labelRows renders a signal's labels as table rows, omitting any whose VALUE
// the card has already stated.
//
// Suppressed by VALUE, not by key name: a label is redundant only when what it
// says is already on the card, which is a fact about this message rather than
// about the k8s-events vocabulary. An adapter naming its labels differently
// gets the same treatment for free.
//
// Sorted, so a re-derived card does not reshuffle and read as a new one.
func labelRows(m *OpMessage, shown map[string]bool) []string {
	keys := make([]string, 0, len(m.Labels))
	for k := range m.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m.Labels[k]
		if v == "" || shown[v] {
			continue
		}
		rows = append(rows, "| `"+k+"` | "+v+" |")
	}
	return rows
}

// SignalPayload is the raw event document of a `signal`, for a surface that can
// put it behind a control. Empty for every other kind — an answer, a relay and
// a notice are prose to be read, not documents to be opened.
func (m *OpMessage) SignalPayload() string {
	if m == nil || m.Kind != "signal" {
		return ""
	}
	return strings.TrimSpace(m.Body)
}
