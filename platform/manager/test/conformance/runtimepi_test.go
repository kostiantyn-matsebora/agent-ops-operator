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

// The AgentRuntime conformance set (docs/contracts/runtime.md), driven
// against the real runtimes/pi — a Node.js runtime, so unlike every Go
// runtime this package builds there is nothing to COMPILE: the artifact
// under test is `node runtime.js` itself, started exactly as its Dockerfile's
// own CMD does, against FakeManager's work contract and a FakePiProvider
// standing in for PI_BASE_URL (design.md's S7 finding: "the same
// fake-provider approach the spikes used works as the conformance lane's
// provider stand-in" — ported from runtimes/pi/test/fake-provider.js, which
// proved the wire shape live against the real SDK first).
//
// runtimes/claude and runtimes/copilot stay out of scope for the reasons
// runtime_test.go already gives; this lane joins runtimes/ollama as the
// SECOND one in scope, for the same reason S7 gives for pi specifically:
// a fake provider at an arbitrary baseUrl is possible and was confirmed live.
//
// WHAT DIFFERS FROM THE OLLAMA LANE, beyond the obvious (a Node process, no
// `build()` step):
//   - pi's own report.js (runtimes/pi/report.js) reports a tool call's
//     `tool`/`server`/`durationMs` only — no `resultBytes` field exists on
//     this runtime's report at all, unlike ollama's. The API-4/API-8
//     assertion below checks only the fields pi actually produces
//     (COMPAT-2: the manager ignores a field a runtime never sends; this
//     suite must not invent one either).
//   - the "calls" scenario's tool call is pi's own REAL built-in `bash` tool
//     (vocabulary.js's BUILTIN.Bash -> 'bash') executing for real inside
//     WORKSPACE — there is no Go-side tool emulation to write, unlike
//     ollama's harness, which implements its own tool loop.
//
// Deliberately NOT asserted here, for the same reasons runtime_test.go's own
// comment gives (API-3/API-6/every MGR-n, LC-3, SEC-1/2/3 — manager- or
// router-side facts no runtime binary could violate from the manager-facing
// wire).
//
// repoRoot, fixture, build, start, waitFor, freePort, contractEnv, postJSON
// etc. all come from main_test.go/runner.go — this file adds only what a
// Node-driven runtime and its own provider double need beyond those.

// piRuntimeScript is the artifact this lane drives: the exact file the
// Dockerfile's CMD runs, in place — no copy, no build step.
func piRuntimeScript(t *testing.T) string {
	t.Helper()
	script := filepath.Join(repoRoot(), "runtimes", "pi", "runtime.js")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("runtimes/pi/runtime.js not found: %v", err)
	}
	return script
}

// startPiRuntime starts the real runtime.js as a subprocess, in the shape
// start() (runner.go) gives every OTHER adapter's binary — a *Process this
// package's own waitFor/Stop/Output helpers already know how to drive —
// built by hand here only because start() assumes a standalone executable
// (`exec.Command(bin)`), never an interpreter plus a script argument.
func startPiRuntime(t *testing.T, env []string) *Process {
	t.Helper()
	script := piRuntimeScript(t)
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node not found on PATH: %v", err)
	}
	cmd := exec.Command(nodeBin, script)
	cmd.Dir = filepath.Dir(script)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, env...)
	buf := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = buf, buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting runtime-pi: %v", err)
	}
	p := &Process{Name: "runtime-pi", cmd: cmd, out: buf, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		p.Stop()
		if t.Failed() {
			t.Logf("---- runtime-pi output ----\n%s", buf.String())
		}
	})
	return p
}

