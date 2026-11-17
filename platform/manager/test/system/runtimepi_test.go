//go:build system

package system

// Section 9 (runtime-pi, task 9) — a route naming `runtimeRef: pi` proves the
// MANAGER's own generic contract: wiring, pod shape, context continuity and
// tool/MCP composition. None of this needs the real `runtimes/pi` image or a
// PI_API_KEY — exactly as TestImagePullThroughAuthenticatedRegistry
// (substrate_test.go) proves the same contract for a route named
// "e2e-pulled" against the stub image. What the real image does with what it
// is handed is pinned already at the unit tier (`runtimes/pi`'s own
// `node --test` suite) and the Go-side conformance tier
// (`test/conformance/runtimepi_test.go`, section 8.4) — this lane proves only
// what only a real cluster can: that the reconciler, the kubelet and a real
// pod agree with what the manager composed.
//
// 9.4/9.5's OWN JUDGMENT CALL, stated once here rather than at each site:
//
//   - 9.4 ("a bound Bash(kubectl:*) grants no shell and is logged") names a
//     RUNTIME-side behaviour — runtimes/pi's own vocabulary.js (design D2,
//     task 2.5) maps a scoped shell pattern to no built-in tool and logs the
//     narrowing. This lane runs the STUB, which has no vocabulary.js, so it
//     cannot exercise that mapping. What IS provable here, and is the
//     system tier's own obligation rather than a repeat of 2.5's: that the
//     MANAGER'S OWN composed `allowedTools` wire field never upgrades a
//     scoped pattern into a bare "Bash" grant, and that a real pod's own log
//     stream carries a narrowing line for it. The stub's `wiring` directive
//     (test/stubruntime/main.go) was extended to decode the wire fields and
//     log a narrowing in the SAME shape vocabulary.js's own doc comment
//     describes, as a stand-in — not a replication of the real mapping,
//     which stays pinned where it always was.
//
//   - 9.5 ("a bound MCPConfig makes its tools reachable and an unbound
//     server's are not") could be read as a live network reachability
//     check, which would need a real MCP server deployed into the cluster
//     as a NEW fixture. That reachability is already pinned twice: at the
//     unit tier (runtimes/pi/mcp.test.js) and at the Go-side conformance
//     tier (fakepiprovider.go driving the real runtime.js over a real SSE
//     double, section 8.4). Building a THIRD double of the same fact, with
//     no existing fixture to extend (test/fixtures and test/system carry
//     none today), would duplicate coverage rather than add it. This lane
//     instead proves the fact ONLY a real cluster can: that the manager's
//     real reconciler and internal/mcpcompile correctly compile a bound
//     MCPConfig into the exact file (MCP_CONFIG) a real pod mounts and a
//     real runtime process opens, while an unbound MCPConfig's server
//     never reaches that file at all — the delivery pipeline, not the
//     protocol it carries.
//
// Every test below runs on the gating (smoke) tier: everything here uses the
// stub image and core CRDs, the same cost class as TestStubMechanisms and
// TestImagePullThroughAuthenticatedRegistry's own non-full-tier siblings, so
// it is not gated on fullTier(t).

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// piStubContextSyncPath is this lane's own declared ContextSync.Paths entry —
// relative to the runtime's HOME exactly as the real pi bundle's own
// `contextSync.paths` is (chart/charts/pi/values.yaml) — chosen to match
// where test/stubruntime's own doc comment says it keeps its context: a FILE
// per handle under $HOME/.agentops-stub/contexts/.
const piStubContextSyncPath = ".agentops-stub/contexts/**"

// piContextLiveMount, piWorkerContainer and the three init-container names
// are pinned literally rather than imported: they are unexported constants
// of internal/runtimepod (contextLiveMount, "context-sync", egressInitContainer,
// egressProxyContainer in contextsync.go/egress.go), and every existing test
// in this package pins a manager-owned literal the same way (e.g.
// "ContextContinuity" in substrate_test.go/agent_test.go) rather than
// reaching into an internal package for an unexported name.
const (
	piContextLiveMount     = "/data/context"
	piWorkerContainer      = "worker"
	piContextSyncContainer = "context-sync"
	piEgressInitContainer  = "egress-init"
	piEgressProxyContainer = "egress-proxy"
)

