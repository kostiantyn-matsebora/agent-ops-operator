// Package metrics exposes agent-ops' operational aggregates in the Prometheus
// exposition format, on the manager's EXISTING :9090 — registered into the
// controller-runtime registry that already serves that port. Nothing new is
// listened on.
//
// ONE INSTRUMENTATION PASS. Counters and histograms are driven by the activity
// event stream (this package is an activity.Observer), not by a second sweep
// over the same call sites. Two passes over the same hops would drift, and a
// console whose stream disagreed with its own charts would be worse than either
// alone. The consequence to preserve: adding an emission site adds its metric,
// and removing one removes it — they cannot be changed apart.
//
// THE CARDINALITY RULE IS BINDING. Labels carry only values bounded by CR count
// — pipeline, adapter, source, channel, kind, status, reason — and are read from
// an event's STRUCTURED fields (node names, Code), never from Detail. A
// conversation id, run id or op id as a label grows series without limit; those
// identify the specific stuck item and live in GET /status. Metrics answer "how
// deep, how old, how many"; /status answers "which one".
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
)

// QueueSample is one adapter's op-queue depth at scrape time.
type QueueSample struct {
	Adapter          string
	Queued           int
	Claimed          int
	OldestQueuedAge  float64
	OldestClaimedAge float64
}

// Sample is the manager's gauge-shaped state, read at scrape time. Gauges are
// LEVELS: they cannot be derived from an event stream, so they are sampled from
// the same in-memory state GET /status reports — one source, two renderings.
type Sample struct {
	Queues                []QueueSample
	RuntimeSlotsInUse     int
	RuntimeSlotsMax       int
	ConversationsInflight map[string]int // by pipeline ("" = unattributed)
	CooldownsActive       map[string]int // by source
}

// SampleFunc supplies a Sample on demand.
type SampleFunc func() Sample

// Collector holds the metric set. Build it with New and register it once.
type Collector struct {
	signalsReceived  *prometheus.CounterVec
	signalsDropped   *prometheus.CounterVec
	conversations    *prometheus.CounterVec
	runs             *prometheus.CounterVec
	runDuration      *prometheus.HistogramVec
	channelOps       *prometheus.CounterVec
	channelOpLatency *prometheus.HistogramVec

	sample SampleFunc

	queued        *prometheus.Desc
	claimed       *prometheus.Desc
	oldestQueued  *prometheus.Desc
	oldestClaimed *prometheus.Desc
	slotsInUse    *prometheus.Desc
	slotsMax      *prometheus.Desc
	inflight      *prometheus.Desc
	cooldowns     *prometheus.Desc
}

// New builds the metric set. sample may be nil, in which case gauges report
// nothing rather than zero — a zero that means "not wired" is a lie an alert
// would act on.
func New(sample SampleFunc) *Collector {
	c := &Collector{
		sample: sample,
		signalsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "agentops_signals_received_total",
			Help: "Signals accepted at /signal/inbound, by source, serving adapter and outcome.",
		}, []string{"source", "adapter", "status"}),
		signalsDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "agentops_signals_dropped_total",
			Help: "Signals dropped without becoming work, by source and reason.",
		}, []string{"source", "reason"}),
		conversations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "agentops_conversations_created_total",
			Help: "Conversations created, by originating pipeline (empty when attribution is ambiguous).",
		}, []string{"pipeline"}),
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "agentops_runs_total",
			Help: "Agent runs completed, by pipeline and outcome.",
		}, []string{"pipeline", "status"}),
		runDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "agentops_run_duration_seconds",
			Help: "Wall time from run dispatch to completion.",
			// Agent runs are seconds-to-minutes, not milliseconds: the default
			// buckets would put almost everything in +Inf.
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800},
		}, []string{"pipeline"}),
		channelOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "agentops_channel_ops_total",
			Help: "Channel operations completed, by adapter, op kind and outcome.",
		}, []string{"adapter", "kind", "status"}),
		channelOpLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "agentops_channel_op_latency_seconds",
			Help:    "Time from enqueue to completion of a channel operation.",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300},
		}, []string{"adapter", "kind"}),

		queued: prometheus.NewDesc("agentops_channel_ops_queued",
			"Channel operations waiting for an adapter to claim them.", []string{"adapter"}, nil),
		claimed: prometheus.NewDesc("agentops_channel_ops_claimed",
			"Channel operations claimed by an adapter and not yet completed.", []string{"adapter"}, nil),
		oldestQueued: prometheus.NewDesc("agentops_channel_op_oldest_queued_age_seconds",
			"Age of the oldest unclaimed channel operation.", []string{"adapter"}, nil),
		oldestClaimed: prometheus.NewDesc("agentops_channel_op_oldest_claimed_age_seconds",
			"Age of the oldest claimed-but-uncompleted channel operation.", []string{"adapter"}, nil),
		slotsInUse: prometheus.NewDesc("agentops_runtime_slots_in_use",
			"Conversations currently backed by a runtime pod.", nil, nil),
		slotsMax: prometheus.NewDesc("agentops_runtime_slots_max",
			"Maximum simultaneously active conversations (MAX_ACTIVE_CONVERSATIONS).", nil, nil),
		inflight: prometheus.NewDesc("agentops_conversations_inflight",
			"Conversations with a run in flight, by pipeline.", []string{"pipeline"}, nil),
		cooldowns: prometheus.NewDesc("agentops_cooldowns_active",
			"Fingerprints currently suppressed by a source's cooldown window.", []string{"source"}, nil),
	}
	return c
}

