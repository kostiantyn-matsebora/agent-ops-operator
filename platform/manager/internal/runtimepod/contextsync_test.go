package runtimepod

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func syncCfg() Config {
	return Config{
		Image: "runtime:1", ServiceAccount: "sa", ControlURL: "http://manager:8080",
		HomePVC: "agentops-home", ContextSyncImage: "context-sync:1",
		ContextLiveSizeLimit: "4Gi",
	}
}

func syncSpec() *agentopsv1alpha1.ContextSync {
	return &agentopsv1alpha1.ContextSync{
		Paths:    []string{".claude/projects/-data-workspace/**"},
		Exclude:  []string{"**/*.lock"},
		Interval: &metav1.Duration{Duration: 2 * time.Minute},
		Retain:   3,
	}
}

func container(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

func envOf(c *corev1.Container, name string) string {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// WITHOUT contextSync the pod must be byte-for-byte the pod it was before. This
// is what lets an existing install upgrade with no migration at all.
func TestNoContextSyncBuildsTodaysPod(t *testing.T) {
	pod := build("c1", syncCfg()) // Resolved.ContextSync nil

	if len(pod.Spec.InitContainers) != 0 {
		t.Fatal("no sidecar may be added when the runtime declares no contextSync")
	}
	if v := volume(pod, "home"); v == nil || v.PersistentVolumeClaim == nil {
		t.Fatal("home must still be the durable claim, mounted directly")
	}
	if volume(pod, "context") != nil {
		t.Fatal("no separate context volume in the unconfigured shape")
	}
	w := container(pod, "worker")
	if got := envOf(w, "CONTROL_URL"); got != "http://manager:8080" {
		t.Fatalf("CONTROL_URL = %q, want the manager directly", got)
	}
	if pod.Spec.TerminationGracePeriodSeconds != nil {
		t.Fatal("the grace period must be left at the cluster default without a sidecar")
	}
}

// WITH contextSync: the agent gets ephemeral storage and no reach into the
// durable volume, and the sidecar gets the volume and nothing else.
func TestContextSyncIsolatesTheAgentFromTheVolume(t *testing.T) {
	pod := buildResolved("c1", Resolved{Config: syncCfg(), ContextSync: syncSpec()})

	home := volume(pod, "home")
	if home == nil || home.EmptyDir == nil {
		t.Fatal("the agent's home must be pod-local in sidecar mode")
	}
	if home.EmptyDir.SizeLimit == nil || home.EmptyDir.SizeLimit.String() != "4Gi" {
		t.Fatalf("the live context must be bounded; got %v", home.EmptyDir.SizeLimit)
	}
	ctxVol := volume(pod, "context")
	if ctxVol == nil || ctxVol.PersistentVolumeClaim == nil {
		t.Fatal("the durable claim must be present as its own volume")
	}

	// THE isolation property: the agent container must not mount the claim.
	w := container(pod, "worker")
	for _, m := range w.VolumeMounts {
		if m.Name == "context" {
			t.Fatal("the agent container must NOT mount the durable volume")
		}
	}

	sc := container(pod, "context-sync")
	if sc == nil {
		t.Fatal("no sidecar container was built")
	}
	var store *corev1.VolumeMount
	for i := range sc.VolumeMounts {
		if sc.VolumeMounts[i].Name == "context" {
			store = &sc.VolumeMounts[i]
		}
	}
	if store == nil {
		t.Fatal("the sidecar must mount the durable volume")
	}
	// Per conversation, or two concurrent pods erase each other's context.
	if store.SubPath != "c1" {
		t.Fatalf("store subPath = %q, want the conversation name", store.SubPath)
	}
}

// The redirection IS the mechanism: the agent talks to the sidecar, which
// forwards to the manager, and that is how work boundaries become observable
// without the runtime image knowing anything.
func TestContextSyncRedirectsTheWorkContract(t *testing.T) {
	pod := buildResolved("c1", Resolved{Config: syncCfg(), ContextSync: syncSpec()})

	w := container(pod, "worker")
	if got := envOf(w, "CONTROL_URL"); !strings.HasPrefix(got, "http://127.0.0.1:") {
		t.Fatalf("agent CONTROL_URL = %q, want the local sidecar", got)
	}
	sc := container(pod, "context-sync")
	if got := envOf(sc, "CONTROL_URL_UPSTREAM"); got != "http://manager:8080" {
		t.Fatalf("sidecar upstream = %q, want the real manager", got)
	}
	if got := envOf(sc, "CONTEXT_SYNC_PATHS"); got != ".claude/projects/-data-workspace/**" {
		t.Fatalf("paths = %q", got)
	}
	if got := envOf(sc, "CONTEXT_SYNC_EXCLUDE"); got != "**/*.lock" {
		t.Fatalf("exclude = %q", got)
	}
	if got := envOf(sc, "CONTEXT_SYNC_INTERVAL"); got != "2m0s" {
		t.Fatalf("interval = %q", got)
	}
	if got := envOf(sc, "CONVO_ID"); got != "c1" {
		t.Fatalf("convo = %q", got)
	}
}

// A NATIVE sidecar: it starts before the agent, so restore cannot race the
// first work request, and it terminates with the agent, so a finished pod still
// reaches Succeeded instead of hanging.
func TestSidecarIsANativeSidecar(t *testing.T) {
	pod := buildResolved("c1", Resolved{Config: syncCfg(), ContextSync: syncSpec()})
	sc := container(pod, "context-sync")
	if sc.RestartPolicy == nil || *sc.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Fatal("the sidecar must be an init container with restartPolicy Always")
	}
	found := false
	for _, c := range pod.Spec.InitContainers {
		if c.Name == "context-sync" {
			found = true
		}
	}
	if !found {
		t.Fatal("the sidecar must be an INIT container, or it cannot start before the agent")
	}
}

// The final checkpoint runs on SIGTERM; a grace period that expires mid-copy
// turns every clean shutdown into the lossy case.
func TestSidecarGetsEnoughGraceForAFinalCheckpoint(t *testing.T) {
	pod := buildResolved("c1", Resolved{Config: syncCfg(), ContextSync: syncSpec()})
	if pod.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("sidecar mode must set a grace period")
	}
	if *pod.Spec.TerminationGracePeriodSeconds < 60 {
		t.Fatalf("grace = %ds, too short for a copy over a network filesystem",
			*pod.Spec.TerminationGracePeriodSeconds)
	}
}

