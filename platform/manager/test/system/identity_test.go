//go:build system

package system

// Task 7 (conversation-identity) — the system-only half of ID-as-identity:
// what only a live cluster adds is the ACTUAL workspace directory on a real
// PVC and the ACTUAL pod label a real kubelet schedules, neither of which
// envtest's fake client ever produces. The pure identifier math — op ids,
// causedBy, the live-match resolution rule — is already pinned at envtest
// (internal/runtimepod/podspec_test.go, internal/controller/orphansweep_test.go,
// internal/chat/coordinate_test.go, internal/chat/ops_test.go).

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// TestConversationIdentitySurvivesNameReuse proves the central hazard
// proposal.md names closed: a conversation deleted outright, and a second
// one created under the SAME display name — namewords.nextSuffix reissues a
// freed suffix once no live record holds the base, so this is not
// contrived — never share a workspace directory, a pod label or a
// correlation id, even though both lives share one pod NAME
// (runtimepod.PodName, keyed by the display Name, the store key that never
// moves).
//
// 7.2 rides the same lane: each life's correlationId is the initiationId it
// adopted verbatim, and a typed-name lookup after the second life exists
// reaches it, not the first life's stale object.
func TestConversationIdentitySurvivesNameReuse(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	// Exactly three non-stopword tokens, so namewords.Words keeps every one
	// of them (it drops nothing past three) — the SAME NameBase both lives
	// below share, which is what makes the second create actually reissue
	// the freed suffix instead of landing on an unrelated name.
	title := "identity-reuse-" + stamp

	idA := "e2e-identity-reuse-a-" + stamp
	e.PostSignal(t, SourceTasks, map[string]any{
		"initiationId": idA, "kind": "task", "payload": "echo identity-reuse first-life", "title": title,
	})
	convA := e.ConversationFor(t, idA, 2*time.Minute)
	convA = e.WaitRun(t, convA.Name, 1, 4*time.Minute)
	if run := convA.Status.Runs[0]; run.Status != "succeeded" {
		t.Fatalf("the first life's run must succeed before its identity means anything: %+v", run)
	}
	if convA.UID == "" {
		t.Fatalf("Create must return the backend-minted ID: %+v", convA.ObjectMeta)
	}
	if convA.Spec.CorrelationID != idA {
		t.Fatalf("a signal-originated conversation must adopt the posted initiationId verbatim as its correlationId, got %q want %q", convA.Spec.CorrelationID, idA)
	}
	podA := runtimePodFor(t, ctx, e, convA)

	// Delete it OUTRIGHT, never /close — the store key is the Name, so this
	// is the only way to free the base for the second life to reissue.
	if err := e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: convA.Name}}); err != nil {
		t.Fatalf("deleting the first life: %v", err)
	}
	waitFor(t, "the first life's object to be gone", 2*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, convA.Name)
		return err != nil, nil
	})

	idB := "e2e-identity-reuse-b-" + stamp
	e.PostSignal(t, SourceTasks, map[string]any{
		"initiationId": idB, "kind": "task", "payload": "echo identity-reuse second-life", "title": title,
	})
	convB := e.ConversationFor(t, idB, 2*time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: convB.Name}})
	})
	convB = e.WaitRun(t, convB.Name, 1, 4*time.Minute)
	if run := convB.Status.Runs[0]; run.Status != "succeeded" {
		t.Fatalf("the second life's run must succeed: %+v", run)
	}

	assertSecondLifeIdentity(t, convA, convB, idB)

	// 7.2: a typed name reaches the NEWEST live conversation, not a first
	// life a real API server somehow still answered for.
	byName, err := e.K.Conversation(ctx, convA.Name)
	if err != nil {
		t.Fatalf("looking up the reused name %q: %v", convA.Name, err)
	}
	if byName.UID != convB.UID {
		t.Fatalf("a typed name must reach the newest live conversation, got UID %q want the second life's %q", byName.UID, convB.UID)
	}

	podB := runtimePodFor(t, ctx, e, convB)

	// SAME pod NAME (both lives share one display name, and the pod name is
	// derived from the Name alone), but a DIFFERENT pod OBJECT — proving the
	// first life's pod never lingered and the second life's is genuinely new.
	if podA.Name != podB.Name {
		t.Fatalf("both lives share one display name, so they must share one pod name: %q vs %q", podA.Name, podB.Name)
	}
	if podA.UID == podB.UID {
		t.Fatalf("the second life's pod must be its own object, not the first life's surviving under a reused name: UID %q reused", podA.UID)
	}
	assertPodLabelsCarryID(t, podA, podB, convA, convB)
	assertWorkspaceSubPathsCarryID(t, podA, podB, convA, convB)
}

