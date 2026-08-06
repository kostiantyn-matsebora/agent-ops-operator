package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// ChannelAdapter condition types.
const (
	// ConditionDeployed: the adapter workload is rendered and owned.
	ConditionDeployed = "Deployed"
	// ConditionReady: the adapter workload is available.
	ConditionReady = "Ready"
	// ConditionTypeConflict: another (older) adapter already serves the type.
	ConditionTypeConflict = "TypeConflict"
)

// CredentialEnvPrefix is the deterministic env-name prefix under which a
// Channel's credential Secret is projected into its adapter pod (envFrom, so
// every key of the Secret appears as <prefix><key> — the kubelet resolves
// values; nothing reads the Secret through the API). Shared with httpapi,
// which advertises it to adapters in the channel listing.
func CredentialEnvPrefix(channelName string) string {
	return "AGENTOPS_CRED_" + sanitizeEnv(channelName) + "_"
}

// sanitizeEnv maps a Kubernetes object name onto the env-var charset:
// upper-cased, every non-alphanumeric rune becomes '_'. Distinct channel names
// may collide after sanitization ("home-ops" vs "home.ops") — the reconciler
// detects that and reports it instead of silently overwriting.
func sanitizeEnv(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// AdapterDeploymentName names the reconciler-owned workload for an adapter CR.
func AdapterDeploymentName(adapterName string) string {
	return "agentops-adapter-" + adapterName
}

// ChannelAdapterReconciler owns adapter workloads: one Deployment (+ zero-RBAC
// ServiceAccount) per ChannelAdapter, with the contract URL, a derived
// per-adapter token, and every served Channel's projected credentials.
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

	// guard 1: credentials never live on the adapter
	if name := secretShapedEnv(adapter.Spec.Env); name != "" {
		return ctrl.Result{}, r.fail(ctx, &adapter, ConditionDeployed, "SecretEnvForbidden",
			fmt.Sprintf("spec.env[%s] references a Secret — credentials belong on Channel.credentialsSecretRef, not on the adapter", name))
	}

	// guard 2: one active adapter per type (oldest wins; ties by name)
	if older, err := r.olderClaimant(ctx, &adapter); err != nil {
		return ctrl.Result{}, err
	} else if older != "" {
		if err := r.deleteWorkload(ctx, &adapter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.fail(ctx, &adapter, ConditionTypeConflict, "TypeConflict",
			fmt.Sprintf("type %q is already served by ChannelAdapter %q — at most one active adapter per type", adapter.Spec.Type, older))
	}

	// served channels + credential projection (collisions reported, first wins)
	channels, collisions, err := r.servedChannels(ctx, &adapter)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureServiceAccount(ctx, &adapter); err != nil {
		return ctrl.Result{}, err
	}
	deploy, err := r.ensureDeployment(ctx, &adapter, channels)
	if err != nil {
		return ctrl.Result{}, err
	}

	// status
	patch := client.MergeFrom(adapter.DeepCopy())
	adapter.Status.ServedChannels = int32(len(channels))
	apimeta.SetStatusCondition(&adapter.Status.Conditions, metav1.Condition{
		Type: ConditionTypeConflict, Status: metav1.ConditionFalse, Reason: "NoConflict",
	})
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
		ready.Message = fmt.Sprintf("serving %d channel(s) of type %q", len(channels), adapter.Spec.Type)
	}
	apimeta.SetStatusCondition(&adapter.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &adapter, patch)
}

// secretShapedEnv returns the name of the first env entry referencing a Secret.
func secretShapedEnv(env []corev1.EnvVar) string {
	for _, e := range env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			return e.Name
		}
	}
	return ""
}

// olderClaimant returns the name of an older ChannelAdapter serving the same
// type ("" when this one is the rightful claimant).
func (r *ChannelAdapterReconciler) olderClaimant(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter) (string, error) {
	var list agentopsv1alpha1.ChannelAdapterList
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

// servedChannels lists this type's channels sorted by name and computes the
// credential projections, reporting sanitized-prefix collisions.
func (r *ChannelAdapterReconciler) servedChannels(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter) ([]agentopsv1alpha1.Channel, []string, error) {
	var list agentopsv1alpha1.ChannelList
	if err := r.List(ctx, &list, client.InNamespace(adapter.Namespace)); err != nil {
		return nil, nil, err
	}
	var served []agentopsv1alpha1.Channel
	for i := range list.Items {
		if list.Items[i].Spec.Type == adapter.Spec.Type {
			served = append(served, list.Items[i])
		}
	}
	sort.Slice(served, func(i, j int) bool { return served[i].Name < served[j].Name })
	byPrefix := map[string]string{}
	var collisions []string
	for i := range served {
		if served[i].Spec.CredentialsSecretRef == nil {
			continue
		}
		prefix := CredentialEnvPrefix(served[i].Name)
		if first, taken := byPrefix[prefix]; taken {
			collisions = append(collisions, fmt.Sprintf("%s and %s both map to %s", first, served[i].Name, prefix))
			continue
		}
		byPrefix[prefix] = served[i].Name
	}
	return served, collisions, nil
}

func (r *ChannelAdapterReconciler) ensureServiceAccount(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter) error {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: AdapterDeploymentName(adapter.Name), Namespace: adapter.Namespace,
	}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		// zero ambient authority: no RBAC is ever bound to this SA, and the
		// pod spec disables token automount anyway
		return controllerutil.SetControllerReference(adapter, sa, r.Scheme)
	})
	return err
}

