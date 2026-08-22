package dispatch

import (
	"strings"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func inlineResolver(item agentopsv1alpha1.InputItem) (string, error) { return item.Payload, nil }

func conv(inputs ...agentopsv1alpha1.InputItem) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name = "c1"
	c.Spec.Inputs = inputs
	return c
}

func profile() *agentopsv1alpha1.AgentProfile {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Spec.Agent = "ha-engineer"
	p.Spec.MaxTurns = 60
	return p
}

var now = time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)

// dispatchNext runs a work unit with a fixed allowlist. Capability resolution
// is the caller's job now (it holds the client that reads the bound toolsets),
// so these tests pin prompt/lane behavior only.
func dispatchNext(c *agentopsv1alpha1.Conversation, p *agentopsv1alpha1.AgentProfile) (WorkUnit, []string, bool, error) {
	// Passing c.ContextID() is what the manager does, so tests that set the
	// RETIRED field also prove the dual-read fallback keeps working.
	return Next(c, p, Tooling{AllowedTools: "Read,Bash", Mode: agentopsv1alpha1.ToolsModeMerge}, inlineResolver, now, c.ContextID())
}

func TestTaskUsesBuiltinTemplate(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "check vacuum"})
	u, ids, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if len(ids) != 1 || ids[0] != "i1" {
		t.Fatalf("ids: %v", ids)
	}
	if u.PromptFile != "" || !strings.Contains(u.PromptText, "check vacuum") || !strings.Contains(u.PromptText, "ha-engineer") {
		t.Fatalf("built-in template not rendered: %+v", u)
	}
	if u.ResumeSessionID != "" {
		t.Fatal("task must be a fresh session")
	}
}

// TestQueuedAgentOverrideStillDispatches pins the DEPRECATED dual-read.
// Nothing writes InputItem.Agent any more, but an input queued before the
// upgrade must still reach the agent it was parsed with — changing agent
// mid-conversation because the manager restarted would be worse than the
// retired field. Delete this test when the field goes.
func TestQueuedAgentOverrideStillDispatches(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x", Agent: "node-doctor"})
	u, _, _, _ := dispatchNext(c, profile())
	if !strings.Contains(u.PromptText, "node-doctor") {
		t.Fatalf("queued agent override ignored:\n%s", u.PromptText)
	}
}

// TestNewInputUsesProfileAgent is the post-change shape: an input carrying no
// agent — which is every input created from now on — runs the agent the
// PROFILE declares. That is the whole point of removing the override: the
// agent comes from the wiring, never from whoever typed the message.
func TestNewInputUsesProfileAgent(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if want := profile().Spec.Agent; u.Agent != want {
		t.Fatalf("agent: want profile's %q, got %q", want, u.Agent)
	}
}

func TestProfilePromptWins(t *testing.T) {
	p := profile()
	p.Spec.Prompt = "scripts/custom.md"
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, _, _ := dispatchNext(c, p)
	if u.PromptFile != "scripts/custom.md" || u.PromptText != "" {
		t.Fatalf("profile prompt must win: %+v", u)
	}
	if u.PromptVars["USER_TASK"] != "x" {
		t.Fatalf("vars: %v", u.PromptVars)
	}
}

func TestReplyBatchingAndResume(t *testing.T) {
	c := conv(
		agentopsv1alpha1.InputItem{ID: "r1", Type: agentopsv1alpha1.InputReply, Payload: "first"},
		agentopsv1alpha1.InputItem{ID: "r2", Type: agentopsv1alpha1.InputReply, Payload: "second"},
		agentopsv1alpha1.InputItem{ID: "a1", Type: agentopsv1alpha1.InputAlert, Payload: "{}"},
	)
	c.Status.SessionID = "sess-1"
	u, ids, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if len(ids) != 2 {
		t.Fatalf("must batch consecutive replies only: %v", ids)
	}
	if u.ResumeSessionID != "sess-1" || !strings.HasSuffix(u.RunID, "-resume") {
		t.Fatalf("resume expected: %+v", u)
	}
	if !strings.Contains(u.PromptText, "first\n---\nsecond") {
		t.Fatalf("joined replies missing:\n%s", u.PromptText)
	}
}

func TestReplyWithoutSessionDegradesToTask(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "r1", Type: agentopsv1alpha1.InputReply, Payload: "hello"})
	u, _, ok, _ := dispatchNext(c, profile())
	if !ok || u.ResumeSessionID != "" || !strings.Contains(u.PromptText, "hello") {
		t.Fatalf("degrade to task: %+v", u)
	}
}

func TestInflightBlocksDispatch(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	c.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r", InputIDs: []string{"other"}}
	_, _, ok, err := dispatchNext(c, profile())
	if ok || err != nil {
		t.Fatal("inflight must block dispatch (strictly serial)")
	}
}