// runtimePiRepoFixture is runtimeOllamaRepoFixture's own fixture, built
// separately rather than shared across the two lanes (structure.md: "a
// runtime is its own component... copies nothing across a directory
// boundary" — the same reasoning runtimes/pi/tools.js's own doc comment
// gives for not importing runtimes/ollama's copy). Same shape: RT-3.4 names
// pi among the runtimes reading `.claude/agents/<agent>.md`, the identical
// path ollama reads.
func runtimePiRepoFixture(t *testing.T) (url, ref string) {
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
	write("merge-agent.md", "---\ntools:\n  - Grep\n  - Write\n---\n\nRole text.\n")
	write("broken-agent.md", "---\ntools:\n  - Bash\n\nRole text with no closing marker.\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "runtime-pi conformance fixture")
	return dir, "main"
}

func runtimePiUnit(runID, convo, promptText string, extra map[string]any) map[string]any {
	u := map[string]any{"runId": runID, "convo": convo, "promptText": promptText}
	for k, v := range extra {
		u[k] = v
	}
	return u
}

func runtimePiCompletion(mgr *FakeManager, runID string) (WorkCompletion, bool) {
	for _, c := range mgr.WorkCompletions() {
		if c.RunID == runID {
			return c, true
		}
	}
	return WorkCompletion{}, false
}

func runtimePiRun(t *testing.T, mgr *FakeManager, unit map[string]any) WorkCompletion {
	t.Helper()
	runID, _ := unit["runId"].(string)
	mgr.QueueWork(unit)
	waitFor(t, "completion of "+runID, 30*time.Second, func() bool { _, ok := runtimePiCompletion(mgr, runID); return ok })
	c, _ := runtimePiCompletion(mgr, runID)
	return c
}

// TestRuntimePiConformance drives one long-lived runtimes/pi process through
// every scenario the requirement IDs above need, against one Conversation —
// MGR-1's "one pod per conversation" is exactly this process's own
// assumption, so every unit below shares one convo.
func TestRuntimePiConformance(t *testing.T) {
	mgr := NewFakeManager(t, "unused")
	provider := NewFakePiProvider(t)
	repoURL, repoRef := runtimePiRepoFixture(t)
	workspace := t.TempDir()
	const convo = "rt-pi-convo"

	env := []string{
		"CONTROL_URL=" + mgr.URL(),
		"CONVO_ID=" + convo,
		"POD_NAME=test-pod",
		"WORKSPACE=" + workspace,
		"HOME=" + t.TempDir(), // the context mount -- PI_AGENT_DIR/SESSIONS_ROOT live under it.
		"REPO_URL=" + repoURL,
		"REPO_REF=" + repoRef,
		"RUNTIME_IDLE_TTL_M=10",
		"PI_API_KEY=test-key",
		"PI_PROVIDER=fake",
		"PI_PROVIDER_API=openai-completions",
		"PI_BASE_URL=" + provider.URL() + "/v1",
		"PI_MODEL=fake-model",
	}
	p := startPiRuntime(t, env)

	runtimePiAssertRT1ClonedTheFixture(t, workspace)
	runtimePiAssertAPI5AndAPI9(t, mgr, convo)
	runtimePiAssertRT3AllowlistComposition(t, mgr, p, convo)
	runtimePiAssertAPI7OmitsWhatPiCannotDetermine(t, mgr, convo)
	runtimePiAssertAPI4AndAPI8(t, mgr, convo)
	runtimePiAssertCOMPAT1(t, mgr, convo)
	runtimePiAssertAPI1AndLC1(t, mgr, provider, convo)
	runtimePiAssertAPI2PromptFile(t, mgr, workspace, convo)

	if p.Exited() {
		t.Fatalf("the runtime must still be alive and polling after every scenario:\n%s", p.Output())
	}
	assertRuntimePiLongPolled(t, mgr)
}

// assertRuntimePiLongPolled pins API-1's long-poll half, same shape as
// assertRuntimeOllamaLongPolled.
func assertRuntimePiLongPolled(t *testing.T, mgr *FakeManager) {
	t.Helper()
	var fromRuntime []WorkRequest
	for _, r := range mgr.WorkRequests() {
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
// before any unit was ever served (runtime.js's runLoop syncs once at
// startup).
func runtimePiAssertRT1ClonedTheFixture(t *testing.T, workspace string) {
	t.Helper()
	waitFor(t, "RT-1: REPO_URL/REPO_REF cloned into WORKSPACE", 15*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(workspace, ".claude", "agents", "merge-agent.md"))
		return err == nil
	})
}

// API-5 (latest-wins) and API-9 (neither array reported when the run made
// neither a model nor a tool call), driven together, same shape as the
// ollama lane's own assertion.
func runtimePiAssertAPI5AndAPI9(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	first := runtimePiRun(t, mgr, runtimePiUnit("api5-first", convo, "hello there", nil))
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

	second := runtimePiRun(t, mgr, runtimePiUnit("api5-second", convo, "", nil))
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

// RT-3 and its sub-items, via the stdout log line runtime.js's runPi writes
// before every chat call — the same log shape the ollama lane drives,
// because composeAllowedTools (RT-3) is vendor-agnostic: the VENDOR mapping
// (vocabulary.js, pi's own 'read'/'write'/... names) only happens AFTER this
// line is written, so its text is identical across runtimes.
func runtimePiAssertRT3AllowlistComposition(t *testing.T, mgr *FakeManager, p *Process, convo string) {
	t.Helper()
	cases := []struct {
		id, agent, allowed, mode string
		want, unwanted           []string
		why                      string
	}{
		{"rt3-merge", "merge-agent", "Bash,Read", "merge",
			[]string{"-> Grep,Write,Bash,Read"}, nil,
			"merge must union with the agent's own tools kept first"},
		{"rt3-overwrite", "merge-agent", "Bash,Read", "overwrite",
			[]string{"-> Bash,Read"}, []string{"Grep", "Write"},
			"overwrite must use allowedTools alone, never the agent's own tools"},
		{"rt3-empty", "merge-agent", "", "overwrite",
			[]string{"-> (none)"}, nil,
			"an empty allowlist must stay empty, no tool substitutes for one nobody declared"},
		{"rt3-absent", "no-such-agent", "Read", "merge",
			[]string{"declared=0 wiring=1 mode=merge -> Read"}, nil,
			"an absent agent definition must contribute nothing"},
		{"rt3-broken", "broken-agent", "Read", "merge",
			[]string{"broken-agent.md", "never closed", "declared=0 wiring=1 mode=merge -> Read"}, nil,
			"an unreadable agent definition must be logged naming why and still contribute nothing"},
	}
	for _, tc := range cases {
		before := len(p.Output())
		c := runtimePiRun(t, mgr, runtimePiUnit(tc.id, convo, tc.id+" case",
			map[string]any{"agent": tc.agent, "allowedTools": tc.allowed, "toolsMode": tc.mode}))
		if c.Raw["status"] != "succeeded" {
			t.Fatalf("%s: %+v", tc.id, c.Raw)
		}
		full := p.Output()
		if before > len(full) {
			t.Fatalf("output shrank, impossible for an append-only buffer")
		}
		out := full[before:]
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Fatalf("%s: %s: %s", tc.id, tc.why, out)
			}
		}
		for _, u := range tc.unwanted {
			if strings.Contains(out, u) {
				t.Fatalf("%s: %s: %s", tc.id, tc.why, out)
			}
		}
	}
}