// piAgentRuntime builds an AgentRuntime CR naming the pack's own stub image —
// no new image, no registry push, exactly as TestImagePullThroughAuthenticatedRegistry's
// own "e2e-pulled" AgentRuntime does — but declaring BOTH contextSync and
// egressMediation, which that test's own runtime does not, so 9.2 has
// something to find. Both stanzas are opt-in per runtime (podspec.go), and
// creating the AgentRuntime CR directly in Go bypasses the CHART entirely —
// `global.agentops.runtimeDefaults.egressMediation.enabled: true` is a
// chart-template-level merge (agentops.renderRuntime/_helpers.tpl), never
// applied by the reconciler itself (internal/runtimepod.ResolveFor copies
// rt.Spec.EgressMediation/.ContextSync verbatim), so this CR must declare
// both stanzas itself to turn either on.
func piAgentRuntime(name string) *agentopsv1alpha1.AgentRuntime {
	rt := &agentopsv1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: name}}
	rt.Spec.Image = "agentops-test-stub-runtime:e2e"
	rt.Spec.ContextStorage = agentopsv1alpha1.ContextOnVolume
	rt.Spec.IdleTTLMinutes = 1
	rt.Spec.ContextSync = &agentopsv1alpha1.ContextSync{Paths: []string{piStubContextSyncPath}}
	rt.Spec.EgressMediation = &agentopsv1alpha1.EgressMediation{}
	return rt
}

// ---- 9.1 -------------------------------------------------------------------

// 9.1 A route naming `runtimeRef: pi` answers a signal and delivers it to
// EVERY bound thread — console and the telegram surface both, which is the
// part a single-channel route could not show.
func TestPiRuntimeRouteAnswersAndDeliversToEveryBoundThread(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	rtName := "pi-9-1-" + stamp
	mustCreate(t, e.K, piAgentRuntime(rtName))
	srcName := "e2e-pi-answers-" + stamp
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	p := pipeline("e2e-pi-answers-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole, ChannelTelegram})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rtName}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-pi-answers-run-" + stamp
	marker := "pi-route-" + stamp
	e.PostTask(t, srcName, fp, "echo "+marker)
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	if run := conv.Status.Runs[0]; run.Status != "succeeded" || !strings.Contains(run.Result, marker) {
		t.Fatalf("a runtimeRef: pi route must answer a signal: %+v", run)
	}

	waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, 2*time.Minute)
	waitThreadOn(t, ctx, e, conv.Name, ChannelTelegram, 2*time.Minute)
}

// ---- 9.2 -------------------------------------------------------------------

// 9.2 The pod's own shape: the context-sync sidecar and the egress init
// container are present (both opt-in, both declared on this lane's own
// AgentRuntime), and the running pod's own live context file sits under the
// declared contextSync.paths root.
func TestPiRuntimePodShapeHasContextSyncAndEgress(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	rtName := "pi-9-2-" + stamp
	mustCreate(t, e.K, piAgentRuntime(rtName))
	srcName := "e2e-pi-shape-" + stamp
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	p := pipeline("e2e-pi-shape-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rtName}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-pi-shape-run-" + stamp
	e.PostTask(t, srcName, fp, "echo pi-shape")
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	if run := conv.Status.Runs[0]; run.Status != "succeeded" {
		t.Fatalf("first run: %+v", run)
	}
	handle := conv.Status.RuntimeContextID
	if handle == "" {
		t.Fatalf("the run must mint a context handle")
	}
	podName := conv.Status.RuntimePod
	if podName == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}

	// Read the pod WHILE it is still around — idleTtlMinutes: 1 on this
	// lane's own AgentRuntime means it exits itself shortly after.
	pods, err := e.K.RuntimePods(ctx, "agentops.dev/conversation="+conv.Name)
	if err != nil || len(pods) == 0 {
		t.Fatalf("runtime pod: %v %d", err, len(pods))
	}
	pod := pods[0]

	hasInit := func(name string) bool {
		for _, c := range pod.Spec.InitContainers {
			if c.Name == name {
				return true
			}
		}
		return false
	}
	if !hasInit(piContextSyncContainer) {
		t.Fatalf("the context-sync sidecar must be present on a runtime declaring contextSync: %+v", pod.Spec.InitContainers)
	}
	if !hasInit(piEgressInitContainer) {
		t.Fatalf("the egress-init container must be present on a runtime declaring egressMediation: %+v", pod.Spec.InitContainers)
	}
	if !hasInit(piEgressProxyContainer) {
		t.Fatalf("the egress-proxy sidecar must be present on a runtime declaring egressMediation: %+v", pod.Spec.InitContainers)
	}

	var syncEnv []string
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == piContextSyncContainer {
			for _, ev := range pod.Spec.InitContainers[i].Env {
				syncEnv = append(syncEnv, ev.Name+"="+ev.Value)
			}
		}
	}
	if !slices.ContainsFunc(syncEnv, func(kv string) bool {
		return strings.HasPrefix(kv, "CONTEXT_SYNC_PATHS=") && strings.Contains(kv, piStubContextSyncPath)
	}) {
		t.Fatalf("the declared contextSync.paths must reach the sidecar's own env: %v", syncEnv)
	}

	// The declared path is not just ECHOED into the sidecar's env — it is
	// where the stub's OWN session state file actually lives, on the SAME
	// pod, right now: the glob root ".agentops-stub/contexts" the runtime
	// declared is a prefix of the live, relative path the stub's own doc
	// comment names ($HOME/.agentops-stub/contexts/<handle>).
	declaredRoot := strings.TrimSuffix(piStubContextSyncPath, "**")
	liveRelPath := ".agentops-stub/contexts/" + handle
	if !strings.HasPrefix(liveRelPath, declaredRoot) {
		t.Fatalf("the session state must sit under the declared path: %q does not start with %q", liveRelPath, declaredRoot)
	}
	statePath := piContextLiveMount + "/" + liveRelPath
	if out, err := e.Cluster.Kubectl(ctx, "-n", ConversationsNamespace, "exec", "-c", piWorkerContainer, podName, "--",
		"test", "-e", statePath); err != nil {
		t.Fatalf("the session state file must exist under the declared path on the live pod: %v\n%s", err, out)
	}
}