// MustRegister installs the metric set into the controller-runtime registry
// that already serves the manager's :9090/metrics.
func (c *Collector) MustRegister() { c.Register(ctrlmetrics.Registry) }

// Register installs the metric set into any registerer (tests use a private
// one so a run cannot collide with the process-wide registry).
func (c *Collector) Register(r prometheus.Registerer) {
	r.MustRegister(
		c.signalsReceived, c.signalsDropped, c.conversations, c.runs,
		c.runDuration, c.channelOps, c.channelOpLatency, c,
	)
}

// Describe implements prometheus.Collector for the sampled gauges.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.queued
	ch <- c.claimed
	ch <- c.oldestQueued
	ch <- c.oldestClaimed
	ch <- c.slotsInUse
	ch <- c.slotsMax
	ch <- c.inflight
	ch <- c.cooldowns
}

// Collect samples the manager's in-memory state at scrape time.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c.sample == nil {
		return
	}
	s := c.sample()
	for _, q := range s.Queues {
		ch <- prometheus.MustNewConstMetric(c.queued, prometheus.GaugeValue, float64(q.Queued), q.Adapter)
		ch <- prometheus.MustNewConstMetric(c.claimed, prometheus.GaugeValue, float64(q.Claimed), q.Adapter)
		ch <- prometheus.MustNewConstMetric(c.oldestQueued, prometheus.GaugeValue, q.OldestQueuedAge, q.Adapter)
		ch <- prometheus.MustNewConstMetric(c.oldestClaimed, prometheus.GaugeValue, q.OldestClaimedAge, q.Adapter)
	}
	ch <- prometheus.MustNewConstMetric(c.slotsInUse, prometheus.GaugeValue, float64(s.RuntimeSlotsInUse))
	ch <- prometheus.MustNewConstMetric(c.slotsMax, prometheus.GaugeValue, float64(s.RuntimeSlotsMax))
	for pipeline, n := range s.ConversationsInflight {
		ch <- prometheus.MustNewConstMetric(c.inflight, prometheus.GaugeValue, float64(n), pipeline)
	}
	for source, n := range s.CooldownsActive {
		ch <- prometheus.MustNewConstMetric(c.cooldowns, prometheus.GaugeValue, float64(n), source)
	}
}

// Observe implements activity.Observer: it is the ONLY place counters and
// histograms move. Every label below is read from a structured field — a node
// name or the bounded Code — so no id can reach a label by accident.
func (c *Collector) Observe(e activity.Event) {
	switch e.Kind {
	case activity.KindSignalReceived:
		c.signalsReceived.WithLabelValues(nodeName(e.To), nodeName(e.From), e.Status).Inc()
	case activity.KindSignalDropped:
		c.signalsDropped.WithLabelValues(nodeName(e.From), e.Code).Inc()
	case activity.KindConversationCreated:
		c.conversations.WithLabelValues(e.Pipeline).Inc()
	case activity.KindRunCompleted:
		c.runs.WithLabelValues(e.Pipeline, e.Status).Inc()
		if e.LatencyMs > 0 {
			c.runDuration.WithLabelValues(e.Pipeline).Observe(float64(e.LatencyMs) / 1000)
		}
	case activity.KindChannelOpCompleted:
		adapter := nodeName(e.From)
		c.channelOps.WithLabelValues(adapter, e.Code, e.Status).Inc()
		if e.LatencyMs > 0 {
			c.channelOpLatency.WithLabelValues(adapter, e.Code).Observe(float64(e.LatencyMs) / 1000)
		}
	}
}

func nodeName(n *activity.NodeRef) string {
	if n == nil {
		return ""
	}
	return n.Name
}
