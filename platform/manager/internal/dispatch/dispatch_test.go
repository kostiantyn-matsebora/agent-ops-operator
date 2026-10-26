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

// THE DECLARATION GATES THE PROMPT, NEVER THE PARSE.
//
// Decoupling them is what makes the flag harmless, and it is the property most
// likely to be "tidied" later into a single switch — which would allow a state
// where the model emits tags nothing is looking for. That is the
// literal-tags-in-chat failure, offered as a configuration option. The other
// half is pinned in internal/chat: the parse runs whatever a profile declared.
func TestOutputFormatGatesThePromptOnly(t *testing.T) {
	build := func(p *agentopsv1alpha1.AgentProfile) WorkUnit {
		c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "check the disk"})
		u, _, ok, err := dispatchNext(c, p)
		if err != nil || !ok {
			t.Fatalf("dispatch: ok=%v err=%v", ok, err)
		}
		return u
	}
	const marker = "MESSAGE FORMAT"

	// `none` is a DECLARATION, not a default — the field is required, so a
	// profile reaching dispatch has stated its contract either way.
	none := profile()
	none.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	off := build(none)
	if strings.Contains(off.SystemPrompt, marker) || strings.Contains(off.PromptText, marker) {
		t.Fatal("a profile declaring `none` must have NOTHING injected — it owns its own formatting")
	}

	p := profile()
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatBlocks
	if on := build(p); !strings.Contains(on.SystemPrompt, marker) {
		t.Fatalf("a profile that asked for the shared format did not get it: %q", on.SystemPrompt)
	}

	// BOTH LANES, because only one of them is a string the manager holds. A
	// profile with its own prompt FILE used to be silently exempt, which made
	// "is this agent told how to format" a property of whether `prompt` was set.
	pf := profile()
	pf.Spec.OutputFormat, pf.Spec.Prompt = agentopsv1alpha1.OutputFormatBlocks, "prompts/task.md"
	file := build(pf)
	if file.PromptFile == "" {
		t.Fatal("a profile prompt must still dispatch as a file")
	}
	if !strings.Contains(file.SystemPrompt, marker) {
		t.Fatal("a profile with its own prompt file asked for the shared format and must get it")
	}

	// The profile's own role text is kept, never replaced.
	pb := profile()
	pb.Spec.OutputFormat, pb.Spec.SystemPrompt = agentopsv1alpha1.OutputFormatBlocks, "You are terse."
	if both := build(pb); !strings.Contains(both.SystemPrompt, "You are terse.") {
		t.Fatal("the profile's own role text was replaced rather than appended to")
	}
}

// ---- coordinator root framing (design D-E) ----------------------------------

// rootConv is a Coordinator's own conversation: coordinatorRef set, no
// causedBy. It is the one shape framed as coordination.
func rootConv(inputs ...agentopsv1alpha1.InputItem) *agentopsv1alpha1.Conversation {
	c := conv(inputs...)
	c.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "triage"}
	return c
}

// forbiddenInRootPrompt is what the spec says a root's framing must NOT carry:
// a checkout to read, a role file to adopt, a fallback role to act in.
var forbiddenInRootPrompt = []string{".claude/agents", "checkout", "working directory", "within your tools", "act as", "repository"}

func assertCoordinationFraming(t *testing.T, u WorkUnit, input, from string) {
	t.Helper()
	if u.PromptFile != "" {
		t.Fatalf("a root without a profile prompt renders the built-in template, got file %q", u.PromptFile)
	}
	if !strings.HasPrefix(u.PromptText, "# Coordination turn") {
		t.Fatalf("coordinate.md not rendered:\n%s", u.PromptText)
	}
	if !strings.Contains(u.PromptText, input) {
		t.Fatalf("input %q missing from prompt:\n%s", input, u.PromptText)
	}
	if !strings.Contains(u.PromptText, "It came from "+from+".") {
		t.Fatalf("FROM %q missing from prompt:\n%s", from, u.PromptText)
	}
	if !strings.Contains(u.PromptText, "printed answer IS the deliverable") {
		t.Fatalf("delivery section missing from prompt:\n%s", u.PromptText)
	}
	if strings.Contains(u.PromptText, "{{") {
		t.Fatalf("unrendered variable left in prompt:\n%s", u.PromptText)
	}
	for _, bad := range forbiddenInRootPrompt {
		if strings.Contains(strings.ToLower(u.PromptText), bad) {
			t.Fatalf("root framing must not mention %q:\n%s", bad, u.PromptText)
		}
	}
}

func TestRootSignalInputIsFramedAsCoordination(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "disk is filling on node-a",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "cron-nightly", SignalKind: "task"}})
	u, ids, ok, err := dispatchNext(c, profile())
	if err != nil || !ok || len(ids) != 1 {
		t.Fatal(err, ok, ids)
	}
	assertCoordinationFraming(t, u, "> disk is filling on node-a", "a signal on source cron-nightly")
	if u.ResumeSessionID != "" || u.RuntimeContextID != "" {
		t.Fatal("a fresh root turn carries no context handle")
	}
}

func TestRootChannelInputIsFramedAsCoordination(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "look into the backlog",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: "ops-room", Sender: "operator-on-duty"}})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	assertCoordinationFraming(t, u, "> look into the backlog", "a person on channel ops-room (operator-on-duty)")
}

func TestRootMemberResultIsFramedAsCoordination(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "pod restarted, root cause OOM",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "c1-k8s-7f", Entry: "k8s"}})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	assertCoordinationFraming(t, u, "> pod restarted, root cause OOM", "the member invoked as k8s (conversation c1-k8s-7f)")
}