func TestProcessedInputsSkipped(t *testing.T) {
	c := conv(
		agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "old"},
		agentopsv1alpha1.InputItem{ID: "i2", Type: agentopsv1alpha1.InputTask, Payload: "new"},
	)
	c.Status.ProcessedInputIDs = []string{"i1"}
	u, ids, ok, _ := dispatchNext(c, profile())
	if !ok || len(ids) != 1 || ids[0] != "i2" || !strings.Contains(u.PromptText, "new") {
		t.Fatalf("processed input not skipped: %v %+v", ids, u)
	}
}

func TestDefaultDeliveryIsPrintedAnswer(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, ok, _ := dispatchNext(c, profile())
	if !ok || !strings.Contains(u.PromptText, "printed answer IS the deliverable") {
		t.Fatalf("default delivery section missing:\n%s", u.PromptText)
	}
	if strings.Contains(u.PromptText, "Telegram") || strings.Contains(u.PromptText, "{{DELIVERY_INSTRUCTIONS}}") {
		t.Fatalf("channel-specific or unrendered delivery text leaked:\n%s", u.PromptText)
	}
}

// Delivery is the operator's job: no channel-supplied text can ever reach a
// prompt, so an agent never learns a transport or handles credentials.
func TestDeliveryWordingIsInvariant(t *testing.T) {
	p := profile()
	p.Spec.Prompt = "scripts/custom.md"
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, _, _ := dispatchNext(c, p)
	if !strings.Contains(u.PromptVars["DELIVERY_INSTRUCTIONS"], "printed answer IS the deliverable") {
		t.Fatalf("delivery var missing for repo prompts: %v", u.PromptVars)
	}
	if !strings.Contains(u.PromptVars["DELIVERY_INSTRUCTIONS"], "Do not attempt to send chat messages yourself") {
		t.Fatalf("prompt must forbid agent-side posting: %v", u.PromptVars)
	}
}

func TestAlertUsesInvestigateTemplate(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "a1", Type: agentopsv1alpha1.InputAlert, Payload: `{"alerts":[]}`})
	u, _, ok, _ := dispatchNext(c, profile())
	if !ok || !strings.Contains(u.PromptText, "READ-ONLY triage") || !strings.Contains(u.PromptText, `{"alerts":[]}`) {
		t.Fatalf("investigate template: %+v", u)
	}
}

// ---- toolsets mode ----------------------------------------------------------

func TestToolsModeDefaultsToMergeWhenUnset(t *testing.T) {
	if got := ToolsModeOf(nil); got != agentopsv1alpha1.ToolsModeMerge {
		t.Fatalf("nil binding: want merge, got %q", got)
	}
	if got := ToolsModeOf(&agentopsv1alpha1.ToolsetBinding{}); got != agentopsv1alpha1.ToolsModeMerge {
		t.Fatalf("empty mode: want merge, got %q", got)
	}
}

func TestToolsModeIsCarriedThrough(t *testing.T) {
	b := &agentopsv1alpha1.ToolsetBinding{Mode: agentopsv1alpha1.ToolsModeOverwrite}
	if got := ToolsModeOf(b); got != agentopsv1alpha1.ToolsModeOverwrite {
		t.Fatalf("want overwrite, got %q", got)
	}
}

// The runtime composes the wiring's tools with the agent definition's, so the
// unit must carry BOTH the mode and the agent whose definition it applies to.
func TestWorkUnitCarriesToolsModeAndAgent(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, ok, err := Next(c, profile(),
		Tooling{AllowedTools: "Bash", Mode: agentopsv1alpha1.ToolsModeOverwrite}, inlineResolver, now, c.ContextID())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if u.ToolsMode != agentopsv1alpha1.ToolsModeOverwrite {
		t.Fatalf("toolsMode: want overwrite, got %q", u.ToolsMode)
	}
	if u.AllowedTools != "Bash" {
		t.Fatalf("allowedTools: want Bash, got %q", u.AllowedTools)
	}
	if u.Agent != "ha-engineer" {
		t.Fatalf("agent: want ha-engineer (the profile's), got %q", u.Agent)
	}
}

// A per-input agent override must reach the runtime too, or it would read the
// wrong definition's tools while being told to act as a different agent.
// Also pins the deprecated dual-read — see TestQueuedAgentOverrideStillDispatches.
func TestWorkUnitAgentFollowsQueuedOverride(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x", Agent: "k8s-engineer"})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if u.Agent != "k8s-engineer" {
		t.Fatalf("agent: want k8s-engineer, got %q", u.Agent)
	}
}
