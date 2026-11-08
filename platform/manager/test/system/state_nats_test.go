//go:build system

package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// nats-state-store-default section 5 (2-4): the server bundle under a real
// kubelet. The chart's own default install already runs THREE nats replicas
// (design D-8) on this pack's single-node cluster — soft anti-affinity, so
// nothing here needs to scale anything up first, unlike replicas_test.go's
// own manager lanes.

const natsSelector = "app.kubernetes.io/name=agentops-nats"

func natsPods(t *testing.T, ctx context.Context, e *Env) []corev1.Pod {
	t.Helper()
	pods, err := e.K.Pods(ctx, natsSelector)
	if err != nil {
		t.Fatal(err)
	}
	return pods
}

// waitNatsStatefulSetReady polls until the StatefulSet reports at least n
// ready replicas.
func waitNatsStatefulSetReady(t *testing.T, ctx context.Context, e *Env, n int32) {
	t.Helper()
	waitFor(t, fmt.Sprintf("the nats StatefulSet to report %d ready replica(s)", n), 5*time.Minute, func() (bool, error) {
		var sts appsv1.StatefulSet
		if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-nats"}, &sts); err != nil {
			return false, err
		}
		return sts.Status.ReadyReplicas >= n, nil
	})
}

// scaleNatsStatefulSet patches the StatefulSet's own replica count — a
// DELETE's replacement would just be recreated by the controller, where a
// scale-down actually withdraws the server for as long as the caller holds
// it there (5.3's own outage window).
func scaleNatsStatefulSet(t *testing.T, ctx context.Context, e *Env, n int32) {
	t.Helper()
	var sts appsv1.StatefulSet
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-nats"}, &sts); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(sts.DeepCopy())
	sts.Spec.Replicas = &n
	if err := e.K.Patch(ctx, &sts, patch); err != nil {
		t.Fatal(err)
	}
	if n >= 2 {
		waitNatsStatefulSetReady(t, ctx, e, n)
		return
	}
	// Below quorum: the survivor(s) never pass their own readiness probe
	// (no meta leader), so this waits for the pod COUNT to settle instead.
	waitFor(t, fmt.Sprintf("the nats StatefulSet to settle at %d pod(s)", n), 3*time.Minute, func() (bool, error) {
		return int32(len(natsPods(t, ctx, e))) <= n, nil
	})
}

// deleteOneNatsPod force-deletes exactly one server pod and waits for the
// StatefulSet's own replacement to rejoin and report Ready — never leaving
// the cluster one short for whatever lane runs after this one.
func deleteOneNatsPod(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	pods := natsPods(t, ctx, e)
	if len(pods) == 0 {
		t.Fatal("no nats pods found to delete")
	}
	victim := pods[0]
	if err := e.K.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: victim.Name}},
		client.GracePeriodSeconds(0)); err != nil {
		t.Fatal(err)
	}
	waitNatsStatefulSetReady(t, ctx, e, int32(len(pods)))
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func postSignalRaw(e *Env, t *testing.T, source, initiationID string) (int, string) {
	t.Helper()
	body := mustJSON(map[string]any{"source": source, "signals": []any{
		map[string]any{"initiationId": initiationID, "kind": "task", "payload": "echo " + initiationID, "title": initiationID},
	}})
	return e.do(t, "POST", e.Manager.URL()+"/signal/inbound", body, bearerPrefix+e.Values.AdapterToken)
}

func conversationExistsFor(ctx context.Context, e *Env, id string) (bool, error) {
	items, err := e.K.Conversations(ctx)
	if err != nil {
		return false, err
	}
	for i := range items {
		if items[i].Spec.Title == id || items[i].Spec.ConversationID == id {
			return true, nil
		}
	}
	return false, nil
}

