// Package activity is the manager's per-hop telemetry: a bounded, in-memory,
// append-only record of every movement the manager mediates, with cursor replay
// and a subscriber fan-out for streaming.
//
// Three properties are load-bearing and every change here must preserve them:
//
//   - BOUNDED AND LOSSY. A fixed-size ring, oldest evicted first, never
//     persisted and never written to a Kubernetes object. The durable record of
//     what happened stays Conversation.status.runs[].
//   - EMISSION NEVER BLOCKS. No dispatch, reconcile or HTTP handler waits on
//     telemetry. A full buffer drops the oldest event; a slow subscriber is
//     marked lagged and told to resync rather than back-pressuring the emitter.
//   - TELEMETRY IS NOT SIGNAL. Events go to this log and to observers; nothing
//     here reaches /signal/inbound, and no component may convert an event into
//     one. agent-ops' own machinery reports STATUS, never SIGNAL — an error
//     event about agent-ops waking an agent about agent-ops is the loop the
//     no-signal-loops invariant exists to prevent.
package activity

import (
	"sort"
	"time"
	"unicode/utf8"
)

// Node kinds. These are the SAME vocabulary the topology graph uses, so an
// event is renderable as motion along an edge the graph already draws — no
// frontend inference, no second naming scheme to keep in sync.
const (
	NodeSignalAdapter  = "signal-adapter"
	NodeSignalSource   = "signal-source"
	NodePipeline       = "pipeline"
	NodeConversation   = "conversation"
	NodeProfile        = "profile"
	NodeRuntime        = "runtime"
	NodeChannel        = "channel"
	NodeChannelAdapter = "channel-adapter"
	NodeToolset        = "toolset"
	NodeMCPConfig      = "mcp-config"
	// NodeManager is the manager process itself — the endpoint of every hop
	// that arrives from outside (signal receipt, op completion reports).
	NodeManager = "manager"
	// NodeRuntimeImage is the image a runtime pod runs, named by its image
	// reference. It is the `from` of every hop the runtime reports it made,
	// because the harness inside the image made the call, not the AgentRuntime
	// object that selected it.
	NodeRuntimeImage = "runtime-image"
	// NodeModel is a model a runtime called, named as the runtime reported it.
	NodeModel = "model"
	// NodeMCPServer is an MCP server a tool call reached, named by its key in
	// the bound MCPConfigs.
	NodeMCPServer = "mcp-server"
	// NodeExternal is a system outside the install an adapter declares it
	// faces, named as the adapter CR declares it.
	NodeExternal = "external"
)

// Event kinds, one per real hop the manager mediates.
const (
	KindSignalReceived      = "signal.received"      // signal-adapter  -> signal-source
	KindSignalClaimed       = "signal.claimed"       // signal-source   -> pipeline
	KindSignalDropped       = "signal.dropped"       // signal-source   -> nothing
	KindConversationCreated = "conversation.created" // pipeline        -> conversation
	KindInputQueued         = "input.queued"         // *               -> conversation
	KindRunDispatched       = "run.dispatched"       // pipeline        -> runtime
	KindRunCompleted        = "run.completed"        // runtime         -> pipeline
	KindChannelOpEnqueued   = "channel.op.enqueued"  // conversation    -> channel
	KindChannelOpCompleted  = "channel.op.completed" // channel-adapter -> manager
	KindChannelInbound      = "channel.inbound"      // channel-adapter -> conversation
	// KindRuntimeStarting marks the manager CREATING a runtime pod.
	//
	// It exists because the largest slice of a conversation's latency was
	// invisible. Between the last channel op and the first work hop sat a gap —
	// 33 seconds in the run that prompted this — covering pod scheduling,
	// container start and the agent booting to its first poll. The sequence
	// showed a hole and no way to tell a slow image pull from a slow agent.
	//
	// It is STATUS, like every other hop here: the manager already knows it
	// created the pod, and this records that it did.
	KindRuntimeStarting = "runtime.starting" // conversation -> runtime

	// Context hops, reported by the context-sync sidecar. The runtime keeps its
	// live context on pod-local storage; these are the moments it meets the
	// durable volume.
	KindContextRestored   = "context.restored"   // runtime -> conversation
	KindContextCheckpoint = "context.checkpoint" // conversation -> runtime
	// KindContextSkipped is emitted when a checkpoint ran and found NOTHING
	// changed. Deliberately not silent: "nothing changed" and "nothing ran" are
	// different facts, and an operator looking at a context that has stopped
	// advancing needs to tell them apart. It is also the evidence that the
	// skip-when-unchanged rule is working — a volume that is never written to
	// is the goal, not a symptom.
	KindContextSkipped = "context.skipped" // conversation -> runtime
	KindContextFailed  = "context.failed"  // conversation -> runtime

	// Runtime hops. The manager never sees a model or a tool call: the runtime
	// reports them with its work result, and the manager emits one hop each.
	KindModelCall = "model.call" // runtime-image -> model
	KindToolCall  = "tool.call"  // runtime-image -> mcp-server, or nothing for a built-in tool
)

