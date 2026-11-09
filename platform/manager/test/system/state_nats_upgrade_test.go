//go:build system

package system

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// nats-state-store-default section 5 (5-6): the two lanes that re-apply the
// chart mid-run — a kube-to-nats migration, and a rolling nats image
// upgrade. Both are invasive to the SHARED install every other lane in this
// pack runs against, so both are FULL TIER ONLY and both restore the
// install to its chart default (nats, the chart's own pinned image) before
// returning, cleanup registered before the risky call that needs restoring
// (replicas_test.go's own scaleManagerReplicas documents why: a t.Fatal mid-
// rollout must still leave nothing for the next lane to inherit).

// pinToKubeBackend upgrades the shared install onto the kube backend and
// clears the nats bundle's own PVCs. The clear matters only because of WHEN
// this runs: TestMain's own initial install already booted on nats (the
// chart's default) against an empty, freshly-created cluster, so the
// one-time import (design D-10) already ran ONCE, trivially, and wrote its
// marker before any test ever got to create a kube record for it to copy.
// Without wiping the nats PVCs here, upgrading back to nats later would
// reattach the SAME JetStream volumes carrying that marker, and the import
// this lane exists to prove would silently no-op.
func pinToKubeBackend(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	if err := e.Upgrade(ctx, "stateStore.backend=kube"); err != nil {
		t.Fatalf("pinning the install to the kube backend: %v", err)
	}
	deleteNatsPVCs(t, ctx, e)
}

// deleteNatsPVCs clears every PVC the nats StatefulSet's volumeClaimTemplate
// minted, named "data-agentops-nats-<ordinal>" by Kubernetes' own
// convention. NOT by label selector: the volumeClaimTemplate in
// chart/charts/nats/templates/_helpers.tpl carries no labels of its own, and
// a StatefulSet copies only the template's OWN metadata onto the PVC it
// creates, never the pod template's labels — natsSelector
// ("app.kubernetes.io/name=agentops-nats") matches the pods and the
// Services, but matches zero PVCs. A label-selector delete would silently
// no-op here, leaving the OLD JetStream volumes (and their already-written
// import marker) in place for the later upgrade back onto nats, which is
// exactly the no-op this function exists to prevent.
func deleteNatsPVCs(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	const pvcNamePrefix = "data-agentops-nats-"
	var pvcs corev1.PersistentVolumeClaimList
	if err := e.K.List(ctx, &pvcs, client.InNamespace(Namespace)); err != nil {
		t.Fatalf("listing PVCs to clear the nats bundle's own: %v", err)
	}
	var names []string
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if !strings.HasPrefix(pvc.Name, pvcNamePrefix) {
			continue
		}
		if err := e.K.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("deleting PVC %s: %v", pvc.Name, err)
		}
		names = append(names, pvc.Name)
	}
	waitFor(t, fmt.Sprintf("%d nats PVC(s) to be gone before a real import", len(names)), 2*time.Minute, func() (bool, error) {
		for _, name := range names {
			var pvc corev1.PersistentVolumeClaim
			err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: name}, &pvc)
			if err == nil {
				return false, nil
			}
			if !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		return true, nil
	})
}