func (r *ChannelAdapterReconciler) ensureDeployment(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter, channels []agentopsv1alpha1.Channel) (*appsv1.Deployment, error) {
	name := AdapterDeploymentName(adapter.Name)
	labels := map[string]string{
		"app.kubernetes.io/name":    "agentops-adapter",
		"agentops.dev/adapter":      adapter.Name,
		"agentops.dev/channel-type": adapter.Spec.Type,
	}

	env := []corev1.EnvVar{
		{Name: "MANAGER_URL", Value: r.ManagerURL},
		{Name: "CHANNEL_TYPE", Value: adapter.Spec.Type},
	}
	if r.MasterToken != "" {
		env = append(env, corev1.EnvVar{Name: "ADAPTER_TOKEN", Value: chat.DeriveAdapterToken(r.MasterToken, adapter.Name)})
	}
	env = append(env, adapter.Spec.Env...)

	// credential projection: envFrom with a deterministic prefix per channel —
	// the kubelet injects every key of the Secret as <prefix><key>; the manager
	// advertises the prefix through the contract's channel listing. First
	// channel wins a sanitized-prefix collision (reported in status).
	var envFrom []corev1.EnvFromSource
	seen := map[string]bool{}
	for i := range channels {
		if channels[i].Spec.CredentialsSecretRef == nil {
			continue
		}
		prefix := CredentialEnvPrefix(channels[i].Name)
		if seen[prefix] {
			continue
		}
		seen[prefix] = true
		envFrom = append(envFrom, corev1.EnvFromSource{
			Prefix:    prefix,
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: *channels[i].Spec.CredentialsSecretRef},
		})
	}

	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: adapter.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		replicas := int32(1)
		deploy.Spec.Replicas = &replicas
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"agentops.dev/adapter": adapter.Name}}
		if adapter.Spec.Singleton == nil || *adapter.Spec.Singleton {
			// pull-based transports: never two instances side by side
			deploy.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
		} else {
			deploy.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType}
		}
		automount := false
		container := corev1.Container{
			Name: "adapter", Image: adapter.Spec.Image, Env: env, EnvFrom: envFrom,
		}
		if adapter.Spec.Resources != nil {
			container.Resources = *adapter.Spec.Resources
		}
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				ServiceAccountName:           name,
				AutomountServiceAccountToken: &automount,
				Containers:                   []corev1.Container{container},
			},
		}
		return controllerutil.SetControllerReference(adapter, deploy, r.Scheme)
	})
	return deploy, err
}

func (r *ChannelAdapterReconciler) deleteWorkload(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter) error {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: AdapterDeploymentName(adapter.Name), Namespace: adapter.Namespace,
	}}
	if err := r.Delete(ctx, deploy); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// fail records a False terminal condition (and flips Deployed off for
// non-Deployed failures' sake, keeping status truthful).
func (r *ChannelAdapterReconciler) fail(ctx context.Context, adapter *agentopsv1alpha1.ChannelAdapter, condType, reason, message string) error {
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

// SetupWithManager wires the controller: adapter CRs, their owned Deployments,
// and Channel events (projection inputs) mapped to the serving adapter; other
// same-type adapters requeue on any adapter event so conflict resolution
// converges when the older claimant goes away.
func (r *ChannelAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapChannel := func(ctx context.Context, obj client.Object) []ctrl.Request {
		ch, ok := obj.(*agentopsv1alpha1.Channel)
		if !ok {
			return nil
		}
		return r.adaptersOfType(ctx, ch.Namespace, ch.Spec.Type)
	}
	mapAdapter := func(ctx context.Context, obj client.Object) []ctrl.Request {
		a, ok := obj.(*agentopsv1alpha1.ChannelAdapter)
		if !ok {
			return nil
		}
		return r.adaptersOfType(ctx, a.Namespace, a.Spec.Type)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.ChannelAdapter{}).
		Owns(&appsv1.Deployment{}).
		Watches(&agentopsv1alpha1.Channel{}, handler.EnqueueRequestsFromMapFunc(mapChannel)).
		Watches(&agentopsv1alpha1.ChannelAdapter{}, handler.EnqueueRequestsFromMapFunc(mapAdapter)).
		Complete(r)
}

func (r *ChannelAdapterReconciler) adaptersOfType(ctx context.Context, namespace, channelType string) []ctrl.Request {
	var list agentopsv1alpha1.ChannelAdapterList
	if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil
	}
	var reqs []ctrl.Request
	for i := range list.Items {
		if list.Items[i].Spec.Type == channelType {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return reqs
}
