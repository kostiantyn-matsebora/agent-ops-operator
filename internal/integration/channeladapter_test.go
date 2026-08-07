package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

const testMasterToken = "test-adapter-token"

func adapterReconciler() *controller.ChannelAdapterReconciler {
	return &controller.ChannelAdapterReconciler{
		Client: k8sClient, Scheme: scheme,
		ManagerURL:  "http://manager:8080",
		MasterToken: testMasterToken,
	}
}

func reconcileAdapter(t *testing.T, name string) {
	t.Helper()
	if _, err := adapterReconciler().Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
}

// mkAdapter creates a ChannelAdapter — its NAME is the type key Channels
// select via spec.type.
func mkAdapter(t *testing.T, name string) *agentopsv1alpha1.ChannelAdapter {
	t.Helper()
	a := &agentopsv1alpha1.ChannelAdapter{}
	a.Name, a.Namespace = name, ns
	a.Spec.Image = "example/adapter:1"
	if err := k8sClient.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func mkTypedChannel(t *testing.T, name, channelType, credSecret string) {
	t.Helper()
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, ns
	ch.Spec.Type = channelType
	if credSecret != "" {
		ch.Spec.CredentialsSecretRef = &corev1.LocalObjectReference{Name: credSecret}
	}
	if err := k8sClient.Create(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

func TestChannelAdapterDeployment(t *testing.T) {
	ctx := context.Background()
	mkTypedChannel(t, "cad-chan-a", "cad-main", "bot-secret-a")
	mkTypedChannel(t, "cad-chan-b", "cad-main", "bot-secret-b")
	mkTypedChannel(t, "cad-nocred", "cad-main", "")
	mkAdapter(t, "cad-main")
	reconcileAdapter(t, "cad-main")

	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("cad-main")}, &deploy); err != nil {
		t.Fatalf("adapter deployment not created: %v", err)
	}
	// ownerRef -> GC on adapter delete
	if len(deploy.OwnerReferences) == 0 || deploy.OwnerReferences[0].Kind != "ChannelAdapter" {
		t.Fatalf("ownerRef missing: %+v", deploy.OwnerReferences)
	}
	// singleton discipline
	if *deploy.Spec.Replicas != 1 || deploy.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Fatalf("singleton not enforced: replicas=%d strategy=%s", *deploy.Spec.Replicas, deploy.Spec.Strategy.Type)
	}
	pod := deploy.Spec.Template.Spec
	// zero ambient authority
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatal("SA token automount not disabled")
	}
	if pod.ServiceAccountName != controller.AdapterDeploymentName("cad-main") {
		t.Fatalf("dedicated SA not set: %s", pod.ServiceAccountName)
	}
	var sa corev1.ServiceAccount
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: pod.ServiceAccountName}, &sa); err != nil {
		t.Fatalf("dedicated SA not created: %v", err)
	}
	// contract wiring: MANAGER_URL, CHANNEL_TYPE (= adapter name), derived
	// (never master) token
	env := map[string]string{}
	for _, e := range pod.Containers[0].Env {
		env[e.Name] = e.Value
	}
	if env["MANAGER_URL"] != "http://manager:8080" || env["CHANNEL_TYPE"] != "cad-main" {
		t.Fatalf("contract env wrong: %+v", env)
	}
	if want := chat.DeriveAdapterToken(testMasterToken, "cad-main"); env["ADAPTER_TOKEN"] != want {
		t.Fatalf("ADAPTER_TOKEN not the derived token")
	}
	// credential projection: envFrom prefix per credentialed channel, none for
	// the credential-less one
	prefixes := map[string]string{}
	for _, ef := range pod.Containers[0].EnvFrom {
		prefixes[ef.Prefix] = ef.SecretRef.Name
	}
	if prefixes[controller.CredentialEnvPrefix("cad-chan-a")] != "bot-secret-a" ||
		prefixes[controller.CredentialEnvPrefix("cad-chan-b")] != "bot-secret-b" || len(prefixes) != 2 {
		t.Fatalf("credential projection wrong: %+v", prefixes)
	}

	// status
	var adapter agentopsv1alpha1.ChannelAdapter
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "cad-main"}, &adapter)
	if adapter.Status.ServedChannels != 3 {
		t.Fatalf("servedChannels: %d", adapter.Status.ServedChannels)
	}
	if !apimeta.IsStatusConditionTrue(adapter.Status.Conditions, controller.ConditionDeployed) {
		t.Fatalf("Deployed condition: %+v", adapter.Status.Conditions)
	}
	if apimeta.IsStatusConditionTrue(adapter.Status.Conditions, controller.ConditionReady) {
		t.Fatal("Ready should be false with no available replicas (no kubelet)")
	}
}

