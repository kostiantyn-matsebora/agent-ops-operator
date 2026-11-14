//go:build system

package system

// split-conversation-namespace tasks.md 10.5 — the migration procedure
// design.md's own "Migration Plan" section gives, in order, run against a
// REAL cluster: close, carry the claim by PV rebind, upgrade, verify that a
// reopened conversation continues its context. Modeled closely on
// state_nats_upgrade_test.go's own migration lane (TestNatsImportFromKube
// OnUpgrade), the closest existing precedent for "re-apply the chart mid-run
// and prove state survived it".
//
// ADAPTED, not run literally as written: task 10.5's own text asks for "the
// migration on a 19.x release", but this harness (install.go, cluster.go)
// only ever renders THIS WORKING TREE's chart — Install's repoRoot() always
// resolves chart/ from the current checkout, with no mechanism to pin an
// older published chart version. Task 10.5 itself names the fallback for
// exactly this case: install THIS chart with
// `global.agentops.conversationNamespace` pinned to the release namespace
// (today's single-namespace layout — design.md decision 9's own "Single
// namespace is one value away", spec.md's "Equal namespaces reproduce
// today's layout"), write context, close, PV-rebind, upgrade onto the split
// layout, confirm continuity. That is what this file does.
//
// Invasive to the shared install exactly as pinToKubeBackend/
// TestNatsImportFromKubeOnUpgrade already are — it changes
// global.agentops.conversationNamespace and persistence.context.
// existingClaim for the WHOLE pack's install while it runs, so it is FULL
// TIER ONLY, and its restoring cleanup is registered BEFORE the first risky
// upgrade (gotchas.md: "a t.Fatal mid-rollout must still leave nothing for
// the next lane to inherit").

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// 10.5 The migration end to end, in this harness's own adapted form: a
// conversation opened and given context under the single-namespace layout
// survives close, a PV rebind of its context claim, and an upgrade onto the
// split layout — reopened, its next run continues the SAME handle (spec.md's
// "A nats-backed conversation reopens into the new namespace") rather than
// answering fresh, and that next run's pod actually lands in the
// conversations namespace.
func TestSplitMigrationFromSingleNamespaceCarriesClaimAndReopens(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	t.Cleanup(func() {
		if err := e.UpgradeManager(context.Background()); err != nil {
			t.Errorf("restoring the install to its chart default (split, no existingClaim): %v", err)
		}
	})

	// The "19.x" stand-in: this working tree's own chart, with the two
	// namespaces pinned equal.
	if err := e.UpgradeManager(ctx, "global.agentops.conversationNamespace="+Namespace); err != nil {
		t.Fatalf("pinning the install to the single-namespace layout: %v", err)
	}

	src := source("e2e-split-migration-src", "e2e", nil)
	mustCreate(t, e.K, src)
	pipe := pipeline("e2e-split-migration-pipe", ProfileStub, []string{src.Name}, []string{ChannelConsole})
	mustCreate(t, e.K, pipe)
	if err := waitPipelineReady(ctx, e.K, pipe.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	// A conversation under the single-namespace layout, with context on its
	// volume worth carrying.
	fp := "e2e-split-migration-" + stamp
	marker := "migration-context-" + stamp
	e.PostTask(t, src.Name, fp, "echo "+marker)
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	assertRunSucceededWith(t, conv.Status.Runs[0], "the pre-migration run", marker)
	handle := conv.Status.RuntimeContextID
	if handle == "" {
		t.Fatalf("the runtime's handle must be recorded before it can be proven to survive the migration")
	}
	pvName := boundContextPVName(t, ctx, e)

	// Migration Plan step 1: close every open conversation, so no pod or
	// claim is in use.
	if code, out := e.ConsoleSend(t, conv.Name, "/close"); code/100 != 2 {
		t.Fatalf("/close: %d %s", code, out)
	}
	waitFor(t, "the conversation closed before the rebind", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})

	// Migration Plan step 4: carry the claim across by PV rebind.
	const migratedClaim = "e2e-split-migration-context"
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: ConversationsNamespace, Name: migratedClaim}})
		_ = e.K.Delete(context.Background(), &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvName}})
	})
	rebindContextPV(t, ctx, e, pvRebind{pvName: pvName, oldNamespace: Namespace, oldClaim: "agentops-context",
		newNamespace: ConversationsNamespace, newClaim: migratedClaim})

	// Migration Plan steps 2 (the conversations namespace already exists —
	// the chart created it before this test ever pinned single-namespace
	// mode) and 3 (this profile resolves no Secret, so nothing to place)
	// are both already satisfied. Step 5: point the chart at the carried
	// claim and upgrade onto the split layout.
	if err := e.UpgradeManager(ctx, "persistence.context.existingClaim="+migratedClaim); err != nil {
		t.Fatalf("upgrading onto the split layout with the carried claim: %v", err)
	}

	// Migration Plan step 7 (verify): reopen the migrated conversation —
	// its record survived unchanged (the nats backend's own promise,
	// spec.md) — and its NEXT run must continue the SAME handle rather
	// than answering fresh.
	reopenCode, reopenOut := e.do(t, "POST", e.Manager.URL()+"/channel/conversations/"+url.PathEscape(conv.Name)+"/reopen",
		[]byte(`{"channel":"console"}`), auth)
	var reopenAccepted acceptedWire
	_ = json.Unmarshal([]byte(reopenOut), &reopenAccepted)
	if reopenCode != 202 || len(reopenAccepted.TaskIDs) != 1 {
		t.Fatalf("reopening the migrated conversation: %d %s", reopenCode, reopenOut)
	}
	waitTaskSettled(t, e, auth, reopenAccepted.TaskIDs[0], 2*time.Minute)

	continueMarker := "migration-continue-" + stamp
	if code, out := e.ConsoleSend(t, conv.Name, "echo "+continueMarker); code/100 != 2 {
		t.Fatalf("continuing the reopened conversation: %d %s", code, out)
	}
	conv = e.WaitRun(t, conv.Name, 2, 4*time.Minute)
	assertRunSucceededWith(t, conv.Status.Runs[len(conv.Status.Runs)-1],
		"the post-migration run must continue, not fail for lost context", continueMarker)
	assertContextCarried(t, conv, handle)

	// And the new pod lands in the conversations namespace — the
	// conversation OPENED under the single-namespace layout, but its NEXT
	// dispatch runs under the layout the manager is now upgraded onto,
	// mounting the carried claim by its NEW name.
	if conv.Status.RuntimePod == "" {
		t.Fatalf("no runtime pod recorded on the reopened conversation %s", conv.Name)
	}
	var pod corev1.Pod
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: conv.Status.RuntimePod}, &pod); err != nil {
		t.Fatalf("the migrated conversation's new pod must land in the conversations namespace: %v", err)
	}
	assertPodMountsClaim(t, &pod, migratedClaim)
}