// Declaring contextSync without an image configured must not half-apply the
// mode — that would give the agent an EMPTY ephemeral home and no sidecar to
// restore into it, silently losing every conversation's context.
func TestContextSyncWithoutAnImageFallsBackSafely(t *testing.T) {
	cfg := syncCfg()
	cfg.ContextSyncImage = ""
	pod := buildResolved("c1", Resolved{Config: cfg, ContextSync: syncSpec()})

	if len(pod.Spec.InitContainers) != 0 {
		t.Fatal("no image means no sidecar")
	}
	home := volume(pod, "home")
	if home == nil || home.PersistentVolumeClaim == nil {
		t.Fatal("without a sidecar the home claim must stay mounted directly, or context is lost")
	}
}

func TestEmptyDirLimitIsForgiving(t *testing.T) {
	if v := emptyDirWithLimit(""); v.SizeLimit != nil {
		t.Fatal("no limit configured means unbounded")
	}
	// A malformed limit must not fail the pod: a bounded disk risk beats a
	// total outage.
	if v := emptyDirWithLimit("four gigabytes"); v.SizeLimit != nil {
		t.Fatal("an unparseable limit must degrade to unbounded, not panic or fail")
	}
	if v := emptyDirWithLimit("2Gi"); v.SizeLimit == nil || v.SizeLimit.String() != "2Gi" {
		t.Fatal("a valid limit must be applied")
	}
}

// ROLLBACK. Clearing contextSync must restore the direct mount exactly — this
// is what makes the feature safe to try, and the durable copy readable without
// the sidecar (a generation is a plain directory tree).
func TestClearingContextSyncRestoresTheDirectMount(t *testing.T) {
	cfg := syncCfg()

	on := buildResolved("c1", Resolved{Config: cfg, ContextSync: syncSpec()})
	if len(on.Spec.InitContainers) != 1 {
		t.Fatal("precondition: sidecar mode should be on")
	}

	// The operator clears `paths`, so the runtime declares nothing.
	off := buildResolved("c1", Resolved{Config: cfg})

	home := volume(off, "home")
	if home == nil || home.PersistentVolumeClaim == nil {
		t.Fatal("rollback must restore the durable claim as the agent's home")
	}
	if home.PersistentVolumeClaim.ClaimName != cfg.HomePVC {
		t.Fatalf("claim = %q, want %q", home.PersistentVolumeClaim.ClaimName, cfg.HomePVC)
	}
	if len(off.Spec.InitContainers) != 0 {
		t.Fatal("rollback must remove the sidecar")
	}
	if volume(off, "context") != nil {
		t.Fatal("rollback must remove the separate context volume")
	}
	w := container(off, "worker")
	if envOf(w, "CONTROL_URL") != cfg.ControlURL {
		t.Fatal("rollback must point the agent back at the manager directly")
	}
	if off.Spec.TerminationGracePeriodSeconds != nil {
		t.Fatal("rollback must drop the extended grace period")
	}
}
