package metrics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
)

// wired builds a log whose ONLY observer is the metric set — the shape the
// manager runs in, so a test that drives events also drives metrics.
func wired(t *testing.T, sample SampleFunc) (*activity.Log, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	c := New(sample)
	c.Register(reg)
	log := activity.New(4096)
	log.AddObserver(c)
	return log, reg
}

func gather(t *testing.T, reg *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*dto.MetricFamily{}
	for _, f := range families {
		out[f.GetName()] = f
	}
	return out
}

// counterValue sums a family's counter values.
func counterValue(f *dto.MetricFamily) float64 {
	total := 0.0
	for _, m := range f.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	return total
}

// 1b.2a: an event and its metric observation cannot occur independently. The
// emitter fans out to the ring buffer AND the registry from one call, so this
// asserts the coupling rather than two instrumentation passes agreeing today.
func TestEveryEventKindMovesItsMetric(t *testing.T) {
	cases := []struct {
		name   string
		event  activity.Event
		metric string
	}{
		{"received", activity.Event{
			Kind: activity.KindSignalReceived,
			From: activity.Node(activity.NodeSignalAdapter, "cron"),
			To:   activity.Node(activity.NodeSignalSource, "nightly"),
		}, "agentops_signals_received_total"},
		{"dropped", activity.Event{
			Kind: activity.KindSignalDropped, Status: activity.StatusError,
			From: activity.Node(activity.NodeSignalSource, "nightly"),
			Code: activity.CodeUnclaimed,
		}, "agentops_signals_dropped_total"},
		{"created", activity.Event{
			Kind: activity.KindConversationCreated, Pipeline: "k8s-ops", Conversation: "c-1",
		}, "agentops_conversations_created_total"},
		{"run", activity.Event{
			Kind: activity.KindRunCompleted, Pipeline: "k8s-ops", RunID: "r-1", LatencyMs: 4218,
		}, "agentops_runs_total"},
		{"op", activity.Event{
			Kind: activity.KindChannelOpCompleted, Code: "send", OpID: "send:1", LatencyMs: 120,
			From: activity.Node(activity.NodeChannelAdapter, "telegram"),
		}, "agentops_channel_ops_total"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log, reg := wired(t, nil)
			log.Emit(tc.event)
			f := gather(t, reg)[tc.metric]
			if f == nil {
				t.Fatalf("%s was not produced by emitting %s", tc.metric, tc.event.Kind)
			}
			if counterValue(f) != 1 {
				t.Fatalf("%s = %v, want 1", tc.metric, counterValue(f))
			}
			// and the event is in the buffer too — same emission, both outputs
			held, _ := log.Since("", 0)
			if len(held) != 1 || held[0].Kind != tc.event.Kind {
				t.Fatalf("the event did not reach the ring buffer: %+v", held)
			}
		})
	}
}

func TestHistogramsObserveOnlyMeasuredLatencies(t *testing.T) {
	log, reg := wired(t, nil)
	log.Emit(activity.Event{Kind: activity.KindRunCompleted, Pipeline: "p", LatencyMs: 2000})
	log.Emit(activity.Event{Kind: activity.KindRunCompleted, Pipeline: "p"}) // unmeasured
	f := gather(t, reg)["agentops_run_duration_seconds"]
	if f == nil {
		t.Fatal("no run duration histogram")
	}
	h := f.GetMetric()[0].GetHistogram()
	if h.GetSampleCount() != 1 {
		t.Fatalf("sample count = %d, want 1 (an unmeasured run must not record 0s)", h.GetSampleCount())
	}
	if h.GetSampleSum() != 2 {
		t.Fatalf("sample sum = %v seconds, want 2 (base units, not milliseconds)", h.GetSampleSum())
	}
}

// 1b.2b, first half: no metric declares a label that CAN carry an id. The rule
// is structural — the label names are what enforce it, so this reads them.
func TestNoMetricDeclaresAnIdentifyingLabel(t *testing.T) {
	allowed := map[string]bool{
		"pipeline": true, "adapter": true, "source": true, "channel": true,
		"kind": true, "status": true, "reason": true,
	}
	log, reg := wired(t, func() Sample {
		return Sample{
			Queues:                []QueueSample{{Adapter: "telegram", Queued: 1}},
			ConversationsInflight: map[string]int{"k8s-ops": 1},
			CooldownsActive:       map[string]int{"nightly": 2},
		}
	})
	// touch every counter/histogram so its family is present to inspect
	log.Emit(activity.Event{Kind: activity.KindSignalReceived,
		From: activity.Node(activity.NodeSignalAdapter, "a"), To: activity.Node(activity.NodeSignalSource, "s")})
	log.Emit(activity.Event{Kind: activity.KindSignalDropped, Code: activity.CodeUnclaimed,
		From: activity.Node(activity.NodeSignalSource, "s")})
	log.Emit(activity.Event{Kind: activity.KindConversationCreated, Pipeline: "p"})
	log.Emit(activity.Event{Kind: activity.KindRunCompleted, Pipeline: "p", LatencyMs: 1})
	log.Emit(activity.Event{Kind: activity.KindChannelOpCompleted, Code: "send", LatencyMs: 1,
		From: activity.Node(activity.NodeChannelAdapter, "telegram")})

	for name, f := range gather(t, reg) {
		if !strings.HasPrefix(name, "agentops_") {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if !allowed[l.GetName()] {
					t.Fatalf("%s declares label %q — labels carry only CR-bounded values; "+
						"identities belong in GET /status", name, l.GetName())
				}
			}
		}
	}
}

