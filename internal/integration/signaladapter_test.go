package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

func signalAdapterReconciler() *controller.SignalAdapterReconciler {
	return &controller.SignalAdapterReconciler{
		Client: k8sClient, Scheme: scheme,
		ManagerURL:  "http://manager:8080",
		MasterToken: testMasterToken,
	}
}

func reconcileSignalAdapter(t *testing.T, name string) {
	t.Helper()
	if _, err := signalAdapterReconciler().Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
}

func mkSignalAdapter(t *testing.T, name, sourceType string) *agentopsv1alpha1.SignalAdapter {
	t.Helper()
	a := &agentopsv1alpha1.SignalAdapter{}
	a.Name, a.Namespace = name, ns
	a.Spec.Type = sourceType
	a.Spec.Image = "example/signal-adapter:1"
	if err := k8sClient.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func mkSignalSource(t *testing.T, name, sourceType, credSecret string) {
	t.Helper()
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, ns
	src.Spec.Type = sourceType
	if credSecret != "" {
		src.Spec.CredentialsSecretRef = &corev1.LocalObjectReference{Name: credSecret}
	}
	if err := k8sClient.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
}

// postSignal posts normalized signals for a source with the given token.
func postSignal(t *testing.T, srv http.Handler, token, source string, signals []map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"source": source, "signals": signals})
	req := httptest.NewRequest("POST", "/signal/inbound", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestSignalInboundRouting(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-signal")
	mkSignalSource(t, "sig-src", "sig-t", "")
	// pipeline-only wiring: the source routes via its claiming pipeline
	mkPipeline(t, "sig-pipe", []string{"sig-src"}, nil, "prof-signal")
	reconcilePipeline(t, "sig-pipe")
	h := apiServer().Handler()

	// unwired sources drop signals loudly, BEFORE burning cooldown slots
	mkSignalSource(t, "sig-unwired", "sig-t", "")
	rec0 := postSignal(t, h, testMasterToken, "sig-unwired", []map[string]any{{
		"fingerprint": "u-1", "labels": map[string]string{"alertname": "x"},
	}})
	if rec0.Code != 200 || !strings.Contains(rec0.Body.String(), "not claimed by a Ready pipeline") {
		t.Fatalf("unwired drop reason expected: %d %s", rec0.Code, rec0.Body.String())
	}

	// job-kind signal with title override → job-lane conversation
	rec := postSignal(t, h, testMasterToken, "sig-src", []map[string]any{{
		"fingerprint": "tick-1", "labels": map[string]string{"alertname": "nightly"},
		"title": "🛠 Nightly job", "payload": "run the nightly checks", "kind": "job",
	}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("inbound: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Spec.Title == "🛠 Nightly job" {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("job conversation not created")
	}
	if !strings.HasPrefix(conv.Name, "job-") {
		t.Fatalf("job conversation name: %s", conv.Name)
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].Type != agentopsv1alpha1.InputJob ||
		conv.Spec.Inputs[0].JobName != "sig-src" || conv.Spec.Inputs[0].PayloadRef == nil {
		t.Fatalf("job input wrong: %+v", conv.Spec.Inputs)
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Spec.Inputs[0].PayloadRef.Name}, &ci); err != nil {
		t.Fatal(err)
	}
	if ci.Spec.Payload != "run the nightly checks" {
		t.Fatalf("payload: %q", ci.Spec.Payload)
	}

	// duplicate fingerprint absorbed by cooldown (at-least-once safe)
	rec = postSignal(t, h, testMasterToken, "sig-src", []map[string]any{{
		"fingerprint": "tick-1", "labels": map[string]string{"alertname": "nightly"}, "kind": "job",
	}})
	if !strings.Contains(rec.Body.String(), `"queued":0`) {
		t.Fatalf("cooldown not applied: %s", rec.Body.String())
	}

	// same signature + session → recurrence into the SAME conversation
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "sess-sig"
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	// (signature label is set at creation by the routing core — no reconcile
	// needed, which also keeps the shared runtime-pod pool untouched)
	rec = postSignal(t, h, testMasterToken, "sig-src", []map[string]any{{
		"fingerprint": "tick-2", "labels": map[string]string{"alertname": "nightly"}, "kind": "job",
	}})
	if !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("second tick: %s", rec.Body.String())
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, conv)
	if len(conv.Spec.Inputs) != 2 || conv.Spec.Inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("recurrence expected: %+v", conv.Spec.Inputs)
	}

	// unknown source → 404
	if rec := postSignal(t, h, testMasterToken, "no-such-source", []map[string]any{{"fingerprint": "x"}}); rec.Code != 404 {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	// missing fingerprint → 400
	if rec := postSignal(t, h, testMasterToken, "sig-src", []map[string]any{{"labels": map[string]string{}}}); rec.Code != 400 {
		t.Fatalf("missing fingerprint: %d", rec.Code)
	}

	// source bookkeeping updated in one place
	var src agentopsv1alpha1.SignalSource
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "sig-src"}, &src)
	if src.Status.ReceivedTotal != 2 || src.Status.LastReceived == nil {
		t.Fatalf("bookkeeping: %+v", src.Status)
	}
}