func assertRunSucceededWith(t *testing.T, run agentopsv1alpha1.RunStatus, what, marker string) {
	t.Helper()
	if run.Status != "succeeded" || !strings.Contains(run.Result, "[stub] "+marker) {
		t.Fatalf("%s: %+v", what, run)
	}
}

func assertContextCarried(t *testing.T, conv *agentopsv1alpha1.Conversation, handle string) {
	t.Helper()
	if conv.Status.RuntimeContextID != handle {
		t.Fatalf("the carried claim must carry the SAME handle across the migration: before=%q after=%q", handle, conv.Status.RuntimeContextID)
	}
	for _, c := range conv.Status.Conditions {
		if c.Type == "ContextContinuity" && c.Status == metav1.ConditionFalse {
			t.Fatalf("a carried claim must never be reported as a context loss: %s %s", c.Reason, c.Message)
		}
	}
}

// boundContextPVName returns the PV behind the context claim, which sits in
// the RELEASE namespace under the single-namespace layout.
func boundContextPVName(t *testing.T, ctx context.Context, e *Env) string {
	t.Helper()
	var oldPVC corev1.PersistentVolumeClaim
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-context"}, &oldPVC); err != nil {
		t.Fatalf("the pre-migration context claim must exist in the release namespace: %v", err)
	}
	if oldPVC.Spec.VolumeName == "" {
		t.Fatalf("the claim must already be bound (first-consumer binding already happened once a run succeeded): %+v", oldPVC)
	}
	return oldPVC.Spec.VolumeName
}

// pvRebind names the PV and the old and new claim a rebind moves it between.
type pvRebind struct {
	pvName                 string
	oldNamespace, oldClaim string
	newNamespace, newClaim string
}

// rebindContextPV performs design.md's Migration Plan step 4 against a REAL
// PersistentVolume: Retain its reclaim policy, delete the old claim, clear
// the PV's claimRef so it becomes Available again, then pre-create the new
// claim in the target namespace with volumeName set — copying the PV's own
// accessModes, capacity and storage class so the new claim actually binds
// to it rather than minting a second volume beside it (the exact failure
// chart/templates/pvc.yaml's own comment names an explicit empty
// storageClassName exists to avoid).
func rebindContextPV(t *testing.T, ctx context.Context, e *Env, r pvRebind) {
	t.Helper()
	pvName, oldNamespace, oldClaim, newNamespace, newClaim := r.pvName, r.oldNamespace, r.oldClaim, r.newNamespace, r.newClaim

	var pv corev1.PersistentVolume
	if err := e.K.Get(ctx, types.NamespacedName{Name: pvName}, &pv); err != nil {
		t.Fatalf("getting the PV to rebind: %v", err)
	}
	pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	if err := e.K.Update(ctx, &pv); err != nil {
		t.Fatalf("setting the PV's reclaim policy to Retain: %v", err)
	}

	oldPVC := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: oldNamespace, Name: oldClaim}}
	if err := e.K.Delete(ctx, oldPVC); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("deleting the old claim %s/%s: %v", oldNamespace, oldClaim, err)
	}
	waitFor(t, "the old claim to be gone", 2*time.Minute, func() (bool, error) {
		var cur corev1.PersistentVolumeClaim
		err := e.K.Get(ctx, types.NamespacedName{Namespace: oldNamespace, Name: oldClaim}, &cur)
		return apierrors.IsNotFound(err), nil
	})

	if err := e.K.Get(ctx, types.NamespacedName{Name: pvName}, &pv); err != nil {
		t.Fatalf("re-getting the PV before clearing its claimRef: %v", err)
	}
	pv.Spec.ClaimRef = nil
	if err := e.K.Update(ctx, &pv); err != nil {
		t.Fatalf("clearing the PV's claimRef so it becomes Available again: %v", err)
	}

	sc := pv.Spec.StorageClassName
	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: newNamespace, Name: newClaim},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      pv.Spec.AccessModes,
			VolumeName:       pv.Name,
			StorageClassName: &sc,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: pv.Spec.Capacity[corev1.ResourceStorage]},
			},
		},
	}
	if err := e.K.Create(ctx, newPVC); err != nil {
		t.Fatalf("pre-creating the carried claim %s/%s: %v", newNamespace, newClaim, err)
	}
	waitFor(t, "the carried claim to bind", 2*time.Minute, func() (bool, error) {
		var cur corev1.PersistentVolumeClaim
		if err := e.K.Get(ctx, types.NamespacedName{Namespace: newNamespace, Name: newClaim}, &cur); err != nil {
			return false, err
		}
		return cur.Status.Phase == corev1.ClaimBound, nil
	})
}
