// The state read surface (state-store-generic-contract design D-5/D-6):
// /state/conversations, /state/conversations/{name}, /state/conversations/stream,
// /state/signals, /state/signals/stream, and the stateAuth credential that
// guards them.
package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// sseEvent is one parsed frame: its event type and its data payload.
type sseEvent struct {
	event string
	data  string
}

// readSSEEvent reads one frame (event:/data: lines up to the blank line that
// ends it), skipping anything that names neither.
func readSSEEvent(t *testing.T, r *bufio.Reader) sseEvent {
	t.Helper()
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			ev.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.data = strings.TrimPrefix(line, "data: ")
		case line == "" && ev.event != "":
			return ev
		}
	}
}

// readSSEEventNamed reads frames until one named name crosses as eventType
// (e.g. a "put" or "delete" whose data carries "name":"<name>"), failing if a
// "resync" frame arrives first or the bound is exhausted — a busy shared
// namespace may replay other tests' own conversations ahead of this one's,
// which this tolerates by skipping anything not about name.
func readSSEEventNamed(t *testing.T, r *bufio.Reader, eventType, name string) {
	t.Helper()
	for i := 0; i < 20000; i++ {
		ev := readSSEEvent(t, r)
		if ev.event == "resync" {
			t.Fatalf("got an unexpected resync frame waiting for %s %q", eventType, name)
		}
		if ev.event == eventType && strings.Contains(ev.data, `"name":"`+name+`"`) {
			return
		}
	}
	t.Fatalf("never saw %s %q on the stream", eventType, name)
}

