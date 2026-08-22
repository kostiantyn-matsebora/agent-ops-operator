package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

// The console end to end, manager-side: the adapter CR the chart ships must
// reconcile into a workload that can reach the API server and a Service the
// browser can hit, its Channel must go Served, and a conversation on a
// console-wired Pipeline must bind a console thread through the ordinary ops
// flow. The console binary itself is tested in console/ — here the question is
// whether the operator gives it what it needs.

func TestConsoleAdapterReconcilesToAReachableWorkload(t *testing.T) {
	ctx := context.Background()

	adapter := &agentopsv1alpha1.ChannelAdapter{}
	adapter.Name, adapter.Namespace = "console", ns
	adapter.Spec.Image = "kmatsebora/agentops-console:0.1.0"
	yes := true
	port := int32(8080)
	adapter.Spec.KubernetesAccess = &yes
	adapter.Spec.Port = &port
	adapter.Spec.Singleton = &yes
	if err := k8sClient.Create(ctx, adapter); err != nil {
		t.Fatal(err)
	}

	// the Channel carries the browser token as an ordinary per-surface credential
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "console", ns
	ch.Spec.Adapter = "console"
	ch.Spec.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "agentops-console-console"}
	ch.Spec.Config = &runtime.RawExtension{Raw: []byte(`{"displayName":"agent-ops console"}`)}
	if err := k8sClient.Create(ctx, ch); err != nil {
		t.Fatal(err)
	}
	reconcileAdapter(t, "console")

	workload := types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("console")}
	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, workload, &deploy); err != nil {
		t.Fatalf("console workload not created: %v", err)
	}
	pod := deploy.Spec.Template.Spec
	// API identity for the watch cache — granted by the chart's Role, never here
	if pod.AutomountServiceAccountToken == nil || !*pod.AutomountServiceAccountToken {
		t.Fatal("console needs its SA token mounted (kubernetesAccess)")
	}
	env := map[string]string{}
	hasNamespace := false
	for _, e := range pod.Containers[0].Env {
		env[e.Name] = e.Value
		if e.Name == "POD_NAMESPACE" && e.ValueFrom != nil {
			hasNamespace = true
		}
	}
	if !hasNamespace {
		t.Fatalf("POD_NAMESPACE missing — the console cannot scope its watches: %+v", pod.Containers[0].Env)
	}
	if env["LISTEN_ADDR"] != ":8080" || env["ADAPTER_NAME"] != "console" || env["MANAGER_URL"] == "" {
		t.Fatalf("contract env wrong: %+v", env)
	}
	if env["ADAPTER_TOKEN"] != chat.DeriveAdapterToken(testMasterToken, "console") {
		t.Fatal("console must receive its own derived contract token")
	}
	// the browser token arrives by projection, never by an API read
	prefix := controller.CredentialEnvPrefix("console")
	found := false
	for _, ef := range pod.Containers[0].EnvFrom {
		if ef.Prefix == prefix && ef.SecretRef != nil && ef.SecretRef.Name == "agentops-console-console" {
			found = true
		}
	}
	if !found {
		t.Fatalf("UI token Secret not projected: %+v", pod.Containers[0].EnvFrom)
	}

	// browser-facing Service, owned by the reconciler
	var svc corev1.Service
	if err := k8sClient.Get(ctx, workload, &svc); err != nil {
		t.Fatalf("console Service not rendered: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 8080 {
		t.Fatalf("console Service port wrong: %+v", svc.Spec.Ports)
	}

	// the Channel is Served once the adapter reports available
	// no kubelet here: report availability by hand so the adapter reconciler
	// can compute Ready
	deploy.Status.Replicas = 1
	deploy.Status.ReadyReplicas = 1
	deploy.Status.UpdatedReplicas = 1
	deploy.Status.AvailableReplicas = 1
	if err := k8sClient.Status().Update(ctx, &deploy); err != nil {
		t.Fatal(err)
	}
	reconcileAdapter(t, "console")
	chanRec := &controller.ChannelReconciler{Client: k8sClient, Registry: chat.NewRegistry()}
	if _, err := chanRec.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "console"}}); err != nil {
		t.Fatal(err)
	}
	var served agentopsv1alpha1.Channel
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "console"}, &served)
	if !apimeta.IsStatusConditionTrue(served.Status.Conditions, controller.ConditionServed) {
		t.Fatalf("console Channel should be Served: %+v", served.Status.Conditions)
	}
}

// A conversation on a console-wired Pipeline binds a console thread through
// the same ops flow every adapter uses — the console gets no special path.
func TestConsoleWiredPipelineBindsAConsoleThread(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-console")
	mkChannel(t, "chan-console-ui", "console-ui")
	mkChatSource(t, "src-console", "chan-console-ui")
	mkPipeline(t, "console-pipe", []string{"src-console"}, []string{"chan-console-ui"}, "prof-console")
	reconcilePipeline(t, "console-pipe")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "console-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-console"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-console-ui"}}
	conv.Spec.Title = "watch me"
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}

	rc := reconcilerWithOps(srv.Ops)
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "console-conv"}}); err != nil {
		t.Fatal(err)
	}
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=console-ui&contract=2&wait=0", nil, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("ensure-topic op expected for the console: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if op.Kind != chat.OpEnsureTopic || op.Conversation != "console-conv" {
		t.Fatalf("op: %+v", op)
	}

	// the console completes with its deterministic, UID-derived thread id —
	// an opaque string like any other adapter's
	var created agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "console-conv"}, &created)
	threadID := "console-" + string(created.UID)
	rec = adapterReq(srv, "POST", fmt.Sprintf("/channel/ops/%s/done", op.ID),
		chat.OpResult{ThreadID: threadID}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("done: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "console-conv"}, &after)
	tid := after.ThreadFor("chan-console-ui")
	if tid == nil || *tid != threadID {
		t.Fatalf("console thread binding not landed: %+v", after.Status)
	}

	// and a message typed in the UI continues the conversation like any reply
	rec = adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-console-ui", "threadId": threadID,
			"text": "also check the disks", "sender": "console/kim"}, testMasterToken)
	if rec.Code != 202 {
		t.Fatalf("console inbound: %d %s", rec.Code, rec.Body.String())
	}
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "console-conv"}, &after)
	last := after.Spec.Inputs[len(after.Spec.Inputs)-1]
	if last.Type != agentopsv1alpha1.InputReply || last.Payload != "also check the disks" {
		t.Fatalf("UI message did not enter the queue as a reply: %+v", after.Spec.Inputs)
	}
}