func TestRootManagerInputIsFramedAsCoordination(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "your last turn made no move",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginManager, Name: "move-per-turn"}})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	assertCoordinationFraming(t, u, "> your last turn made no move", "the manager, under its rule move-per-turn")
}

// An input recorded before origins existed still dispatches, and says so
// rather than inventing a door it came through.
func TestRootInputWithoutOriginSaysSo(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	assertCoordinationFraming(t, u, "> x", "an input with no recorded origin")
}

// A member is a domain agent and may have a repository: `causedBy` alone keeps
// the lane templates exactly as today.
func TestMemberWithCausedByKeepsTaskTemplate(t *testing.T) {
	c := conv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "check vacuum",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "root-1", Entry: "ha"}})
	c.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "ha"}
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if !strings.HasPrefix(u.PromptText, "# Direct task") || !strings.Contains(u.PromptText, ".claude/agents/ha-engineer.md") {
		t.Fatalf("a member must keep task.md:\n%s", u.PromptText)
	}
	if strings.Contains(u.PromptText, "Coordination turn") {
		t.Fatalf("a member must not be framed as coordination:\n%s", u.PromptText)
	}
}

// Every input type, not only tasks: an alert on a root renders the coordination
// framing with the payload fenced, never the read-only triage lane.
func TestRootAlertRendersCoordinateNotInvestigate(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "a1", Type: agentopsv1alpha1.InputAlert, Payload: `{"alerts":[{"name":"HighLoad"}]}`,
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "alertmanager", SignalKind: "alert"}})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	assertCoordinationFraming(t, u, "```json\n{\"alerts\":[{\"name\":\"HighLoad\"}]}\n```", "a signal on source alertmanager")
	if strings.Contains(u.PromptText, "READ-ONLY triage") {
		t.Fatalf("investigate.md leaked into a root's alert:\n%s", u.PromptText)
	}
}

// An adopter's own prompt file is their framing: it still wins on a root, with
// the same variables plus FROM so the file can name the origin too.
func TestRootProfilePromptStillWins(t *testing.T) {
	p := profile()
	p.Spec.Prompt = "prompts/coordinate.md"
	c := rootConv(agentopsv1alpha1.InputItem{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "cron-nightly"}})
	u, _, ok, err := dispatchNext(c, p)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if u.PromptFile != "prompts/coordinate.md" || u.PromptText != "" {
		t.Fatalf("profile prompt must win on a root: %+v", u)
	}
	if u.PromptVars["USER_TASK"] != "x" || u.PromptVars["FROM"] != "a signal on source cron-nightly" {
		t.Fatalf("vars: %v", u.PromptVars)
	}
	if !strings.Contains(u.PromptVars["DELIVERY_INSTRUCTIONS"], "printed answer IS the deliverable") {
		t.Fatalf("delivery var missing: %v", u.PromptVars)
	}
}

// A batched resume on a root carries every input verbatim and one FROM phrase
// per input, in order — a member's result followed by a person's reply reads
// as both, not as one anonymous blob.
func TestRootBatchedReplyNamesEveryOrigin(t *testing.T) {
	c := rootConv(
		agentopsv1alpha1.InputItem{ID: "r1", Type: agentopsv1alpha1.InputReply, Payload: "member says: OOM",
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "c1-k8s-7f", Entry: "k8s"}},
		agentopsv1alpha1.InputItem{ID: "r2", Type: agentopsv1alpha1.InputReply, Payload: "person says: raise the limit",
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: "ops-room"}},
	)
	c.Status.SessionID = "sess-1"
	u, ids, ok, err := dispatchNext(c, profile())
	if err != nil || !ok || len(ids) != 2 {
		t.Fatal(err, ok, ids)
	}
	if u.RuntimeContextID != "sess-1" || !strings.HasSuffix(u.RunID, "-resume") {
		t.Fatalf("resume expected: %+v", u)
	}
	assertCoordinationFraming(t, u, "> member says: OOM\n> ---\n> person says: raise the limit",
		"the member invoked as k8s (conversation c1-k8s-7f); then a person on channel ops-room")
	if strings.Contains(u.PromptText, "Approval") {
		t.Fatalf("reply.md leaked into a root's resume:\n%s", u.PromptText)
	}
}

// A root's reply before any context handle exists degrades to a fresh turn,
// still framed as coordination.
func TestRootReplyWithoutContextStillFramedAsCoordination(t *testing.T) {
	c := rootConv(agentopsv1alpha1.InputItem{ID: "r1", Type: agentopsv1alpha1.InputReply, Payload: "hello",
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: "ops-room"}})
	u, _, ok, err := dispatchNext(c, profile())
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if u.RuntimeContextID != "" {
		t.Fatal("no handle to continue")
	}
	assertCoordinationFraming(t, u, "> hello", "a person on channel ops-room")
}

// originPhrase is the one place the FROM wording lives; pin every door, the
// sender-less channel, and the two non-answers (no origin, unknown kind).
func TestOriginPhrasePerKind(t *testing.T) {
	cases := []struct {
		name string
		in   *agentopsv1alpha1.InputOrigin
		want string
	}{
		{"nil", nil, "an input with no recorded origin"},
		{"signal", &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "cron"}, "a signal on source cron"},
		{"channel", &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: "tg"}, "a person on channel tg"},
		{"channel+sender", &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginChannel, Name: "tg", Sender: "alice"}, "a person on channel tg (alice)"},
		{"member", &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "c-9", Entry: "k8s"}, "the member invoked as k8s (conversation c-9)"},
		{"manager", &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginManager, Name: "move-per-turn"}, "the manager, under its rule move-per-turn"},
		{"unknown", &agentopsv1alpha1.InputOrigin{Kind: "portal"}, `an input of unrecognized origin kind "portal"`},
	}
	for _, tc := range cases {
		if got := originPhrase(tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
