// POST /channel/read against a real API server: the field must round-trip off
// the object, which is what catches CRDs that were not re-applied — a pruned
// field answers 200 and changes nothing, so a fake client could not see it.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

type readResp struct {
	Results []struct {
		ThreadID string `json:"threadId"`
		Outcome  string `json:"outcome"`
		Reason   string `json:"reason"`
	} `json:"results"`
	Marked  int `json:"marked"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// mkBoundConv creates a conversation already holding a thread on a channel,
// with lastActivity set — the two facts unreadness is derived from.
func mkBoundConv(t *testing.T, name, channel, threadID string, activity time.Time) *agentopsv1alpha1.Conversation {
	t.Helper()
	ctx := context.Background()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "read-prof"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: channel}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{
		{Channel: channel, ThreadID: threadID, ReadTracked: true},
	}
	at := metav1.NewTime(activity)
	conv.Status.LastActivity = &at
	if err := k8sClient.Status().Update(ctx, conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

func postRead(t *testing.T, srv *httpapi.Server, channel string, reads []map[string]any, token string) (int, readResp) {
	t.Helper()
	rec := adapterReq(srv, "POST", "/channel/read",
		map[string]any{"channel": channel, "reads": reads}, token)
	var out readResp
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

func threadOf(t *testing.T, name, channel string) agentopsv1alpha1.ThreadBinding {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	b := conv.Status.Thread(channel)
	if b == nil {
		t.Fatalf("conversation %s holds no thread on %s", name, channel)
	}
	return *b
}

// The watermark round-trips through the API server, and only the reported
// channel's binding moves.
func TestChannelReadMarksAndIsPerChannel(t *testing.T) {
	mkProfile(t, "read-prof")
	mkChannel(t, "chan-read", "tg-read")
	mkChannel(t, "chan-read2", "tg-read")
	srv := apiServer()

	activity := time.Now().Add(-time.Hour)
	conv := mkBoundConv(t, "conv-read", "chan-read", "77", activity)
	// a second binding on another channel, which must stay untouched
	conv.Status.Threads = append(conv.Status.Threads,
		agentopsv1alpha1.ThreadBinding{Channel: "chan-read2", ThreadID: "88", ReadTracked: true})
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}

	code, out := postRead(t, srv, "chan-read",
		[]map[string]any{{"threadId": "77", "readAt": activity.Format(time.RFC3339)}}, "test-adapter-token")
	if code != 200 || out.Marked != 1 || out.Results[0].Outcome != "marked" {
		t.Fatalf("mark: %d %+v", code, out)
	}

	got := threadOf(t, "conv-read", "chan-read")
	if got.ReadAt == nil || !got.ReadTracked {
		t.Fatalf("watermark did not round-trip off the object: %+v — are the CRDs re-applied?", got)
	}
	if other := threadOf(t, "conv-read", "chan-read2"); other.ReadAt != nil {
		t.Fatalf("reading one channel moved another channel's watermark: %+v", other)
	}
	// unreadness follows: activity == watermark is read, later activity is not
	var fresh agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "conv-read"}, &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Status.UnreadFor("chan-read", "") {
		t.Fatal("thread read up to its last activity is still unread")
	}
	if !fresh.Status.UnreadFor("chan-read2", "") {
		t.Fatal("a tracked, never-read binding must be unread")
	}
	if fresh.Status.UnreadFor("chan-absent", "") {
		t.Fatal("a channel with no binding must never be unread")
	}
}

// A stale client cannot un-read a thread, and a re-report that would not
// advance writes nothing.
func TestChannelReadIsMonotonic(t *testing.T) {
	mkProfile(t, "read-prof-mono")
	mkChannel(t, "chan-mono", "tg-mono")
	srv := apiServer()
	t2 := time.Now().Add(-time.Minute)
	mkBoundConv(t, "conv-mono", "chan-mono", "10", t2)

	if code, out := postRead(t, srv, "chan-mono",
		[]map[string]any{{"threadId": "10", "readAt": t2.Format(time.RFC3339)}}, "test-adapter-token"); code != 200 || out.Marked != 1 {
		t.Fatalf("first report: %d %+v", code, out)
	}
	stored := threadOf(t, "conv-mono", "chan-mono").ReadAt.DeepCopy()

	// an earlier watermark from a stale view
	code, out := postRead(t, srv, "chan-mono",
		[]map[string]any{{"threadId": "10", "readAt": t2.Add(-30 * time.Minute).Format(time.RFC3339)}}, "test-adapter-token")
	if code != 200 || out.Skipped != 1 || out.Results[0].Outcome != "skipped" {
		t.Fatalf("stale report must skip: %d %+v", code, out)
	}
	// the same watermark again writes nothing either
	if _, out := postRead(t, srv, "chan-mono",
		[]map[string]any{{"threadId": "10", "readAt": t2.Format(time.RFC3339)}}, "test-adapter-token"); out.Skipped != 1 {
		t.Fatalf("unchanged watermark must skip: %+v", out)
	}
	if now := threadOf(t, "conv-mono", "chan-mono").ReadAt; !now.Time.Equal(stored.Time) {
		t.Fatalf("watermark moved backwards: %v -> %v", stored, now)
	}
}

// A skewed clock cannot mark the future read: the report is clamped to the
// manager's own now, so activity arriving later is still unread.
func TestChannelReadClampsTheFuture(t *testing.T) {
	mkProfile(t, "read-prof-clamp")
	mkChannel(t, "chan-clamp", "tg-clamp")
	srv := apiServer()
	mkBoundConv(t, "conv-clamp", "chan-clamp", "20", time.Now().Add(-time.Hour))

	future := time.Now().Add(48 * time.Hour)
	if code, out := postRead(t, srv, "chan-clamp",
		[]map[string]any{{"threadId": "20", "readAt": future.Format(time.RFC3339)}}, "test-adapter-token"); code != 200 || out.Marked != 1 {
		t.Fatalf("clamped report: %d %+v", code, out)
	}
	got := threadOf(t, "conv-clamp", "chan-clamp")
	if got.ReadAt == nil || !got.ReadAt.Time.Before(future.Add(-time.Hour)) {
		t.Fatalf("future watermark was not clamped to the manager's clock: %v", got.ReadAt)
	}
}

// A mixed batch is a success with per-entry detail, and one bad entry never
// stops the rest.
func TestChannelReadMixedBatchAndBounds(t *testing.T) {
	mkProfile(t, "read-prof-batch")
	mkChannel(t, "chan-batch", "tg-batch")
	srv := apiServer()
	at := time.Now().Add(-time.Hour)
	mkBoundConv(t, "conv-batch-a", "chan-batch", "a1", at)
	mkBoundConv(t, "conv-batch-b", "chan-batch", "b1", at)

	// b1 is already read up to `at`; a1 is not, and "ghost" belongs to nobody
	if _, out := postRead(t, srv, "chan-batch",
		[]map[string]any{{"threadId": "b1", "readAt": at.Format(time.RFC3339)}}, "test-adapter-token"); out.Marked != 1 {
		t.Fatalf("seed: %+v", out)
	}
	code, out := postRead(t, srv, "chan-batch", []map[string]any{
		{"threadId": "a1", "readAt": at.Format(time.RFC3339)},
		{"threadId": "b1", "readAt": at.Format(time.RFC3339)},
		{"threadId": "ghost", "readAt": at.Format(time.RFC3339)},
	}, "test-adapter-token")
	if code != 200 {
		t.Fatalf("mixed batch must be a 200: %d", code)
	}
	if out.Marked != 1 || out.Skipped != 1 || out.Failed != 1 {
		t.Fatalf("mixed batch totals: %+v", out)
	}
	byThread := map[string]string{}
	for _, r := range out.Results {
		byThread[r.ThreadID] = r.Outcome
		if r.Outcome != "marked" && r.Reason == "" {
			t.Fatalf("%s: %s with no reason", r.ThreadID, r.Outcome)
		}
	}
	if byThread["a1"] != "marked" || byThread["b1"] != "skipped" || byThread["ghost"] != "failed" {
		t.Fatalf("per-entry outcomes: %+v", byThread)
	}
	if threadOf(t, "conv-batch-a", "chan-batch").ReadAt == nil {
		t.Fatal("an unknown thread in the batch stopped a good entry being marked")
	}

	// empty and oversized batches are refused outright
	if rec := adapterReq(srv, "POST", "/channel/read",
		map[string]any{"channel": "chan-batch", "reads": []any{}}, "test-adapter-token"); rec.Code != 400 {
		t.Fatalf("empty batch: %d", rec.Code)
	}
	over := make([]map[string]any, httpapi.MaxReadBatch+1)
	for i := range over {
		over[i] = map[string]any{"threadId": "a1", "readAt": at.Format(time.RFC3339)}
	}
	if rec := adapterReq(srv, "POST", "/channel/read",
		map[string]any{"channel": "chan-batch", "reads": over}, "test-adapter-token"); rec.Code != 400 {
		t.Fatalf("oversized batch: %d", rec.Code)
	}
}

// Same auth and channel scope as every other /channel/* route.
func TestChannelReadAuthAndScope(t *testing.T) {
	mkProfile(t, "read-prof-auth")
	mkChannel(t, "chan-auth", "tg-auth-read")
	mkAdapter(t, "read-other-adapter")
	srv := apiServer()
	at := time.Now().Add(-time.Hour)
	mkBoundConv(t, "conv-auth", "chan-auth", "z1", at)
	body := map[string]any{"channel": "chan-auth",
		"reads": []map[string]any{{"threadId": "z1", "readAt": at.Format(time.RFC3339)}}}

	if rec := adapterReq(srv, "POST", "/channel/read", body, ""); rec.Code != 401 {
		t.Fatalf("unauthenticated: %d", rec.Code)
	}
	if rec := adapterReq(srv, "POST", "/channel/read", body, "nonsense"); rec.Code != 401 {
		t.Fatalf("bad token: %d", rec.Code)
	}
	other := chat.DeriveAdapterToken(testMasterToken, "read-other-adapter")
	if rec := adapterReq(srv, "POST", "/channel/read", body, other); rec.Code != 403 {
		t.Fatalf("cross-adapter scope: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	if threadOf(t, "conv-auth", "chan-auth").ReadAt != nil {
		t.Fatal("a refused report wrote a watermark")
	}
	if rec := adapterReq(srv, "POST", "/channel/read",
		map[string]any{"channel": "chan-nope", "reads": body["reads"]}, testMasterToken); rec.Code != 404 {
		t.Fatalf("unknown channel: %d", rec.Code)
	}
}

// A binding written before the mechanism existed is READ, and the manager
// stamps readTracked on every binding it creates from here on.
func TestUntrackedBindingIsRead(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{}
	old := metav1.NewTime(time.Now())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c", ThreadID: "1"}}
	conv.Status.LastActivity = &old
	if conv.Status.UnreadFor("c", "") {
		t.Fatal("a binding predating read tracking must be treated as read")
	}
	conv.Status.Threads[0].ReadTracked = true
	if !conv.Status.UnreadFor("c", "") {
		t.Fatal("a tracked binding with no watermark must be unread")
	}
}

// ---- per-identity overlay -----------------------------------------------------

func postReaderRead(t *testing.T, srv *httpapi.Server, channel, thread, reader string, at time.Time) readResp {
	t.Helper()
	e := map[string]any{"threadId": thread, "readAt": at.Format(time.RFC3339)}
	if reader != "" {
		e["reader"] = reader
	}
	code, out := postRead(t, srv, channel, []map[string]any{e}, "test-adapter-token")
	if code != 200 {
		t.Fatalf("read (%s): %d", reader, code)
	}
	return out
}

// Two identities hold independent watermarks on ONE thread, and neither touches
// the channel-wide mark — one person reading must not clear a colleague's badge.
func TestReadersAreIndependent(t *testing.T) {
	mkProfile(t, "read-prof-two")
	mkChannel(t, "chan-two", "tg-two")
	srv := apiServer()
	at := time.Now().Add(-time.Hour)
	mkBoundConv(t, "conv-two", "chan-two", "r1", at)

	if out := postReaderRead(t, srv, "chan-two", "r1", "sha256:alice", at); out.Marked != 1 {
		t.Fatalf("alice: %+v", out)
	}
	got := threadOf(t, "conv-two", "chan-two")
	if len(got.Readers) != 1 || got.Readers[0].Key != "sha256:alice" {
		t.Fatalf("reader entry did not round-trip: %+v — are the CRDs re-applied?", got.Readers)
	}
	if got.ReadAt != nil {
		t.Fatalf("a reader report advanced the CHANNEL-WIDE mark: %v", got.ReadAt)
	}

	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "conv-two"}, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.Status.UnreadFor("chan-two", "sha256:alice") {
		t.Fatal("alice read it; it must not be unread for her")
	}
	if !conv.Status.UnreadFor("chan-two", "sha256:bob") {
		t.Fatal("bob has not read it; it must still be unread for him")
	}

	// bob's report is NOT skipped just because alice is further ahead
	if out := postReaderRead(t, srv, "chan-two", "r1", "sha256:bob", at); out.Marked != 1 {
		t.Fatalf("bob must be able to mark independently: %+v", out)
	}
	// ...and his own re-report still skips
	if out := postReaderRead(t, srv, "chan-two", "r1", "sha256:bob", at); out.Skipped != 1 {
		t.Fatalf("bob's unchanged watermark must skip: %+v", out)
	}
}

// A reader with no entry — never reported, or evicted — inherits the
// channel-wide mark rather than being handed a backlog.
func TestUnknownReaderFallsBackToTheChannelMark(t *testing.T) {
	mkProfile(t, "read-prof-fb")
	mkChannel(t, "chan-fb", "tg-fb")
	srv := apiServer()
	at := time.Now().Add(-time.Hour)
	mkBoundConv(t, "conv-fb", "chan-fb", "f1", at)

	// a reader-less report: the channel as a whole has now seen it
	if out := postReaderRead(t, srv, "chan-fb", "f1", "", at); out.Marked != 1 {
		t.Fatalf("channel-wide report: %+v", out)
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "conv-fb"}, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.Status.UnreadFor("chan-fb", "sha256:newcomer") {
		t.Fatal("a reader with no entry must inherit the channel mark, not a backlog")
	}
	// and a report that would not pass the inherited mark is not an advance
	if out := postReaderRead(t, srv, "chan-fb", "f1", "sha256:newcomer", at.Add(-time.Minute)); out.Skipped != 1 {
		t.Fatalf("a report behind the inherited mark must skip: %+v", out)
	}
}

// The overlay stays bounded, and the entry that caused the eviction survives it.
func TestReaderListIsBounded(t *testing.T) {
	mkProfile(t, "read-prof-cap")
	mkChannel(t, "chan-readcap", "tg-readcap")
	srv := apiServer()
	at := time.Now().Add(-24 * time.Hour)
	mkBoundConv(t, "conv-readcap", "chan-readcap", "c1", time.Now())

	for i := 0; i < agentopsv1alpha1.MaxReadersPerThread; i++ {
		// ascending watermarks, so reader 0 is the oldest and evicted first
		postReaderRead(t, srv, "chan-readcap", "c1", fmt.Sprintf("sha256:r%02d", i), at.Add(time.Duration(i)*time.Minute))
	}
	if n := len(threadOf(t, "conv-readcap", "chan-readcap").Readers); n != agentopsv1alpha1.MaxReadersPerThread {
		t.Fatalf("want %d readers, got %d", agentopsv1alpha1.MaxReadersPerThread, n)
	}
	postReaderRead(t, srv, "chan-readcap", "c1", "sha256:last", at.Add(-time.Hour))

	got := threadOf(t, "conv-readcap", "chan-readcap")
	if len(got.Readers) != agentopsv1alpha1.MaxReadersPerThread {
		t.Fatalf("overlay grew past its bound: %d", len(got.Readers))
	}
	keys := map[string]bool{}
	for _, r := range got.Readers {
		keys[r.Key] = true
	}
	if !keys["sha256:last"] {
		t.Fatal("the entry that caused the eviction was itself evicted")
	}
	if keys["sha256:r00"] {
		t.Fatal("the oldest watermark was not the one evicted")
	}
}

// The manager cannot tell a hash from a plaintext address, so it refuses the
// one shape that is obviously the latter — for the whole request, because an
// adapter sending identities is a bug to fix, not a data condition.
func TestReaderKeyMustNotBeAnIdentity(t *testing.T) {
	mkProfile(t, "read-prof-pii")
	mkChannel(t, "chan-pii", "tg-pii")
	srv := apiServer()
	at := time.Now().Add(-time.Hour)
	mkBoundConv(t, "conv-pii", "chan-pii", "p1", at)

	rec := adapterReq(srv, "POST", "/channel/read", map[string]any{
		"channel": "chan-pii",
		"reads": []map[string]any{
			{"threadId": "p1", "readAt": at.Format(time.RFC3339), "reader": "sha256:fine"},
			{"threadId": "p1", "readAt": at.Format(time.RFC3339), "reader": "alice@example.com"},
		},
	}, "test-adapter-token")
	if rec.Code != 400 {
		t.Fatalf("an address as a reader key must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if len(threadOf(t, "conv-pii", "chan-pii").Readers) != 0 {
		t.Fatal("a refused request wrote a reader entry")
	}
}

// The person who STARTED a conversation has seen it: their own watermark is
// stamped at the one moment their thread comes into existence, so it is never
// presented back to them as unread before an answer could exist — and it stays
// unread for everyone else.
func TestOriginReaderIsStampedWhenTheThreadIsCreated(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "read-prof-origin")
	mkChannel(t, "chan-origin", "tg-origin")
	mkChannel(t, "chan-other", "tg-origin")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-origin", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "read-prof-origin"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-origin"}, {Name: "chan-other"}}
	conv.Spec.Title = "started by somebody"
	// the opaque key the originating surface computed — no identity anywhere
	conv.Spec.OriginReader = &agentopsv1alpha1.OriginReader{Channel: "chan-origin", Key: "sha256:starter"}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{
		ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "check the nodes",
	}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, "conv-origin") })

	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "conv-origin"}}); err != nil {
		t.Fatal(err)
	}
	// complete every ensure-topic this conversation produced
	for {
		rec := adapterReq(srv, "GET", "/channel/ops?adapter=tg-origin&contract=2&wait=0", nil, "test-adapter-token")
		if rec.Code != 200 {
			break
		}
		var op chat.Op
		_ = json.Unmarshal(rec.Body.Bytes(), &op)
		if op.Kind != chat.OpEnsureTopic {
			continue
		}
		if rec := adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
			chat.OpResult{ThreadID: "t-" + op.Channel}, "test-adapter-token"); rec.Code != 200 {
			t.Fatalf("done: %d %s", rec.Code, rec.Body.String())
		}
	}

	started := threadOf(t, "conv-origin", "chan-origin")
	if len(started.Readers) != 1 || started.Readers[0].Key != "sha256:starter" || started.Readers[0].ReadAt == nil {
		t.Fatalf("the starter's own watermark was not stamped: %+v", started.Readers)
	}
	if started.ReadAt != nil {
		t.Fatalf("stamping the starter must not move the CHANNEL-WIDE mark: %v", started.ReadAt)
	}
	// the OTHER channel's binding is untouched: keys are per-channel, so a key
	// from one surface means nothing on another
	if other := threadOf(t, "conv-origin", "chan-other"); len(other.Readers) != 0 {
		t.Fatalf("the starter's key leaked onto another channel: %+v", other.Readers)
	}

	var fresh agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "conv-origin"}, &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Status.UnreadFor("chan-origin", "sha256:starter") {
		t.Fatal("the person who started it must not be shown it as unread")
	}
	if !fresh.Status.UnreadFor("chan-origin", "sha256:colleague") {
		t.Fatal("...but it is genuinely new to everybody else")
	}
}
