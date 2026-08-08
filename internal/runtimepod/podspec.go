// Package runtimepod builds the per-conversation runtime pod from an
// AgentProfile: repository checkout config, compiled MCP, credentials via
// valueFrom (the manager never touches secret material).
package runtimepod

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	HomePVC      string
	NodeSelector map[string]string
	// Command/Args override the image entrypoint (e.g. a stub worker script).
	Command []string
	Args    []string
	// Env: runtime-level extra environment.
	Env []corev1.EnvVar
	// DefaultResources when the profile doesn't override.
	DefaultResources *corev1.ResourceRequirements
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
	cfg.DefaultResources = rt.Resources
	return cfg
}

// PodName for a conversation.
func PodName(convName string) string { return "agentops-conv-" + convName }

// Build renders the runtime pod (namespace + ownerRef are set by the caller).
// mcpCM names the ConfigMap holding the compiled mcp.json — the shared
// profile-keyed one, or the conversation's own when its wiring binds MCPConfigs
// (raw refs in mcp override it).
func Build(conv *agentopsv1alpha1.Conversation, profile *agentopsv1alpha1.AgentProfile,
	mcp mcpcompile.Result, mcpCM string, cfg Config) *corev1.Pod {

	env := []corev1.EnvVar{
		{Name: "ROLE", Value: "worker"},
		{Name: "CONVO_ID", Value: conv.Name},
		{Name: "CONTROL_URL", Value: cfg.ControlURL},
		{Name: "REPO_URL", Value: profile.Spec.Repository.URL},
		{Name: "REPO_REF", Value: profile.Spec.Repository.Ref},
		{Name: "RUNTIME_IDLE_TTL_M", Value: itoa(cfg.IdleTTLMinutes)},
		{Name: "HOME", Value: "/data/home"},
		{Name: "MCP_CONFIG", Value: "/etc/agentops/mcp.json"},
		{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
		}},
	}

	volumes := []corev1.Volume{
		{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	// /data/workspace: Claude sessions are keyed by cwd — this matches the
	// pre-operator claude-runner path so existing sessions resume seamlessly.
	mounts := []corev1.VolumeMount{{Name: "workspace", MountPath: "/data/workspace"}}

	// home: durable session state (RWX PVC) or ephemeral
	if cfg.HomePVC != "" {
		volumes = append(volumes, corev1.Volume{Name: "home", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: cfg.HomePVC},
		}})
	} else {
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
