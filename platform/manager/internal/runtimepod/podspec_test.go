package runtimepod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// idFor is the fixture's stand-in for a backend-minted ID, distinct from the
// name so a test asserting on the ID cannot pass by accident against the name.
func idFor(name string) string { return name + "-id" }

func conversation(name string) *state.ConversationRecord {
	return &state.ConversationRecord{Name: name, ID: idFor(name)}
}

func build(convName string, cfg Config) *corev1.Pod {
	return buildResolved(convName, Resolved{Config: cfg})
}

// buildResolved is build for tests that need the sidecar declaration too.
func buildResolved(convName string, r Resolved) *corev1.Pod {
	return Build(conversation(convName), &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", r, "master-key")
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
	// The subPath is the conversation's ID, not its Name — a name is unique
	// only among live conversations, so subPath'ing by Name would let a
	// recreated name mount the previous conversation's tree.
	if m.SubPath != idFor("conv-a") {
		t.Fatalf("subPath = %q, want the conversation's ID", m.SubPath)
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

// Context and workspace are independent: enabling one must not enable the
// other.
func TestContextAndWorkspaceAreIndependent(t *testing.T) {
	pod := build("conv-a", Config{Image: "img", ContextPVC: "agentops-context"})

	if h := volume(pod, "context"); h == nil || h.PersistentVolumeClaim == nil {
		t.Fatalf("context should be a claim, got %+v", h)
	}
	if w := volume(pod, "workspace"); w == nil || w.EmptyDir == nil {
		t.Fatalf("workspace should stay ephemeral, got %+v", w)
	}
	// The PATH moves with the name. Nothing inside the volume relocates: a
	// claim's contents appear AT the mount path, and the stored transcript
	// directory is named for the WORKING directory, not for $HOME.
	if m := mount(pod, "context"); m == nil || m.MountPath != "/data/context" || m.SubPath != "" {
		t.Fatalf("context mount = %+v, want /data/context with no subPath", m)
	}
}

// The ID label must be LABEL-SAFE on every backend that could mint an ID
// (design Decision 7): at most 63 characters, [A-Za-z0-9._-], alphanumeric
// ends. NUID (22 chars) and a Kubernetes UID (36 chars) both fit; a value
// that is not label-safe fails pod creation outright.
func TestConversationIDLabelIsLabelSafe(t *testing.T) {
	for _, id := range []string{
		"nxK3v9f2QhT1Bz0mR7cJ8x",               // nuid shape, 22 chars
		"550e8400-e29b-41d4-a716-446655440000", // uuid shape, 36 chars
	} {
		pod := Build(&state.ConversationRecord{Name: "conv-a", ID: id},
			&agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", Resolved{Config: Config{Image: "img"}}, "master-key")
		if errs := validation.IsValidLabelValue(pod.Labels[LabelConversationID]); len(errs) != 0 {
			t.Fatalf("ID %q is not a valid label value: %v", id, errs)
		}
	}
}

// A recreated conversation under a REUSED name must not mount the previous
// conversation's workspace directory or carry its label — the hazard
// design.md's Decision 1 names. Same Name, different ID.
func TestRecreatedNameGetsADistinctWorkspaceAndLabel(t *testing.T) {
	cfg := Config{Image: "img", WorkspacePVC: "agentops-workspace"}
	deleted := &state.ConversationRecord{Name: "conv-a", ID: "id-1"}
	recreated := &state.ConversationRecord{Name: "conv-a", ID: "id-2"}

	podOne := Build(deleted, &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", Resolved{Config: cfg}, "master-key")
	podTwo := Build(recreated, &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm", Resolved{Config: cfg}, "master-key")

	if s1, s2 := mount(podOne, "workspace").SubPath, mount(podTwo, "workspace").SubPath; s1 == s2 {
		t.Fatalf("recreated name %q reused the deleted conversation's workspace subPath %q", "conv-a", s1)
	}
	if l1, l2 := podOne.Labels[LabelConversationID], podTwo.Labels[LabelConversationID]; l1 == l2 {
		t.Fatalf("recreated name %q reused the deleted conversation's ID label %q", "conv-a", l1)
	}
	if podOne.Labels[LabelConversation] != podTwo.Labels[LabelConversation] {
		t.Fatalf("the display label should still agree on the shared name")
	}
}

// A runtime OVERLAY leaves both claims exactly as the conversation resolved
// them. This is the whole of the move: an AgentRuntime declares neither volume,
// so there is nothing of a runtime's for FromRuntime to apply.
// CONVO_ID carries the conversation's ID, not its Name — the runtime passes
// it back as `convo=` on its long poll, and /work must resolve it strictly by
// ID (conversation-identity, Decision 1).
func TestConvoIDEnvCarriesTheConversationID(t *testing.T) {
	pod := build("conv-a", Config{Image: "img"})

	w := pod.Spec.Containers[0]
	got := ""
	for _, e := range w.Env {
		if e.Name == "CONVO_ID" {
			got = e.Value
		}
	}
	if got != idFor("conv-a") {
		t.Fatalf("CONVO_ID = %q, want the conversation's ID %q", got, idFor("conv-a"))
	}
}

func TestFromRuntimeLeavesBothVolumesAlone(t *testing.T) {
	spec := &agentopsv1alpha1.AgentRuntimeSpec{Image: "img"}

	cfg := FromRuntime(spec, Config{ContextPVC: "resolved-context", WorkspacePVC: "resolved-ws"})

	if cfg.ContextPVC != "resolved-context" || cfg.WorkspacePVC != "resolved-ws" {
		t.Fatalf("context=%q workspace=%q, want the resolved claims untouched",
			cfg.ContextPVC, cfg.WorkspacePVC)
	}
}