// ---- 9.3 -------------------------------------------------------------------

// 9.3 Context continuity: a second run resumes the SAME context, and
// deleting the session state file in the running pod fails the NEXT run with
// the cannot-be-continued message, rather than answering fresh.
func TestPiRuntimeContextContinuityAndLoss(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	rtName := "pi-9-3-" + stamp
	rt := piAgentRuntime(rtName)
	// Longer than the shared default: this test's own three runs must land
	// on the SAME pod (console sends plus a kubectl exec in between), or an
	// idle-triggered respawn would RESTORE the live tree from the sidecar's
	// last durable checkpoint — which could still carry the handle file this
	// test just deleted, if a checkpoint ran before the deletion and none
	// after — and that race, not the manager's own behaviour, would decide
	// the test. The runtime is unique to this test (stamped name), so
	// widening its own TTL costs nothing else.
	rt.Spec.IdleTTLMinutes = 5
	mustCreate(t, e.K, rt)
	srcName := "e2e-pi-continuity-" + stamp
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	p := pipeline("e2e-pi-continuity-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rtName}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-pi-continuity-run-" + stamp
	e.PostTask(t, srcName, fp, "echo one")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	if run := conv.Status.Runs[0]; run.Status != "succeeded" {
		t.Fatalf("first run: %+v", run)
	}
	handle := conv.Status.RuntimeContextID
	if handle == "" {
		t.Fatalf("the first run must mint a context handle")
	}
	podName := conv.Status.RuntimePod
	if podName == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}

	// A second run resumes the SAME context: no loss reported, same handle.
	if code, out := e.ConsoleSend(t, conv.Name, "echo two"); code/100 != 2 {
		t.Fatalf("console send: %d %s", code, out)
	}
	conv = e.WaitRun(t, conv.Name, 2, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || !strings.Contains(run.Result, "two") {
		t.Fatalf("the second run must succeed and continue: %+v", run)
	}
	if conv.Status.RuntimeContextID != handle {
		t.Fatalf("the handle must survive a resume: %q -> %q", handle, conv.Status.RuntimeContextID)
	}
	for _, c := range conv.Status.Conditions {
		if c.Type == "ContextContinuity" && c.Status == metav1.ConditionFalse {
			t.Fatalf("a resume must not report continuity lost: %s %s", c.Reason, c.Message)
		}
	}

	// Delete the session state FILE directly in the running pod — the SAME
	// pod the first two runs used, since the runtime idles rather than
	// exiting between ordinary runs — then the NEXT run must FAIL with the
	// cannot-be-continued message rather than answer fresh.
	statePath := piContextLiveMount + "/.agentops-stub/contexts/" + handle
	if out, err := e.Cluster.Kubectl(ctx, "-n", ConversationsNamespace, "exec", "-c", piWorkerContainer, podName, "--",
		"rm", "-f", statePath); err != nil {
		t.Fatalf("deleting the session state file: %v\n%s", err, out)
	}
	if code, out := e.ConsoleSend(t, conv.Name, "echo three"); code/100 != 2 {
		t.Fatalf("console send: %d %s", code, out)
	}
	conv = e.WaitRun(t, conv.Name, 3, 4*time.Minute)
	run = conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "failed" {
		t.Fatalf("a deleted session state must FAIL the next run, not answer fresh: %+v", run)
	}
	if !strings.Contains(run.Result, "refusing to continue without it") {
		t.Fatalf("the failure must carry the cannot-be-continued message: %+v", run)
	}
	var cond *metav1.Condition
	for i := range conv.Status.Conditions {
		if conv.Status.Conditions[i].Type == "ContextContinuity" {
			cond = &conv.Status.Conditions[i]
		}
	}
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("the loss must be visible as a ContextContinuity condition: %+v", conv.Status.Conditions)
	}
	if cond.Reason != "ContextUnavailable" {
		t.Fatalf("a single conversation's own lost context must report ContextUnavailable, not an install-wide outage: %+v", *cond)
	}
	if !strings.Contains(cond.Message, handle) {
		t.Fatalf("the condition message must name the lost handle: %+v", *cond)
	}
}

