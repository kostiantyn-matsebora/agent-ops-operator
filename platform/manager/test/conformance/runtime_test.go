//go:build conformance

package conformance

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The AgentRuntime conformance set (docs/contracts/runtime.md). It builds
// the real runtimes/ollama binary and drives it against FakeManager's work
// contract (GET /work, POST /work/done) and a FakeOllama double standing in
// for OLLAMA_URL — the same shape channel_test.go/signal_test.go use for
// their own adapters.
//
// A new AgentRuntime implementation joins this set by being LISTED here —
// runtimes/claude and runtimes/copilot are explicitly out of scope for this
// change (proposal.md: runtimes/claude's CLAUDE_BIN seam is being replaced
// by claude-runtime-sdk, runtimes/copilot's own seam is unconfirmed).
//
// Deliberately NOT asserted here, with the reason each stays owned
// elsewhere:
//
//   - API-3, API-6, every MGR-n: each obligates whoever ANSWERS GET /work —
//     the manager, not the runtime. FakeManager trivially satisfies them by
//     construction, so asserting them here would prove nothing about a real
//     manager (design.md, spec.md).
//   - LC-3 (/exit refused while a run is in progress): entirely MANAGER-side
//     business logic (internal/chat/router.go's isExitCommand path, keyed on
//     the Conversation's phase) with NO HTTP call to the runtime pod at all —
//     runtimes/ollama exposes no endpoint named or shaped like "/exit" and
//     never observes one. There is no runtime-observable behavior this
//     black-box suite could assert for LC-3; it is already exercised by
//     internal/chat/router_test.go against the real router.
//   - SEC-1, SEC-2, SEC-3: each is a fact about the MANAGER/OPERATOR's own
//     pod-spec and RBAC rendering (no Secret read, AOPS_MCP_TOKEN derivation,
//     no ServiceAccount granted) — none of it is something the runtime
//     BINARY does or could violate from the manager-facing wire. Already
//     owned by internal/runtimepod's own tests and the chart's RBAC
//     templates; fabricating a "the runtime never echoes an env secret" check
//     here and labeling it SEC-1/2/3 would test an invented, adjacent
//     property rather than the documented one.

