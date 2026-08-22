package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// kindsOf lists the event kinds in the log, in order.
func kindsOf(events []activity.Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

// firstOf returns the first event of a kind, or nil.
func firstOf(events []activity.Event, kind string) *activity.Event {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

// indexOf reports the position of the first event of a kind (-1 when absent).
func indexOf(events []activity.Event, kind string) int {
	for i := range events {
		if events[i].Kind == kind {
			return i
		}
	}
	return -1
}

// convEvents filters the log to one conversation.
func convEvents(events []activity.Event, conv string) []activity.Event {
	var out []activity.Event
	for _, e := range events {
		if e.Conversation == conv {
			out = append(out, e)
		}
	}
	return out
}

// 1.9: one conversation driven end to end emits the full hop sequence, in
// order, with from/to naming graph nodes, a shared runId across the run pair,
// and latencies consistent with the timestamps the manager itself stamped.
func TestActivityRecordsAConversationEndToEnd(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-act")
	mkChannel(t, "chan-act", "telegram")
	mkSignalSource(t, "src-act", "am-act", "")
	mkPipeline(t, "act-pipe", []string{"src-act"}, []string{"chan-act"}, "prof-act")
	reconcilePipeline(t, "act-pipe")

	srv, acts := apiServerWithActivity()
	h := srv.Handler()

	rec := postSignal(t, h, testMasterToken, "src-act", []map[string]any{
		{"fingerprint": "act-fp-1", "labels": map[string]string{"alertname": "ActTest"}, "payload": "disk full"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}

	all, _ := acts.Since("", 0)
	if got := firstOf(all, activity.KindSignalReceived); got == nil {
		t.Fatalf("no signal.received in %v", kindsOf(all))
	} else if got.From.Kind != activity.NodeSignalAdapter || got.From.Name != "am-act" ||
		got.To.Kind != activity.NodeSignalSource || got.To.Name != "src-act" {
		t.Fatalf("signal.received names the wrong nodes: %+v -> %+v", got.From, got.To)
	}

	created := firstOf(all, activity.KindConversationCreated)
	if created == nil {
		t.Fatalf("no conversation.created in %v", kindsOf(all))
	}
	convName := created.Conversation
	// The shared namespace is FIFO-sensitive: a conversation left needing a
	// runtime pod jumps the capacity tests' queue. Every conversation a test
	// creates gets cleaned up.
	t.Cleanup(func() { cleanupConversation(t, convName) })
	if created.From.Kind != activity.NodePipeline || created.From.Name != "act-pipe" {
		t.Fatalf("conversation.created must flow from the CLAIMING pipeline: %+v", created.From)
	}

	claimed := firstOf(all, activity.KindSignalClaimed)
	if claimed == nil || claimed.Pipeline != "act-pipe" ||
		claimed.From.Name != "src-act" || claimed.To.Name != "act-pipe" {
		t.Fatalf("signal.claimed must name the claiming pipeline: %+v", claimed)
	}
	// The claim precedes creation: it is the decision that PRODUCES the
	// conversation, so it carries none.
	if indexOf(all, activity.KindSignalClaimed) > indexOf(all, activity.KindConversationCreated) {
		t.Fatalf("the claim must precede the conversation it creates: %v", kindsOf(all))
	}
	if q := firstOf(all, activity.KindInputQueued); q == nil || q.InputID == "" || q.Conversation != convName {
		t.Fatalf("input.queued must carry the input id and conversation: %+v", q)
	}

	// A channel-bound conversation waits for a thread binding before its first
	// dispatch, so drive the topic through the real op path — which also puts
	// channel.op.enqueued and channel.op.completed on the log.
	if _, err := reconcilerWithOps(srv.Ops).Reconcile(ctx,
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: convName}}); err != nil {
		t.Fatal(err)
	}
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("ensure-topic op not queued: %d", rec.Code)
	}
	var topicOp chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &topicOp)
	if rec = adapterReq(srv, "POST", "/channel/ops/"+topicOp.ID+"/done",
		map[string]any{"threadId": "thread-act"}, testMasterToken); rec.Code != 200 {
		t.Fatalf("op done: %d %s", rec.Code, rec.Body.String())
	}

	// dispatch
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/work?convo="+convName+"&wait=0", nil))
	if rec.Code != 200 {
		t.Fatalf("work: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID string `json:"runId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)

	done, _ := json.Marshal(map[string]any{
		"convo": convName, "runId": unit.RunID, "status": "succeeded", "result": "cleaned up",
	})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/work/done", strings.NewReader(string(done))))
	if rec.Code != 200 {
		t.Fatalf("done: %d %s", rec.Code, rec.Body.String())
	}

	all, _ = acts.Since("", 0)
	mine := convEvents(all, convName)
	kinds := kindsOf(mine)

	// The whole conversation-bearing lifecycle, in the order it happened.
	want := []string{
		activity.KindConversationCreated,
		activity.KindInputQueued,
		activity.KindRunDispatched,
		activity.KindRunCompleted,
	}
	prev := -1
	for _, k := range want {
		i := indexOf(mine, k)
		if i < 0 {
			t.Fatalf("missing %s in %v", k, kinds)
		}
		if i < prev {
			t.Fatalf("%s out of order in %v", k, kinds)
		}
		prev = i
	}

	dispatched, completed := firstOf(mine, activity.KindRunDispatched), firstOf(mine, activity.KindRunCompleted)
	if dispatched.RunID == "" || dispatched.RunID != completed.RunID {
		t.Fatalf("run events must share a runId: %q vs %q", dispatched.RunID, completed.RunID)
	}
	if dispatched.To.Kind != activity.NodeRuntime || completed.From.Kind != activity.NodeRuntime {
		t.Fatalf("run hops must name the runtime node: %+v / %+v", dispatched.To, completed.From)
	}
	// Latency is derived from the manager's own dispatch stamp, so it must be
	// consistent with the two events' timestamps rather than independent of them.
	elapsed := completed.TS.Sub(dispatched.TS).Milliseconds()
	if completed.LatencyMs < 0 || completed.LatencyMs > elapsed+2000 {
		t.Fatalf("latency %dms is not consistent with %dms between the events",
			completed.LatencyMs, elapsed)
	}

	// The channel half: intent recorded on enqueue, delivery confirmed on
	// completion — the two are separate events on purpose, so an edge can say
	// "sent, unconfirmed" rather than claiming success.
	enq := firstOf(all, activity.KindChannelOpEnqueued)
	if enq == nil || enq.To.Kind != activity.NodeChannel {
		t.Fatalf("channel.op.enqueued must flow TO a channel node: %+v", enq)
	}
	comp := firstOf(all, activity.KindChannelOpCompleted)
	if comp == nil || comp.From.Kind != activity.NodeChannelAdapter || comp.OpID == "" {
		t.Fatalf("channel.op.completed must come FROM the adapter and name the op: %+v", comp)
	}
}

// A failing run is RECORDED as an error, not omitted. An operations console
// that only shows successes is worse than none.
func TestActivityRecordsFailure(t *testing.T) {
	mkProfile(t, "prof-actfail")
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "act-fail-1", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-actfail"}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"}}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "act-fail-1") })
	srv, acts := apiServerWithActivity()
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/work?convo=act-fail-1&wait=0", nil))
	var unit struct {
		RunID string `json:"runId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)

	body, _ := json.Marshal(map[string]any{
		"convo": "act-fail-1", "runId": unit.RunID, "status": "failed", "exitCode": 2,
	})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/work/done", strings.NewReader(string(body))))

	all, _ := acts.Since("", 0)
	e := firstOf(all, activity.KindRunCompleted)
	if e == nil || e.Status != activity.StatusError || !strings.Contains(e.Detail, "exit 2") {
		t.Fatalf("failure must be recorded with its reason: %+v", e)
	}
}

// A dropped signal is recorded with the Wired=False reason and no destination.
func TestActivityRecordsDroppedSignals(t *testing.T) {
	mkSignalSource(t, "src-actdrop", "am-actdrop", "")
	srv, acts := apiServerWithActivity()

	rec := postSignal(t, srv.Handler(), testMasterToken, "src-actdrop", []map[string]any{
		{"fingerprint": "drop-fp-1", "payload": "nobody is listening"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	all, _ := acts.Since("", 0)
	e := firstOf(all, activity.KindSignalDropped)
	if e == nil {
		t.Fatalf("no signal.dropped in %v", kindsOf(all))
	}
	if e.To != nil {
		t.Fatalf("a dropped signal has no destination: %+v", e.To)
	}
	if e.Status != activity.StatusError || e.Code != activity.CodeUnclaimed ||
		!strings.Contains(e.Detail, "Wired=False") {
		t.Fatalf("drop must carry the Wired=False reason and a bounded code: %+v", e)
	}
}

// 1.10 (the invariant): telemetry is not signal. A storm of events — including
// error events ABOUT agent-ops' own components — creates zero Conversations and
// writes zero Kubernetes objects. This is the no-signal-loops invariant asserted
// from the telemetry side: agent-ops' own health is STATUS, never SIGNAL.
func TestActivityStormCreatesNoWork(t *testing.T) {
	ctx := context.Background()
	srv, acts := apiServerWithActivity()

	before, err := countEverything(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000; i++ {
		acts.Emit(activity.Event{
			Kind:   activity.KindChannelOpCompleted,
			From:   activity.Node(activity.NodeChannelAdapter, "console"),
			To:     activity.Node(activity.NodeManager, activity.NodeManager),
			Status: activity.StatusError,
			Detail: fmt.Sprintf("agentops-manager CrashLoopBackOff #%d", i),
		})
	}
	// and the same through the reporting endpoint an adapter would use
	for i := 0; i < 50; i++ {
		rec := adapterReq(srv, "POST", "/activity", map[string]any{
			"kind": activity.KindChannelOpCompleted, "status": "error",
			"adapter": "telegram", "detail": "agentops-conv pod failed to start",
		}, testMasterToken)
		if rec.Code != 202 {
			t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
		}
	}

	after, err := countEverything(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("telemetry wrote to the cluster: %+v -> %+v", before, after)
	}
	// and the buffer stayed bounded rather than growing with the storm
	held, _ := acts.Since("", 0)
	if len(held) > 512 {
		t.Fatalf("ring buffer grew past its capacity: %d events", len(held))
	}
}

type clusterCounts struct {
	Conversations, Inputs, Channels, Sources, Pipelines, Pods int
}

func countEverything(ctx context.Context) (clusterCounts, error) {
	var c clusterCounts
	var convs agentopsv1alpha1.ConversationList
	if err := k8sClient.List(ctx, &convs, client.InNamespace(ns)); err != nil {
		return c, err
	}
	c.Conversations = len(convs.Items)
	var ins agentopsv1alpha1.ConversationInputList
	if err := k8sClient.List(ctx, &ins, client.InNamespace(ns)); err != nil {
		return c, err
	}
	c.Inputs = len(ins.Items)
	var chans agentopsv1alpha1.ChannelList
	if err := k8sClient.List(ctx, &chans, client.InNamespace(ns)); err != nil {
		return c, err
	}
	c.Channels = len(chans.Items)
	var srcs agentopsv1alpha1.SignalSourceList
	if err := k8sClient.List(ctx, &srcs, client.InNamespace(ns)); err != nil {
		return c, err
	}
	c.Sources = len(srcs.Items)
	var pipes agentopsv1alpha1.PipelineList
	if err := k8sClient.List(ctx, &pipes, client.InNamespace(ns)); err != nil {
		return c, err
	}
	c.Pipelines = len(pipes.Items)
	return c, nil
}

// 1.6: replay by cursor, and an evicted cursor answered with a resync rather
// than a silent gap.
func TestActivityReplayAndResync(t *testing.T) {
	srv, acts := apiServerWithActivity()
	for i := 0; i < 3; i++ {
		acts.Emit(activity.Event{Kind: activity.KindInputQueued, Conversation: "replay"})
	}
	rec := adapterReq(srv, "GET", "/activity", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Events []activity.Event `json:"events"`
		Cursor string           `json:"cursor"`
		Resync bool             `json:"resync"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Events) != 3 || out.Resync {
		t.Fatalf("first replay: %d events, resync=%v", len(out.Events), out.Resync)
	}
	stale := out.Events[0].Cursor

	// nothing new since the head
	rec = adapterReq(srv, "GET", "/activity?since="+out.Cursor, nil, testMasterToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Events) != 0 {
		t.Fatalf("want no events after the head, got %d", len(out.Events))
	}

	// evict everything the client saw
	for i := 0; i < 600; i++ {
		acts.Emit(activity.Event{Kind: activity.KindInputQueued})
	}
	rec = adapterReq(srv, "GET", "/activity?since="+stale, nil, testMasterToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Resync {
		t.Fatalf("an evicted cursor must answer with a resync, got %+v", out.Resync)
	}
}