func assertPodLabelsCarryID(t *testing.T, podA, podB *corev1.Pod, convA, convB *agentopsv1alpha1.Conversation) {
	t.Helper()
	labelA, labelB := podA.Labels[runtimepod.LabelConversationID], podB.Labels[runtimepod.LabelConversationID]
	if labelA == "" || labelB == "" {
		t.Fatalf("both pods must carry the %s label: %q %q", runtimepod.LabelConversationID, labelA, labelB)
	}
	if labelA == labelB {
		t.Fatalf("the %s label must carry the ID, not the Name, so it must differ across lives: both %q", runtimepod.LabelConversationID, labelA)
	}
	if labelA != string(convA.UID) || labelB != string(convB.UID) {
		t.Fatalf("the %s label must equal the conversation's own ID: life A %q != %q, life B %q != %q",
			runtimepod.LabelConversationID, labelA, convA.UID, labelB, convB.UID)
	}
}

func assertWorkspaceSubPathsCarryID(t *testing.T, podA, podB *corev1.Pod, convA, convB *agentopsv1alpha1.Conversation) {
	t.Helper()
	subA, okA := workspaceSubPath(podA)
	subB, okB := workspaceSubPath(podB)
	if !okA || !okB {
		t.Fatalf("both pods must mount the workspace claim with a subPath (persistence.workspace must be configured for this install)")
	}
	if subA == subB {
		t.Fatalf("the workspace subPath must be the ID, not the Name, so a recreated name must never mount the predecessor's tree: both %q", subA)
	}
	if subA != string(convA.UID) || subB != string(convB.UID) {
		t.Fatalf("the workspace subPath must equal the conversation's own ID: life A %q != %q, life B %q != %q", subA, convA.UID, subB, convB.UID)
	}
}

func assertSecondLifeIdentity(t *testing.T, convA, convB *agentopsv1alpha1.Conversation, idB string) {
	t.Helper()
	if convB.Name != convA.Name {
		t.Fatalf("namewords.nextSuffix must reissue the freed base once the first life is actually deleted, got %q then %q from the same title", convA.Name, convB.Name)
	}
	if convB.UID == "" || convB.UID == convA.UID {
		t.Fatalf("the second life must mint its OWN ID, distinct from the first life's %q, got %q", convA.UID, convB.UID)
	}
	if convB.Spec.CorrelationID != idB {
		t.Fatalf("the second life must adopt its OWN initiationId verbatim, got %q want %q", convB.Spec.CorrelationID, idB)
	}
	if convB.Spec.CorrelationID == convA.Spec.CorrelationID {
		t.Fatalf("two unrelated lives must never share a correlation id: both carry %q", convB.Spec.CorrelationID)
	}
}

// runtimePodFor reads the runtime pod a conversation's own status names,
// from the conversations namespace.
func runtimePodFor(t *testing.T, ctx context.Context, e *Env, conv *agentopsv1alpha1.Conversation) *corev1.Pod {
	t.Helper()
	if conv.Status.RuntimePod == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}
	var pod corev1.Pod
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: conv.Status.RuntimePod}, &pod); err != nil {
		t.Fatalf("getting the runtime pod %s: %v", conv.Status.RuntimePod, err)
	}
	return &pod
}

// workspaceSubPath reads the "workspace" volume mount's subPath off a
// runtime pod's first container, ok is false when the claim is not mounted
// at all (an ephemeral-workspace install).
func workspaceSubPath(pod *corev1.Pod) (string, bool) {
	for _, c := range pod.Spec.Containers {
		for _, m := range c.VolumeMounts {
			if m.Name == "workspace" && m.SubPath != "" {
				return m.SubPath, true
			}
		}
	}
	return "", false
}
