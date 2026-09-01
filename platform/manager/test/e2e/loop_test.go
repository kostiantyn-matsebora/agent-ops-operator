//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Section 8 — the signal loop breaker, under a runtime image that cannot
// start. The cycle it guards: a runtime pod that cannot start emits a Warning
// event, the event becomes a signal, the signal opens a Conversation, the
// Conversation creates another runtime pod under a NEW name, forever. Nothing
// downstream catches it — the fingerprint is fresh, the workload is fresh —
// so it is bounded ONLY by signal-k8s-events' three self-exclusion
// mechanisms, which need a real pod that really fails and a real Event
// stream to be exercised at all. FULL TIER: it watches for minutes.
func TestSignalLoopBreaker(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()

	srcName := setupBrokenRuntimePipeline(t, ctx, e)
	before, err := e.K.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conv := postTaskAndWaitPodFailsToStart(t, ctx, e, srcName)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	assertFailingPodEmitsWarningEvents(t, ctx, e)
	assertLoopStaysBoundedAcrossWindow(t, ctx, e, len(before)+1)
}

// setupBrokenRuntimePipeline (8.1): a runtime that cannot start, and a
// Pipeline pointed at it. Returns the source name to post tasks to.
func setupBrokenRuntimePipeline(t *testing.T, ctx context.Context, e *Env) string {
	t.Helper()
	broken := &agentopsv1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: "e2e-broken"}}
	broken.Spec.Image = "agentops-e2e-runtime-that-does-not-exist:never"
	broken.Spec.ContextStorage = agentopsv1alpha1.ContextStorage("none")
	broken.Spec.IdleTTLMinutes = 1
	mustCreate(t, e.K, broken)
	src := source("e2e-broken-tasks", "e2e", nil)
	mustCreate(t, e.K, src)
	p := pipeline("e2e-broken", ProfileStub, []string{src.Name}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: broken.Name}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	return src.Name
}

// postTaskAndWaitPodFailsToStart: the pod really fails — ErrImagePull /
// ImagePullBackOff Warning events in the operator's own namespace, about a
// pod named agentops-conv-*.
func postTaskAndWaitPodFailsToStart(t *testing.T, ctx context.Context, e *Env, srcName string) *agentopsv1alpha1.Conversation {
	t.Helper()
	fp := "e2e-loop-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, srcName, fp, "echo never")
	conv := e.ConversationFor(t, fp, time.Minute)
	waitFor(t, "the runtime pod to fail to start", 3*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil || c.Status.RuntimePod == "" {
			return false, err
		}
		pods, err := e.K.Pods(ctx, "agentops.dev/conversation="+conv.Name)
		if err != nil || len(pods) == 0 {
			return false, err
		}
		for _, cs := range pods[0].Status.ContainerStatuses {
			if cs.State.Waiting != nil && strings.Contains(cs.State.Waiting.Reason, "ImagePull") {
				return true, nil
			}
		}
		return false, nil
	})
	return conv
}

func assertFailingPodEmitsWarningEvents(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	events, _ := e.Cluster.Kubectl(ctx, "-n", Namespace, "get", "events", "--field-selector", "type=Warning", "-o", "name")
	if !strings.Contains(events, "event/") {
		t.Fatalf("the failing pod must be emitting Warning events for the test to mean anything")
	}
}

// assertLoopStaysBoundedAcrossWindow (8.2 + 8.3): bounded count AND bounded
// rate across the window — a slow leak fails — while, half-way through, the
// adapter is restarted (cold cache) to prove the name-prefix mechanism holds
// before the object cache is warm.
func assertLoopStaysBoundedAcrossWindow(t *testing.T, ctx context.Context, e *Env, created int) {
	t.Helper()
	window := 4 * time.Minute
	deadline := time.Now().Add(window)
	maxSeen := created
	for time.Now().Before(deadline) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > maxSeen {
			maxSeen = len(items)
		}
		assertNoSelfSignalConversation(t, items)
		maybeRestartEventsAdapterAtHalfway(t, ctx, e, deadline, window)
		time.Sleep(10 * time.Second)
	}
	after, _ := e.K.Conversations(ctx)
	grew := len(after) - created
	if grew > 0 || maxSeen > created {
		t.Fatalf("the conversation count must stay bounded: created %d, max seen %d, now %d", created, maxSeen, len(after))
	}
	pods, _ := e.K.Pods(ctx, "agentops.dev/signal-adapter=k8s-events")
	if len(pods) == 0 {
		t.Fatalf("the events adapter must be running again after its restart")
	}
}

func assertNoSelfSignalConversation(t *testing.T, items []agentopsv1alpha1.Conversation) {
	t.Helper()
	for _, c := range items {
		if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceEvents {
			if strings.Contains(c.Spec.Title+c.Spec.Signature, "agentops-") || strings.Contains(c.Spec.Title+c.Spec.Signature, Namespace) {
				t.Fatalf("a conversation was opened from an event about agent-ops' own machinery: %s %q", c.Name, c.Spec.Title)
			}
		}
	}
}

// maybeRestartEventsAdapterAtHalfway (8.3): cold cache — half-way through,
// restart the adapter while the pod keeps failing — the name-prefix
// mechanism must hold before the object cache is warm.
func maybeRestartEventsAdapterAtHalfway(t *testing.T, ctx context.Context, e *Env, deadline time.Time, window time.Duration) {
	t.Helper()
	if time.Until(deadline) >= window/2 || restarted(t) {
		return
	}
	pods, _ := e.K.Pods(ctx, "agentops.dev/signal-adapter=k8s-events")
	for _, pod := range pods {
		_ = e.K.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: pod.Name}})
	}
	markRestarted(t)
}

var restartedFlag = map[string]bool{}

func restarted(t *testing.T) bool { return restartedFlag[t.Name()] }
func markRestarted(t *testing.T)  { restartedFlag[t.Name()] = true }