// 1.6/1.7: every activity surface refuses an unauthenticated caller, and no
// events leak in the rejection.
func TestActivityEndpointsRequireAToken(t *testing.T) {
	srv, acts := apiServerWithActivity()
	acts.Emit(activity.Event{Kind: activity.KindInputQueued, Detail: "secret-payload-marker"})

	for _, path := range []string{"/activity", "/activity/stream"} {
		rec := adapterReq(srv, "GET", path, nil, "")
		if rec.Code != 401 {
			t.Fatalf("%s without a token: %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret-payload-marker") {
			t.Fatalf("%s disclosed events to an unauthenticated caller", path)
		}
	}
	if rec := adapterReq(srv, "POST", "/activity", map[string]any{"kind": "x"}, ""); rec.Code != 401 {
		t.Fatalf("POST /activity without a token: %d", rec.Code)
	}
	for _, path := range []string{"/status", "/pipelines/anything/resolved"} {
		if rec := adapterReq(srv, "GET", path, nil, ""); rec.Code != 401 {
			t.Fatalf("%s without a token: %d", path, rec.Code)
		}
	}
}

// 1.7/1.8: an adapter may report only for itself.
func TestAdapterCannotReportAsAnother(t *testing.T) {
	mkChannelAdapter(t, "reporter-a")
	mkChannelAdapter(t, "reporter-b")
	srv, acts := apiServerWithActivity()
	token := adapterTokenFor("reporter-a")

	// its own hop: accepted, attributed to it, with the adapter taken from the
	// TOKEN rather than the body
	rec := adapterReq(srv, "POST", "/activity", map[string]any{
		"kind": activity.KindChannelOpCompleted, "opId": "send:1", "latencyMs": 42,
	}, token)
	if rec.Code != 202 {
		t.Fatalf("own report: %d %s", rec.Code, rec.Body.String())
	}
	all, _ := acts.Since("", 0)
	e := firstOf(all, activity.KindChannelOpCompleted)
	if e == nil || e.Adapter != "reporter-a" {
		t.Fatalf("report must be attributed to the token's adapter: %+v", e)
	}

	// another adapter's hop: refused
	rec = adapterReq(srv, "POST", "/activity", map[string]any{
		"kind": activity.KindChannelOpCompleted, "adapter": "reporter-b",
	}, token)
	if rec.Code != 403 {
		t.Fatalf("cross-adapter report must be refused, got %d %s", rec.Code, rec.Body.String())
	}
}

// 1b.1/1b.4: /status reports the queue state that exists in no Kubernetes
// object — a queued-but-unclaimed op, and a claimed-but-uncompleted one.
func TestStatusReportsOpQueueState(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-status")
	mkChannel(t, "chan-status", "status-adapter")
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "status-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-status"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-status"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "status-conv") })
	srv, _ := apiServerWithActivity()
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "chan-status"}, &ch); err != nil {
		t.Fatal(err)
	}
	srv.Ops.EnqueueEnsureTopic(ctx, &ch, conv, chat.TopicDescriptor{})

	st := getStatus(t, srv)
	q := queueFor(st, "status-adapter")
	if q == nil || q.Queued != 1 || q.Claimed != 0 {
		t.Fatalf("queued-but-unclaimed op not reported: %+v", st.Queues)
	}
	if q.OldestQueuedOpID == "" {
		t.Fatalf("/status must name WHICH op is oldest — that is what a metric label may not carry: %+v", q)
	}

	// claim it and leave it uncompleted: the other failure mode, which looks
	// identical from outside and must not
	if op := srv.Ops.Claim("status-adapter"); op == nil {
		t.Fatal("claim returned nothing")
	}
	st = getStatus(t, srv)
	q = queueFor(st, "status-adapter")
	if q == nil || q.Queued != 0 || q.Claimed != 1 || q.OldestClaimedOpID == "" {
		t.Fatalf("claimed-but-uncompleted op not reported: %+v", st.Queues)
	}
	if st.RuntimeSlots.Max != 5 {
		t.Fatalf("runtime ceiling not reported: %+v", st.RuntimeSlots)
	}
	// no CR spec or status is proxied through this endpoint
	raw := rawStatus(t, srv)
	for _, forbidden := range []string{"\"spec\"", "profileRef", "channelRefs"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("/status must not proxy CR state, found %s in %s", forbidden, raw)
		}
	}
}