func TestSignalContractSurface(t *testing.T) {
	mkProfile(t, "prof-sigsurf")
	mkSignalSource(t, "surf-src", "surf-t", "surf-secret")
	h := apiServer().Handler()
	get := func(method, path, token string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// listing carries opaque config slot + credentialEnvPrefix
	rec := get("GET", "/signal/sources?type=surf-t", testMasterToken, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), controller.CredentialEnvPrefix("surf-src")) {
		t.Fatalf("sources listing: %d %s", rec.Code, rec.Body.String())
	}

	// cursor state round-trip
	if rec := get("PUT", "/signal/state/surf-src/last-fire", testMasterToken, []byte(`{"value":"2026-08-06T06:00:00Z"}`)); rec.Code != 200 {
		t.Fatalf("state put: %d", rec.Code)
	}
	rec = get("GET", "/signal/state/surf-src/last-fire", testMasterToken, nil)
	if !strings.Contains(rec.Body.String(), "2026-08-06T06:00:00Z") {
		t.Fatalf("state get: %s", rec.Body.String())
	}

	// status reporting lands the Ready condition
	if rec := get("POST", "/signal/sources/surf-src/status", testMasterToken, []byte(`{"ready":false,"reason":"InvalidConfig","message":"schedule is bad"}`)); rec.Code != 200 {
		t.Fatalf("status post: %d", rec.Code)
	}
	var src agentopsv1alpha1.SignalSource
	_ = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "surf-src"}, &src)
	ready := apimeta.FindStatusCondition(src.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || ready.Reason != "InvalidConfig" {
		t.Fatalf("ready condition: %+v", src.Status.Conditions)
	}
}

