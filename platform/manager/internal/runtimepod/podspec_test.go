package runtimepod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
)

func conversation(name string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name = name
	return c
}

func build(convName string, cfg Config) *corev1.Pod {
	return buildResolved(convName, Resolved{Config: cfg})
}

// buildResolved is build for tests that need the sidecar declaration too.
func buildResolved(convName string, r Resolved) *corev1.Pod {
	return Build(conversation(convName), &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", r)
}

// volume finds a pod volume by name.
func volume(pod *corev1.Pod, name string) *corev1.Volume {
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == name {
			return &pod.Spec.Volumes[i]
		}
	}
	return nil
}

// mount finds the worker container's mount by volume name.
func mount(pod *corev1.Pod, name string) *corev1.VolumeMount {
	for i := range pod.Spec.Containers[0].VolumeMounts {
		if pod.Spec.Containers[0].VolumeMounts[i].Name == name {
			return &pod.Spec.Containers[0].VolumeMounts[i]
		}
	}
	return nil
}

// The default: a checkout that dies with the pod costs a re-clone and nothing
// else, so no claim is the correct shipped behavior.
func TestWorkspaceWithoutClaimIsEphemeral(t *testing.T) {
	pod := build("conv-a", Config{Image: "img"})

	v := volume(pod, "workspace")
	if v == nil || v.EmptyDir == nil {
		t.Fatalf("workspace should be an emptyDir, got %+v", v)
	}
	if v.PersistentVolumeClaim != nil {
		t.Fatalf("no claim was configured, yet one was mounted: %+v", v.PersistentVolumeClaim)
	}
	m := mount(pod, "workspace")
	if m == nil || m.MountPath != "/data/workspace" || m.SubPath != "" {
		t.Fatalf("want /data/workspace with no subPath, got %+v", m)
	}
}

func TestWorkspaceClaimMountsWithConversationSubPath(t *testing.T) {
	pod := build("conv-a", Config{Image: "img", WorkspacePVC: "agentops-workspace"})

	v := volume(pod, "workspace")
	if v == nil || v.PersistentVolumeClaim == nil {
		t.Fatalf("workspace should be a claim, got %+v", v)
	}
	if got := v.PersistentVolumeClaim.ClaimName; got != "agentops-workspace" {
		t.Fatalf("claim name = %q, want agentops-workspace", got)
	}
	m := mount(pod, "workspace")
	// The path must NOT move: claude-code keys sessions by cwd, so isolation is
	// bought with subPath rather than with a per-conversation mount path.
	if m == nil || m.MountPath != "/data/workspace" {
		t.Fatalf("mount path moved: %+v", m)
	}
	if m.SubPath != "conv-a" {
		t.Fatalf("subPath = %q, want the conversation name", m.SubPath)
	}
}

// Two concurrent conversations on ONE claim must land on different directories
// at the same path — a shared working tree must not be reachable by config.
func TestConcurrentConversationsGetDistinctSubPaths(t *testing.T) {
	cfg := Config{Image: "img", WorkspacePVC: "agentops-workspace"}
	a, b := mount(build("conv-a", cfg), "workspace"), mount(build("conv-b", cfg), "workspace")

	if a.SubPath == b.SubPath {
		t.Fatalf("two conversations share subPath %q", a.SubPath)
	}
	if a.MountPath != b.MountPath || a.MountPath != "/data/workspace" {
		t.Fatalf("mount paths differ or moved: %q vs %q", a.MountPath, b.MountPath)
	}
}

// Home and workspace are independent: enabling one must not enable the other.
func TestHomeAndWorkspaceAreIndependent(t *testing.T) {
	pod := build("conv-a", Config{Image: "img", HomePVC: "agentops-home"})

	if h := volume(pod, "home"); h == nil || h.PersistentVolumeClaim == nil {
		t.Fatalf("home should be a claim, got %+v", h)
	}
	if w := volume(pod, "workspace"); w == nil || w.EmptyDir == nil {
		t.Fatalf("workspace should stay ephemeral, got %+v", w)
	}
	if m := mount(pod, "home"); m == nil || m.MountPath != "/data/home" || m.SubPath != "" {
		t.Fatalf("home mount = %+v, want /data/home with no subPath", m)
	}
}

func TestFromRuntimeResolvesBothVolumes(t *testing.T) {
	spec := &agentopsv1alpha1.AgentRuntimeSpec{
		Image:     "img",
		Home:      &agentopsv1alpha1.HomeVolume{PVCRef: &agentopsv1alpha1.ObjectRef{Name: "home-claim"}},
		Workspace: &agentopsv1alpha1.WorkspaceVolume{PVCRef: &agentopsv1alpha1.ObjectRef{Name: "ws-claim"}},
	}
	// A runtime declaring volumes must OVERRIDE the bootstrap defaults, not
	// inherit them alongside.
	cfg := FromRuntime(spec, Config{HomePVC: "bootstrap-home", WorkspacePVC: "bootstrap-ws"})

	if cfg.HomePVC != "home-claim" || cfg.WorkspacePVC != "ws-claim" {
		t.Fatalf("home=%q workspace=%q, want home-claim/ws-claim", cfg.HomePVC, cfg.WorkspacePVC)
	}
	bare := FromRuntime(&agentopsv1alpha1.AgentRuntimeSpec{Image: "img"},
		Config{HomePVC: "bootstrap-home", WorkspacePVC: "bootstrap-ws"})
	if bare.HomePVC != "" || bare.WorkspacePVC != "" {
		t.Fatalf("a runtime declaring no volumes must clear both, got home=%q workspace=%q",
			bare.HomePVC, bare.WorkspacePVC)
	}
}
