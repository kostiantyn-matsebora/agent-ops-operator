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

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// AdapterDeploymentName names the reconciler-owned workload for a
// ChannelAdapter CR.
func AdapterDeploymentName(adapterName string) string {
	return "agentops-adapter-" + adapterName
}

const channelAdapterLabelKey = "agentops.dev/adapter"

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
	// FloorServiceAccount is the release's floor account, from bootstrap
	// configuration. An adapter naming none runs as it — bound to nothing, and
	// refused as a binding target by the chart that renders it.
	FloorServiceAccount string
}

// Reconcile renders the adapter workload for one ChannelAdapter. Each step
// below is a single named call — the "why" behind any one of them lives on
// that step's own function, not here.
func (r *ChannelAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var adapter agentopsv1alpha1.ChannelAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	served, envFrom, collisions, err := r.projectServedChannelCredentials(ctx, &adapter)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Looked up every reconcile, never cached: absence must remove the second
	// identity, not leave a token behind in a pod nobody re-rendered.
	signalIdentity, err := r.servedSignalAdapter(ctx, &adapter)
	if err != nil {
		return ctrl.Result{}, err
	}
	env := r.buildAdapterEnv(&adapter, signalIdentity)
	labels := buildAdapterLabels(&adapter)

	deploy, err := r.reconcileWorkload(ctx, &adapter, labels, env, envFrom)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.patchStatus(ctx, &adapter, served, collisions, deploy)
}

// projectServedChannelCredentials counts the Channels served by this adapter
// (spec.adapter naming it) and projects their credentials into env-from
// sources, collisions reported but never blocking (first channel wins).
func (r *ChannelAdapterReconciler) projectServedChannelCredentials(ctx context.Context,
	adapter *agentopsv1alpha1.ChannelAdapter) (served int, envFrom []corev1.EnvFromSource, collisions []string, err error) {

	var channels agentopsv1alpha1.ChannelList
	if err := r.List(ctx, &channels, client.InNamespace(adapter.Namespace)); err != nil {
		return 0, nil, nil, err
	}
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
	envFrom, collisions = projectCredentials(creds)
	return served, envFrom, collisions, nil
}

// buildAdapterEnv assembles the workload's env vars from already-resolved
// inputs — the contract endpoint, the derived per-adapter token, the second
// SignalAdapter identity (empty when none serves through this workload), and
// LISTEN_ADDR when a port is declared. Pure: no client calls, so it needs no
// context and is unit-testable on its own.
//
// The two tokens (channel, signal) use distinct derivation contexts, and each
// surface validates only against its own CRD list, so the identities stay
// separate — a channel token is still a stranger to /signal/*, and vice
// versa.
func (r *ChannelAdapterReconciler) buildAdapterEnv(adapter *agentopsv1alpha1.ChannelAdapter, signalIdentity string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "MANAGER_URL", Value: r.ManagerURL},
		{Name: "ADAPTER_NAME", Value: adapter.Name},
	}
	if r.MasterToken != "" {
		env = append(env, corev1.EnvVar{Name: "ADAPTER_TOKEN", Value: chat.DeriveAdapterToken(r.MasterToken, adapter.Name)})
	}
	if signalIdentity != "" && r.MasterToken != "" {
		env = append(env,
			corev1.EnvVar{Name: "SIGNAL_ADAPTER_NAME", Value: signalIdentity},
			corev1.EnvVar{Name: "SIGNAL_ADAPTER_TOKEN", Value: chat.DeriveSignalAdapterToken(r.MasterToken, signalIdentity)})
	}
	if adapter.Spec.Port != nil {
		env = append(env, corev1.EnvVar{Name: "LISTEN_ADDR", Value: fmt.Sprintf(":%d", *adapter.Spec.Port)})
	}
	return env
}

// buildAdapterLabels returns the label set shared by the Deployment, its pod
// template and the Service — one definition so all three stay in lock-step.
func buildAdapterLabels(adapter *agentopsv1alpha1.ChannelAdapter) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "agentops-adapter",
		"app.kubernetes.io/instance": adapter.Name,
		channelAdapterLabelKey:       adapter.Name,
	}
}

// reconcileWorkload ensures the Deployment and, when ported, the Service the
// adapter CR owns. Returns the Deployment so the caller can read its
// availability for the Ready condition.
func (r *ChannelAdapterReconciler) reconcileWorkload(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter,
	labels map[string]string, env []corev1.EnvVar, envFrom []corev1.EnvFromSource) (*appsv1.Deployment, error) {

	deploy, err := ensureAdapterWorkload(ctx, r.Client, r.Scheme, adapterWorkload{
		Owner:       adapter,
		Name:        AdapterDeploymentName(adapter.Name),
		Labels:      labels,
		SelectorKey: channelAdapterLabelKey,
		Image:       adapter.Spec.Image,
		Env:         env,
		EnvFrom:     envFrom,
		Singleton:   adapter.Spec.Singleton == nil || *adapter.Spec.Singleton,
		Resources:   adapter.Spec.Resources,
		// identity only — the operator binds no RBAC to this SA, ever
		ServiceAccount: resolveAdapterServiceAccount(adapter.Spec.ServiceAccountName, r.FloorServiceAccount),
	})
	if err != nil {
		return nil, err
	}
	if err := ensureAdapterService(ctx, r.Client, r.Scheme, adapterService{
		Owner:       adapter,
		Name:        AdapterDeploymentName(adapter.Name),
		Labels:      labels,
		SelectorKey: channelAdapterLabelKey,
		Port:        adapter.Spec.Port,
	}); err != nil {
		return nil, err
	}
	return deploy, nil
}