func TestChannelAdapterCredentialChangeRollsTemplate(t *testing.T) {
	ctx := context.Background()
	mkTypedChannel(t, "roll-chan", "roll-main", "roll-secret-1")
	mkAdapter(t, "roll-main")
	reconcileAdapter(t, "roll-main")

	name := types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("roll-main")}
	var before appsv1.Deployment
	_ = k8sClient.Get(ctx, name, &before)

	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "roll-chan"}, &ch); err != nil {
		t.Fatal(err)
	}
	ch.Spec.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "roll-secret-2"}
	if err := k8sClient.Update(ctx, &ch); err != nil {
		t.Fatal(err)
	}
	reconcileAdapter(t, "roll-main")

	var after appsv1.Deployment
	_ = k8sClient.Get(ctx, name, &after)
	got := after.Spec.Template.Spec.Containers[0].EnvFrom
	if len(got) != 1 || got[0].SecretRef.Name != "roll-secret-2" {
		t.Fatalf("credential change not projected: %+v", got)
	}
	if len(before.Spec.Template.Spec.Containers[0].EnvFrom) != 1 ||
		before.Spec.Template.Spec.Containers[0].EnvFrom[0].SecretRef.Name != "roll-secret-1" {
		t.Fatalf("precondition wrong: %+v", before.Spec.Template.Spec.Containers[0].EnvFrom)
	}
}

func TestChannelAdapterCredentialCollision(t *testing.T) {
	ctx := context.Background()

	// sanitized-prefix collision reported, first channel wins
	mkTypedChannel(t, "col.chan", "guard-col", "sec-1")
	mkTypedChannel(t, "col-chan", "guard-col", "sec-2")
	col := mkAdapter(t, "guard-col")
	reconcileAdapter(t, "guard-col")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "guard-col"}, col)
	dep := apimeta.FindStatusCondition(col.Status.Conditions, controller.ConditionDeployed)
	if dep == nil || dep.Reason != "CredentialCollision" {
		t.Fatalf("collision not reported: %+v", col.Status.Conditions)
	}
	var deploy appsv1.Deployment
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("guard-col")}, &deploy)
	efs := deploy.Spec.Template.Spec.Containers[0].EnvFrom
	if len(efs) != 1 || efs[0].SecretRef.Name != "sec-2" { // "col-chan" sorts first
		t.Fatalf("collision handling wrong (first sorted channel should win): %+v", efs)
	}
}