// Context checkpoint codes — BOUNDED, so they are safe as metric labels.
// What triggered the operation, never how big it was.
const (
	// CodeContextWorkBoundary: taken as a work unit completed. Quiesced.
	CodeContextWorkBoundary = "work-done"
	// CodeContextInterval: taken by the periodic timer, possibly mid-run.
	CodeContextInterval = "interval"
	// CodeContextShutdown: the final checkpoint on SIGTERM, which is what
	// covers every ordinary end of a pod.
	CodeContextShutdown = "shutdown"
	// CodeContextStart: the restore before the first work unit.
	CodeContextStart = "start"
)

// Drop codes for KindSignalDropped — bounded, so they may be metric labels.
const (
	// CodeUnclaimed: no Ready Pipeline claims the source (Wired=False).
	CodeUnclaimed = "unclaimed"
	// CodeAtCapacity: the pending conversation backlog is full.
	CodeAtCapacity = "at-capacity"
	// CodeAmbiguous: a bare chat message on a surface SEVERAL Ready Pipelines
	// serve. It has its own code because it is the one drop that is a correct
	// outcome rather than a fault — the person was answered with the choices —
	// and a surface that stops opening conversations must still be diagnosable
	// without reading chat scrollback.
	CodeAmbiguous = "ambiguous"
)

// Status values. Failure is RECORDED, never omitted: an operations console that
// only shows successes is worse than none.
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// The bound on Event.Data. Truncated rather than refused: telemetry never
// fails the request that produced it.
const (
	MaxDataKeys  = 16
	MaxDataValue = 200
)

// BoundData returns data within MaxDataKeys and MaxDataValue. Keys beyond the
// bound are dropped in sorted order, so the same map always keeps the same
// keys. The input is never modified.
func BoundData(data map[string]string) map[string]string {
	if len(data) == 0 {
		return nil
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > MaxDataKeys {
		keys = keys[:MaxDataKeys]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[Truncate(k, MaxDataValue)] = Truncate(data[k], MaxDataValue)
	}
	return out
}

// Truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// NodeRef names one graph node.
type NodeRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Node is the constructor used at emission sites.
func Node(kind, name string) *NodeRef { return &NodeRef{Kind: kind, Name: name} }

// Event is one recorded hop. Deliberately flat and correlation-first: the
// consumer joins on conversation/runId/opId rather than walking a tree.
//
// The shape is span-like on purpose (runId is a trace id in all but name), so
// an OpenTelemetry exporter stays an additive change rather than a rewrite.
type Event struct {
	// Cursor is assigned by the log at emit time — monotonic, zero-padded so
	// lexicographic order is numeric order. Emitters leave it empty.
	Cursor string `json:"cursor"`
	// TS defaults to emit time when an emitter leaves it zero.
	TS   time.Time `json:"ts"`
	Kind string    `json:"kind"`
	// From/To name graph nodes. A hop with no destination (a dropped signal)
	// carries no To.
	From *NodeRef `json:"from,omitempty"`
	To   *NodeRef `json:"to,omitempty"`
	// Status is StatusOK or StatusError; empty is normalized to StatusOK.
	Status string `json:"status"`

	// Correlation. Each is set where it applies and left empty where it does
	// not — an empty pipeline means "not attributable", never "none".
	Conversation string `json:"conversation,omitempty"`
	Pipeline     string `json:"pipeline,omitempty"`
	RunID        string `json:"runId,omitempty"`
	OpID         string `json:"opId,omitempty"`
	InputID      string `json:"inputId,omitempty"`

	// LatencyMs is the measured duration of the hop, where one is measurable.
	LatencyMs int64 `json:"latencyMs,omitempty"`
	// Detail is human-readable context: an exit code, an adapter error, the
	// Wired=False reason a signal was dropped for. FREE TEXT — it may carry a
	// fingerprint, a title or an error message, so it is never a metric label.
	Detail string `json:"detail,omitempty"`
	// Code is the BOUNDED classifier for this event kind: the drop reason for
	// signal.dropped, the op kind for channel ops, the input lane for
	// input.queued. It exists so metrics have something label-safe to key on —
	// the cardinality rule says labels carry only CR-bounded values, and `detail`
	// carries ids that would grow series without limit. Metrics answer "how many,
	// how deep, how old"; the ids stay in the event and in GET /status.
	Code string `json:"code,omitempty"`

	// Data holds bounded facts that exist nowhere else — tokens, a tool's name,
	// a stop reason, checkpoint bytes. Never content: an input's text, a run's
	// result and an op's message have durable homes and are joined from there.
	// Emit bounds it to MaxDataKeys keys of MaxDataValue bytes each.
	Data map[string]string `json:"data,omitempty"`

	// Adapter records which adapter reported an event that arrived over
	// POST /activity. Manager-emitted events leave it empty. It is set from the
	// authenticated token's scope, never from the request body, which is what
	// makes "an adapter cannot report as another" enforceable.
	Adapter string `json:"adapter,omitempty"`
}
