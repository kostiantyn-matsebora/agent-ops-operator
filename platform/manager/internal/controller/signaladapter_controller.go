package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
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
// per-adapter derived token (signal derivation context), ADAPTER_NAME env, and
// credential projection from served SignalSources. The adapter CR's NAME is
// the routing key — SignalSources select it via spec.adapter. When spec.port is
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
	// FloorServiceAccount is the release's floor account, from bootstrap
	// configuration. An adapter naming none runs as it — bound to nothing, and
	// refused as a binding target by the chart that renders it.
	FloorServiceAccount string
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

	// Externally served: this identity lives in ANOTHER adapter's pod, so this
	// reconciler owns no workload at all. Returning before ensureAdapterWorkload
	// is the whole mechanism — there is no "empty deployment" to garbage-collect,
	// because none was ever rendered.
	if adapter.Spec.ServedBy != nil {
		return ctrl.Result{}, r.reconcileExternallyServed(ctx, &adapter, served)
	}

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
		Owner:          &adapter,
		Name:           SignalAdapterDeploymentName(adapter.Name),
		Labels:         labels,
		SelectorKey:    "agentops.dev/signal-adapter",
		Image:          adapter.Spec.Image,
		Env:            env,
		EnvFrom:        envFrom,
		Singleton:      adapter.Spec.Singleton == nil || *adapter.Spec.Singleton,
		Resources:      adapter.Spec.Resources,
		ServiceAccount: resolveAdapterServiceAccount(adapter.Spec.ServiceAccountName, r.FloorServiceAccount),
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

// reconcileExternallyServed writes status for an adapter whose process belongs
// to another workload. Nothing is created: no Deployment, no Service, no
// ServiceAccount — the point of the mode is that two identities cost one pod.
//
// A dangling servedBy is reported rather than ignored: an adapter naming a
// workload that does not exist would otherwise sit silently Ready while nothing
// holds its token, and its sources would look Served while nothing serves them.
func (r *SignalAdapterReconciler) reconcileExternallyServed(ctx context.Context,
	adapter *agentopsv1alpha1.SignalAdapter, served int) error {

	target := adapter.Spec.ServedBy
	// Switching an adapter INTO this mode must take its old workload away.
	// OwnerRef GC will not: the owner is still there, so a Deployment left from
	// the image mode would keep running — and "two identities, two pods" is
	// precisely what the mode exists to prevent.
	if err := r.removeOwnedWorkload(ctx, adapter); err != nil {
		return err
	}
	patch := client.MergeFrom(adapter.DeepCopy())
	_, schemaCond := compileDeclaredSchema(adapter.Spec.ConfigSchema)
	applySchemaCondition(&adapter.Status.Conditions, schemaCond)
	adapter.Status.ServedSources = int32(served)

	apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
		Type: ConditionDeployed, Status: metav1.ConditionTrue, Reason: ReasonServedBy,
		Message: fmt.Sprintf("no workload of its own: %s/%s serves this identity", target.Kind, target.Name),
	})

	ready := metav1.Condition{
		Type: ConditionReady, Status: metav1.ConditionTrue, Reason: ReasonServedBy,
		Message: fmt.Sprintf("served by %s/%s; %d source(s)", target.Kind, target.Name, served),
	}
	var host agentopsv1alpha1.ChannelAdapter
	if err := r.Get(ctx, types.NamespacedName{Namespace: adapter.Namespace, Name: target.Name}, &host); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		ready.Status = metav1.ConditionFalse
		ready.Reason = "ServingAdapterMissing"
		ready.Message = fmt.Sprintf("servedBy names %s/%s, which does not exist — nothing holds this adapter's token",
			target.Kind, target.Name)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, ready)
	return r.Status().Patch(ctx, adapter, patch)
}

// removeOwnedWorkload deletes what this reconciler would otherwise own and what
// would keep RUNNING or ROUTING if left behind: the Deployment and the Service.
// Only ever called for an externally-served adapter, where the correct number of
// each is zero.
//
// The ServiceAccount is deliberately NOT deleted. The manager holds no `delete`
// verb on serviceaccounts — it creates them and lets ownerRef GC remove them
// when the adapter CR goes away — and widening that grant to tidy up an inert
// object would be the wrong trade: a leftover SA carries zero RBAC, no token
// automount and no workload, so it costs nothing, while a leftover Deployment is
// the second pod this whole mode exists to prevent.
func (r *SignalAdapterReconciler) removeOwnedWorkload(ctx context.Context, adapter *agentopsv1alpha1.SignalAdapter) error {
	name := SignalAdapterDeploymentName(adapter.Name)
	meta := metav1.ObjectMeta{Name: name, Namespace: adapter.Namespace}
	for _, obj := range []client.Object{
		&appsv1.Deployment{ObjectMeta: meta},
		&corev1.Service{ObjectMeta: meta},
	} {
		if err := client.IgnoreNotFound(r.Delete(ctx, obj)); err != nil {
			return err
		}
	}
	return nil
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