// runtimeOllamaRepoFixture lays out a real, local git repository (RT-1) with
// a handful of `.claude/agents/<agent>.md` definitions (RT-3's own read
// path, RT-3.4) exercising every composition case: a declared-tools agent
// (merge/overwrite), an unreadable one (RT-3.3), and none at all for an
// agent name this fixture never defines (RT-3.1/RT-3.3 — this runtime's own
// code treats "no checkout" and "checkout without this file" identically:
// agentDeclaredTools silently returns nil from either, so one fixture proves
// both documented cases). Returns the repository path (REPO_URL) and its
// branch (REPO_REF).
func runtimeOllamaRepoFixture(t *testing.T) (url, ref string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	agentsDir := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Declares two tools, used by both the merge and the overwrite case.
	write("merge-agent.md", "---\ntools:\n  - Grep\n  - Write\n---\n\nRole text.\n")
	// Frontmatter opened but never closed -- parseFrontmatterTools' own
	// "never closed" error path (RT-3.3's "unreadable" half).
	write("broken-agent.md", "---\ntools:\n  - Bash\n\nRole text with no closing marker.\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "runtime conformance fixture")
	return dir, "main"
}

// runtimeOllamaUnit builds one GET /work body. extra overrides/adds fields —
// including, deliberately, ones WorkUnit does not declare (COMPAT-1).
func runtimeOllamaUnit(runID, convo, promptText string, extra map[string]any) map[string]any {
	u := map[string]any{"runId": runID, "convo": convo, "promptText": promptText}
	for k, v := range extra {
		u[k] = v
	}
	return u
}

func runtimeOllamaCompletion(mgr *FakeManager, runID string) (WorkCompletion, bool) {
	for _, c := range mgr.WorkCompletions() {
		if c.RunID == runID {
			return c, true
		}
	}
	return WorkCompletion{}, false
}

// runtimeOllamaRun queues a unit and blocks until its completion is posted.
func runtimeOllamaRun(t *testing.T, mgr *FakeManager, unit map[string]any) WorkCompletion {
	t.Helper()
	runID, _ := unit["runId"].(string)
	mgr.QueueWork(unit)
	waitFor(t, "completion of "+runID, 20*time.Second, func() bool { _, ok := runtimeOllamaCompletion(mgr, runID); return ok })
	c, _ := runtimeOllamaCompletion(mgr, runID)
	return c
}

// TestRuntimeOllamaConformance drives one long-lived runtimes/ollama process
// through every scenario the requirement IDs above need, in order, against
// one Conversation — MGR-1's "one pod per conversation" is exactly this
// process's own assumption, so every unit below shares one convo.
func TestRuntimeOllamaConformance(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	ollama := NewFakeOllama(t)
	repoURL, repoRef := runtimeOllamaRepoFixture(t)
	workspace := t.TempDir()
	const convo = "rt-convo"

	bin := build(t, "runtimes/ollama")
	env := []string{
		"CONTROL_URL=" + mgr.URL(),
		"CONVO_ID=" + convo,
		"OLLAMA_URL=" + ollama.URL(),
		"POD_NAME=test-pod",
		"WORKSPACE=" + workspace,
		"HOME=" + t.TempDir(), // the context mount -- overrides start()'s own default HOME, see runner.go.
		"REPO_URL=" + repoURL,
		"REPO_REF=" + repoRef,
		"RUNTIME_IDLE_TTL_M=10",
	}
	p := start(t, "runtime-ollama", bin, env)

	runtimeOllamaAssertRT1ClonedTheFixture(t, workspace)
	runtimeOllamaAssertAPI5AndAPI9(t, mgr, convo)
	runtimeOllamaAssertRT3AllowlistComposition(t, mgr, p, convo)
	runtimeOllamaAssertAPI7OmitsWhatOllamaCannotDetermine(t, mgr, convo)
	runtimeOllamaAssertAPI4AndAPI8(t, mgr, convo)
	runtimeOllamaAssertCOMPAT1(t, mgr, convo)
	runtimeOllamaAssertAPI1AndLC1(t, mgr, ollama, convo)
	runtimeOllamaAssertAPI2PromptFile(t, mgr, workspace, convo)

	if p.Exited() {
		t.Fatalf("the runtime must still be alive and polling after every scenario:\n%s", p.Output())
	}
	assertRuntimeOllamaLongPolled(t, mgr)
}

// assertRuntimeOllamaLongPolled pins API-1's long-poll half: every GET /work
// this runtime issued waited (runtimes/ollama/work.go's Control.Poll always
// sends wait=25), and a 204 never shows up anywhere as an error in its own
// output.
func assertRuntimeOllamaLongPolled(t *testing.T, mgr *FakeManager) {
	t.Helper()
	var fromRuntime []WorkRequest
	for _, r := range mgr.WorkRequests() {
		// "probe" is this suite's OWN direct HTTP poll (LC-1's probe), never
		// the runtime's — excluded, since it deliberately asks wait=0.
		if r.Pod != "probe" {
			fromRuntime = append(fromRuntime, r)
		}
	}
	if len(fromRuntime) == 0 {
		t.Fatalf("the runtime never polled GET /work")
	}
	for _, r := range fromRuntime {
		if r.Wait <= 0 {
			t.Fatalf("every poll the runtime itself issues must long-poll (wait>0), got %+v", r)
		}
	}
}

// RT-1: the checkout under WORKSPACE carries the fixture's own files, cloned
// before any unit was ever served (main.go syncs once at startup).
func runtimeOllamaAssertRT1ClonedTheFixture(t *testing.T, workspace string) {
	t.Helper()
	waitFor(t, "RT-1: REPO_URL/REPO_REF cloned into WORKSPACE", 15*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(workspace, ".claude", "agents", "merge-agent.md"))
		return err == nil
	})
}

