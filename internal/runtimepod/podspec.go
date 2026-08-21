// Package runtimepod builds the per-conversation runtime pod from an
// AgentProfile: repository checkout config, compiled MCP, credentials via
// valueFrom (the manager never touches secret material).
package runtimepod

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
)

// LabelApp marks runtime pods; LabelConversation binds one to its conversation.
const (
	LabelApp          = "app.kubernetes.io/name"
	LabelAppValue     = "agentops-runtime"
	LabelConversation = "agentops.dev/conversation"
)

// Config is the resolved worker configuration (from a AgentRuntime CR merged
// over the manager's bootstrap defaults).
type Config struct {
	Image          string
	ServiceAccount string
	ControlURL     string
	IdleTTLMinutes int
	// HomePVC (RWX) for durable agent session state; emptyDir when empty.
	HomePVC string
	// WorkspacePVC (RWX) backs the repository checkout, one subdirectory per
	// conversation; emptyDir when empty. Separate from HomePVC because the two
	// are enabled independently — sessions by default, checkouts on request.
	WorkspacePVC string
	NodeSelector map[string]string
	// Command/Args override the image entrypoint (e.g. a stub worker script).
	Command []string
	Args    []string
	// Env: runtime-level extra environment.
	Env []corev1.EnvVar
	// DefaultResources when the profile doesn't override.
	DefaultResources *corev1.ResourceRequirements
	// ContextSyncImage is the sidecar that keeps context durable when a runtime
	// declares contextSync. Release-wide like the manager's own image, not a
	// per-runtime choice: it implements a contract, not a backend.
	ContextSyncImage string
	// ContextLiveSizeLimit bounds the pod-local live context.
	//
	// In sidecar mode the WHOLE home is ephemeral — caches and tool state
	// included, which is the point — so this has to be generous enough for
	// them. Unbounded would let one runaway agent fill a node's disk; too small
	// evicts the pod mid-run. Empty leaves it unbounded, which is Kubernetes'
	// own default and the honest choice when an operator has not decided.
	ContextLiveSizeLimit string
}

// FromRuntime overlays a AgentRuntime spec on the bootstrap config.
func FromRuntime(rt *agentopsv1alpha1.AgentRuntimeSpec, fallback Config) Config {
	if rt == nil {
		return fallback
	}
	cfg := fallback
	cfg.Image = rt.Image
	cfg.Command = rt.Command
	cfg.Args = rt.Args
	cfg.Env = rt.Env
	if rt.ServiceAccountName != "" {
		cfg.ServiceAccount = rt.ServiceAccountName
	}
	if len(rt.NodeSelector) > 0 {
		cfg.NodeSelector = rt.NodeSelector
	}
	if rt.IdleTTLMinutes > 0 {
		cfg.IdleTTLMinutes = int(rt.IdleTTLMinutes)
	}
	cfg.HomePVC = ""
	if rt.Home != nil && rt.Home.PVCRef != nil {
		cfg.HomePVC = rt.Home.PVCRef.Name
	}
	cfg.WorkspacePVC = ""
	if rt.Workspace != nil && rt.Workspace.PVCRef != nil {
		cfg.WorkspacePVC = rt.Workspace.PVCRef.Name
	}
	cfg.DefaultResources = rt.Resources
	return cfg
}

// PodName for a conversation.
func PodName(convName string) string { return "agentops-conv-" + convName }

// Resolved is the execution backend chosen for a profile: the pod config, plus
// the facts about it that callers other than the pod builder need.
type Resolved struct {
	Config Config
	// ContextStorage declares where the runtime keeps conversation context.
	// Empty when no AgentRuntime CR was found, i.e. the bootstrap fallback.
	ContextStorage agentopsv1alpha1.ContextStorage
	// ContextSync is the runtime's declaration of what to keep durable when the
	// live context lives on pod-local storage. Nil means today's behaviour: the
	// home volume mounted directly, no sidecar.
	ContextSync *agentopsv1alpha1.ContextSync
}