// 1b.3/1b.4: resolved capabilities equal what dispatch composes for the same
// pipeline — asserted against the dispatch path, not a reimplementation.
func TestPipelineResolvedMatchesDispatch(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-resolved")
	mkToolset(t, "ts-resolved-a", "Read", "Grep")
	mkToolset(t, "ts-resolved-b", "Grep", "Bash(kubectl get *)")

	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "resolved-pipe", ns
	p.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-resolved"}
	p.Spec.Toolsets = &agentopsv1alpha1.ToolsetBinding{
		Mode: agentopsv1alpha1.ToolsModeOverwrite,
		Refs: []agentopsv1alpha1.ObjectRef{{Name: "ts-resolved-a"}, {Name: "ts-resolved-b"}},
	}
	if err := k8sClient.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	srv, _ := apiServerWithActivity()

	rec := adapterReq(srv, "GET", "/pipelines/resolved-pipe/resolved", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("resolved: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		AllowedTools []string `json:"allowedTools"`
		ToolsMode    string   `json:"toolsMode"`
		Toolsets     []string `json:"toolsets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)

	// Drive the SAME wiring through dispatch and compare. If these ever
	// disagree, the console is rendering something the system does not do.
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "resolved-conv", ns
	conv.Spec.ProfileRef = p.Spec.ProfileRef
	conv.Spec.Toolsets = p.Spec.Toolsets.DeepCopy()
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "resolved-conv") })
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/work?convo=resolved-conv&wait=0", nil))
	if rec.Code != 200 {
		t.Fatalf("work: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		AllowedTools string `json:"allowedTools"`
		ToolsMode    string `json:"toolsMode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)

	if got := strings.Join(out.AllowedTools, ","); got != unit.AllowedTools {
		t.Fatalf("resolved allowlist %q != dispatched %q", got, unit.AllowedTools)
	}
	if out.ToolsMode != unit.ToolsMode {
		t.Fatalf("resolved mode %q != dispatched %q", out.ToolsMode, unit.ToolsMode)
	}
}

// An empty binding resolves to an EMPTY allowlist, never a fallback: a pipeline
// that grants no tools is a configuration, not a defect to paper over.
func TestPipelineResolvedReportsEmptyAsEmpty(t *testing.T) {
	mkProfile(t, "prof-notools")
	mkPipeline(t, "notools-pipe", nil, nil, "prof-notools")
	srv, _ := apiServerWithActivity()

	rec := adapterReq(srv, "GET", "/pipelines/notools-pipe/resolved", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("resolved: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.AllowedTools == nil || len(out.AllowedTools) != 0 {
		t.Fatalf("empty allowlist must serialize as [], got %v", out.AllowedTools)
	}
	if rec := adapterReq(srv, "GET", "/pipelines/no-such-pipeline/resolved", nil, testMasterToken); rec.Code != 404 {
		t.Fatalf("unknown pipeline must 404, got %d", rec.Code)
	}
}

// ---- helpers -----------------------------------------------------------------

type statusOut struct {
	Version      string `json:"version"`
	RuntimeSlots struct {
		InUse   int `json:"inUse"`
		Max     int `json:"max"`
		Waiting int `json:"waiting"`
	} `json:"runtimeSlots"`
	Queues []struct {
		Adapter           string `json:"adapter"`
		Queued            int    `json:"queued"`
		Claimed           int    `json:"claimed"`
		OldestQueuedOpID  string `json:"oldestQueuedOpId"`
		OldestClaimedOpID string `json:"oldestClaimedOpId"`
	} `json:"queues"`
}

func getStatus(t *testing.T, srv *httpapi.Server) statusOut {
	t.Helper()
	rec := adapterReq(srv, "GET", "/status", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var out statusOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func queueFor(st statusOut, adapter string) *struct {
	Adapter           string `json:"adapter"`
	Queued            int    `json:"queued"`
	Claimed           int    `json:"claimed"`
	OldestQueuedOpID  string `json:"oldestQueuedOpId"`
	OldestClaimedOpID string `json:"oldestClaimedOpId"`
} {
	for i := range st.Queues {
		if st.Queues[i].Adapter == adapter {
			return &st.Queues[i]
		}
	}
	return nil
}

func rawStatus(t *testing.T, srv *httpapi.Server) string {
	t.Helper()
	return adapterReq(srv, "GET", "/status", nil, testMasterToken).Body.String()
}

// mkChannelAdapter creates the minimal ChannelAdapter an adapter token derives
// against (validation is by re-derivation against the CR list).
func mkChannelAdapter(t *testing.T, name string) {
	t.Helper()
	a := &agentopsv1alpha1.ChannelAdapter{}
	a.Name, a.Namespace = name, ns
	a.Spec.Image = "example/adapter:test"
	if err := k8sClient.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func adapterTokenFor(name string) string {
	return chat.DeriveAdapterToken(testMasterToken, name)
}