// API-5 (latest-wins) and API-9 (neither array reported when the run made
// neither a model nor a tool call), driven together: one ordinary run sets
// runtimeContextId+brief, then a run that fails BEFORE any chat call (an
// empty prompt) posts neither field and neither array — proving both that
// the omission happens and that FakeManager's own latest-wins state stays
// exactly as the first run left it.
func runtimeOllamaAssertAPI5AndAPI9(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	first := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("api5-first", convo, "hello there", nil))
	if first.Raw["status"] != "succeeded" {
		t.Fatalf("api5-first: %+v", first.Raw)
	}
	ctxID, _ := first.Raw["runtimeContextId"].(string)
	brief, _ := first.Raw["brief"].(string)
	if ctxID == "" || brief == "" {
		t.Fatalf("a run whose answer carries a <brief> tag must report both runtimeContextId and brief: %+v", first.Raw)
	}
	gotCtx, gotBrief := mgr.WorkState(convo)
	if gotCtx != ctxID || gotBrief != brief {
		t.Fatalf("FakeManager's own latest-wins state must match the first report: got (%q,%q) want (%q,%q)", gotCtx, gotBrief, ctxID, brief)
	}

	second := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("api5-second", convo, "", nil))
	if second.Raw["status"] != "failed" {
		t.Fatalf("an empty prompt must fail the run before any chat call: %+v", second.Raw)
	}
	for _, key := range []string{"runtimeContextId", "brief", "turns", "toolCalls"} {
		if _, present := second.Raw[key]; present {
			t.Fatalf("a run that never reached the model must omit %q entirely, got %+v", key, second.Raw)
		}
	}
	gotCtx, gotBrief = mgr.WorkState(convo)
	if gotCtx != ctxID || gotBrief != brief {
		t.Fatalf("a report omitting runtimeContextId/brief must leave the stored value untouched (API-5): got (%q,%q) want (%q,%q)",
			gotCtx, gotBrief, ctxID, brief)
	}
}

// RT-3 and its sub-items, via the stdout log line agent.go's run() writes
// before every chat call ("[runtime] tools agent=... -> <granted>") — the
// only observable signal of what was actually granted, since the completion
// report never carries it.
func runtimeOllamaAssertRT3AllowlistComposition(t *testing.T, mgr *FakeManager, p *Process, convo string) {
	t.Helper()
	outputSince := func(before int, run func()) string {
		run()
		full := p.Output()
		if before > len(full) {
			t.Fatalf("output shrank, impossible for an append-only buffer")
		}
		return full[before:]
	}

	// merge: the union, the agent's own tools keeping position (RT-3).
	before := len(p.Output())
	out := outputSince(before, func() {
		c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("rt3-merge", convo, "merge case",
			map[string]any{"agent": "merge-agent", "allowedTools": "Bash,Read", "toolsMode": "merge"}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("rt3-merge: %+v", c.Raw)
		}
	})
	if !strings.Contains(out, "-> Grep,Write,Bash,Read") {
		t.Fatalf("merge must union with the agent's own tools kept first: %s", out)
	}

	// overwrite: allowedTools alone, the agent's own declared tools dropped.
	before = len(p.Output())
	out = outputSince(before, func() {
		c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("rt3-overwrite", convo, "overwrite case",
			map[string]any{"agent": "merge-agent", "allowedTools": "Bash,Read", "toolsMode": "overwrite"}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("rt3-overwrite: %+v", c.Raw)
		}
	})
	if !strings.Contains(out, "-> Bash,Read") || strings.Contains(out, "Grep") || strings.Contains(out, "Write") {
		t.Fatalf("overwrite must use allowedTools alone, never the agent's own tools: %s", out)
	}

	// RT-3.2: an empty allowlist stays empty, even against an agent that
	// declares tools of its own.
	before = len(p.Output())
	out = outputSince(before, func() {
		c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("rt3-empty", convo, "empty case",
			map[string]any{"agent": "merge-agent", "allowedTools": "", "toolsMode": "overwrite"}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("rt3-empty: %+v", c.Raw)
		}
	})
	if !strings.Contains(out, "-> (none)") {
		t.Fatalf("an empty allowlist must stay empty, no tool substitutes for one nobody declared: %s", out)
	}

	// RT-3.1/RT-3.3: an agent name this fixture never defines contributes
	// nothing -- the same silent "declares nothing" path a from a checkout
	// that cannot be seen at all takes.
	before = len(p.Output())
	out = outputSince(before, func() {
		c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("rt3-absent", convo, "absent case",
			map[string]any{"agent": "no-such-agent", "allowedTools": "Read", "toolsMode": "merge"}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("rt3-absent: %+v", c.Raw)
		}
	})
	if !strings.Contains(out, "declared=0 wiring=1 mode=merge -> Read") {
		t.Fatalf("an absent agent definition must contribute nothing: %s", out)
	}

	// RT-3.3, the "unreadable" half: frontmatter that opened but never
	// closed is logged by name and, like the absent case, contributes
	// nothing.
	before = len(p.Output())
	out = outputSince(before, func() {
		c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("rt3-broken", convo, "broken case",
			map[string]any{"agent": "broken-agent", "allowedTools": "Read", "toolsMode": "merge"}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("rt3-broken: %+v", c.Raw)
		}
	})
	if !strings.Contains(out, "broken-agent.md") || !strings.Contains(out, "never closed") {
		t.Fatalf("an unreadable agent definition must be logged naming why: %s", out)
	}
	if !strings.Contains(out, "declared=0 wiring=1 mode=merge -> Read") {
		t.Fatalf("an unreadable agent definition must still contribute nothing: %s", out)
	}
}