// ---- 9.4 -------------------------------------------------------------------

// 9.4 Tools: with observation-only toolsets the shell is absent on one route
// while the same profile keeps it on another, and a bound Bash(kubectl:*)
// grants no [bare] shell and is logged. See this file's own top-of-file
// comment for the judgment call on what "logged" can mean at this tier.
func TestPiRuntimeToolsetsNarrowAndScopeTheShell(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	rtName := "pi-9-4-" + stamp
	mustCreate(t, e.K, piAgentRuntime(rtName))

	scoped := &agentopsv1alpha1.MCPToolset{ObjectMeta: metav1.ObjectMeta{Name: "e2e-pi-kubectl-scoped-" + stamp}}
	scoped.Spec.Tools = []string{"Bash(kubectl:*)"}
	mustCreate(t, e.K, scoped)

	wiringResultOn := func(routeSuffix string, toolsets *agentopsv1alpha1.ToolsetBinding) string {
		t.Helper()
		srcName := "e2e-pi-tools-" + routeSuffix + "-" + stamp
		mustCreate(t, e.K, source(srcName, "e2e", nil))
		p := pipeline("e2e-pi-tools-"+routeSuffix+"-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole})
		p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rtName}
		p.Spec.Toolsets = toolsets
		mustCreate(t, e.K, p)
		if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
			t.Fatal(err)
		}
		fp := "e2e-pi-tools-" + routeSuffix + "-run-" + stamp
		e.PostTask(t, srcName, fp, "wiring")
		conv := e.ConversationFor(t, fp, time.Minute)
		conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
		run := conv.Status.Runs[0]
		if run.Status != "succeeded" {
			t.Fatalf("route %s: %+v", routeSuffix, run)
		}
		if routeSuffix == "kubectl-scoped" {
			podName := conv.Status.RuntimePod
			if podName == "" {
				t.Fatalf("no runtime pod recorded on %s", conv.Name)
			}
			logs, err := e.Cluster.Kubectl(ctx, "-n", ConversationsNamespace, "logs", podName, "-c", piWorkerContainer)
			if err != nil {
				t.Fatalf("reading the runtime pod's own logs: %v", err)
			}
			if !strings.Contains(logs, "narrowed") {
				t.Fatalf("a scoped shell pattern must be LOGGED as a narrowing: %s", logs)
			}
		}
		return run.Result
	}

	observeOnly := wiringResultOn("observe-only",
		&agentopsv1alpha1.ToolsetBinding{Mode: "overwrite", Refs: []agentopsv1alpha1.ObjectRef{{Name: "agentops-observe"}}})
	withShell := wiringResultOn("with-shell",
		&agentopsv1alpha1.ToolsetBinding{Mode: "overwrite", Refs: []agentopsv1alpha1.ObjectRef{{Name: "agentops-shell"}}})
	kubectlScoped := wiringResultOn("kubectl-scoped",
		&agentopsv1alpha1.ToolsetBinding{Mode: "overwrite", Refs: []agentopsv1alpha1.ObjectRef{{Name: scoped.Name}}})

	if slices.Contains(allowedToolsFrom(observeOnly), "Bash") {
		t.Fatalf("an observation-only toolset must not grant a shell: %q", observeOnly)
	}
	if !slices.Contains(allowedToolsFrom(withShell), "Bash") {
		t.Fatalf("the shell toolset must grant Bash on its own route: %q", withShell)
	}
	tools := allowedToolsFrom(kubectlScoped)
	if slices.Contains(tools, "Bash") {
		t.Fatalf("a scoped Bash(kubectl:*) pattern must never compose into a bare Bash grant: %q", kubectlScoped)
	}
	if !slices.Contains(tools, "Bash(kubectl:*)") {
		t.Fatalf("the scoped pattern itself must still reach the runtime: %q", kubectlScoped)
	}
}

