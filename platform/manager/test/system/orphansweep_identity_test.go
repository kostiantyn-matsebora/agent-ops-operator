//go:build system

package system

// conversation-identity (design.md's own Risk entry, "A pod of a deleted
// conversation outlives it", and the ID-keyed half of
// internal/controller/orphansweep.go: deleteIfOrphanedByID/
// recordAbsentByID): the sweep that stands in for Kubernetes GC when a
// runtime pod carries no owner reference. The pure math is already pinned at
// envtest (orphansweep_test.go, internal/integration/namespacesplit_test.go)
// by fabricating a pod and calling Sweep directly; this lane is the one
// thing only a live cluster adds — OrphanSweeper.Start running as a real
// leader-elected Runnable inside the ACTUAL deployed manager, on its own
// real ticker. There is no HTTP trigger for it (unlike the housekeeping
// CronJob this pack runs once by hand, or the cron SignalAdapter this pack
// reschedules to once a minute), so this waits out the real
// DefaultOrphanSweepInterval (5m) past the real orphanMinAge (2m) — the same
// kind of real-schedule wait coordinator_test.go's self-heal lane already
// budgets for.
//
// Wired only under the nats backend (cmd/manager/main.go registers
// OrphanSweeper only when backend == backendNats — under kube, ownerRef-
// based GC already does this job, so there is nothing here to wait for).

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// orphanSweepStandinPod builds a minimal, never-scheduled-to-run pod — the
// same never-pulled image/ImagePullPolicy idiom lanes_test.go's drain
// subject already uses to get a real Pod object admitted without needing it
// to actually start — carrying the runtime app label so the sweeper's own
// `client.HasLabels{LabelConversationID}` pod listing notices it once the
// conversation-id label key is set (idValue == nil omits the key entirely,
// standing in for a pod built before this change ever labelled one).
func orphanSweepStandinPod(name string, idValue *string) *corev1.Pod {
	labels := map[string]string{runtimepod.LabelApp: runtimepod.LabelAppValue}
	if idValue != nil {
		labels[runtimepod.LabelConversationID] = *idValue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ConversationsNamespace, Name: name, Labels: labels},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "runtime", Image: "agentops-e2e-image-that-does-not-exist:never",
			ImagePullPolicy: corev1.PullNever,
		}}},
	}
}

// TestOrphanSweeperDeletesAPodWhoseConversationIsGoneButLeavesAnEmptyLabelAlone
// pins two things in one real sweep pass: a pod labelled with an ID no live
// conversation carries is removed (conversation-identity Decision 7 — by ID,
// never by Name), and a pod whose conversation-id label is PRESENT but EMPTY
// — the shape an un-labelled, pre-upgrade pod reads as once the sweep's own
// `HasLabels` listing notices the key at all — is left alone:
// recordAbsentByID's own doc comment, "an empty ID is never treated as
// absent... which would otherwise read as always orphaned".
func TestOrphanSweeperDeletesAPodWhoseConversationIsGoneButLeavesAnEmptyLabelAlone(t *testing.T) {
	fullTier(t)
	requireNats(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	bogusID := "e2e-orphan-no-such-conversation-" + stamp
	orphan := orphanSweepStandinPod("e2e-orphan-pod-"+stamp, &bogusID)
	emptyID := ""
	legacy := orphanSweepStandinPod("e2e-orphan-legacy-pod-"+stamp, &emptyID)

	// Cleanup registered before either risky Create call: a run that fails
	// before both exist still leaves neither stand-in pod behind.
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), orphan, client.GracePeriodSeconds(0))
		_ = e.K.Delete(context.Background(), legacy, client.GracePeriodSeconds(0))
	})
	if err := e.K.Create(ctx, orphan); err != nil {
		t.Fatalf("creating the orphan stand-in pod: %v", err)
	}
	if err := e.K.Create(ctx, legacy); err != nil {
		t.Fatalf("creating the empty-ID-label stand-in pod: %v", err)
	}

	// orphanMinAge (2m) keeps the sweeper off anything just created, and its
	// own ticker only looks every DefaultOrphanSweepInterval (5m) from
	// whatever phase it started at relative to this test — a real wait on a
	// real leader-elected Runnable, not something this test can trigger over
	// HTTP. The budget below covers a full interval plus margin.
	waitFor(t, "a real sweep pass to remove the pod whose ID names no live conversation", 9*time.Minute, func() (bool, error) {
		var p corev1.Pod
		err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: orphan.Name}, &p)
		return apierrors.IsNotFound(err), nil
	})

	var p corev1.Pod
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: legacy.Name}, &p); err != nil {
		t.Fatalf("the empty-ID-label pod must never be swept (an empty ID reads as \"no label at all\", never as \"orphaned\"), but it is gone: %v", err)
	}
}
