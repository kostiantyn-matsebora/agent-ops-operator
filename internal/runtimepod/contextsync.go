package runtimepod

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// contextSyncPort is where the sidecar listens for the agent's work contract.
// Localhost only — it is reachable from the agent container and nothing else,
// because the two share a network namespace and no Service names it.
const contextSyncPort = 8099

// contextStoreMount is where the sidecar sees the durable volume. The agent
// container never mounts it, which is the isolation half of this design.
const contextStoreMount = "/data/context"

// contextSyncGrace is the termination grace period a pod gets in sidecar mode.
//
// The FINAL checkpoint happens on SIGTERM, and it is what covers every ordinary
// end of a pod — an idle timeout, /exit, an eviction, a close, a node drain.
// The default 30 seconds is enough for an agent that is merely exiting but not
// necessarily for a copy over a network filesystem, and a grace period that
// expires mid-checkpoint turns a clean shutdown into the lossy case.
const contextSyncGrace int64 = 120

// emptyDirWithLimit builds the pod-local live-context volume.
//
// An unparseable limit is treated as no limit rather than failing the pod:
// refusing to run an agent because a size string is malformed trades a bounded
// disk risk for a total outage, which is the wrong way round.
func emptyDirWithLimit(limit string) *corev1.EmptyDirVolumeSource {
	src := &corev1.EmptyDirVolumeSource{}
	if limit == "" {
		return src
	}
	q, err := resource.ParseQuantity(limit)
	if err != nil {
		return src
	}
	src.SizeLimit = &q
	return src
}

// contextSyncContainer builds the sidecar.
//
// A NATIVE sidecar — an init container with restartPolicy: Always — rather than
// an ordinary container. Two properties come from that and both matter: it
// starts BEFORE the agent, so the restore cannot race the first work request;
// and it TERMINATES when the agent exits, so a pod whose worker has finished
// still reaches Succeeded instead of hanging on a sidecar that never returns.
func contextSyncContainer(conv *agentopsv1alpha1.Conversation, cfg Config,
	sync *agentopsv1alpha1.ContextSync, mounts []corev1.VolumeMount) corev1.Container {

	always := corev1.ContainerRestartPolicyAlways
	interval := "0s"
	if d, on := sync.SyncInterval(); on {
		interval = d.String()
	}

	env := []corev1.EnvVar{
		{Name: "LISTEN_ADDR", Value: ":" + itoa(contextSyncPort)},
		// The REAL manager. The agent's own CONTROL_URL points at this sidecar,
		// so this is the only place the true address survives in the pod.
		{Name: "CONTROL_URL_UPSTREAM", Value: cfg.ControlURL},
		{Name: "CONVO_ID", Value: conv.Name},
		{Name: "CONTEXT_LIVE_DIR", Value: "/data/home"},
		{Name: "CONTEXT_STORE_DIR", Value: contextStoreMount},
		{Name: "CONTEXT_SYNC_PATHS", Value: strings.Join(sync.Paths, "\n")},
		{Name: "CONTEXT_SYNC_INTERVAL", Value: interval},
		{Name: "CONTEXT_SYNC_RETAIN", Value: itoa(int(retainOrDefault(sync)))},
		{Name: "CONTEXT_REPORT_URL", Value: strings.TrimSuffix(cfg.ControlURL, "/") + "/work/context"},
	}
	if len(sync.Exclude) > 0 {
		env = append(env, corev1.EnvVar{Name: "CONTEXT_SYNC_EXCLUDE", Value: strings.Join(sync.Exclude, "\n")})
	}

	return corev1.Container{
		Name:          "context-sync",
		Image:         cfg.ContextSyncImage,
		RestartPolicy: &always,
		Env:           env,
		VolumeMounts:  mounts,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
	}
}

// retainOrDefault mirrors the CRD default, so a hand-written AgentRuntime that
// omits `retain` still keeps more than one generation. Keeping exactly one
// would defeat the fallback that mid-run copies depend on.
func retainOrDefault(sync *agentopsv1alpha1.ContextSync) int32 {
	if sync.Retain > 0 {
		return sync.Retain
	}
	return 3
}

// contextSyncMounts are the sidecar's own mounts: the LIVE tree it copies from,
// and the durable store it copies to.
//
// The store is subPath'd per conversation so concurrent pods cannot read or
// overwrite one another's context — the same isolation the workspace claim has
// always had, and a prerequisite rather than a nicety: without it, two pods
// copying a shared tree back would each erase the other's writes.
func contextSyncMounts(conv *agentopsv1alpha1.Conversation) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: "home", MountPath: "/data/home"},
		{Name: "context", MountPath: contextStoreMount, SubPath: conv.Name},
	}
}
