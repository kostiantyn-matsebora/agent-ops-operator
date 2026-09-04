// cd runtime-claude && node --test
//
// Drives runClaude()/spawnClaude() against a REAL subprocess standing in for
// the `claude` CLI — a small shell script on PATH, resolved by resolveBin()
// exactly as the production binary would be. Branching on $FAKE_MODE (an
// ordinary env var spawnClaude's child inherits) lets one process exercise
// several CLI behaviours without re-requiring the module: a stream-json
// success, a resume whose session is gone, one that reappears mid-check, the
// unparsable-tool-call spin breaker, and a line the JSON parser rejects. None
// of this mocks spawnClaude itself — every case is a real process exiting
// with real stdout/stderr/exit code.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

const FAKE_CLAUDE = `#!/bin/sh
case "$FAKE_MODE" in
  success)
    echo '{"type":"system","subtype":"init","model":"m","tools":["Read"],"mcp_servers":[]}'
    echo '{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}'
    echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"x"}}]}}'
    echo '{"type":"result","subtype":"success","num_turns":2,"duration_ms":10,"result":"done","session_id":"sess-1"}'
    exit 0
    ;;
  fail_with_session)
    echo '{"type":"result","subtype":"error","num_turns":1,"duration_ms":5,"result":"oops","session_id":"sess-2"}'
    exit 1
    ;;
  lost)
    echo 'no conversation found with session ID xyz' 1>&2
    exit 1
    ;;
  spin)
    i=0
    while [ $i -lt 6 ]; do
      echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Foo","input":{"__unparsedToolInput":{"raw":"{bad}"}}}]}}'
      i=$((i+1))
    done
    # exec, not a plain call: a forked child would inherit the stdout pipe's
    # write end, and Node's 'close' event then waits for THAT process to exit
    # too (it holds the pipe open even after this shell is SIGTERM'd) — a real
    # 30-second false failure mode this test hit before switching to exec.
    exec sleep 30
    ;;
  badjson)
    echo 'not json at all'
    exit 0
    ;;
  recovered)
    echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Foo","input":{"__unparsedToolInput":{"raw":"{bad}"}}}]}}'
    echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Foo","input":{"__unparsedToolInput":{"raw":"{bad}"}}}]}}'
    echo '{"type":"result","subtype":"success","num_turns":1,"duration_ms":1,"result":"answered anyway","session_id":"sess-3"}'
    exit 0
    ;;
esac
`;

const binDir = tmp('agentops-fakebin-');
const claudePath = path.join(binDir, 'claude');
fs.writeFileSync(claudePath, FAKE_CLAUDE, { mode: 0o755 });

process.env.PATH = `${binDir}${path.delimiter}${process.env.PATH}`;
process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-cli';
process.env.WORKSPACE = tmp('agentops-ws-');
process.env.HOME = tmp('agentops-home-'); // controls SESSIONS_DIR
process.env.RUNTIME_UNPARSED_REPEAT_LIMIT = '3'; // shrink the spin ladder for a fast test

const { runClaude, CLAUDE_BIN } = require('./runtime');

test('CLAUDE_BIN resolved to the fake script placed on PATH', () => {
  assert.strictEqual(CLAUDE_BIN, claudePath);
});

test('a fresh run parses a stream-json success, surfacing the session id and result', async () => {
  process.env.FAKE_MODE = 'success';
  const out = await runClaude({ promptText: 'hi', runId: 'r-success' });
  assert.strictEqual(out.status, 'succeeded');
  assert.strictEqual(out.sessionId, 'sess-1');
  assert.strictEqual(out.continuity, 'new');
  assert.match(out.result, /done/);
  assert.ok(!('stderr' in out), 'strip() must remove the internal capture');
});

test('a resumed run that succeeds reports continuity continued', async () => {
  process.env.FAKE_MODE = 'success';
  const out = await runClaude({ promptText: 'hi', runId: 'r-resume-ok', runtimeContextId: 'sess-abc' });
  assert.strictEqual(out.continuity, 'continued');
});

test('a resumed run that fails but still names a session is not treated as lost', async () => {
  process.env.FAKE_MODE = 'fail_with_session';
  const out = await runClaude({ promptText: 'hi', runId: 'r-resume-fail', runtimeContextId: 'sess-abc' });
  assert.strictEqual(out.status, 'failed');
  assert.strictEqual(out.continuity, 'continued');
});

