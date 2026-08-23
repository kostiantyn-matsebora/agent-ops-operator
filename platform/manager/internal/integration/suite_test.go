// Integration tests: real API server (envtest), no kubelet. Covers the
// conversation lifecycle: input -> runtime pod -> dispatch -> done -> prune,
// plus alert routing with signature grouping and recurrence.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    = runtime.NewScheme()
	ns        = "default"
)

// chartDir locates the chart from a test's working directory, which is this
// package's own directory.
//
// Named ONCE because six call sites counted the `..` segments by hand, and the
// day the manager moved from the repository root into platform/manager/ every
// one of them was wrong — with a failure that reads as a broken envtest rather
// than as a moved directory.
func chartDir(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "..", "..", "chart"}, parts...)...)
}

func TestMain(m *testing.M) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentopsv1alpha1.AddToScheme(scheme))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{chartDir("crds")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}
	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

// testOps builds the channel plumbing against the envtest client; the queue's
// registry is empty, so every channel type behaves as adapter-served.
func testOps() *chat.OpQueue {
	return &chat.OpQueue{Client: k8sClient, Namespace: ns, Registry: chat.NewRegistry()}
}

func reconciler() *controller.ConversationReconciler {
	return reconcilerWithOps(nil) // chat disabled in lifecycle tests
}

// reconcilerWithOps: the shared reconciler for tests that are NOT about
// capacity. Its cap is deliberately far above anything they create — envtest
// runs no kubelet, so every pod an earlier test left behind is still Pending
// and still counts as live, and a small shared cap would silently park an
// unrelated test's conversation in Pending with no topic and no pod. Capacity
// behavior gets its own reconciler with an explicit cap (capacity_test.go).
func reconcilerWithOps(ops *chat.OpQueue) *controller.ConversationReconciler {
	return reconcilerWithCap(ops, 100)
}

func reconcilerWithCap(ops *chat.OpQueue, cap int) *controller.ConversationReconciler {
	return &controller.ConversationReconciler{
		Client:                 k8sClient,
		Scheme:                 scheme,
		MaxActiveConversations: cap,
		Ops:                    ops,
		Runtime: runtimepod.Config{
			Image: "busybox:stub", ServiceAccount: "default",
			// left at the manager's default so the pod-env assertion below
			// pins the shipped value rather than a test-only one
			ControlURL: "http://manager:8080", IdleTTLMinutes: 1,
		},
	}
}

func apiServer() *httpapi.Server {
	srv, _ := apiServerWithActivity()
	return srv
}

// apiServerWithActivity wires the telemetry log through every emitting
// component, and hands it back for assertions. Plain apiServer() uses the same
// path — so every existing test also exercises emission, and a hop that panics
// or blocks fails a test about something else, which is where it should fail.
func apiServerWithActivity() (*httpapi.Server, *activity.Log) {
	acts := activity.New(512)
	rt := runtimepod.Config{ContextPVC: "test-context"}
	ops := &chat.OpQueue{Client: k8sClient, Namespace: ns, Registry: chat.NewRegistry(), Activity: acts}
	return &httpapi.Server{
		Client: k8sClient, Reader: k8sClient, Namespace: ns,
		// A deployment that CAN carry context: continuity is now promised only
		// where the runtime has somewhere to keep it, so a fixture with no home
		// volume would (correctly) withhold every handle and make the resume
		// tests assert the wrong thing.
		Runtime: rt,
		Ops:     ops,
		// The router carries the SAME runtime config, exactly as main.go wires
		// it: /exit reports whether a released conversation keeps its context,
		// and it must not answer that differently from dispatch.
		Router:                 &chat.Router{Client: k8sClient, Reader: k8sClient, Namespace: ns, Ops: ops, Activity: acts, Runtime: rt},
		AdapterToken:           "test-adapter-token",
		Activity:               acts,
		Version:                "test",
		MaxActiveConversations: 5,
	}, acts
}