// 5.5 — the migration path end to end: a fresh kube install with real
// state, the upgrade to the chart's own default (nats) importing it,
// survival of a second manager restart with no duplicate, and the kube
// custom resources left exactly as they were (design D-10: "it never
// patches a Kubernetes object").
func TestNatsImportFromKubeOnUpgrade(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	t.Cleanup(func() {
		if err := e.Upgrade(context.Background(), "stateStore.backend=nats"); err != nil {
			t.Errorf("restoring the install to its default (nats) backend: %v", err)
		}
	})
	pinToKubeBackend(t, ctx, e)

	const sourceName = "e2e-nats-import-src"
	const pipelineName = "e2e-nats-import-pipe"
	convID := "e2e-nats-import-conv-" + stamp

	src := source(sourceName, "e2e", nil)
	src.Spec.CooldownHours = 6
	mustCreate(t, e.K, src)
	mustCreate(t, e.K, pipeline(pipelineName, ProfileStub, []string{sourceName}, []string{ChannelConsole}))
	if err := waitPipelineReady(ctx, e.K, pipelineName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	e.PostSignal(t, sourceName, map[string]any{"initiationId": convID, "kind": "task", "payload": "echo " + convID, "title": convID})
	conv := e.ConversationFor(t, convID, time.Minute)
	// NOT deleted on cleanup, unlike every other lane's own fixture: the
	// full tier's system.yml caller runs this Go pack FIRST and then
	// `platform/console/ui/e2e-live` against the SAME, reused cluster
	// (SYSTEM_REUSE=1) — task 5.10's spec finds this exact conversation by
	// its stable `e2e-nats-import-conv-` title prefix and renders it, which
	// is the whole point of leaving it behind rather than tidying it away.
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)

	// A repeat within the window stamps SignalSource.Status.Cooldown
	// (kube.CooldownLayout) — the third kind design D-10's table names,
	// recurring into the SAME conversation rather than opening a second one.
	e.PostSignal(t, sourceName, map[string]any{"initiationId": convID, "kind": "task", "payload": "repeat", "title": convID})
	waitFor(t, "a cooldown entry for "+convID, time.Minute, func() (bool, error) {
		return cooldownHolds(ctx, e, sourceName, convID)
	})
	assertExactlyOneConversationPerClaimant(t, ctx, e, convID, pipelineName)

	sigBefore, err := signalForInitiationID(ctx, e, sourceName, convID)
	if err != nil || sigBefore == nil {
		t.Fatalf("the signal must exist under kube before the upgrade: %v", err)
	}
	convBefore, err := e.K.Conversation(ctx, conv.Name)
	if err != nil {
		t.Fatal(err)
	}
	var sourceBefore agentopsv1alpha1.SignalSource
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: sourceName}, &sourceBefore); err != nil {
		t.Fatal(err)
	}

	if err := e.Upgrade(ctx, "stateStore.backend=nats"); err != nil {
		t.Fatalf("upgrading to the nats backend: %v", err)
	}

	// /state/*: the conversation and the signal, through the manager's own
	// read surface — the readiness gate (store.go's natsReadinessCheck)
	// already waited out the import inside e.Upgrade's own WaitReady, so a
	// 200 here is the import having actually completed, not a race with it.
	code, out := e.do(t, "GET", e.Manager.URL()+"/state/conversations/"+conv.Name, nil, bearerPrefix+e.Values.AdapterToken)
	if code != 200 || !strings.Contains(out, convID) {
		t.Fatalf("the conversation must read through /state/* after the import: %d %s", code, out)
	}
	code, out = e.do(t, "GET", e.Manager.URL()+"/state/signals", nil, bearerPrefix+e.Values.AdapterToken)
	if code != 200 || !strings.Contains(out, convID) {
		t.Fatalf("the signal must read through /state/signals after the import: %d %s", code, out)
	}

	// The cooldown window has no /state/* surface of its own (design D-10's
	// table: it lives in the nats `cooldowns` bucket, never served over
	// HTTP) — its survival is read through its OBSERVABLE EFFECT instead: a
	// third post of the same initiationId, still inside the 6-hour window,
	// must still recur into the SAME conversation rather than open a second.
	e.PostSignal(t, sourceName, map[string]any{"initiationId": convID, "kind": "task", "payload": "repeat again", "title": convID})
	waitFor(t, "the post-import repeat to still fold into the one conversation", time.Minute, func() (bool, error) {
		n, err := countConversationsFor(ctx, e, convID)
		return n == 1, err
	})

	// Idempotent re-import on a restart (design D-10 step 4): nothing short
	// of the marker in the nats adapterstate bucket stops a second pass, and
	// a manager restart is the only way this pack can ask the importer to
	// run again.
	restartManager(t, ctx, e)
	code, out = e.do(t, "GET", e.Manager.URL()+"/state/conversations/"+conv.Name, nil, bearerPrefix+e.Values.AdapterToken)
	if code != 200 || !strings.Contains(out, convID) {
		t.Fatalf("the conversation must still read after a second manager restart: %d %s", code, out)
	}
	n, err := countConversationsFor(ctx, e, convID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("a repeated import pass must never duplicate the conversation, got %d", n)
	}

	// The kube custom resources are untouched (design D-10: "it never
	// patches a Kubernetes object"). Compared by VALUE — the importer's own
	// read cannot move anything on the source regardless, but a reconciler
	// loop reacting to a cache resync could, and that is what this guards.
	assertKubeRecordsUntouched(t, ctx, e, kubeSnapshot{
		convName: conv.Name, conv: convBefore, sig: sigBefore,
		sourceName: sourceName, cooldown: sourceBefore.Status.Cooldown,
	})
}

// kubeSnapshot is what the kube custom resources held before the upgrade.
type kubeSnapshot struct {
	convName   string
	conv       *agentopsv1alpha1.Conversation
	sig        *agentopsv1alpha1.Signal
	sourceName string
	cooldown   []agentopsv1alpha1.CooldownEntry
}