test('a resumed run whose context reappears mid-check retries once and reports continued', async () => {
  const projDir = path.join(process.env.HOME, '.claude', 'projects', 'proj1');
  fs.mkdirSync(projDir, { recursive: true });
  fs.writeFileSync(path.join(projDir, 'sess-reappear.jsonl'), '{}');

  process.env.FAKE_MODE = 'lost';
  const start = Date.now();
  const out = await runClaude({ promptText: 'hi', runId: 'r-reappear', runtimeContextId: 'sess-reappear' });
  assert.strictEqual(out.continuity, 'continued');
  assert.ok(Date.now() - start < 3000, 'found on the first check — the full ladder must not run');
});

test('a resumed run whose context is genuinely gone fails without answering', async () => {
  process.env.FAKE_MODE = 'lost';
  const out = await runClaude({ promptText: 'hi', runId: 'r-gone', runtimeContextId: 'sess-really-gone' });
  assert.strictEqual(out.continuity, 'unavailable');
  assert.match(out.result, /cannot be continued/);
  assert.match(out.continuityReason, /no session files under/);
});

test('a model stuck repeating the same unparsable tool call is stopped, not left to spin', async () => {
  process.env.FAKE_MODE = 'spin';
  const out = await runClaude({ promptText: 'hi', runId: 'r-spin' });
  assert.strictEqual(out.status, 'failed');
  assert.match(out.result, /Stopped: a tool call that could not be formed/);
});

test('a line the JSON parser rejects is logged and skipped, not fatal to the run', async () => {
  process.env.FAKE_MODE = 'badjson';
  const out = await runClaude({ promptText: 'hi', runId: 'r-badjson' });
  assert.strictEqual(out.status, 'succeeded');
  assert.strictEqual(out.result, '');
  assert.strictEqual(out.sessionId, null, 'no result event carried one, so it stays the unset default');
});

// Below the spin limit (2 identical unparsable calls, limit is 3): the run is
// never killed, and finishes normally. This is the "recovered on its own"
// branch — the run still succeeds, but a notice is appended warning that some
// tool calls never ran, because the model may have answered from stale state
// without saying so.
test('a run with fewer unparsable repeats than the limit finishes normally, with a discarded-calls notice appended', async () => {
  process.env.FAKE_MODE = 'recovered';
  const out = await runClaude({ promptText: 'hi', runId: 'r-recovered' });
  assert.strictEqual(out.status, 'succeeded');
  assert.match(out.result, /answered anyway/);
  assert.match(out.result, /2 tool calls never ran/);
});

test('a promptFile is read from the workspace and its {{vars}} substituted', async () => {
  fs.writeFileSync(path.join(process.env.WORKSPACE, 'prompt.tmpl'), 'Hello {{name}}, using {{tool}}');
  process.env.FAKE_MODE = 'success';
  const out = await runClaude({ promptFile: 'prompt.tmpl', promptVars: { name: 'Ada', tool: 'Read' }, runId: 'r-promptfile' });
  assert.strictEqual(out.status, 'succeeded');
});

test('an empty prompt (no promptText, no promptFile) fails without spawning anything', async () => {
  const out = await runClaude({ runId: 'r-empty' });
  assert.deepStrictEqual(out, { status: 'failed', exitCode: -1, result: 'empty prompt' });
});

test('a promptFile that escapes the workspace fails to read rather than following it out', async () => {
  const out = await runClaude({ promptFile: '../../../etc/passwd', runId: 'r-escape' });
  assert.strictEqual(out.status, 'failed');
  assert.match(out.result, /^prompt read: /);
  assert.match(out.result, /escapes the workspace/);
});

test('an agent declaration and full wiring (system prompt, thread, max-turns) all reach a successful run', async () => {
  fs.mkdirSync(path.join(process.env.WORKSPACE, '.claude', 'agents'), { recursive: true });
  fs.writeFileSync(path.join(process.env.WORKSPACE, '.claude', 'agents', 'k8s-ops.md'), '---\ntools: Read, Grep\n---\n');
  process.env.FAKE_MODE = 'success';
  const out = await runClaude({
    promptText: 'hi', runId: 'r-full', agent: 'k8s-ops', allowedTools: 'Bash', toolsMode: 'merge',
    systemPrompt: 'be nice', maxTurns: 5, threadId: 42,
  });
  assert.strictEqual(out.status, 'succeeded');
});