func TestChannelServedCondition(t *testing.T) {
	ctx := context.Background()
	registry := chat.NewRegistry()
	chanRec := &controller.ChannelReconciler{Client: k8sClient, Registry: registry}
	reconcileChan := func(name string) {
		t.Helper()
		if _, err := chanRec.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
			t.Fatal(err)
		}
	}

	// unserved type (the typo case) -> Served=False
	mkTypedChannel(t, "srv-typo", "slak", "")
	reconcileChan("srv-typo")
	var ch agentopsv1alpha1.Channel
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "srv-typo"}, &ch)
	if !apimeta.IsStatusConditionFalse(ch.Status.Conditions, controller.ConditionServed) {
		t.Fatalf("unserved type should be Served=False: %+v", ch.Status.Conditions)
	}

	// in-process registry entry -> Served=True
	registry.Register("srv-builtin", func(ctx context.Context, name string) (chat.Provider, error) { return nil, nil })
	mkTypedChannel(t, "srv-web", "srv-builtin", "")
	reconcileChan("srv-web")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "srv-web"}, &ch)
	if !apimeta.IsStatusConditionTrue(ch.Status.Conditions, controller.ConditionServed) {
		t.Fatalf("registry-served type should be Served=True: %+v", ch.Status.Conditions)
	}

	// adapter becomes Ready -> Served flips True (channel names the adapter)
	mkTypedChannel(t, "srv-ext", "srv-adapter", "")
	reconcileChan("srv-ext")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "srv-ext"}, &ch)
	if !apimeta.IsStatusConditionFalse(ch.Status.Conditions, controller.ConditionServed) {
		t.Fatal("expected Served=False before adapter exists")
	}
	mkAdapter(t, "srv-adapter")
	reconcileAdapter(t, "srv-adapter")
	// no kubelet: make the workload report availability, then let the adapter
	// reconciler compute Ready
	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("srv-adapter")}, &deploy); err != nil {
		t.Fatal(err)
	}
	deploy.Status.AvailableReplicas = 1
	deploy.Status.ReadyReplicas = 1
	deploy.Status.UpdatedReplicas = 1
	deploy.Status.Replicas = 1
	if err := k8sClient.Status().Update(ctx, &deploy); err != nil {
		t.Fatal(err)
	}
	reconcileAdapter(t, "srv-adapter")
	reconcileChan("srv-ext")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "srv-ext"}, &ch)
	served := apimeta.FindStatusCondition(ch.Status.Conditions, controller.ConditionServed)
	if served == nil || served.Status != "True" || served.Reason != "AdapterReady" {
		t.Fatalf("Served should flip True via ready adapter: %+v", served)
	}
}

func TestAdapterAuthScoping(t *testing.T) {
	ctx := context.Background()
	mkAdapter(t, "scope-slack")
	mkTypedChannel(t, "scope-chan", "scope-tg-type", "scope-secret")

	srv := apiServer() // AdapterToken = testMasterToken
	h := srv.Handler()
	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	derived := chat.DeriveAdapterToken(testMasterToken, "scope-slack")

	// cross-key: derived token polling another adapter's key -> 403
	if rec := get("/channel/ops?type=scope-tg-type&wait=0", derived); rec.Code != 403 {
		t.Fatalf("cross-type ops: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	// own key (the adapter's NAME) -> served (204: no ops queued)
	if rec := get("/channel/ops?type=scope-slack&wait=0", derived); rec.Code != 204 {
		t.Fatalf("own-type ops: want 204, got %d %s", rec.Code, rec.Body.String())
	}
	// channel-scoped endpoints enforce the resolved channel's type
	if rec := get("/channel/state/scope-chan/offset", derived); rec.Code != 403 {
		t.Fatalf("cross-type state: want 403, got %d", rec.Code)
	}
	// master token keeps full scope
	if rec := get("/channel/ops?type=scope-tg-type&wait=0", testMasterToken); rec.Code != 204 {
		t.Fatalf("master scope: want 204, got %d", rec.Code)
	}
	// garbage token -> 401
	if rec := get("/channel/ops?type=scope-tg-type&wait=0", "nonsense"); rec.Code != 401 {
		t.Fatalf("bad token: want 401, got %d", rec.Code)
	}

	// listing carries credentialEnvPrefix for credentialed channels (3.2)
	rec := get("/channel/channels?type=scope-tg-type", testMasterToken)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), controller.CredentialEnvPrefix("scope-chan")) {
		t.Fatalf("listing missing credentialEnvPrefix: %d %s", rec.Code, rec.Body.String())
	}
	_ = ctx
}
