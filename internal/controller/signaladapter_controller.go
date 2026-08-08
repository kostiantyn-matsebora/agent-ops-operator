package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// SignalAdapterDeploymentName names the reconciler-owned workload for a
// SignalAdapter CR (distinct namespace from channel adapters so same-named
// CRs never collide on workload objects). The Service rendered for a ported
// adapter shares this name.
func SignalAdapterDeploymentName(adapterName string) string {
	return "agentops-signal-" + adapterName
}

// SignalAdapterReconciler owns signal-adapter workloads — the SignalAdapter
// sibling of ChannelAdapterReconciler, on the shared workload machinery:
// per-adapter derived token (signal derivation context), SOURCE_TYPE env, and
// credential projection from served SignalSources. The adapter CR's NAME is
// the routing key — SignalSources select it via spec.type. When spec.port is
// declared the reconciler also owns a Service agentops-signal-<name> and
// injects LISTEN_ADDR, so enabling a webhook-receiving adapter is a complete
// appliance with no chart-side connectivity.
type SignalAdapterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ManagerURL is injected as MANAGER_URL (the signal contract endpoint).
	ManagerURL string
	// MasterToken derives per-adapter contract tokens; empty disables auth
	// injection (the signal surface is then 503 manager-side anyway).
	MasterToken string
}

// Reconcile renders the adapter workload for one SignalAdapter.
func (r *SignalAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var adapter agentopsv1alpha1.SignalAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// served sources + credential projection (collisions reported, first wins)
	var sources agentopsv1alpha1.SignalSourceList
	if err := r.List(ctx, &sources, client.InNamespace(adapter.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	served := 0
	var creds []credentialItem
	for i := range sources.Items {
		src := &sources.Items[i]
		if src.Spec.Adapter != adapter.Name {
			continue
		}
		served++
		if src.Spec.CredentialsSecretRef != nil {
			creds = append(creds, credentialItem{Name: src.Name, SecretName: src.Spec.CredentialsSecretRef.Name})
		}
	}
	envFrom, collisions := projectCredentials(creds)

	env := []corev1.EnvVar{
		{Name: "MANAGER_URL", Value: r.ManagerURL},
		{Name: "ADAPTER_NAME", Value: adapter.Name},
	}
	if r.MasterToken != "" {
		env = append(env, corev1.EnvVar{Name: "ADAPTER_TOKEN", Value: chat.DeriveSignalAdapterToken(r.MasterToken, adapter.Name)})
	}
	if adapter.Spec.Port != nil {
		env = append(env, corev1.EnvVar{Name: "LISTEN_ADDR", Value: fmt.Sprintf(":%d", *adapter.Spec.Port)})
	}

	labels := map[string]string{
		"app.kubernetes.io/name":      "agentops-signal-adapter",
		"agentops.dev/signal-adapter": adapter.Name,
	}
	deploy, err := ensureAdapterWorkload(ctx, r.Client, r.Scheme, adapterWorkload{
		Owner:            &adapter,
		Name:             SignalAdapterDeploymentName(adapter.Name),
		Labels:           labels,
		SelectorKey:      "agentops.dev/signal-adapter",
		Image:            adapter.Spec.Image,
		Env:              env,
		EnvFrom:          envFrom,
		Singleton:        adapter.Spec.Singleton == nil || *adapter.Spec.Singleton,
		Resources:        adapter.Spec.Resources,
		KubernetesAccess: adapter.Spec.KubernetesAccess != nil && *adapter.Spec.KubernetesAccess,
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := ensureAdapterService(ctx, r.Client, r.Scheme, adapterService{
		Owner:       &adapter,
		Name:        SignalAdapterDeploymentName(adapter.Name),
		Labels:      labels,
		SelectorKey: "agentops.dev/signal-adapter",
		Port:        adapter.Spec.Port,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// status. The declared schema is compile-checked here, where it was
	// authored — a broken one is reported but never blocks the workload.
	_, schemaCond := compileDeclaredSchema(adapter.Spec.ConfigSchema)
	patch := client.MergeFrom(adapter.DeepCopy())
	applySchemaCondition(&adapter.Status.Conditions, schemaCond)
	adapter.Status.ServedSources = int32(served)
	deployed := metav1.Condition{Type: ConditionDeployed, Status: metav1.ConditionTrue, Reason: "WorkloadRendered"}
	if len(collisions) > 0 {
		deployed.Reason = "CredentialCollision"
		deployed.Message = "credential env prefixes collide after sanitization (first source wins): " + strings.Join(collisions, "; ")
		logger.Info("credential projection collision", "adapter", adapter.Name, "collisions", collisions)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, deployed)
	ready := metav1.Condition{Type: ConditionReady, Status: metav1.ConditionFalse, Reason: "WorkloadUnavailable"}
	if deploy.Status.AvailableReplicas > 0 {
		ready.Status = metav1.ConditionTrue
		ready.Reason = "WorkloadAvailable"
		ready.Message = fmt.Sprintf("serving %d source(s)", served)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &adapter, patch)
}

// SetupWithManager wires the controller (mirrors ChannelAdapterReconciler):
// adapter CRs, their owned Deployments and Services, and SignalSource events
// mapped straight to the adapter the source names in spec.type.
func (r *SignalAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapSource := func(ctx context.Context, obj client.Object) []ctrl.Request {
		src, ok := obj.(*agentopsv1alpha1.SignalSource)
		if !ok || src.Spec.Adapter == "" {
			return nil
		}
		return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: src.Namespace, Name: src.Spec.Adapter}}}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.SignalAdapter{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&agentopsv1alpha1.SignalSource{}, handler.EnqueueRequestsFromMapFunc(mapSource)).
		Complete(r)
}