// API-7: a fact pi could not determine (here, the usage counters) is omitted
// from the posted turn, never reported as a zero.
// runtimePiAssertAPI7OmitsWhatPiCannotDetermine drives the "unparsable"
// directive: a completion with no `usage` field anywhere in the SSE stream.
//
// MEASURED LIVE, and narrower than first assumed: driven through the real
// SDK's openai-completions provider (not report.js's own pure-function unit
// test, which replays a HAND-WRITTEN event straight into newCallRecorder —
// test/fixtures/pi-session-events.jsonl, report.test.js), a provider that
// never sends a `usage` object at all still comes back with
// tokensIn/tokensOut/cacheReadTokens/cacheCreationTokens reported as REAL
// ZEROS, not omitted — `ModelRuntime`'s own usage accumulator default-fills
// those four numerically regardless of what the wire sent. `reasoningTokens`
// is the one sub-fact that stays genuinely ABSENT in this same case, which
// is the omission this assertion pins as reachable live.
//
// Flagged in this change's hand-back: report.js's own doc comment ("A fact
// pi does not report is OMITTED rather than written as zero") is true at
// the unit tier against a hand-built event, but is NARROWER than it reads
// once driven end-to-end against the real openai-completions provider path
// — this conformance lane is deliberately NOT asserting the broader claim
// for tokensIn/tokensOut/cacheReadTokens/cacheCreationTokens, since doing so
// would assert something false about the shipped vendor behaviour.
func runtimePiAssertAPI7OmitsWhatPiCannotDetermine(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimePiRun(t, mgr, runtimePiUnit("api7", convo, "unparsable task",
		map[string]any{"allowedTools": "", "toolsMode": "overwrite"}))
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("api7: %+v", c.Raw)
	}
	turnsRaw, ok := c.Raw["turns"].([]any)
	if !ok || len(turnsRaw) != 1 {
		t.Fatalf("exactly one model call must be reported: %+v", c.Raw["turns"])
	}
	turn, _ := turnsRaw[0].(map[string]any)
	if v, present := turn["reasoningTokens"]; present {
		t.Fatalf("a count the provider never sent must be OMITTED, never zero: reasoningTokens=%v in %+v", v, turn)
	}
	if _, present := c.Raw["brief"]; present {
		t.Fatalf("a run whose answer carries no <brief> tag must omit brief entirely: %+v", c.Raw)
	}
}

