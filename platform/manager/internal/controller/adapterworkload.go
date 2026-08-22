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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/configschema"
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
	// ReasonServedBy marks an adapter that owns no workload because another
	// adapter's pod serves its identity (SignalAdapter.spec.servedBy). Ready is
	// TRUE with this reason: there is nothing to become available, and reporting
	// "unavailable" for a deliberately workload-less adapter would read as a
	// fault on every dashboard.
	ReasonServedBy = "ServedBy"
	// ConditionSchemaValid: the adapter CR's declared spec.configSchema
	// compiles. Absent when nothing is declared; False never blocks the
	// workload, it only disables downstream config validation for the type.
	ConditionSchemaValid = "SchemaValid"
	// ReasonInvalidSchema: the declared configSchema does not compile.
	ReasonInvalidSchema = "InvalidSchema"

	// ConditionConfigValid reports a served Channel/SignalSource's spec.config
	// against the schema its adapter declares. ADVISORY: absent when nothing is
	// declared, and False never affects Served, projection, or ingestion — the
	// adapter's own Ready report stays authoritative.
	ConditionConfigValid = "ConfigValid"
	// ReasonSchemaValidated: config conforms to the declared schema.
	ReasonSchemaValidated = "SchemaValidated"
	// ReasonSchemaViolation: config violates the declared schema.
	ReasonSchemaViolation = "SchemaViolation"
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

// adapterWorkload describes one adapter Deployment to render.
type adapterWorkload struct {
	Owner       client.Object // the adapter CR (ownerRef → GC)
	Name        string        // Deployment + ServiceAccount name
	Labels      map[string]string
	SelectorKey string // pod-selector label key (value = adapter CR name)
	Image       string
	Env         []corev1.EnvVar // fully assembled contract env
	EnvFrom     []corev1.EnvFromSource
	Singleton   bool
	Resources   *corev1.ResourceRequirements
	// KubernetesAccess mounts the SA token and injects POD_NAMESPACE. The SA
	// still carries zero operator-created RBAC — permissions are granted
	// externally against the deterministic SA name.
	KubernetesAccess bool
}

// ensureAdapterWorkload creates/updates the dedicated zero-RBAC SA and the
// Deployment for an adapter.
func ensureAdapterWorkload(ctx context.Context, c client.Client, scheme *runtime.Scheme, w adapterWorkload) (*appsv1.Deployment, error) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: w.Name, Namespace: w.Owner.GetNamespace(),
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, sa, func() error {
		// zero ambient authority: the operator never binds RBAC to this SA;
		// token automount stays off unless the implementation declares
		// kubernetesAccess (and even then, permissions come from outside)
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
		automount := w.KubernetesAccess
		env := w.Env
		if w.KubernetesAccess {
			env = append(append([]corev1.EnvVar{}, env...), corev1.EnvVar{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
				},
			})
		}
		container := corev1.Container{
			Name: "adapter", Image: w.Image, Env: env, EnvFrom: w.EnvFrom,
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

// adapterService describes the inbound Service for an adapter that declares
// spec.port. Both adapter kinds render it identically — the Service shares the
// Deployment's name and selector, so a ported adapter is reachable in-cluster
// at <workload-name>:<port> with nothing chart-side.
type adapterService struct {
	Owner       client.Object // the adapter CR (ownerRef → GC + ownership check)
	Name        string        // same as the Deployment name
	Labels      map[string]string
	SelectorKey string
	Port        *int32 // nil = no inbound surface
}

// ensureAdapterService renders the adapter's inbound Service when spec.port is
// declared, and removes a previously reconciler-owned one when it is not.
//
// Removal is ownership-gated: a Service this reconciler does not control (a
// hand-made one under the same name) is left alone rather than deleted.
func ensureAdapterService(ctx context.Context, c client.Client, scheme *runtime.Scheme, s adapterService) error {
	if s.Port == nil {
		var svc corev1.Service
		err := c.Get(ctx, types.NamespacedName{Namespace: s.Owner.GetNamespace(), Name: s.Name}, &svc)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if metav1.IsControlledBy(&svc, s.Owner) {
			return client.IgnoreNotFound(c.Delete(ctx, &svc))
		}
		return nil
	}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Owner.GetNamespace()}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
		svc.Labels = s.Labels
		svc.Spec.Selector = map[string]string{s.SelectorKey: s.Owner.GetName()}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "http",
			Port:       *s.Port,
			TargetPort: intstr.FromInt32(*s.Port),
		}}
		return controllerutil.SetControllerReference(s.Owner, svc, scheme)
	})
	return err
}

// compileDeclaredSchema compiles an adapter CR's optional spec.configSchema
// and returns the SchemaValid condition to record, or nil when nothing is
// declared (absence means "nothing to check").
//
// A schema that does not compile is reported where it was authored and MUST
// NOT affect the workload — it only disables downstream config validation.
func compileDeclaredSchema(raw *runtime.RawExtension) (*configschema.Schema, *metav1.Condition) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}
	schema, err := configschema.Compile(raw.Raw)
	if err != nil {
		return nil, &metav1.Condition{
			Type: ConditionSchemaValid, Status: metav1.ConditionFalse, Reason: ReasonInvalidSchema,
			Message: truncate("configSchema does not compile: "+err.Error(), 1024),
		}
	}
	return schema, &metav1.Condition{
		Type: ConditionSchemaValid, Status: metav1.ConditionTrue, Reason: "SchemaCompiled",
		Message: "spec.configSchema compiles; served CRs are validated against it",
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// applySchemaCondition records (or clears) SchemaValid on an adapter CR's
// condition list.
func applySchemaCondition(conds *[]metav1.Condition, cond *metav1.Condition) {
	if cond == nil {
		apimeta.RemoveStatusCondition(conds, ConditionSchemaValid)
		return
	}
	apimeta.SetStatusCondition(conds, *cond)
}

// validateAgainstAdapter renders the advisory ConfigValid condition for one
// served CR, or nil when there is nothing to check (no adapter CR, no declared
// schema, or a schema that does not compile — absence means "no contract",
// never "unknown").
//
// Advisory by construction: callers record the condition and change nothing
// else, so a violation can never stop a channel being served or a signal being
// ingested. The adapter's own Ready report stays authoritative, because a
// CR-declared schema may drift from the running image.
func validateAgainstAdapter(declared *runtime.RawExtension, config *runtime.RawExtension, adapterName string) *metav1.Condition {
	schema, cond := compileDeclaredSchema(declared)
	if schema == nil || cond == nil || cond.Status != metav1.ConditionTrue {
		return nil
	}
	var raw []byte
	if config != nil {
		raw = config.Raw
	}
	violations := configschema.Validate(schema, raw)
	if len(violations) == 0 {
		return &metav1.Condition{
			Type: ConditionConfigValid, Status: metav1.ConditionTrue, Reason: ReasonSchemaValidated,
			Message: fmt.Sprintf("spec.config conforms to the schema declared by %q", adapterName),
		}
	}
	return &metav1.Condition{
		Type: ConditionConfigValid, Status: metav1.ConditionFalse, Reason: ReasonSchemaViolation,
		Message: truncate(fmt.Sprintf("spec.config violates the schema declared by %q: %s",
			adapterName, configschema.Summarize(violations, 0)), 1024),
	}
}

// applyConfigValid records or clears ConfigValid on a served CR.
func applyConfigValid(conds *[]metav1.Condition, cond *metav1.Condition) {
	if cond == nil {
		apimeta.RemoveStatusCondition(conds, ConditionConfigValid)
		return
	}
	apimeta.SetStatusCondition(conds, *cond)
}
