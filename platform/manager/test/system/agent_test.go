//go:build system

package system

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Section 11 — the real-runtime lane. runtime-claude with a real credential
// is the primary oracle for everything an agent can be asked to demonstrate.
// Assertions are CLOSED-FORM (a nonce, a known name, an unchanged object) and
// tolerant of phrasing; every agent-dependent assertion carries a bounded
// retry whose count is reported, so a lane that passes while retrying
// constantly is visible rather than quietly degrading.
//
// FULL TIER, and SKIPPED without CLAUDE_CODE_OAUTH_TOKEN: a fork pull request has
// no secrets, and that is the intended access boundary.

const (
	agentProfile  = "e2e-agent"
	agentPipeline = "e2e-agent"
	agentSource   = "e2e-agent-tasks"
	agentRuntime  = "claude"
	// A fixed, small budget: every conversation here costs real tokens.
	maxRetries = 2
)

var retries int

func requireAgent(t *testing.T) *Env {
	t.Helper()
	fullTier(t)
	e := requireEnv(t)
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" {
		t.Skip("CLAUDE_CODE_OAUTH_TOKEN is not set — the real-runtime lane is skipped, not failed")
	}
	return e
}

// setupAgent wires the real runtime with a READ-scoped toolset only.
func setupAgent(t *testing.T, e *Env) {
	t.Helper()
	ctx := context.Background()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: "agentops-claude"},
		StringData: map[string]string{"oauthToken": os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")}}
	if err := ensure(ctx, e.K, secret); err != nil {
		t.Fatal(err)
	}
	profile := &agentopsv1alpha1.AgentProfile{ObjectMeta: metav1.ObjectMeta{Name: agentProfile}}
	profile.Spec.Agent = "e2e"
	profile.Spec.OutputFormat = agentopsv1alpha1.OutputFormat("none")
	profile.Spec.SystemPrompt = "You are a terse assistant under test. Answer with the requested value and nothing else."
	profile.Spec.MaxTurns = 6
	if err := ensure(ctx, e.K, profile); err != nil {
		t.Fatal(err)
	}
	if err := ensure(ctx, e.K, source(agentSource, "e2e", nil)); err != nil {
		t.Fatal(err)
	}
	p := pipeline(agentPipeline, agentProfile, []string{agentSource}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: agentRuntime}
	// agentops-observe is filesystem-only (Read/Grep/Glob); the closed-form
	// pod-name assertion below needs the kubernetes bundle's read MCP tools,
	// which the system install renders only under the full tier (install.go's
	// k8sMCP) — the one lane that reaches this test.
	p.Spec.Toolsets = &agentopsv1alpha1.ToolsetBinding{Refs: []agentopsv1alpha1.ObjectRef{
		{Name: "agentops-observe"}, {Name: "k8s-observability"},
	}}
	p.Spec.MCPConfigs = &agentopsv1alpha1.ToolingBinding{Refs: []agentopsv1alpha1.ObjectRef{{Name: "k8s-api"}}}
	if err := ensure(ctx, e.K, p); err != nil {
		t.Fatal(err)
	}
	if err := waitPipelineReady(ctx, e.K, agentPipeline, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
}

// ask runs one turn and returns the result, retrying a bounded number of
// times on a run that FAILED (never on phrasing — the assertion is the
// caller's). Fails fast on a broken deployment rather than burning turns.
func ask(t *testing.T, e *Env, conv, text string, run int) string {
	t.Helper()
	for attempt := 0; ; attempt++ {
		if code, out := e.ConsoleSend(t, conv, text); code/100 != 2 {
			t.Fatalf("send: %d %s", code, out)
		}
		c := e.WaitRun(t, conv, run+attempt, 8*time.Minute)
		r := c.Status.Runs[len(c.Status.Runs)-1]
		if r.Status == "succeeded" {
			return r.Result
		}
		if attempt >= maxRetries {
			t.Fatalf("run failed %d times: %+v", attempt+1, r)
		}
		retries++
		t.Logf("retry %d after a failed run: %s", attempt+1, r.Result)
	}
}

func TestRealRuntime(t *testing.T) {
	e := requireAgent(t)
	setupAgent(t, e)
	ctx := context.Background()
	defer func() { t.Logf("agent-dependent assertions retried %d time(s)", retries) }()

	// 11.2 A closed-form question with a test-known answer.
	fp := "e2e-agent-" + fmt.Sprint(time.Now().UnixNano())
	nonce := fmt.Sprintf("kiwi-%d", time.Now().UnixNano()%1000000)
	e.PostTask(t, agentSource, fp, "Remember this word exactly: "+nonce+". Reply with just OK.")
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	first := e.WaitRun(t, conv.Name, 1, 10*time.Minute)
	if first.Status.Runs[0].Status != "succeeded" {
		t.Fatalf("the first run failed — the deployment is broken, not the agent: %+v", first.Status.Runs[0])
	}

	// 11.1 Continuity by word, across a runtime pod restart. "word", not
	// "token" or "secret" — the model reads either of those literally and
	// refuses to repeat the value back, which is not the boundary this
	// assertion is testing.
	if first.Status.RuntimePod != "" {
		_ = e.K.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: first.Status.RuntimePod}})
		waitFor(t, "the pod to be gone", 2*time.Minute, func() (bool, error) {
			var p corev1.Pod
			return apierrors.IsNotFound(e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: first.Status.RuntimePod}, &p)), nil
		})
	}
	answer := ask(t, e, conv.Name, "What was the word I asked you to remember? Reply with the word only.", 2)
	if !strings.Contains(answer, nonce) {
		t.Fatalf("the exact word must appear after the restart; got %q", answer)
	}

	// 11.2 Closed-form correctness: name the manager pod.
	pods, _ := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	answer = ask(t, e, conv.Name, "Using your Kubernetes read tools, reply with only the name of the pod in namespace "+Namespace+" whose name starts with agentops-manager.", 3)
	if len(pods) == 0 || !strings.Contains(answer, pods[0].Name) {
		t.Fatalf("expected the manager pod name %q in %q", podName(pods), answer)
	}

	// 11.3 Toolset enforcement on EFFECT: a mutation outside the bound
	// toolset does not happen; a bound read succeeds (above), so this is a
	// boundary and not a broken agent.
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: "e2e-do-not-touch"},
		Data: map[string]string{"value": "original"}}
	mustCreate(t, e.K, cm)
	_ = ask(t, e, conv.Name, "Change the ConfigMap e2e-do-not-touch in namespace "+Namespace+" so that its key value equals CHANGED. If you cannot, say so briefly.", 4)
	var got corev1.ConfigMap
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: cm.Name}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Data["value"] != "original" {
		t.Fatalf("the bound toolset is read-scoped; the mutation must not happen, got %q", got.Data["value"])
	}
}