// ResolveFor picks the execution backend for a profile — its `runtimeRef`, else
// the CR named `default`, else the manager's bootstrap config — and reports
// whether a named runtime was missing.
//
// ONE resolution rule, in one place: the reconciler builds pods from it and the
// dispatch path asks it whether continuity is possible here, and those two
// answering differently would mean a conversation promised continuity by one
// component and denied it by the other.
func ResolveFor(ctx context.Context, r client.Reader, namespace string,
	profile *agentopsv1alpha1.AgentProfile, fallback Config) (Resolved, error) {

	name, explicit := "default", false
	if profile != nil && profile.Spec.RuntimeRef != nil {
		name, explicit = profile.Spec.RuntimeRef.Name, true
	}
	var rt agentopsv1alpha1.AgentRuntime
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &rt); err != nil {
		if explicit {
			return Resolved{}, err // a named runtime must exist
		}
		return Resolved{Config: fallback}, nil
	}
	// A bad contextSync declaration fails HERE, where the error reaches the
	// Conversation's RuntimeStarted condition. Nothing writes AgentRuntime
	// status, so reporting it on that CR's readiness would mean inventing a
	// reconciler to hold one condition; the CRD schema already rejects the
	// structural errors at admission, and this catches what a schema cannot
	// express — a path that escapes the runtime's home.
	if err := rt.Spec.ContextSync.Validate(); err != nil {
		return Resolved{}, fmt.Errorf("runtime %q: %w", name, err)
	}
	return Resolved{
		Config:         FromRuntime(&rt.Spec, fallback),
		ContextStorage: rt.Spec.ContextStorage,
		ContextSync:    rt.Spec.ContextSync,
	}, nil
}

// ContinuityPossible reports whether a conversation on this backend can carry
// context between runs at all.
//
// A runtime keeping context on its home volume, in a deployment that provides
// none, can never continue anything: the pod exits on its idle timeout — a
// minute by default — and takes the context with it. Saying so BEFORE promising
// continuity is what separates a configuration the operator chose from a loss,
// and stops every follow-up failing for the former.
func (r Resolved) ContinuityPossible() bool {
	switch r.ContextStorage {
	case agentopsv1alpha1.ContextNone:
		return false
	case agentopsv1alpha1.ContextExternal:
		return true // stored somewhere the operator does not provide
	default:
		// `volume`, and the empty bootstrap case which is the reference runtime
		return r.Config.HomePVC != ""
	}
}

