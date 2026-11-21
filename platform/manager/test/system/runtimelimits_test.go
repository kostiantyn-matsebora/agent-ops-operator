//go:build system

package system

// The model's declared limits (`contextWindow`, `maxOutputTokens`) travel from
// chart values to a RUNNING runtime pod: the chart renders them as env on the
// AgentRuntime, the manager copies that env onto the worker container, and the
// kubelet starts the pod with it. The render and the runtime's own use of the
// values are pinned at the chart and conformance tiers; this lane proves the
// path only a real cluster can — that the stored AgentRuntime keeps the env and
// a real pod carries it.
//
// The pi bundle is already ON in this pack (install.go), with the real image
// and a credential Secret no one created. The test points the bundle at the
// pack's own stub image and drops the credential reference, so a pod can run.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func workerEnv(pod corev1.Pod) map[string]string {
	out := map[string]string{}
	for _, c := range pod.Spec.Containers {
		if c.Name != piWorkerContainer {
			continue
		}
		for _, ev := range c.Env {
			out[ev.Name] = ev.Value
		}
	}
	return out
}

func TestRuntimeModelLimitsReachTheRuntimePod(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	// Restore the pack's own install afterwards, whatever happens below. The
	// upgrade rolls the manager, so it goes through UpgradeManager.
	t.Cleanup(func() {
		if err := e.UpgradeManager(context.Background()); err != nil {
			t.Errorf("restoring the install: %v", err)
		}
	})
	if err := e.UpgradeManager(ctx,
		"pi.image=agentops-test-stub-runtime:e2e",
		"pi.credentialsSecret.name=",
		"pi.contextWindow=500000",
		"pi.maxOutputTokens=64000",
	); err != nil {
		t.Fatal(err)
	}

	var rt agentopsv1alpha1.AgentRuntime
	if err := e.K.Get(ctx, client.ObjectKey{Namespace: Namespace, Name: "pi"}, &rt); err != nil {
		t.Fatalf("the pi AgentRuntime: %v", err)
	}
	stored := map[string]string{}
	for _, ev := range rt.Spec.Env {
		stored[ev.Name] = ev.Value
	}
	if stored["PI_CONTEXT_WINDOW"] != "500000" || stored["PI_MAX_TOKENS"] != "64000" {
		t.Fatalf("the stored AgentRuntime must keep the limits as env: %+v", rt.Spec.Env)
	}

	srcName := "e2e-limits-" + stamp
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	p := pipeline("e2e-limits-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "pi"}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-limits-run-" + stamp
	e.PostTask(t, srcName, fp, "echo limits")
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	if run := conv.Status.Runs[0]; run.Status != "succeeded" {
		t.Fatalf("the run on the pi runtime: %+v", run)
	}

	pods, err := e.K.RuntimePods(ctx, "agentops.dev/conversation="+conv.Name)
	if err != nil || len(pods) == 0 {
		t.Fatalf("runtime pod: %v %d", err, len(pods))
	}
	env := workerEnv(pods[0])
	if env["PI_CONTEXT_WINDOW"] != "500000" || env["PI_MAX_TOKENS"] != "64000" {
		var got []string
		for k, v := range env {
			got = append(got, k+"="+v)
		}
		t.Fatalf("the worker container must carry the limits: %s", strings.Join(got, " "))
	}
}