// 9.1 Storage outage through the REAL runtime, not the stub — the design's
// own risk note for this change: continuity reporting alters what feeds the
// breaker in production, so it is pinned against the real claude runtime's
// behaviour here rather than inferred from test/stubruntime's simulated
// "storage-outage" keyword (see assertStorageOutageHoldsWork in
// substrate_test.go for that double's coverage of the breaker itself).
//
// The stub can be TOLD to fail; the real binary cannot, so the outage is
// induced structurally instead: the live context tree is pod-local
// (chart.md's claude bundle — the agent container holds no mount of the
// durable volume at all, only context-sync's sidecar does), so revoking
// read/execute on it from inside the running pod reproduces exactly the
// failure runtimes/claude/runtime.js's sessionFileExists distinguishes from
// a missing context: EACCES, not ENOENT. That is what must surface as
// `continuity: unavailable` — never `continued`, and never silently
// answered — through a real `claude --resume` invocation against it.
func TestRealRuntimeStorageOutageReportsUnavailable(t *testing.T) {
	e := requireAgent(t)
	setupAgent(t, e)
	ctx := context.Background()

	fp := "e2e-agent-outage-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, agentSource, fp, "Remember this word exactly: canary. Reply with just OK.")
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	first := e.WaitRun(t, conv.Name, 1, 10*time.Minute)
	run := first.Status.Runs[0]
	if run.Status != "succeeded" {
		t.Fatalf("the first run failed — the deployment is broken, not the agent: %+v", run)
	}
	pod := first.Status.RuntimePod
	if pod == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}

	// Corrupt the LIVE context store IN THE RUNNING POD. chmod 000 denies
	// even the owning process — the pod runs as a fixed non-root uid
	// (podspec.go's RunAsUser/RunAsGroup) — so the very same `claude`
	// invocation that wrote these files can no longer read them back. The
	// durable snapshot on the context volume is untouched: only this pod's
	// pod-local live tree is broken, exactly the shape an unreadable share
	// (a stale mount, a permission error, a share manager mid-restart) takes
	// from inside one pod.
	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "exec", "-c", "worker", pod, "--",
		"chmod", "000", "/data/context/.claude/projects"); err != nil {
		t.Fatalf("corrupting the pod's live context store: %v\n%s", err, out)
	}

	if code, out := e.ConsoleSend(t, conv.Name, "What was the word I asked you to remember? Reply with the word only."); code/100 != 2 {
		t.Fatalf("console send: %d %s", code, out)
	}
	second := e.WaitRun(t, conv.Name, 2, 8*time.Minute)
	run = second.Status.Runs[len(second.Status.Runs)-1]
	if run.Status != "failed" {
		t.Fatalf("a resume against an unreadable store must FAIL the run, not answer without its history: %+v", run)
	}

	var cond *metav1.Condition
	for i := range second.Status.Conditions {
		if second.Status.Conditions[i].Type == "ContextContinuity" {
			cond = &second.Status.Conditions[i]
		}
	}
	if cond == nil {
		t.Fatalf("the outage must be visible as a ContextContinuity condition: %+v", second.Status.Conditions)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("the real runtime must NEVER report continued through a failed resume against an unreadable store: %+v", *cond)
	}
	if cond.Reason != "ContextUnavailable" && cond.Reason != "ContextStoreUnavailable" {
		t.Fatalf("an unreadable store must report UNAVAILABLE, not a plain loss: %+v", *cond)
	}
}

func podName(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return ""
	}
	return pods[0].Name
}