func mkProfile(t *testing.T, name string) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/repo.git", Ref: "main"}
	p.Spec.Agent = "tester"
	p.Spec.MaxTurns = 5
	// REQUIRED, no default — the API refuses a profile that omits it. `none`
	// keeps these fixtures about the thing they test rather than about prompts.
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func reconcile(t *testing.T, name string) {
	t.Helper()
	_, err := reconciler().Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConversationLifecycle(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-life")

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "life-1", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-life"}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}

	// reconcile -> runtime pod created
	reconcile(t, "life-1")
	var pod corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName("life-1")}, &pod); err != nil {
		t.Fatalf("runtime pod not created: %v", err)
	}
	if pod.Spec.Containers[0].Image != "busybox:stub" {
		t.Fatalf("image: %s", pod.Spec.Containers[0].Image)
	}
	if len(pod.OwnerReferences) == 0 || pod.OwnerReferences[0].Kind != "Conversation" {
		t.Fatalf("ownerRef missing: %+v", pod.OwnerReferences)
	}

	// dispatch via /work
	srv := apiServer()
	req := httptest.NewRequest("GET", "/work?convo=life-1&pod="+pod.Name+"&wait=0", nil)
	rec := httptest.NewRecorder()
	srvStartMux(srv).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("work: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID      string `json:"runId"`
		PromptText string `json:"promptText"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	if unit.RunID == "" || unit.PromptText == "" {
		t.Fatalf("unit: %s", rec.Body.String())
	}

	// second /work while inflight -> 204 (strictly serial)
	rec2 := httptest.NewRecorder()
	srvStartMux(srv).ServeHTTP(rec2, httptest.NewRequest("GET", "/work?convo=life-1&wait=0", nil))
	if rec2.Code != 204 {
		t.Fatalf("expected 204 while inflight, got %d", rec2.Code)
	}

	// done
	done, _ := json.Marshal(map[string]any{
		"convo": "life-1", "runId": unit.RunID, "status": "succeeded",
		"sessionId": "sess-life", "result": "all good",
	})
	rec3 := httptest.NewRecorder()
	srvStartMux(srv).ServeHTTP(rec3, httptest.NewRequest("POST", "/work/done", bytes.NewReader(done)))
	if rec3.Code != 200 {
		t.Fatalf("done: %d %s", rec3.Code, rec3.Body.String())
	}

	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-1"}, &after)
	if after.ContextID() != "sess-life" || after.Status.Inflight != nil ||
		after.Status.Phase != agentopsv1alpha1.ConversationIdle || len(after.Status.Runs) != 1 {
		t.Fatalf("status after done: %+v", after.Status)
	}

	// reconcile prunes the processed input
	reconcile(t, "life-1")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-1"}, &after)
	if len(after.Spec.Inputs) != 0 {
		t.Fatalf("processed input not pruned: %+v", after.Spec.Inputs)
	}
}

// TestSignalBatchGrouping pins the manager-side grouping semantics the
// removed built-in webhook used to exercise: a batch of same-signature
// signals collapses to ONE conversation with ONE combined input, and
// same-signature reuse across batches with a session becomes a recurrence.
func TestSignalBatchGrouping(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-alert")
	mkSignalSource(t, "am", "batch-sig-t", "")
	// pipeline-only wiring: the source routes via its claiming pipeline
	mkPipeline(t, "am-pipe", []string{"am"}, nil, "prof-alert")
	reconcilePipeline(t, "am-pipe")

	h := apiServer().Handler()
	labels := map[string]string{"alertgroup": "g", "alertname": "TestAlert", "namespace": "ha"}

	// two fresh same-signature signals in ONE batch → one conversation, one
	// combined out-of-line input
	rec := postSignal(t, h, testMasterToken, "am", []map[string]any{
		{"fingerprint": "fp-a", "labels": labels, "payload": "alert-a"},
		{"fingerprint": "fp-a2", "labels": labels, "payload": "alert-a2"},
	})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"conversations":1`) {
		t.Fatalf("batch: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Spec.Signature == "g/TestAlert/ha" {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("alert conversation not created")
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].PayloadRef == nil {
		t.Fatalf("one combined input with payloadRef expected: %+v", conv.Spec.Inputs)
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Spec.Inputs[0].PayloadRef.Name}, &ci); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ci.Spec.Payload, "alert-a") || !strings.Contains(ci.Spec.Payload, "alert-a2") {
		t.Fatalf("combined payload expected: %q", ci.Spec.Payload)
	}

	// same fingerprint suppressed by cooldown
	rec = postSignal(t, h, testMasterToken, "am", []map[string]any{{"fingerprint": "fp-a", "labels": labels}})
	if !strings.Contains(rec.Body.String(), `"queued":0`) {
		t.Fatalf("cooldown: %s", rec.Body.String())
	}

	// new fingerprint, same signature, session present -> recurrence into SAME conversation
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "sess-x"
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	if rec = postSignal(t, h, testMasterToken, "am", []map[string]any{{"fingerprint": "fp-b", "labels": labels}}); rec.Code != 200 {
		t.Fatal("recurrence post")
	}
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	count := 0
	for i := range list.Items {
		if list.Items[i].Spec.Signature == "g/TestAlert/ha" {
			count++
			conv = &list.Items[i]
		}
	}
	if count != 1 {
		t.Fatalf("duplicate conversation created: %d for one signature", count)
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, conv)
	if len(conv.Spec.Inputs) != 2 || conv.Spec.Inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("recurrence input expected: %+v", conv.Spec.Inputs)
	}
}

// srvStartMux returns the server's handler for in-process requests.
func srvStartMux(s *httpapi.Server) http.Handler {
	return s.Handler()
}

func TestAgentRuntimeSelection(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-rt")

	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = "custom-rt", ns
	rt.Spec.Image = "example/custom-agent:1.0"
	rt.Spec.Command = []string{"/agent-loop"}
	rt.Spec.IdleTTLMinutes = 42
	if err := k8sClient.Create(ctx, rt); err != nil {
		t.Fatal(err)
	}
	var prof agentopsv1alpha1.AgentProfile
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "prof-rt"}, &prof); err != nil {
		t.Fatal(err)
	}
	prof.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "custom-rt"}
	if err := k8sClient.Update(ctx, &prof); err != nil {
		t.Fatal(err)
	}

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "rt-1", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-rt"}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcile(t, "rt-1")

	var pod corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName("rt-1")}, &pod); err != nil {
		t.Fatalf("runtime pod: %v", err)
	}
	c := pod.Spec.Containers[0]
	if c.Image != "example/custom-agent:1.0" || len(c.Command) != 1 || c.Command[0] != "/agent-loop" {
		t.Fatalf("runtime not applied: image=%s command=%v", c.Image, c.Command)
	}
	ttlSeen := false
	for _, e := range c.Env {
		if e.Name == "RUNTIME_IDLE_TTL_M" && e.Value == "42" {
			ttlSeen = true
		}
	}
	if !ttlSeen {
		t.Fatalf("runtime TTL not applied: %+v", c.Env)
	}
}