// 1b.2b, second half: series count is unchanged after driving thousands of
// distinct conversations, runs and ops. This is the property the rule exists
// for — a fingerprint or op id reaching a label would fail here, loudly.
func TestSeriesCountIsUnchangedByVolume(t *testing.T) {
	log, reg := wired(t, nil)
	seed := func(i int) {
		log.Emit(activity.Event{
			Kind: activity.KindConversationCreated, Pipeline: "k8s-ops",
			Conversation: fmt.Sprintf("chat-%d", i),
		})
		log.Emit(activity.Event{
			Kind: activity.KindRunCompleted, Pipeline: "k8s-ops", Status: activity.StatusOK,
			Conversation: fmt.Sprintf("chat-%d", i), RunID: fmt.Sprintf("r-%d", i), LatencyMs: 100,
		})
		log.Emit(activity.Event{
			Kind: activity.KindChannelOpCompleted, Code: "send", Status: activity.StatusOK,
			From:      activity.Node(activity.NodeChannelAdapter, "telegram"),
			OpID:      fmt.Sprintf("send:%d", i),
			Detail:    fmt.Sprintf("fingerprint-%d", i),
			LatencyMs: 10,
		})
	}
	seed(0)
	before := seriesCount(t, reg)
	for i := 1; i <= 3000; i++ {
		seed(i)
	}
	if after := seriesCount(t, reg); after != before {
		t.Fatalf("series count grew from %d to %d over 3000 conversations — "+
			"an unbounded value reached a label", before, after)
	}
}

func seriesCount(t *testing.T, reg *prometheus.Registry) int {
	t.Helper()
	n := 0
	for name, f := range gather(t, reg) {
		if strings.HasPrefix(name, "agentops_") {
			n += len(f.GetMetric())
		}
	}
	return n
}

// Gauges are LEVELS sampled at scrape time, from the same in-memory state
// /status reports. With no sampler wired they report nothing rather than zero:
// a zero meaning "not wired" is a lie an alert would act on.
func TestGaugesComeFromTheSampler(t *testing.T) {
	_, reg := wired(t, func() Sample {
		return Sample{
			Queues: []QueueSample{{Adapter: "telegram", Queued: 3, Claimed: 1,
				OldestQueuedAge: 42, OldestClaimedAge: 7}},
			RuntimeSlotsInUse: 5, RuntimeSlotsMax: 5,
			ConversationsInflight: map[string]int{"k8s-ops": 2},
			CooldownsActive:       map[string]int{"nightly": 9},
		}
	})
	got := gather(t, reg)
	for name, want := range map[string]float64{
		"agentops_channel_ops_queued":                    3,
		"agentops_channel_ops_claimed":                   1,
		"agentops_channel_op_oldest_queued_age_seconds":  42,
		"agentops_channel_op_oldest_claimed_age_seconds": 7,
		"agentops_runtime_slots_in_use":                  5,
		"agentops_runtime_slots_max":                     5,
		"agentops_conversations_inflight":                2,
		"agentops_cooldowns_active":                      9,
	} {
		f := got[name]
		if f == nil {
			t.Fatalf("%s missing", name)
		}
		if v := f.GetMetric()[0].GetGauge().GetValue(); v != want {
			t.Fatalf("%s = %v, want %v", name, v, want)
		}
	}

	_, empty := wired(t, nil)
	if f := gather(t, empty)["agentops_runtime_slots_max"]; f != nil {
		t.Fatal("with no sampler the gauges must report nothing, not zero")
	}
}

// Every series carries HELP and TYPE — the standard conventions, so an ordinary
// Prometheus stack consumes this without house knowledge.
func TestMetricsCarryHelpAndUseStandardNaming(t *testing.T) {
	log, reg := wired(t, func() Sample { return Sample{RuntimeSlotsMax: 5} })
	log.Emit(activity.Event{Kind: activity.KindConversationCreated, Pipeline: "p"})
	for name, f := range gather(t, reg) {
		if !strings.HasPrefix(name, "agentops_") {
			continue
		}
		if f.GetHelp() == "" {
			t.Fatalf("%s has no HELP", name)
		}
		if f.GetType() == dto.MetricType_COUNTER && !strings.HasSuffix(name, "_total") {
			t.Fatalf("counter %s must end in _total", name)
		}
		if f.GetType() != dto.MetricType_COUNTER && strings.HasSuffix(name, "_total") {
			t.Fatalf("%s is not a counter but ends in _total", name)
		}
	}
}
