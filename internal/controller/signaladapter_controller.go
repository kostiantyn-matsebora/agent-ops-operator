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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// SignalAdapterDeploymentName names the reconciler-owned workload for a
// SignalAdapter CR (distinct namespace from channel adapters so same-named
// CRs never collide on workload objects).
func SignalAdapterDeploymentName(adapterName string) string {
	return "agentops-signal-" + adapterName
}

// SignalAdapterReconciler owns signal-adapter workloads — the SignalAdapter
// sibling of ChannelAdapterReconciler, on the shared workload machinery:
// per-adapter derived token (signal derivation context), SOURCE_TYPE env, and
// credential projection from served SignalSources.
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

	// guard 1: credentials never live on the adapter
	if name := secretShapedEnv(adapter.Spec.Env); name != "" {
		return ctrl.Result{}, r.fail(ctx, &adapter, ConditionDeployed, "SecretEnvForbidden",
			fmt.Sprintf("spec.env[%s] references a Secret — credentials belong on SignalSource.credentialsSecretRef, not on the adapter", name))
	}

	// guard 2: one active adapter per type (oldest wins; ties by name)
	if older, err := r.olderClaimant(ctx, &adapter); err != nil {
		return ctrl.Result{}, err
	} else if older != "" {
		if err := r.deleteWorkload(ctx, &adapter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.fail(ctx, &adapter, ConditionTypeConflict, "TypeConflict",
			fmt.Sprintf("type %q is already served by SignalAdapter %q — at most one active adapter per type", adapter.Spec.Type, older))
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
		if src.Spec.Type != adapter.Spec.Type {
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
		{Name: "SOURCE_TYPE", Value: adapter.Spec.Type},
	}
	if r.MasterToken != "" {
		env = append(env, corev1.EnvVar{Name: "ADAPTER_TOKEN", Value: chat.DeriveSignalAdapterToken(r.MasterToken, adapter.Name)})
	}
	env = append(env, adapter.Spec.Env...)

	deploy, err := ensureAdapterWorkload(ctx, r.Client, r.Scheme, adapterWorkload{
		Owner: &adapter,
		Name:  SignalAdapterDeploymentName(adapter.Name),
		Labels: map[string]string{
			"app.kubernetes.io/name":      "agentops-signal-adapter",
			"agentops.dev/signal-adapter": adapter.Name,
			"agentops.dev/signal-type":    adapter.Spec.Type,
		},
		SelectorKey: "agentops.dev/signal-adapter",
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
	adapter.Status.ServedSources = int32(served)
	apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
		Type: ConditionTypeConflict, Status: metav1.ConditionFalse, Reason: "NoConflict",
	})
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
		ready.Message = fmt.Sprintf("serving %d source(s) of type %q", served, adapter.Spec.Type)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &adapter, patch)
}

func (r *SignalAdapterReconciler) olderClaimant(ctx context.Context, adapter *agentopsv1alpha1.SignalAdapter) (string, error) {
	var list agentopsv1alpha1.SignalAdapterList
	if err := r.List(ctx, &list, client.InNamespace(adapter.Namespace)); err != nil {
		return "", err
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == adapter.Name || other.Spec.Type != adapter.Spec.Type {
			continue
		}
		if other.CreationTimestamp.Time.Before(adapter.CreationTimestamp.Time) ||
			(other.CreationTimestamp.Time.Equal(adapter.CreationTimestamp.Time) && other.Name < adapter.Name) {
			return other.Name, nil
		}
	}
	return "", nil
}

func (r *SignalAdapterReconciler) deleteWorkload(ctx context.Context, adapter *agentopsv1alpha1.SignalAdapter) error {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: SignalAdapterDeploymentName(adapter.Name), Namespace: adapter.Namespace,
	}}
	if err := r.Delete(ctx, deploy); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *SignalAdapterReconciler) fail(ctx context.Context, adapter *agentopsv1alpha1.SignalAdapter, condType, reason, message string) error {
	patch := client.MergeFrom(adapter.DeepCopy())
	status := metav1.ConditionFalse
	if condType == ConditionTypeConflict {
		status = metav1.ConditionTrue // conflict PRESENT
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, Reason: reason, Message: message,
	})
	if condType != ConditionDeployed {
		apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
			Type: ConditionDeployed, Status: metav1.ConditionFalse, Reason: reason,
		})
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
		Type: ConditionReady, Status: metav1.ConditionFalse, Reason: reason,
	})
	return r.Status().Patch(ctx, adapter, patch)
}

// SetupWithManager wires the controller (mirrors ChannelAdapterReconciler).
func (r *SignalAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapSource := func(ctx context.Context, obj client.Object) []ctrl.Request {
		src, ok := obj.(*agentopsv1alpha1.SignalSource)
		if !ok {
			return nil
		}
		return r.adaptersOfType(ctx, src.Namespace, src.Spec.Type)
	}
	mapAdapter := func(ctx context.Context, obj client.Object) []ctrl.Request {
		a, ok := obj.(*agentopsv1alpha1.SignalAdapter)
		if !ok {
			return nil
		}
		return r.adaptersOfType(ctx, a.Namespace, a.Spec.Type)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.SignalAdapter{}).
		Owns(&appsv1.Deployment{}).
		Watches(&agentopsv1alpha1.SignalSource{}, handler.EnqueueRequestsFromMapFunc(mapSource)).
		Watches(&agentopsv1alpha1.SignalAdapter{}, handler.EnqueueRequestsFromMapFunc(mapAdapter)).
		Complete(r)
}

func (r *SignalAdapterReconciler) adaptersOfType(ctx context.Context, namespace, sourceType string) []ctrl.Request {
	var list agentopsv1alpha1.SignalAdapterList
	if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil
	}
	var reqs []ctrl.Request
	for i := range list.Items {
		if list.Items[i].Spec.Type == sourceType {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return reqs
}
