package runtimepod

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The shared config sets a context claim, which is why the no-claim case went
// unconstructed until it was a defect: every case here inherited a durable
// volume. It stays the default — the sidecar's ordinary deployment has one —
// and a case wanting the absence clears it on its own copy.
func syncCfg() Config {
	return Config{
		Image: "runtime:1", ServiceAccount: "sa", ControlURL: "http://manager:8080",
		ContextPVC: "agentops-context", ContextSyncImage: "context-sync:1",
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
	if v := volume(pod, "context"); v == nil || v.PersistentVolumeClaim == nil {
		t.Fatal("the context volume must still be the durable claim, mounted directly")
	}
	if volume(pod, "context-store") != nil {
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

	ctx := volume(pod, "context")
	if ctx == nil || ctx.EmptyDir == nil {
		t.Fatal("the agent's live context must be pod-local in sidecar mode")
	}
	if ctx.EmptyDir.SizeLimit == nil || ctx.EmptyDir.SizeLimit.String() != "4Gi" {
		t.Fatalf("the live context must be bounded; got %v", ctx.EmptyDir.SizeLimit)
	}
	ctxVol := volume(pod, "context-store")
	if ctxVol == nil || ctxVol.PersistentVolumeClaim == nil {
		t.Fatal("the durable claim must be present as its own volume")
	}

	// THE isolation property: the agent container must not mount the claim.
	w := container(pod, "worker")
	for _, m := range w.VolumeMounts {
		if m.Name == "context-store" {
			t.Fatal("the agent container must NOT mount the durable volume")
		}
	}
	// The LIVE context is at the same path in both modes, and it is the path
	// the volume is named for. The sidecar's own live mount has to agree with
	// the agent's, or it checkpoints an empty tree and reports success.
	if m := mount(pod, "context"); m == nil || m.MountPath != "/data/context" {
		t.Fatalf("agent live context mount = %+v, want /data/context", m)
	}
	agentHome := ""
	for _, e := range w.Env {
		if e.Name == "HOME" {
			agentHome = e.Value
		}
	}
	if agentHome != "/data/context" {
		t.Fatalf("HOME = %q, want /data/context — the agent and the sidecar must agree on the "+
			"live path, or the sidecar checkpoints an empty tree and reports success", agentHome)
	}

	sc := container(pod, "context-sync")
	if sc == nil {
		t.Fatal("no sidecar container was built")
	}
	var store *corev1.VolumeMount
	for i := range sc.VolumeMounts {
		if sc.VolumeMounts[i].Name == "context-store" {
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

// THE FALLBACK SHAPE, asserted by every case that takes it. There is ONE
// fallback rule with two conditions — no sidecar image, no durable volume — and
// a case asserting a different shape would be describing an exception rather
// than the rule.
//
// What it does NOT assert is the context volume itself: that follows the
// ordinary unsynchronised rule, which is the claim where there is one and
// ephemeral where there is not. Each caller states its own.
func assertUnsynchronisedPod(t *testing.T, pod *corev1.Pod, controlURL string) {
	t.Helper()

	if container(pod, "context-sync") != nil {
		t.Fatal("the fallback must build no synchronising container")
	}
	if volume(pod, "context-store") != nil {
		t.Fatal("the fallback must build no durable store volume")
	}
	// The agent talks to the manager itself. Pointing it at 127.0.0.1 with no
	// sidecar listening there is a pod that starts and then answers nothing.
	if got := envOf(container(pod, "worker"), "CONTROL_URL"); got != controlURL {
		t.Fatalf("CONTROL_URL = %q, want the manager at %q", got, controlURL)
	}
	if m := mount(pod, "context"); m == nil || m.MountPath != "/data/context" {
		t.Fatalf("agent context mount = %+v, want /data/context", m)
	}
	if pod.Spec.TerminationGracePeriodSeconds != nil {
		t.Fatal("the extended grace period exists for the final checkpoint; " +
			"with no sidecar there is none to make")
	}
}

// Declaring contextSync without an image configured must not half-apply the
// mode — that would give the agent an EMPTY ephemeral context and no sidecar to
// restore into it, silently losing every conversation's context.
func TestContextSyncWithoutAnImageFallsBackSafely(t *testing.T) {
	cfg := syncCfg()
	cfg.ContextSyncImage = ""
	pod := buildResolved("c1", Resolved{Config: cfg, ContextSync: syncSpec()})

	assertUnsynchronisedPod(t, pod, cfg.ControlURL)
	ctx := volume(pod, "context")
	if ctx == nil || ctx.PersistentVolumeClaim == nil {
		t.Fatal("without a sidecar the context claim must stay mounted directly, or context is lost")
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

	ctx := volume(off, "context")
	if ctx == nil || ctx.PersistentVolumeClaim == nil {
		t.Fatal("rollback must restore the durable claim as the agent's context volume")
	}
	if ctx.PersistentVolumeClaim.ClaimName != cfg.ContextPVC {
		t.Fatalf("claim = %q, want %q", ctx.PersistentVolumeClaim.ClaimName, cfg.ContextPVC)
	}
	if len(off.Spec.InitContainers) != 0 {
		t.Fatal("rollback must remove the sidecar")
	}
	if volume(off, "context-store") != nil {
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

// NO CONTAINER MAY MOUNT TWO VOLUMES AT ONE PATH, and the API server is the
// one that says so — `mountPath must be unique`, refused outright.
//
// THIS IS WRITTEN FROM A LIVE FAILURE, and it is the shape that made it
// expensive: the pod is never created, so there is no pod to describe, no
// events on one, and the conversation sits with NO PHASE while the reconciler
// retries forever. It reads as "nothing is happening", not as an invalid spec.
//
// What caused it: the sidecar mounts BOTH the live context and the durable
// store, and the store's path had been chosen as `/data/context` back when the
// live path was `/data/home`. Renaming the live path walked into it.
//
// Every container is checked, not the one that broke, because the next
// collision will be somewhere else. Unit tests passed throughout — they asserted
// mount NAMES and the AGENT container's paths, and the collision was between two
// mounts of the SIDECAR.
func TestNoContainerMountsTwoVolumesAtOnePath(t *testing.T) {
	for _, tc := range []struct {
		name string
		pod  *corev1.Pod
	}{
		{"plain", build("conv-paths", Config{Image: "img", ContextPVC: "ctx", WorkspacePVC: "ws"})},
		{"sidecar", buildResolved("conv-paths-sync", Resolved{Config: syncCfg(), ContextSync: syncSpec()})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			all := append(append([]corev1.Container{}, tc.pod.Spec.InitContainers...),
				tc.pod.Spec.Containers...)
			for _, c := range all {
				seen := map[string]string{}
				for _, m := range c.VolumeMounts {
					if prev, dup := seen[m.MountPath]; dup {
						t.Fatalf("container %q mounts both %q and %q at %q. The API server REFUSES "+
							"that pod, so the conversation never gets one and never gets a phase",
							c.Name, prev, m.Name, m.MountPath)
					}
					seen[m.MountPath] = m.Name
				}
			}
		})
	}
}

// The two paths are DISTINCT, stated directly rather than only as a corollary
// of the collision test above. A reader changing one should meet this.
func TestTheLiveAndDurableContextPathsDiffer(t *testing.T) {
	if contextLiveMount == contextStoreMount {
		t.Fatalf("the live and durable context paths are both %q. The sidecar mounts both, so "+
			"one value for the two is a pod that cannot be created", contextLiveMount)
	}
}

// PERSISTENCE OFF while the runtime declares contextSync. The sidecar has
// nothing to snapshot TO, and the branch that builds it mounted the durable
// claim unconditionally — so the claim reference was rendered with an EMPTY
// name, and the pod refused at admission. The conversation then never started,
// having been told its context was not promised anyway.
//
// The promise and the pod have to agree. ContinuityPossible() already answered
// "no durable volume, no promise", so a builder that fails instead of falling
// back makes the manager offer a fresh answer and then produce nothing.
func TestContextSyncWithoutADurableVolumeFallsBackSafely(t *testing.T) {
	cfg := syncCfg()
	cfg.ContextPVC = ""
	pod := buildResolved("c1", Resolved{Config: cfg, ContextSync: syncSpec()})

	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == "" {
			t.Fatalf("volume %q references a persistent claim by an EMPTY name. The API server "+
				"refuses that pod, so the conversation never starts at all", v.Name)
		}
	}

	// The SAME fallback the missing image takes, and then today's ephemeral
	// pod exactly — not merely a pod that happens to lack the store.
	assertUnsynchronisedPod(t, pod, cfg.ControlURL)
	ctx := volume(pod, "context")
	if ctx == nil || ctx.EmptyDir == nil {
		t.Fatalf("with no durable volume the context must be ephemeral, got %+v", ctx)
	}
}

// THE INVARIANT THE DEFECT BROKE, stated directly: where there is no durable
// context volume, no pod references a persistent context claim — whatever the
// runtime declares. ContinuityPossible() reads the same field, so this is what
// keeps the promise and the pod on one answer.
//
// ContextNone is excluded deliberately, and it is the case that makes the
// stronger reading of this rule false. That backend keeps no context on disk,
// so continuity is impossible for it even WITH a volume — and the claim it
// still mounts as $HOME is not a promise, it is where the pod's filesystem
// lives. The condition that matters here is the absent volume, not the absent
// promise.
func TestNoDurableVolumeMeansNoPersistentContextClaim(t *testing.T) {
	cfg := syncCfg()
	cfg.ContextPVC = ""

	for _, tc := range []struct {
		name     string
		resolved Resolved
	}{
		{"declares nothing", Resolved{Config: cfg}},
		{"declares contextSync", Resolved{Config: cfg, ContextSync: syncSpec()}},
		{"keeps context on a volume", Resolved{Config: cfg,
			ContextStorage: agentopsv1alpha1.ContextOnVolume, ContextSync: syncSpec()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.resolved.ContinuityPossible() {
				t.Fatal("precondition: with no context volume continuity is not possible")
			}
			pod := buildResolved("c1", tc.resolved)
			for _, v := range pod.Spec.Volumes {
				if v.Name != "context" && v.Name != "context-store" {
					continue
				}
				if v.PersistentVolumeClaim != nil {
					t.Fatalf("volume %q references claim %q while no context volume is "+
						"configured; the manager has already told this conversation its "+
						"context is not promised", v.Name, v.PersistentVolumeClaim.ClaimName)
				}
			}
		})
	}
}

// The other half of the same rule, and the case that keeps it honest: a backend
// keeping NO context on disk is never promised continuity, and still mounts the
// volume it was given. Reading "continuity impossible" as "must be ephemeral"
// would take a configured volume away from the one runtime that never asked for
// continuity in the first place.
func TestABackendWithNoOnDiskContextKeepsTheVolumeItIsGiven(t *testing.T) {
	r := Resolved{Config: syncCfg(), ContextStorage: agentopsv1alpha1.ContextNone}
	if r.ContinuityPossible() {
		t.Fatal("precondition: a backend keeping no context on disk promises no continuity")
	}
	ctx := volume(buildResolved("c1", r), "context")
	if ctx == nil || ctx.PersistentVolumeClaim == nil {
		t.Fatalf("the configured volume must still be mounted; it is where the pod's "+
			"filesystem lives, not a promise about what outlives the pod. got %+v", ctx)
	}
}