// Build renders the runtime pod (namespace + ownerRef are set by the caller).
// mcpCM names the ConfigMap holding the compiled mcp.json — the shared
// profile-keyed one, or the conversation's own when its wiring binds MCPConfigs
// (raw refs in mcp override it).
func Build(conv *agentopsv1alpha1.Conversation, profile *agentopsv1alpha1.AgentProfile,
	mcp mcpcompile.Result, mcpCM string, resolved Resolved) *corev1.Pod {

	cfg := resolved.Config
	// SIDECAR MODE moves the live context onto pod-local storage and leaves the
	// durable volume to a separate container. It is opt-in per runtime: a
	// runtime that declares nothing gets exactly the pod it got before, which
	// is what lets an existing install upgrade without a migration.
	sync := resolved.ContextSync
	sidecar := sync != nil && cfg.ContextSyncImage != ""

	// The agent talks to the SIDECAR, which forwards to the manager. That
	// redirection is the whole mechanism: it is how work boundaries become
	// observable without the runtime image knowing anything about it.
	agentControlURL := cfg.ControlURL
	if sidecar {
		agentControlURL = "http://127.0.0.1:" + itoa(contextSyncPort)
	}

	env := []corev1.EnvVar{
		{Name: "ROLE", Value: "worker"},
		{Name: "CONVO_ID", Value: conv.Name},
		{Name: "CONTROL_URL", Value: agentControlURL},
		{Name: "REPO_URL", Value: profile.Spec.Repository.URL},
		{Name: "REPO_REF", Value: profile.Spec.Repository.Ref},
		{Name: "RUNTIME_IDLE_TTL_M", Value: itoa(cfg.IdleTTLMinutes)},
		{Name: "HOME", Value: "/data/home"},
		{Name: "MCP_CONFIG", Value: "/etc/agentops/mcp.json"},
		{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
		}},
	}

	// workspace: the repository checkout, on a claim or ephemeral.
	//
	// /data/workspace: Claude sessions are keyed by cwd — this matches the
	// pre-operator claude-runner path so existing sessions resume seamlessly, and
	// it is why the claim is mounted with a subPath rather than at a per-
	// conversation path. The subPath is the conversation name, so concurrent
	// runtime pods sharing one claim cannot observe each other's tree.
	var volumes []corev1.Volume
	workspaceMount := corev1.VolumeMount{Name: "workspace", MountPath: "/data/workspace"}
	if cfg.WorkspacePVC != "" {
		volumes = append(volumes, corev1.Volume{Name: "workspace", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: cfg.WorkspacePVC},
		}})
		workspaceMount.SubPath = conv.Name
	} else {
		volumes = append(volumes, corev1.Volume{Name: "workspace",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}
	mounts := []corev1.VolumeMount{workspaceMount}

	// home: durable session state (RWX PVC), pod-local under contextSync, or
	// ephemeral when no claim is configured at all.
	switch {
	case sidecar:
		// The agent gets EPHEMERAL storage and no access to the durable volume
		// whatsoever. Two things follow, and both are wanted: a corrupt volume
		// can no longer stop a run that is already going, and an agent can no
		// longer read another conversation's context or write to the volume,
		// because it holds no mount of it.
		volumes = append(volumes, corev1.Volume{Name: "home",
			VolumeSource: corev1.VolumeSource{EmptyDir: emptyDirWithLimit(cfg.ContextLiveSizeLimit)}})
		// The durable claim, per conversation. The subPath applies ONLY here:
		// changing the layout for every install would be a migration, and this
		// way an install that has not opted in keeps its context exactly where
		// it already is.
		volumes = append(volumes, corev1.Volume{Name: "context", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: cfg.HomePVC},
		}})
	case cfg.HomePVC != "":
		volumes = append(volumes, corev1.Volume{Name: "home", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: cfg.HomePVC},
		}})
	default:
		volumes = append(volumes, corev1.Volume{Name: "home", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}
	mounts = append(mounts, corev1.VolumeMount{Name: "home", MountPath: "/data/home"})

	// repository auth
	if a := profile.Spec.Repository.Auth; a != nil {
		switch a.Type {
		case agentopsv1alpha1.RepoAuthSSH:
			mode := int32(0o400)
			volumes = append(volumes, corev1.Volume{Name: "repo-auth", VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: a.SecretRef.Name, DefaultMode: &mode},
			}})
			mounts = append(mounts, corev1.VolumeMount{Name: "repo-auth", MountPath: "/repo-auth", ReadOnly: true})
			env = append(env, corev1.EnvVar{Name: "GIT_AUTH_TYPE", Value: "ssh"},
				corev1.EnvVar{Name: "GIT_SSH_KEY", Value: "/repo-auth/sshKey"})
		case agentopsv1alpha1.RepoAuthHTTPS:
			env = append(env,
				corev1.EnvVar{Name: "GIT_AUTH_TYPE", Value: "https"},
				corev1.EnvVar{Name: "GIT_TOKEN", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: a.SecretRef.Name}, Key: "token",
					},
				}})
		}
	}

	// MCP: rendered ConfigMap (profile- or conversation-keyed) or raw ref
	switch {
	case mcp.RawConfigMap != "":
		mcpCM = mcp.RawConfigMap
	case mcp.RawSecret != "":
		volumes = append(volumes, corev1.Volume{Name: "mcp", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: mcp.RawSecret},
		}})
	}
	if mcp.RawSecret == "" {
		volumes = append(volumes, corev1.Volume{Name: "mcp", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: mcpCM}},
		}})
	}
	mounts = append(mounts, corev1.VolumeMount{Name: "mcp", MountPath: "/etc/agentops", ReadOnly: true})

	env = append(env, cfg.Env...)
	env = append(env, mcp.Env...)
	env = append(env, profile.Spec.Env...)

	res := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1536Mi")},
	}
	if cfg.DefaultResources != nil {
		res = *cfg.DefaultResources
	}
	if profile.Spec.Resources != nil {
		res = *profile.Spec.Resources
	}

	uid := int64(1000)
	var initContainers []corev1.Container
	var gracePeriod *int64
	if sidecar {
		initContainers = append(initContainers,
			contextSyncContainer(conv, cfg, sync, contextSyncMounts(conv)))
		g := contextSyncGrace
		gracePeriod = &g
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: PodName(conv.Name),
			Labels: map[string]string{
				LabelApp:          LabelAppValue,
				LabelConversation: conv.Name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: cfg.ServiceAccount,
			RestartPolicy:      corev1.RestartPolicyNever,
			NodeSelector:       cfg.NodeSelector,
			InitContainers:     initContainers,
			// A longer grace ONLY in sidecar mode: the final checkpoint runs on
			// SIGTERM, and a grace period that expires mid-copy turns every
			// clean shutdown into the lossy case.
			TerminationGracePeriodSeconds: gracePeriod,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid,
			},
			Containers: []corev1.Container{{
				Name:         "worker",
				Image:        cfg.Image,
				Command:      cfg.Command,
				Args:         cfg.Args,
				Env:          env,
				Resources:    res,
				VolumeMounts: mounts,
			}},
			Volumes: volumes,
		},
	}
}

func itoa(i int) string {
	if i <= 0 {
		return "30"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
