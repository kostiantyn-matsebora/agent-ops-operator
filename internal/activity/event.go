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

import "time"

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
)

// Drop codes for KindSignalDropped — bounded, so they may be metric labels.
const (
	// CodeUnclaimed: no Ready Pipeline claims the source (Wired=False).
	CodeUnclaimed = "unclaimed"
	// CodeAtCapacity: the pending conversation backlog is full.
	CodeAtCapacity = "at-capacity"
)

// Status values. Failure is RECORDED, never omitted: an operations console that
// only shows successes is worse than none.
const (
	StatusOK    = "ok"
	StatusError = "error"
)

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

	// Adapter records which adapter reported an event that arrived over
	// POST /activity. Manager-emitted events leave it empty. It is set from the
	// authenticated token's scope, never from the request body, which is what
	// makes "an adapter cannot report as another" enforceable.
	Adapter string `json:"adapter,omitempty"`
}
