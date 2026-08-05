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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    = runtime.NewScheme()
	ns        = "default"
)

func TestMain(m *testing.M) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentopsv1alpha1.AddToScheme(scheme))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "chart", "files", "crds")},
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

func reconcilerWithOps(ops *chat.OpQueue) *controller.ConversationReconciler {
	return &controller.ConversationReconciler{
		Client:      k8sClient,
		Scheme:      scheme,
		MaxRuntimes: 2,
		Ops:         ops,
		Runtime: runtimepod.Config{
			Image: "busybox:stub", ServiceAccount: "default",
			ControlURL: "http://manager:8080", IdleTTLMinutes: 10,
		},
	}
}

func apiServer() *httpapi.Server {
	ops := testOps()
	return &httpapi.Server{
		Client: k8sClient, Reader: k8sClient, Namespace: ns,
		Ops:          ops,
		Router:       &chat.Router{Client: k8sClient, Reader: k8sClient, Namespace: ns, Ops: ops},
		AdapterToken: "test-adapter-token",
	}
}

func mkProfile(t *testing.T, name string) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/repo.git", Ref: "main"}
	p.Spec.Agent = "tester"
	p.Spec.AllowedTools = "Read"
	p.Spec.MaxTurns = 5
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
	if after.Status.SessionID != "sess-life" || after.Status.Inflight != nil ||
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

func TestAlertGroupingAndRecurrence(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-alert")

	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = "am", ns
	src.Spec.Type = agentopsv1alpha1.SourceAlertmanager
	src.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-alert"}
	if err := k8sClient.Create(ctx, src); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	post := func(fp string) int {
		payload, _ := json.Marshal(map[string]any{"alerts": []map[string]any{{
			"status": "firing", "fingerprint": fp,
			"labels": map[string]string{"alertgroup": "g", "alertname": "TestAlert", "namespace": "ha"},
		}}})
		rec := httptest.NewRecorder()
		srvStartMux(srv).ServeHTTP(rec, httptest.NewRequest("POST", "/ingest/alertmanager/am", bytes.NewReader(payload)))
		return rec.Code
	}
	if code := post("fp-a"); code != 200 {
		t.Fatalf("ingest: %d", code)
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
		t.Fatalf("input with payloadRef expected: %+v", conv.Spec.Inputs)
	}

	// same fingerprint suppressed by cooldown
	if code := post("fp-a"); code != 200 {
		t.Fatal("cooldown post")
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, conv)
	if len(conv.Spec.Inputs) != 1 {
		t.Fatalf("cooldown failed, inputs: %d", len(conv.Spec.Inputs))
	}

	// new fingerprint, same signature, session present -> recurrence into SAME conversation
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "sess-x"
	now := time.Now()
	_ = now
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	// reconcile to ensure signature label is set for lookup
	reconcile(t, conv.Name)
	if code := post("fp-b"); code != 200 {
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
		t.Fatalf("duplicate topic created: %d conversations for one signature", count)
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