// countConversationsFor counts the kube Conversations opened for one initiation id.
func countConversationsFor(ctx context.Context, e *Env, convID string) (int, error) {
	items, err := e.K.Conversations(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range items {
		if items[i].Spec.ConversationID == convID {
			n++
		}
	}
	return n, nil
}

// cooldownHolds reports whether the source's kube status carries a cooldown entry for convID.
func cooldownHolds(ctx context.Context, e *Env, sourceName, convID string) (bool, error) {
	var s agentopsv1alpha1.SignalSource
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: sourceName}, &s); err != nil {
		return false, err
	}
	for _, c := range s.Status.Cooldown {
		if c.InitiationID == convID {
			return true, nil
		}
	}
	return false, nil
}

// assertKubeRecordsUntouched compares the kube custom resources, by VALUE,
// against what they were before the upgrade.
func assertKubeRecordsUntouched(t *testing.T, ctx context.Context, e *Env, before kubeSnapshot) {
	t.Helper()
	convName, convBefore, sigBefore := before.convName, before.conv, before.sig
	sourceName, cooldownBefore := before.sourceName, before.cooldown
	var convAfter agentopsv1alpha1.Conversation
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: convName}, &convAfter); err != nil {
		t.Fatalf("the kube Conversation must still exist, untouched, after the upgrade: %v", err)
	}
	if !reflect.DeepEqual(convBefore.Spec, convAfter.Spec) || !reflect.DeepEqual(convBefore.Status, convAfter.Status) {
		t.Fatalf("the kube Conversation must be byte-identical across the upgrade:\nbefore=%+v\nafter=%+v", convBefore, &convAfter)
	}
	var sigAfter agentopsv1alpha1.Signal
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: sigBefore.Name}, &sigAfter); err != nil {
		t.Fatalf("the kube Signal must still exist, untouched, after the upgrade: %v", err)
	}
	if !reflect.DeepEqual(sigBefore.Spec, sigAfter.Spec) || !reflect.DeepEqual(sigBefore.Status, sigAfter.Status) {
		t.Fatalf("the kube Signal must be byte-identical across the upgrade:\nbefore=%+v\nafter=%+v", sigBefore, &sigAfter)
	}
	var sourceAfter agentopsv1alpha1.SignalSource
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: sourceName}, &sourceAfter); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cooldownBefore, sourceAfter.Status.Cooldown) {
		t.Fatalf("the kube SignalSource's own cooldown record must be byte-identical across the upgrade:\nbefore=%+v\nafter=%+v",
			cooldownBefore, sourceAfter.Status.Cooldown)
	}
}

// 5.6 — a rolling nats image change (the StatefulSet's own RollingUpdate,
// one pod at a time, ordinal descending under podManagementPolicy: Parallel
// — see chart/charts/nats/templates/_helpers.tpl) must never fail a write
// in flight. The image is digest-pinned (design D-8), so bumping only the
// TAG changes the rendered container spec (forcing the rollout) without
// changing what actually runs — the safe way to force one on this pack's
// own single node, with no second real image to pull.
func TestNatsRollingServerImageUpgradeKeepsWritesSucceeding(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	t.Cleanup(func() {
		if err := e.Upgrade(context.Background()); err != nil {
			t.Errorf("restoring the nats image tag: %v", err)
		}
	})

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var failed int32
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			id := fmt.Sprintf("e2e-nats-rolling-%s-%d", stamp, i)
			code, out := postSignalRaw(e, t, SourceTasks, id)
			if code != 200 {
				atomic.AddInt32(&failed, 1)
				t.Logf("write %d during the rolling nats upgrade: %d %s", i, code, out)
			}
			time.Sleep(time.Second)
		}
	}()

	upgradeErr := e.Upgrade(ctx, "nats.image.tag=e2e-rolling-"+stamp)
	close(stop)
	wg.Wait()
	if upgradeErr != nil {
		t.Fatalf("the rolling nats image upgrade: %v", upgradeErr)
	}
	waitNatsStatefulSetReady(t, ctx, e, 3)
	if failed > 0 {
		t.Fatalf("%d write(s) failed during the rolling nats image upgrade", failed)
	}

	t.Cleanup(func() {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return
		}
		for i := range items {
			if strings.HasPrefix(items[i].Spec.Title, "e2e-nats-rolling-"+stamp) ||
				strings.HasPrefix(items[i].Spec.ConversationID, "e2e-nats-rolling-"+stamp) {
				_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: items[i].Name}})
			}
		}
	})
}