func TestSignalAuthScoping(t *testing.T) {
	mkSignalAdapter(t, "auth-sig", "auth-sig-type")
	mkAdapter(t, "auth-chan", "auth-chan-type") // channel adapter for cross-surface checks
	h := apiServer().Handler()
	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	sigTok := chat.DeriveSignalAdapterToken(testMasterToken, "auth-sig")
	chanTok := chat.DeriveAdapterToken(testMasterToken, "auth-chan")

	// own type OK, cross-type 403
	if rec := get("/signal/sources?type=auth-sig-type", sigTok); rec.Code != 200 {
		t.Fatalf("own type: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get("/signal/sources?type=other-type", sigTok); rec.Code != 403 {
		t.Fatalf("cross type: %d", rec.Code)
	}
	// cross-surface tokens are strangers (distinct derivation contexts)
	if rec := get("/signal/sources?type=auth-chan-type", chanTok); rec.Code != 401 {
		t.Fatalf("channel token on signal surface: %d", rec.Code)
	}
	if rec := get("/channel/ops?type=auth-sig-type&wait=0", sigTok); rec.Code != 401 {
		t.Fatalf("signal token on channel surface: %d", rec.Code)
	}
	// same-name adapters on both surfaces never share a token
	if chat.DeriveAdapterToken(testMasterToken, "x") == chat.DeriveSignalAdapterToken(testMasterToken, "x") {
		t.Fatal("derivation contexts collide")
	}
	// master keeps full scope on both surfaces
	if rec := get("/signal/sources?type=anything", testMasterToken); rec.Code != 200 {
		t.Fatalf("master scope: %d", rec.Code)
	}
}

func TestSignalAdapterLifecycle(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-siglife")
	mkSignalSource(t, "life-src-a", "life-sig-t", "life-secret")
	mkSignalSource(t, "life-src-b", "life-sig-t", "")
	mkSignalAdapter(t, "life-sig", "life-sig-t")
	reconcileSignalAdapter(t, "life-sig")

	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.SignalAdapterDeploymentName("life-sig")}, &deploy); err != nil {
		t.Fatalf("workload not created: %v", err)
	}
	pod := deploy.Spec.Template.Spec
	if *deploy.Spec.Replicas != 1 || deploy.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Fatal("singleton not enforced")
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken ||
		pod.ServiceAccountName != controller.SignalAdapterDeploymentName("life-sig") {
		t.Fatal("zero-authority posture missing")
	}
	env := map[string]string{}
	for _, e := range pod.Containers[0].Env {
		env[e.Name] = e.Value
	}
	if env["SOURCE_TYPE"] != "life-sig-t" || env["MANAGER_URL"] == "" {
		t.Fatalf("contract env: %+v", env)
	}
	if env["ADAPTER_TOKEN"] != chat.DeriveSignalAdapterToken(testMasterToken, "life-sig") {
		t.Fatal("ADAPTER_TOKEN is not the signal-context derived token")
	}
	efs := pod.Containers[0].EnvFrom
	if len(efs) != 1 || efs[0].Prefix != controller.CredentialEnvPrefix("life-src-a") || efs[0].SecretRef.Name != "life-secret" {
		t.Fatalf("projection wrong: %+v", efs)
	}
	var adapter agentopsv1alpha1.SignalAdapter
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-sig"}, &adapter)
	if adapter.Status.ServedSources != 2 || !apimeta.IsStatusConditionTrue(adapter.Status.Conditions, controller.ConditionDeployed) {
		t.Fatalf("status: %+v", adapter.Status)
	}

	// conflict guard
	second := mkSignalAdapter(t, "life-sig2", "life-sig-t")
	reconcileSignalAdapter(t, "life-sig2")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-sig2"}, second)
	if !apimeta.IsStatusConditionTrue(second.Status.Conditions, controller.ConditionTypeConflict) {
		t.Fatalf("TypeConflict missing: %+v", second.Status.Conditions)
	}

	// Served condition: built-in type always served; adapter type flips on Ready
	srcRec := &controller.SignalSourceReconciler{Client: k8sClient}
	reconcileSrc := func(name string) {
		t.Helper()
		if _, err := srcRec.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
			t.Fatal(err)
		}
	}
	mkSignalSource(t, "life-am", agentopsv1alpha1.SourceAlertmanager, "")
	reconcileSrc("life-am")
	var src agentopsv1alpha1.SignalSource
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-am"}, &src)
	if c := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionServed); c == nil || c.Status != "True" || c.Reason != "InProcessProvider" {
		t.Fatalf("built-in Served: %+v", src.Status.Conditions)
	}
	reconcileSrc("life-src-a")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-src-a"}, &src)
	if !apimeta.IsStatusConditionFalse(src.Status.Conditions, controller.ConditionServed) {
		t.Fatal("expected Served=False before adapter Ready")
	}
	// pipeline-only wiring: unclaimed → Wired=False; claim flips it
	if !apimeta.IsStatusConditionFalse(src.Status.Conditions, controller.ConditionWired) {
		t.Fatalf("expected Wired=False while unclaimed: %+v", src.Status.Conditions)
	}
	mkProfile(t, "prof-wired")
	mkPipeline(t, "life-wire-pipe", []string{"life-src-a"}, nil, "prof-wired")
	reconcilePipeline(t, "life-wire-pipe")
	reconcileSrc("life-src-a")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-src-a"}, &src)
	if w := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionWired); w == nil || w.Status != "True" || !strings.Contains(w.Message, "life-wire-pipe") {
		t.Fatalf("Wired should flip True naming the pipeline: %+v", src.Status.Conditions)
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.SignalAdapterDeploymentName("life-sig")}, &deploy)
	deploy.Status.AvailableReplicas, deploy.Status.ReadyReplicas, deploy.Status.UpdatedReplicas, deploy.Status.Replicas = 1, 1, 1, 1
	if err := k8sClient.Status().Update(ctx, &deploy); err != nil {
		t.Fatal(err)
	}
	reconcileSignalAdapter(t, "life-sig")
	reconcileSrc("life-src-a")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "life-src-a"}, &src)
	if c := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionServed); c == nil || c.Status != "True" || c.Reason != "AdapterReady" {
		t.Fatalf("Served should flip via ready adapter: %+v", src.Status.Conditions)
	}
}