// API-7: a fact ollama could not determine (here, the usage counters) is
// omitted from the posted turn, never reported as a zero.
func runtimeOllamaAssertAPI7OmitsWhatOllamaCannotDetermine(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("api7", convo, "unparsable task",
		map[string]any{"allowedTools": "", "toolsMode": "overwrite"}))
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("api7: %+v", c.Raw)
	}
	turnsRaw, ok := c.Raw["turns"].([]any)
	if !ok || len(turnsRaw) != 1 {
		t.Fatalf("exactly one model call must be reported: %+v", c.Raw["turns"])
	}
	turn, _ := turnsRaw[0].(map[string]any)
	for _, key := range []string{"tokensIn", "tokensOut"} {
		if v, present := turn[key]; present {
			t.Fatalf("a count ollama did not send must be OMITTED, never zero: %q=%v in %+v", key, v, turn)
		}
	}
	if _, present := c.Raw["brief"]; present {
		t.Fatalf("a run whose answer carries no <brief> tag must omit brief entirely: %+v", c.Raw)
	}
}

// API-4 (trimmed to the documented bounds -- the entry-count/field-size
// numeric bound itself is already pinned at the unit tier,
// runtimes/ollama/report_test.go's TestReportIsTrimmedToTheManagersBounds;
// this tier proves the WIRING: a real model+tool-call round trip produces
// the right SHAPE, with every integer field non-negative) and API-8 (a
// tool's input/output never reported), via the "calls" directive.
func runtimeOllamaAssertAPI4AndAPI8(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("api4-8", convo, "calls now",
		map[string]any{"allowedTools": "Bash", "toolsMode": "overwrite"}))
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("api4-8: %+v", c.Raw)
	}
	turns, _ := c.Raw["turns"].([]any)
	toolCalls, _ := c.Raw["toolCalls"].([]any)
	if len(turns) != 2 || len(toolCalls) != 1 {
		t.Fatalf("one tool-call round trip must report two turns and one tool call: turns=%v toolCalls=%v", turns, toolCalls)
	}
	call, _ := toolCalls[0].(map[string]any)
	if call["tool"] != "Bash" {
		t.Fatalf("the tool call must name the tool: %+v", call)
	}
	for _, key := range []string{"durationMs", "resultBytes"} {
		n, ok := call[key].(float64)
		if !ok || n < 0 {
			t.Fatalf("%q must be a non-negative integer field, got %v", key, call[key])
		}
	}
	raw, _ := json.Marshal(c.Raw)
	if strings.Contains(string(raw), "echo AGENTOPS_CONFORMANCE_TOOL_MARKER") || strings.Contains(string(raw), "AGENTOPS_CONFORMANCE_TOOL_MARKER") {
		t.Fatalf("a tool's input or output must never be reported: %s", raw)
	}
}