func TestStateConversationsListIncludesRunsPhaseBlind(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-state-list")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "state-list-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-state-list"}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed // phase-blind: still listed
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	if err := stores.Runs.Append(ctx, state.Key(conv.Name), state.Run{RunID: "r1", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "GET", "/state/conversations", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Revision string          `json:"revision"`
		Items    json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Revision == "" {
		t.Fatal("want a non-empty list revision")
	}
	if !strings.Contains(string(out.Items), `"name":"state-list-conv"`) {
		t.Fatalf("listing missing the Closed conversation (must be phase-blind): %s", out.Items)
	}
	if !strings.Contains(string(out.Items), `"runId":"r1"`) {
		t.Fatalf("listing missing runs[]: %s", out.Items)
	}
}

func TestStateConversationGetUnknownNameIs404(t *testing.T) {
	srv := apiServer()
	rec := adapterReq(srv, "GET", "/state/conversations/does-not-exist", nil, testMasterToken)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestStateListRefusesAnUnknownQueryParameterByName(t *testing.T) {
	srv := apiServer()
	rec := adapterReq(srv, "GET", "/state/conversations?phase=Idle", nil, testMasterToken)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "phase") {
		t.Fatalf("code = %d, body = %s, want 400 naming phase", rec.Code, rec.Body.String())
	}
}

// TestStateAuthRefusesAnAdapterWithoutStateAccess: a real derived adapter
// token is refused with the adapter named, until its CR declares
// spec.stateAccess (design D-6) — then it opens the surface.
func TestStateAuthRefusesAnAdapterWithoutStateAccessThenGrantsIt(t *testing.T) {
	ctx := context.Background()
	srv := apiServer()

	a := &agentopsv1alpha1.ChannelAdapter{}
	a.Name, a.Namespace = "state-console", ns
	a.Spec.Image = "example/console:test"
	if err := k8sClient.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	waitForRoutingCacheToObserve(t, a, a.ResourceVersion)
	token := adapterTokenFor(a.Name)

	rec := adapterReq(srv, "GET", "/state/conversations", nil, token)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), a.Name) || !strings.Contains(rec.Body.String(), "stateAccess") {
		t.Fatalf("code = %d, body = %s, want 403 naming %q and stateAccess", rec.Code, rec.Body.String(), a.Name)
	}

	patch := client.MergeFrom(a.DeepCopy())
	a.Spec.StateAccess = true
	if err := k8sClient.Patch(ctx, a, patch); err != nil {
		t.Fatal(err)
	}
	waitForRoutingCacheToObserve(t, a, a.ResourceVersion)

	rec = adapterReq(srv, "GET", "/state/conversations", nil, token)
	if rec.Code != 200 {
		t.Fatalf("stateAccess granted: code = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// TestStateAuthUnrecognisedTokenIs401: a token that derives from nothing
// (not the master, not any adapter, not STATE_READ_TOKEN) is 401 rather than
// 403 — the distinction design D-6 draws between "not you" and "you, but not
// granted".
func TestStateAuthUnrecognisedTokenIs401(t *testing.T) {
	srv := apiServer()
	rec := adapterReq(srv, "GET", "/state/conversations", nil, "nonsense")
	if rec.Code != 401 {
		t.Fatalf("code = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestStateReadTokenOpensStateAndNothingElse: STATE_READ_TOKEN
// (housekeeping's own credential) opens /state/*, and is refused on
// /channel/* and /control-plane/* — it must not widen either surface
// (design D-6).
func TestStateReadTokenOpensStateAndNothingElse(t *testing.T) {
	srv, _ := apiServerWithActivity()
	srv.StateReadToken = "state-read-token"
	srv.ControlPlaneToken = "state-read-control-plane-token"
	mkChannel(t, "state-read-chan", "tg-state-read")

	rec := adapterReq(srv, "GET", "/state/conversations", nil, srv.StateReadToken)
	if rec.Code != 200 {
		t.Fatalf("/state/conversations with STATE_READ_TOKEN: %d %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/state/signals", nil, srv.StateReadToken)
	if rec.Code != 200 {
		t.Fatalf("/state/signals with STATE_READ_TOKEN: %d %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/channel/channels?adapter=tg-state-read", nil, srv.StateReadToken)
	if rec.Code != 401 {
		t.Fatalf("/channel/* with STATE_READ_TOKEN: code = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	rec = adapterReq(srv, "GET", "/control-plane/probes/state-read-probe", nil, srv.StateReadToken)
	if rec.Code != 401 {
		t.Fatalf("/control-plane/* with STATE_READ_TOKEN: code = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

// TestStateSurfaceServedByANonLeaderWithNoProxyDial: design D-7 — /state/
// joins /healthz in leaderProxyMiddleware's bypass, so a non-leader answers
// directly. Proven the same way the proxy tests prove the OPPOSITE
// (TestChannelOpsProxiesToLeaderWhenReachable): a "leader" the follower
// could proxy to, instrumented to record whether it was ever dialled. A
// dumb-proxy bug (losing the /state/ exemption) would show up here as a hit.
func TestStateSurfaceServedByANonLeaderWithNoProxyDial(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-state-noproxy")

	dialed := make(chan struct{}, 1)
	leaderHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case dialed <- struct{}{}:
		default:
		}
		w.WriteHeader(599)
	}))
	t.Cleanup(leaderHTTP.Close)

	followerSrv, _ := apiServerWithActivity()
	followerSrv.ReplicaIdentity = "state-noproxy-follower"
	followerSrv.Addr = leaderHTTP.URL[strings.LastIndex(leaderHTTP.URL, ":"):]

	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = "state-noproxy-leader", ns
	pod.Labels = map[string]string{"app.kubernetes.io/name": "agentops-manager"}
	pod.Spec.Containers = []corev1.Container{{Name: "manager", Image: "busybox:stub"}}
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), pod) })
	pod.Status.PodIP = strings.TrimPrefix(leaderHTTP.URL[:strings.LastIndex(leaderHTTP.URL, ":")], "http://")
	if err := k8sClient.Status().Update(ctx, pod); err != nil {
		t.Fatal(err)
	}

	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = httpapi.LeaseName, ns
	holder := "state-noproxy-leader"
	lease.Spec.HolderIdentity = &holder
	if err := k8sClient.Create(ctx, lease); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), lease) })

	rec := adapterReq(followerSrv, "GET", "/state/conversations", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("a non-leader must still answer /state/* itself: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-dialed:
		t.Fatal("the \"leader\" was dialled — /state/ must bypass leaderProxyMiddleware entirely")
	default:
	}
}

// TestStateConversationsStreamResyncsOnUnknownSinceThenCarriesPutAndDelete:
// design D-5's Feed — an unknown/stale `since` yields `resync` first (never
// an error), and a stream resumed from a kept revision carries a put and a
// delete made meanwhile.
func TestStateConversationsStreamResyncsOnUnknownSinceThenCarriesPutAndDelete(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-state-stream")
	srv := apiServer()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	dialCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(dialCtx, "GET", ts.URL+"/state/conversations/stream?since=not-a-real-revision", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testMasterToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != 200 {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	r := bufio.NewReader(resp.Body)
	if first := readSSEEvent(t, r); first.event != "resync" {
		t.Fatalf("first frame = %q, want resync for an unknown since", first.event)
	}

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "state-stream-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-state-stream"}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	readSSEEventNamed(t, r, "put", conv.Name)

	if err := stores.Conversations.Delete(ctx, state.Key(conv.Name)); err != nil {
		t.Fatal(err)
	}
	readSSEEventNamed(t, r, "delete", conv.Name)
}

// TestStateConversationsStreamResumesFromAKeptRevisionWithNoResync: the
// companion case to the unknown-since test above — a GOOD since, taken from
// a real listing, skips the resync frame and resumes straight into live
// deltas.
func TestStateConversationsStreamResumesFromAKeptRevisionWithNoResync(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-state-goodsince")
	srv := apiServer()

	seed := &agentopsv1alpha1.Conversation{}
	seed.Name, seed.Namespace = "state-goodsince-seed", ns
	seed.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-state-goodsince"}
	if err := k8sClient.Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, seed.Name) })

	listRec := adapterReq(srv, "GET", "/state/conversations", nil, testMasterToken)
	if listRec.Code != 200 {
		t.Fatalf("list: %d %s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Revision == "" {
		t.Fatal("want a non-empty list revision to resume from")
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	dialCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(dialCtx, "GET", ts.URL+"/state/conversations/stream?since="+listed.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testMasterToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != 200 {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	r := bufio.NewReader(resp.Body)

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "state-goodsince-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-state-goodsince"}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	// A known-good since skips the resync frame entirely: readSSEEventNamed
	// itself fails on any resync frame it meets on the way to this put.
	readSSEEventNamed(t, r, "put", conv.Name)
}

// TestStateConversationSpecAndStatusBytesEqualTheObjects: under kube,
// spec/status marshal to the SAME bytes as the Conversation's own (design
// D-5).
func TestStateConversationSpecAndStatusBytesEqualTheObjects(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-state-bytes")
	mkChannel(t, "state-bytes-chan", "tg-state-bytes")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "state-bytes-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-state-bytes"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "state-bytes-chan"}}
	conv.Spec.Title = "byte-identical"
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Phase = agentopsv1alpha1.ConversationIdle
	conv.Status.Brief = "a byte-identical check"
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	var live agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(conv), &live); err != nil {
		t.Fatal(err)
	}
	wantSpec, err := json.Marshal(live.Spec)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus, err := json.Marshal(live.Status)
	if err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "GET", "/state/conversations/"+conv.Name, nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var wire struct {
		Spec   json.RawMessage `json:"spec"`
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire.Spec) != string(wantSpec) {
		t.Fatalf("spec bytes differ:\n got  %s\n want %s", wire.Spec, wantSpec)
	}
	if string(wire.Status) != string(wantStatus) {
		t.Fatalf("status bytes differ:\n got  %s\n want %s", wire.Status, wantStatus)
	}
}
