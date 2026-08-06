package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Adapter workload machinery shared by the ChannelAdapter and SignalAdapter
// reconcilers: both deploy an out-of-process contract consumer with the same
// security posture (dedicated zero-RBAC SA, no SA token automount, derived
// contract token, kubelet-resolved envFrom credential projection, singleton
// discipline).

// Adapter condition types (shared by both adapter kinds).
const (
	// ConditionDeployed: the adapter workload is rendered and owned.
	ConditionDeployed = "Deployed"
	// ConditionReady: the adapter workload is available.
	ConditionReady = "Ready"
	// ConditionTypeConflict: another (older) adapter already serves the type.
	ConditionTypeConflict = "TypeConflict"
)

// CredentialEnvPrefix is the deterministic env-name prefix under which a
// Channel's or SignalSource's credential Secret is projected into its adapter
// pod (envFrom, so every key of the Secret appears as <prefix><key> — the
// kubelet resolves values; nothing reads the Secret through the API). Shared
// with httpapi, which advertises it to adapters in the channel/source listing.
func CredentialEnvPrefix(name string) string {
	return "AGENTOPS_CRED_" + sanitizeEnv(name) + "_"
}

// sanitizeEnv maps a Kubernetes object name onto the env-var charset:
// upper-cased, every non-alphanumeric rune becomes '_'. Distinct names may
// collide after sanitization ("home-ops" vs "home.ops") — the reconcilers
// detect that and report it instead of silently overwriting.
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

// credentialItem is one credential-bearing CR (a Channel or SignalSource)
// served by an adapter.
type credentialItem struct {
	Name       string
	SecretName string
}

// projectCredentials computes the envFrom projection for the served CRs
// (sorted by name; first wins a sanitized-prefix collision) and returns the
// collision report.
func projectCredentials(items []credentialItem) ([]corev1.EnvFromSource, []string) {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	byPrefix := map[string]string{}
	var envFrom []corev1.EnvFromSource
	var collisions []string
	for _, item := range items {
		prefix := CredentialEnvPrefix(item.Name)
		if first, taken := byPrefix[prefix]; taken {
			collisions = append(collisions, fmt.Sprintf("%s and %s both map to %s", first, item.Name, prefix))
			continue
		}
		byPrefix[prefix] = item.Name
		envFrom = append(envFrom, corev1.EnvFromSource{
			Prefix:    prefix,
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: item.SecretName}},
		})
	}
	return envFrom, collisions
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

// adapterWorkload describes one adapter Deployment to render.
type adapterWorkload struct {
	Owner       client.Object // the adapter CR (ownerRef → GC)
	Name        string        // Deployment + ServiceAccount name
	Labels      map[string]string
	SelectorKey string // pod-selector label key (value = adapter CR name)
	Image       string
	Env         []corev1.EnvVar // fully assembled (contract env + adapter spec env)
	EnvFrom     []corev1.EnvFromSource
	Singleton   bool
	Resources   *corev1.ResourceRequirements
}

// ensureAdapterWorkload creates/updates the dedicated zero-RBAC SA and the
// Deployment for an adapter.
func ensureAdapterWorkload(ctx context.Context, c client.Client, scheme *runtime.Scheme, w adapterWorkload) (*appsv1.Deployment, error) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: w.Name, Namespace: w.Owner.GetNamespace(),
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, sa, func() error {
		// zero ambient authority: no RBAC is ever bound to this SA, and the
		// pod spec disables token automount anyway
		return controllerutil.SetControllerReference(w.Owner, sa, scheme)
	}); err != nil {
		return nil, err
	}

	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Owner.GetNamespace()}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, deploy, func() error {
		deploy.Labels = w.Labels
		replicas := int32(1)
		deploy.Spec.Replicas = &replicas
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{w.SelectorKey: w.Owner.GetName()}}
		if w.Singleton {
			// pull-based transports / schedulers: never two instances side by side
			deploy.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
		} else {
			deploy.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType}
		}
		automount := false
		container := corev1.Container{
			Name: "adapter", Image: w.Image, Env: w.Env, EnvFrom: w.EnvFrom,
		}
		if w.Resources != nil {
			container.Resources = *w.Resources
		}
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: w.Labels},
			Spec: corev1.PodSpec{
				ServiceAccountName:           w.Name,
				AutomountServiceAccountToken: &automount,
				Containers:                   []corev1.Container{container},
			},
		}
		return controllerutil.SetControllerReference(w.Owner, deploy, scheme)
	})
	return deploy, err
}
