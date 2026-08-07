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

// AdapterDeploymentName names the reconciler-owned workload for a
// ChannelAdapter CR.
func AdapterDeploymentName(adapterName string) string {
	return "agentops-adapter-" + adapterName
}

// ChannelAdapterReconciler owns channel-adapter workloads: one Deployment
// (+ zero-RBAC ServiceAccount) per ChannelAdapter, with the contract URL, a
// derived per-adapter token, and every served Channel's projected credentials.
// The adapter CR's NAME is the routing key — Channels select it via spec.type,
// so one adapter per implementation holds by construction.
type ChannelAdapterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ManagerURL is injected as MANAGER_URL (the adapter contract endpoint).
	ManagerURL string
	// MasterToken derives per-adapter contract tokens; empty disables auth
	// injection (the adapter surface is then 503 manager-side anyway).
	MasterToken string
}

// Reconcile renders the adapter workload for one ChannelAdapter.
func (r *ChannelAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var adapter agentopsv1alpha1.ChannelAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// served channels + credential projection (collisions reported, first wins)
	var channels agentopsv1alpha1.ChannelList
	if err := r.List(ctx, &channels, client.InNamespace(adapter.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	served := 0
	var creds []credentialItem
	for i := range channels.Items {
		ch := &channels.Items[i]
		if ch.Spec.Adapter != adapter.Name {
			continue
		}
		served++
		if ch.Spec.CredentialsSecretRef != nil {
			creds = append(creds, credentialItem{Name: ch.Name, SecretName: ch.Spec.CredentialsSecretRef.Name})
		}
	}
	envFrom, collisions := projectCredentials(creds)

	env := []corev1.EnvVar{
		{Name: "MANAGER_URL", Value: r.ManagerURL},
		{Name: "ADAPTER_NAME", Value: adapter.Name},
	}
	if r.MasterToken != "" {
		env = append(env, corev1.EnvVar{Name: "ADAPTER_TOKEN", Value: chat.DeriveAdapterToken(r.MasterToken, adapter.Name)})
	}

	deploy, err := ensureAdapterWorkload(ctx, r.Client, r.Scheme, adapterWorkload{
		Owner: &adapter,
		Name:  AdapterDeploymentName(adapter.Name),
		Labels: map[string]string{
			"app.kubernetes.io/name": "agentops-adapter",
			"agentops.dev/adapter":   adapter.Name,
		},
		SelectorKey: "agentops.dev/adapter",
		Image:       adapter.Spec.Image,
		Env:         env,
		EnvFrom:     envFrom,
		Singleton:   adapter.Spec.Singleton == nil || *adapter.Spec.Singleton,
		Resources:   adapter.Spec.Resources,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// status
	patch := client.MergeFrom(adapter.DeepCopy())
	adapter.Status.ServedChannels = int32(served)
	deployed := metav1.Condition{Type: ConditionDeployed, Status: metav1.ConditionTrue, Reason: "WorkloadRendered"}
	if len(collisions) > 0 {
		deployed.Reason = "CredentialCollision"
		deployed.Message = "credential env prefixes collide after sanitization (first channel wins): " + strings.Join(collisions, "; ")
		logger.Info("credential projection collision", "adapter", adapter.Name, "collisions", collisions)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, deployed)
	ready := metav1.Condition{Type: ConditionReady, Status: metav1.ConditionFalse, Reason: "WorkloadUnavailable"}
	if deploy.Status.AvailableReplicas > 0 {
		ready.Status = metav1.ConditionTrue
		ready.Reason = "WorkloadAvailable"
		ready.Message = fmt.Sprintf("serving %d channel(s)", served)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &adapter, patch)
}

// SetupWithManager wires the controller: adapter CRs, their owned Deployments,
// and Channel events (projection inputs) mapped straight to the adapter the
// channel names in spec.type.
func (r *ChannelAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapChannel := func(ctx context.Context, obj client.Object) []ctrl.Request {
		ch, ok := obj.(*agentopsv1alpha1.Channel)
		if !ok || ch.Spec.Adapter == "" {
			return nil
		}
		return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: ch.Namespace, Name: ch.Spec.Adapter}}}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.ChannelAdapter{}).
		Owns(&appsv1.Deployment{}).
		Watches(&agentopsv1alpha1.Channel{}, handler.EnqueueRequestsFromMapFunc(mapChannel)).
		Complete(r)
}