// allowedToolsFrom reads the comma-separated allowlist out of the stub's own
// `wiring` directive result ("[stub] wiring allowedTools=[...] toolsMode=[...] mcp=[...]").
func allowedToolsFrom(result string) []string {
	const marker = "allowedTools=["
	i := strings.Index(result, marker)
	if i < 0 {
		return nil
	}
	rest := result[i+len(marker):]
	j := strings.Index(rest, "]")
	if j < 0 || rest[:j] == "" {
		return nil
	}
	return strings.Split(rest[:j], ",")
}

// mcpKeysFrom reads the MCP server keys out of the stub's own `wiring`
// directive result.
func mcpKeysFrom(result string) []string {
	const marker = "mcp=["
	i := strings.Index(result, marker)
	if i < 0 {
		return nil
	}
	rest := result[i+len(marker):]
	j := strings.Index(rest, "]")
	if j < 0 || rest[:j] == "" {
		return nil
	}
	return strings.Split(rest[:j], ",")
}

// ---- 9.5 -------------------------------------------------------------------

// 9.5 MCP: a bound MCPConfig's server key reaches the file the runtime
// actually mounts and reads (MCP_CONFIG), while an unbound server's key —
// from a REAL MCPConfig CR that exists in the same cluster — never does. See
// this file's own top-of-file comment for why this is the delivery pipeline,
// not the protocol, and why that is the right thing to prove here.
func TestPiRuntimeMCPBindingReachesTheMountedConfigAndExcludesTheUnbound(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	rtName := "pi-9-5-" + stamp
	mustCreate(t, e.K, piAgentRuntime(rtName))

	bound := &agentopsv1alpha1.MCPConfig{ObjectMeta: metav1.ObjectMeta{Name: "e2e-pi-mcp-bound-" + stamp}}
	bound.Spec.Servers = map[string]agentopsv1alpha1.MCPServer{
		"pibound": {Type: "stdio", Command: "true"},
	}
	mustCreate(t, e.K, bound)
	// A REAL MCPConfig CR that exists in the cluster but is bound to NOTHING
	// this test wires — its server must never reach the mounted file.
	unbound := &agentopsv1alpha1.MCPConfig{ObjectMeta: metav1.ObjectMeta{Name: "e2e-pi-mcp-unbound-" + stamp}}
	unbound.Spec.Servers = map[string]agentopsv1alpha1.MCPServer{
		"piunbound": {Type: "stdio", Command: "true"},
	}
	mustCreate(t, e.K, unbound)

	srcName := "e2e-pi-mcp-" + stamp
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	p := pipeline("e2e-pi-mcp-"+stamp, ProfileStub, []string{srcName}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rtName}
	p.Spec.MCPConfigs = &agentopsv1alpha1.ToolingBinding{Refs: []agentopsv1alpha1.ObjectRef{{Name: bound.Name}}}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-pi-mcp-run-" + stamp
	e.PostTask(t, srcName, fp, "wiring")
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[0]
	if run.Status != "succeeded" {
		t.Fatalf("the wiring run: %+v", run)
	}

	keys := mcpKeysFrom(run.Result)
	if !slices.Contains(keys, "pibound") {
		t.Fatalf("a bound MCPConfig's server must reach the mounted mcp.json: %q (%v)", run.Result, keys)
	}
	if slices.Contains(keys, "piunbound") {
		t.Fatalf("an unbound MCPConfig's server must NEVER reach it, even though the CR exists: %q (%v)", run.Result, keys)
	}
}