// 5.2 — one server pod force-deleted while a conversation is genuinely
// in flight (the stub's own `hold` directive delays its report, so the
// manager has an inflight run with nothing yet processed) must not lose the
// posted input, must still finish the run, and the manager's own readiness
// (gated on the nats connection AND the import marker, store.go's
// natsReadinessCheck) must survive it: one of three down is comfortably
// inside quorum.
func TestNatsServerPodDeletionMidConversationCompletes(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	fp := "e2e-nats-pod-delete-" + stamp

	e.PostTask(t, SourceTasks, fp, "hold 20")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	waitFor(t, "the held run to be inflight", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		return c.Status.Inflight != nil, nil
	})

	deleteOneNatsPod(t, ctx, e)

	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[0]
	if run.Status != "succeeded" {
		t.Fatalf("the held run must still complete across a server pod's death: %+v", run)
	}
	recorded := false
	for _, in := range run.Inputs {
		if strings.Contains(in.Text, "hold 20") {
			recorded = true
		}
	}
	if !recorded {
		t.Fatalf("no input lost: the posted input must be recorded on the completed run: %+v", run)
	}
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 1)
}

// 5.3 — losing quorum (two of three servers down) must hold work rather
// than lose it: a write attempted during the outage must not be silently
// accepted, and a read through /state/* must say so rather than answer a
// stale view — never asserted as one exact status code, since nothing in
// design D-4/D-8 pins one; only that neither path reports success while
// quorum is down. Once the third server is restored, the pre-outage record
// must still read back correctly and the held write must resume on its own.
func TestNatsTwoServersDownHoldsWorkThenResumes(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	baselineID := "e2e-nats-outage-baseline-" + stamp
	e.PostTask(t, SourceTasks, baselineID, "echo "+baselineID)
	baseline := e.ConversationFor(t, baselineID, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: baseline.Name}})
	})
	baseline = e.WaitRun(t, baseline.Name, 1, 4*time.Minute)

	t.Cleanup(func() {
		scaleNatsStatefulSet(t, context.Background(), e, 3)
	})
	scaleNatsStatefulSet(t, ctx, e, 1)

	outageID := "e2e-nats-outage-write-" + stamp
	waitFor(t, "a write attempted during the outage to be refused rather than silently accepted", 2*time.Minute, func() (bool, error) {
		code, _ := postSignalRaw(e, t, SourceTasks, outageID)
		return code != 200, nil
	})
	waitFor(t, "/state/* to report the outage rather than answer stale", time.Minute, func() (bool, error) {
		code, _ := e.do(t, "GET", e.Manager.URL()+"/state/conversations", nil, bearerPrefix+e.Values.AdapterToken)
		return code != 200, nil
	})

	scaleNatsStatefulSet(t, ctx, e, 3)

	waitFor(t, "the pre-outage conversation to read back through /state/* once quorum returns", 3*time.Minute, func() (bool, error) {
		code, out := e.do(t, "GET", e.Manager.URL()+"/state/conversations/"+baseline.Name, nil, bearerPrefix+e.Values.AdapterToken)
		return code == 200 && strings.Contains(out, baselineID), nil
	})
	waitFor(t, "the held write to resume once quorum returns", 3*time.Minute, func() (bool, error) {
		return conversationExistsFor(ctx, e, outageID)
	})
	t.Cleanup(func() {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return
		}
		for i := range items {
			if items[i].Spec.Title == outageID || items[i].Spec.ConversationID == outageID {
				_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: items[i].Name}})
			}
		}
	})
}

// 5.4 — the PodDisruptionBudget design D-8 renders (chart task 3.1) actually
// exists on the live cluster and still names maxUnavailable: 1. This pack's
// own cluster is single-node (cluster.go's `k3d cluster create --no-lb`, one
// node): every replica already shares that node's one failure domain
// regardless of the budget, so draining a node that holds all three proves
// nothing an eviction-blocking assertion would not already prove by
// deleting a second pod outright — which TestNatsTwoServersDownHoldsWorkThenResumes
// already does, at the StatefulSet level. The drain itself is therefore NOT
// EXERCISED here; only the object the chart renders is.
func TestNatsPodDisruptionBudgetAllowsOnlyOneDown(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	var pdb policyv1.PodDisruptionBudget
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-nats"}, &pdb); err != nil {
		t.Fatalf("the nats PodDisruptionBudget must exist: %v", err)
	}
	if pdb.Spec.MaxUnavailable == nil || pdb.Spec.MaxUnavailable.IntValue() != 1 {
		t.Fatalf("maxUnavailable must be 1, got %+v", pdb.Spec.MaxUnavailable)
	}
}