// API-4 (trimmed to the documented bounds, and every reported field shaped
// right) and API-8 (a tool's input/output never reported), via the "calls"
// directive — pi's own REAL built-in `bash` tool executes
// `echo AGENTOPS_CONFORMANCE_TOOL_MARKER` for real inside WORKSPACE; this
// asserts only the fields runtimes/pi/report.js actually produces for a tool
// call (`tool`/`durationMs`) — it has no `resultBytes` field, unlike
// runtimes/ollama's own report (COMPAT-2: never invent a field a runtime
// does not send).
func runtimePiAssertAPI4AndAPI8(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimePiRun(t, mgr, runtimePiUnit("api4-8", convo, "calls now",
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
		t.Fatalf("the tool call must name the tool in the agent-ops vocabulary (vocabulary.js's back-mapping): %+v", call)
	}
	if d, ok := call["durationMs"].(float64); !ok || d < 0 {
		t.Fatalf("durationMs, when pi's own built-in reports it, must be a non-negative number: %+v", call)
	}
	raw, _ := json.Marshal(c.Raw)
	if strings.Contains(string(raw), "echo AGENTOPS_CONFORMANCE_TOOL_MARKER") || strings.Contains(string(raw), "AGENTOPS_CONFORMANCE_TOOL_MARKER") {
		t.Fatalf("a tool's input or output must never be reported: %s", raw)
	}
}

// COMPAT-1: a work-unit field this runtime does not know is ignored, not
// rejected.
func runtimePiAssertCOMPAT1(t *testing.T, mgr *FakeManager, convo string) {
	t.Helper()
	c := runtimePiRun(t, mgr, runtimePiUnit("compat1", convo, "compat case",
		map[string]any{"totallyUnknownField": map[string]any{"nested": true}}))
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("an unknown work-unit field must be ignored, not rejected: %+v", c.Raw)
	}
}

// API-1 (204 is not an error) and LC-1 (one inflight unit): the "slow"
// directive holds the provider call open well past this process's own
// long-poll cadence, and while it is still outstanding a direct probe
// against FakeManager gets 204, never a second unit.
func runtimePiAssertAPI1AndLC1(t *testing.T, mgr *FakeManager, provider *FakePiProvider, convo string) {
	t.Helper()
	provider.SlowDelay = 2 * time.Second
	mgr.QueueWork(runtimePiUnit("lc1-slow", convo, "slow now", map[string]any{"allowedTools": "", "toolsMode": "overwrite"}))
	time.Sleep(500 * time.Millisecond) // let the unit be dispatched and the chat call begin

	resp, err := http.Get(mgr.URL() + "/work?convo=" + convo + "&pod=probe&wait=0")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("while a run is in progress, nothing else is queued, so a poll must still answer 204 (LC-1), got %d", resp.StatusCode)
	}

	waitFor(t, "the slow run to finish", 15*time.Second, func() bool { _, ok := runtimePiCompletion(mgr, "lc1-slow"); return ok })
	c, _ := runtimePiCompletion(mgr, "lc1-slow")
	if c.Raw["status"] != "succeeded" {
		t.Fatalf("the slow run must still complete once the provider answers: %+v", c.Raw)
	}
}

// API-2: a promptFile-carrying unit is handled exactly like a promptText
// one.
func runtimePiAssertAPI2PromptFile(t *testing.T, mgr *FakeManager, workspace, convo string) {
	t.Helper()
	promptsDir := filepath.Join(workspace, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "task.md"), []byte("hello {{NAME}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := runtimePiRun(t, mgr, map[string]any{
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

// TestRuntimePiIdleExit pins LC-2: no work for RUNTIME_IDLE_TTL_M minutes
// ends the process with exit 0, no crash.
func TestRuntimePiIdleExit(t *testing.T) {
	provider := NewFakePiProvider(t)
	env := []string{
		// A closed port, like the other three runtimes' identical test: the
		// loop's first fetch fails and sleeps out its 5s retry before the
		// idle check gets a second, successful look, rather than this test
		// paying FakeManager's own 25s long-poll block per cycle.
		"CONTROL_URL=http://127.0.0.1:1",
		"CONVO_ID=idle-convo-pi",
		"WORKSPACE=" + t.TempDir(),
		"HOME=" + t.TempDir(),
		"RUNTIME_IDLE_TTL_M=1",
		"PI_API_KEY=test-key",
		"PI_PROVIDER=fake",
		"PI_PROVIDER_API=openai-completions",
		"PI_BASE_URL=" + provider.URL() + "/v1",
		"PI_MODEL=fake-model",
		"REPO_URL=",
	}
	p := startPiRuntime(t, env)
	waitFor(t, "the idle runtime to exit 0 after its TTL, with no work ever queued", 90*time.Second, func() bool {
		return p.Exited()
	})
	if !strings.Contains(p.Output(), "idle TTL reached") {
		t.Fatalf("the exit must be the documented idle-TTL lifecycle exit, not a crash: %s", p.Output())
	}
}