// patchStatus compiles the declared schema, sets the Deployed and Ready
// conditions and persists the status subresource. The schema is
// compile-checked here, where it was authored — a broken one is reported but
// never blocks the workload.
func (r *ChannelAdapterReconciler) patchStatus(ctx context.Context,
	adapter *agentopsv1alpha1.ChannelAdapter, served int, collisions []string, deploy *appsv1.Deployment) error {

	_, schemaCond := compileDeclaredSchema(adapter.Spec.ConfigSchema)
	patch := client.MergeFrom(adapter.DeepCopy())
	applySchemaCondition(&adapter.Status.Conditions, schemaCond)
	adapter.Status.ServedChannels = int32(served)

	if len(collisions) > 0 {
		log.FromContext(ctx).Info("credential projection collision", "adapter", adapter.Name, "collisions", collisions)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, buildDeployedCondition(collisions))
	apimeta.SetStatusCondition(&adapter.Status.Conditions, buildReadyCondition(deploy, served))
	return r.Status().Patch(ctx, adapter, patch)
}

// buildDeployedCondition reports the workload as rendered, downgrading only
// the message — never the status — when credential projection collided; a
// collision is reported, not blocking (first channel wins).
func buildDeployedCondition(collisions []string) metav1.Condition {
	cond := metav1.Condition{Type: ConditionDeployed, Status: metav1.ConditionTrue, Reason: "WorkloadRendered"}
	if len(collisions) > 0 {
		cond.Reason = "CredentialCollision"
		cond.Message = "credential env prefixes collide after sanitization (first channel wins): " + strings.Join(collisions, "; ")
	}
	return cond
}

// buildReadyCondition reports Ready once the Deployment has at least one
// available replica.
func buildReadyCondition(deploy *appsv1.Deployment, served int) metav1.Condition {
	cond := metav1.Condition{Type: ConditionReady, Status: metav1.ConditionFalse, Reason: "WorkloadUnavailable"}
	if deploy.Status.AvailableReplicas > 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "WorkloadAvailable"
		cond.Message = fmt.Sprintf("serving %d channel(s)", served)
	}
	return cond
}

// servedSignalAdapter finds the SignalAdapter (if any) that names this
// ChannelAdapter in spec.servedBy. Deterministic when several do — the oldest,
// name-tiebroken — because a pod gets ONE signal identity: a second would be a
// token the process has no way to choose between.
func (r *ChannelAdapterReconciler) servedSignalAdapter(ctx context.Context,
	adapter *agentopsv1alpha1.ChannelAdapter) (string, error) {

	var list agentopsv1alpha1.SignalAdapterList
	if err := r.List(ctx, &list, client.InNamespace(adapter.Namespace)); err != nil {
		return "", err
	}
	best := ""
	var bestCreated metav1.Time
	for i := range list.Items {
		sa := &list.Items[i]
		if sa.Spec.ServedBy == nil || sa.Spec.ServedBy.Name != adapter.Name {
			continue
		}
		if best == "" || sa.CreationTimestamp.Before(&bestCreated) ||
			(sa.CreationTimestamp.Equal(&bestCreated) && sa.Name < best) {
			best, bestCreated = sa.Name, sa.CreationTimestamp
		}
	}
	return best, nil
}

// SetupWithManager wires the controller: adapter CRs, their owned Deployments
// and Services, Channel events (projection inputs) mapped straight to the
// adapter the channel names in spec.type, and SignalAdapter events mapped to
// the workload their servedBy points at — without that watch, granting a second
// identity would not re-render the pod that needs the token.
func (r *ChannelAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapChannel := func(ctx context.Context, obj client.Object) []ctrl.Request {
		ch, ok := obj.(*agentopsv1alpha1.Channel)
		if !ok || ch.Spec.Adapter == "" {
			return nil
		}
		return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: ch.Namespace, Name: ch.Spec.Adapter}}}
	}
	mapSignalAdapter := func(ctx context.Context, obj client.Object) []ctrl.Request {
		sa, ok := obj.(*agentopsv1alpha1.SignalAdapter)
		if !ok || sa.Spec.ServedBy == nil || sa.Spec.ServedBy.Name == "" {
			return nil
		}
		return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: sa.Namespace, Name: sa.Spec.ServedBy.Name}}}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.ChannelAdapter{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&agentopsv1alpha1.Channel{}, handler.EnqueueRequestsFromMapFunc(mapChannel)).
		Watches(&agentopsv1alpha1.SignalAdapter{}, handler.EnqueueRequestsFromMapFunc(mapSignalAdapter)).
		Complete(r)
}