// COMPAT-1: a work-unit field this runtime does not know is ignored, not
// rejected.
func runtimeOllamaAssertCOMPAT1(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimeOllamaRun(t, mgr, runtimeOllamaUnit("compat1", convo, "compat case",
		map[string]any{"totallyUnknownField": map[string]any{"nested": true}}))
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("an unknown work-unit field must be ignored, not rejected: %+v", c.Raw)
	}
}

// API-1 (204 is not an error) and LC-1 (one inflight unit -- a second poll
// while Working gets nothing back): the "slow" directive holds the chat
// call open well past this process's own long-poll cadence, and while it is
// still outstanding a direct probe against FakeManager -- standing in for
// "what if another poll landed right now" -- gets 204, never a second unit,
// because nothing else was ever queued for this conversation.
func runtimeOllamaAssertAPI1AndLC1(t *testing.T, mgr *FakeManager, ollama *FakeOllama, convo string) {
	t.Helper()
	ollama.SlowDelay = 2 * time.Second
	mgr.QueueWork(runtimeOllamaUnit("lc1-slow", convo, "slow now", map[string]any{"allowedTools": "", "toolsMode": "overwrite"}))
	time.Sleep(500 * time.Millisecond) // let the unit be dispatched and the chat call begin

	resp, err := http.Get(mgr.URL() + "/work?convo=" + convo + "&pod=probe&wait=0")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("while a run is in progress, nothing else is queued, so a poll must still answer 204 (LC-1), got %d", resp.StatusCode)
	}

	waitFor(t, "the slow run to finish", 10*time.Second, func() bool { _, ok := runtimeOllamaCompletion(mgr, "lc1-slow"); return ok })
	c, _ := runtimeOllamaCompletion(mgr, "lc1-slow")
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("the slow run must still complete once ollama answers: %+v", c.Raw)
	}
}

// API-2: a promptFile-carrying unit (promptFile+promptVars, no promptText)
// is handled exactly like a promptText one -- the manager sends exactly one
// of the two, and this proves both are read correctly.
func runtimeOllamaAssertAPI2PromptFile(t *testing.T, mgr *FakeManager, workspace, convo string) {
	t.Helper()
	promptsDir := filepath.Join(workspace, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "task.md"), []byte("hello {{NAME}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := runtimeOllamaRun(t, mgr, map[string]any{
		"runId": "api2-promptfile", "convo": convo,
		"promptFile": "prompts/task.md", "promptVars": map[string]string{"NAME": "world"},
		"allowedTools": "", "toolsMode": "overwrite",
	})
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("a promptFile-carrying unit must run like any other: %+v", c.Raw)
	}
	result, _ := c.Raw["result"].(string)
	if !strings.Contains(result, "hello world") {
		t.Fatalf("promptFile must be rendered with promptVars substituted, got %q", result)
	}
}

// TestRuntimeOllamaIdleExit pins LC-2: no work for RUNTIME_IDLE_TTL_M
// minutes ends the process with exit 0, no crash. A whole minute is the
// finest grain the documented env var allows (it is read in MINUTES), so
// this is set to "1" and simply waited out.
func TestRuntimeOllamaIdleExit(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	ollama := NewFakeOllama(t)
	bin := build(t, "runtimes/ollama")
	env := []string{
		"CONTROL_URL=" + mgr.URL(),
		"CONVO_ID=idle-convo",
		"OLLAMA_URL=" + ollama.URL(),
		"WORKSPACE=" + t.TempDir(),
		"HOME=" + t.TempDir(),
		"RUNTIME_IDLE_TTL_M=1",
	}
	p := start(t, "runtime-ollama-idle", bin, env)
	waitFor(t, "the idle runtime to exit 0 after its TTL, with no work ever queued", 90*time.Second, func() bool {
		return p.Exited()
	})
	if !strings.Contains(p.Output(), "idle TTL reached") {
		t.Fatalf("the exit must be the documented idle-TTL lifecycle exit, not a crash: %s", p.Output())
	}
}
