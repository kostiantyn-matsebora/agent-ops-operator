// Capability resolution after capabilities left the AgentProfile: what an agent
// may do comes only from the Pipeline routing it, and — for conversations with
// no routing pipeline — from the profile's capability-only baseline (design D1).
package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

// postTask fires a task and returns the created conversation's name, cleaning
// it up afterwards.
func postTask(t *testing.T, body map[string]any) string {
	t.Helper()
	rec := adapterReq(apiServer(), "POST", "/task", body, "")
	if rec.Code != 202 {
		t.Fatalf("task: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Conversation string `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, created.Conversation) })
	return created.Conversation
}

func convSpec(t *testing.T, name string) agentopsv1alpha1.ConversationSpec {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	return conv.Spec
}

// The five-minute demo is a bare POST /task with no pipeline. Without the
// baseline that agent would have no tools at all, so this is the assertion that
// keeps the README's onboarding flow honest.
func TestBareTaskResolvesTheProfileBaseline(t *testing.T) {
	mkIdentityProfile(t, "prof-baseline")
	mkToolset(t, "baseline-ts", "Read", "Bash")
	mkMCPConfig(t, "baseline-cfg", "victorialogs", "http://vl/sse")
	mkCapabilityPipeline(t, "baseline-pipe", "prof-baseline", bind("baseline-ts"), bind("baseline-cfg"))

	name := postTask(t, map[string]any{"profile": "prof-baseline", "task": "why is pod X crashlooping?"})
	spec := convSpec(t, name)
	if spec.Toolsets == nil || spec.Toolsets.Refs[0].Name != "baseline-ts" {
		t.Fatalf("bare /task must pick up the baseline toolsets: %+v", spec.Toolsets)
	}
	if spec.MCPConfigs == nil || spec.MCPConfigs.Refs[0].Name != "baseline-cfg" {
		t.Fatalf("bare /task must pick up the baseline mcpConfigs: %+v", spec.MCPConfigs)
	}
	if got := dispatchedAllowedTools(t, name); got != "Read,Bash" {
		t.Fatalf("baseline tools must reach the work unit: %q", got)
	}
}

// A profile nobody declared a baseline for is genuinely unwired: it gets
// nothing, exactly as an unclaimed signal source routes nothing.
func TestBareTaskWithoutBaselineHasNoCapabilities(t *testing.T) {
	mkIdentityProfile(t, "prof-nobaseline")

	name := postTask(t, map[string]any{"profile": "prof-nobaseline", "task": "anything"})
	spec := convSpec(t, name)
	if spec.Toolsets != nil || spec.MCPConfigs != nil {
		t.Fatalf("no baseline means no bindings: %+v %+v", spec.Toolsets, spec.MCPConfigs)
	}
	if got := dispatchedAllowedTools(t, name); got != "" {
		t.Fatalf("an unwired profile must grant nothing, got %q", got)
	}
}

// A routing pipeline wins when named, and is NEVER picked up implicitly — that
// ambiguity is exactly why resolving the oldest routing pipeline was rejected.
func TestRoutingPipelineOverridesBaselineAndIsNeverImplicit(t *testing.T) {
	mkIdentityProfile(t, "prof-override")
	mkToolset(t, "ovr-base-ts", "Read")
	mkToolset(t, "ovr-route-ts", "Bash")
	mkCapabilityPipeline(t, "ovr-baseline", "prof-override", bind("ovr-base-ts"), nil)
	// The routing pipeline is distinguished by a SOURCE, not a channel: a
	// channel-bound conversation waits for a thread binding before dispatching,
	// and a pipeline with neither sources nor channels would itself be a second
	// baseline.
	mkSignalSource(t, "ovr-src", "ovr-sig", "")
	mkToolPipeline(t, "ovr-route", []string{"ovr-src"}, nil, "prof-override", bind("ovr-route-ts"), nil)
	reconcilePipeline(t, "ovr-route")

	routed := postTask(t, map[string]any{"profile": "prof-override", "task": "routed", "pipeline": "ovr-route"})
	if got := dispatchedAllowedTools(t, routed); got != "Bash" {
		t.Fatalf("a named routing pipeline must win: %q", got)
	}

	bare := postTask(t, map[string]any{"profile": "prof-override", "task": "bare"})
	if got := dispatchedAllowedTools(t, bare); got != "Read" {
		t.Fatalf("bare /task must take the baseline, never a routing pipeline: %q", got)
	}
}

// Two baselines for one profile: neither applies and both say why. Guessing
// which the operator meant is worse than granting nothing.
func TestDuplicateBaselinesRefuseRatherThanGuess(t *testing.T) {
	mkIdentityProfile(t, "prof-dup")
	mkToolset(t, "dup-a", "Read")
	mkToolset(t, "dup-b", "Bash")
	mkCapabilityPipeline(t, "dup-one", "prof-dup", bind("dup-a"), nil)
	mkCapabilityPipeline(t, "dup-two", "prof-dup", bind("dup-b"), nil)

	for _, name := range []string{"dup-one", "dup-two"} {
		p := reconcilePipeline(t, name)
		cond := apimeta.FindStatusCondition(p.Status.Conditions, controller.ConditionBaselineConflict)
		if cond == nil || cond.Status != "True" || !strings.Contains(cond.Message, "prof-dup") {
			t.Fatalf("%s must report the duplicate baseline: %+v", name, p.Status.Conditions)
		}
	}

	name := postTask(t, map[string]any{"profile": "prof-dup", "task": "x"})
	if got := dispatchedAllowedTools(t, name); got != "" {
		t.Fatalf("an ambiguous baseline must grant nothing, got %q", got)
	}
}

// Every MCP-bound conversation owns its ConfigMap; no profile-keyed document
// exists to collide over, because profiles declare no MCP.
func TestBoundConversationOwnsItsConfigMap(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-cm")
	mkMCPConfig(t, "cm-logs", "victorialogs", "http://vl/sse")
	mkConv(t, "cm-conv", "prof-cm", nil, bind("cm-logs"))
	reconcile(t, "cm-conv")

	name := mountedMCPConfigMap(t, "cm-conv")
	if name != "agentops-mcp-conv-cm-conv" {
		t.Fatalf("conversation-owned ConfigMap expected, got %q", name)
	}
	if keys := mcpServerKeys(t, name); !keys["victorialogs"] || len(keys) != 1 {
		t.Fatalf("bound configs are the whole MCP: %v", keys)
	}
	var cm corev1.ConfigMap
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		t.Fatal(err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Kind != "Conversation" {
		t.Fatalf("must be conversation-owned for GC: %+v", cm.OwnerReferences)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "agentops-mcp-prof-cm"}, &cm); err == nil {
		t.Fatal("no profile-keyed ConfigMap may exist — profiles declare no MCP")
	}
}

// A hand-written mcp.json is opaque, so binding it alongside another config
// refuses loudly rather than mounting one side and dropping the other.
func TestRawConfigNotExclusiveFailsVisibly(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-raw")
	mkRawMCPConfig(t, "raw-hand", "hand-written-mcp")
	mkMCPConfig(t, "raw-other", "victorialogs", "http://vl/sse")
	mkConv(t, "raw-conv", "prof-raw", nil, bind("raw-hand", "raw-other"))

	err := reconcileErr("raw-conv")
	if err == nil || !strings.Contains(err.Error(), "raw-hand") {
		t.Fatalf("combining a raw config must fail naming it: %v", err)
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "raw-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" || cond.Reason != "RawConfigNotExclusive" {
		t.Fatalf("must surface on the conversation: %+v", conv.Status.Conditions)
	}

	// bound alone, the escape hatch still works
	mkConv(t, "raw-alone", "prof-raw", nil, bind("raw-hand"))
	if err := reconcileErr("raw-alone"); err != nil {
		t.Fatalf("a raw config bound alone must work: %v", err)
	}
}

// Dispatch reads the bound toolsets, so editing one reaches running
// conversations without a pod restart.
func TestToolsetEditsReachRunningConversations(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-edit")
	mkToolset(t, "edit-ts", "Grep")
	mkConv(t, "edit-conv", "prof-edit", bind("edit-ts"), nil)

	if got := dispatchedAllowedTools(t, "edit-conv"); got != "Grep" {
		t.Fatalf("initial allowlist: %q", got)
	}
	var ts agentopsv1alpha1.MCPToolset
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "edit-ts"}, &ts); err != nil {
		t.Fatal(err)
	}
	ts.Spec.Tools = []string{"Grep", "mcp__victorialogs__*"}
	if err := k8sClient.Update(ctx, &ts); err != nil {
		t.Fatal(err)
	}
	clearInflight(t, "edit-conv")
	if got := dispatchedAllowedTools(t, "edit-conv"); got != "Grep,mcp__victorialogs__*" {
		t.Fatalf("toolset edit must reach the next work unit: %q", got)
	}
}

// A binding that stops resolving fails the dispatch visibly rather than
// quietly handing the agent fewer tools than its wiring promised.
func TestMissingToolsetRefFailsVisibly(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-gone")
	mkConv(t, "gone-conv", "prof-gone", bind("never-existed"), nil)

	rec := adapterReq(apiServer(), "GET", "/work?convo=gone-conv&wait=0", nil, "")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "never-existed") {
		t.Fatalf("missing toolset must fail the dispatch naming the ref: %d %s", rec.Code, rec.Body.String())
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "gone-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" {
		t.Fatalf("failure must be visible on the conversation: %+v", conv.Status.Conditions)
	}
}
