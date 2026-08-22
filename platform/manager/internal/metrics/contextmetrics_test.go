package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// A SKIP is counted, never dropped. An unchanged context writing nothing is the
// design working — and a run of skips with no checkpoint between them is
// exactly how a stalled context looks from outside.
func TestContextSkipsAreCounted(t *testing.T) {
	c := New(nil)
	reg := prometheus.NewRegistry()
	if err := reg.Register(c.contextOps); err != nil {
		t.Fatal(err)
	}
	c.Observe(activity.Event{
		Kind: activity.KindContextSkipped, Status: activity.StatusOK,
		Code: activity.CodeContextInterval, Conversation: "c1",
	})
	got := testutil.ToFloat64(c.contextOps.WithLabelValues("skip", activity.CodeContextInterval, activity.StatusOK))
	if got != 1 {
		t.Fatalf("skip count = %v, want 1", got)
	}
}

func TestContextCheckpointRecordsBytes(t *testing.T) {
	c := New(nil)
	c.Observe(activity.Event{
		Kind: activity.KindContextCheckpoint, Status: activity.StatusOK,
		Code: activity.CodeContextWorkBoundary, Detail: `{"bytes":4096,"files":2}`,
	})
	out := collect(t, c.contextBytes)
	if !strings.Contains(out, "agentops_context_checkpoint_bytes") {
		t.Fatalf("histogram not exported: %s", out)
	}
}

// Detail is FREE TEXT. A malformed one must not panic the metrics path — a
// telemetry bug taking down the manager would be far worse than a lost sample.
func TestMalformedDetailIsHarmless(t *testing.T) {
	c := New(nil)
	c.Observe(activity.Event{
		Kind: activity.KindContextCheckpoint, Status: activity.StatusOK,
		Code: activity.CodeContextWorkBoundary, Detail: "not json at all",
	})
	if got := contextBytes(activity.Event{Detail: "not json"}); got != 0 {
		t.Fatalf("bytes = %d, want 0 for unparseable detail", got)
	}
}

func TestContextOperationLabelsAreBounded(t *testing.T) {
	for kind, want := range map[string]string{
		activity.KindContextRestored:   "restore",
		activity.KindContextCheckpoint: "checkpoint",
		activity.KindContextSkipped:    "skip",
		activity.KindContextFailed:     "failed",
		"something.else":               "",
	} {
		if got := contextOperation(kind); got != want {
			t.Errorf("contextOperation(%q) = %q, want %q", kind, got, want)
		}
	}
}

func collect(t *testing.T, c prometheus.Collector) string {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	fams, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fams {
		sb.WriteString(f.GetName())
	}
	return sb.String()
}
