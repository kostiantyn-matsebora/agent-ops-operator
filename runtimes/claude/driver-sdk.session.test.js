// cd runtime-claude && node --test
//
// driver-sdk.js's session lifecycle: open/reuse/re-open (D4), the live
// callback tracking a changed allowlist with no re-open, the continuity
// ladder ported to the re-open path (D5/6.2/6.3), and session teardown on
// an unattributable mid-turn failure (D6). Every test drives executeUnit()
// directly against a fake query() (__setQueryForTest) — no network, no real
// @anthropic-ai/claude-agent-sdk. Options never carry `allowedTools`,
// `agent`, or a `dontAsk` permission mode (5.2) is asserted here too, since
// it is a property of the SAME query() calls these tests already capture.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-sdk-session';
process.env.REPO_URL = '';
const HOME = tmp('agentops-sdksession-home-');
process.env.HOME = HOME; // controls SESSIONS_DIR, via driver-cli.js
process.env.WORKSPACE = tmp('agentops-sdksession-ws-');
const MCP_CONFIG = path.join(tmp('agentops-sdksession-mcp-'), 'mcp.json');
fs.writeFileSync(MCP_CONFIG, JSON.stringify({ mcpServers: {} }));
process.env.MCP_CONFIG = MCP_CONFIG;

const sdk = require('./driver-sdk');
const { SESSIONS_DIR } = require('./driver-cli');

// fakeQueryFactory returns a query() that logs every open (one entry per
// call, each carrying `.options`, `.closed`, `.interrupted`) and serves
// `.next()` from one FLAT, shared event queue — the driver never reads
// ahead of a turn's `result`, so pre-flattening every scripted turn's
// events in call order is enough; no per-open feeding is needed.
function fakeQueryFactory(callLog, eventQueue) {
  return function query({ prompt, options }) {
    const handle = {
      options,
      closed: false,
      interrupted: false,
      next() {
        if (eventQueue.length === 0) return Promise.resolve({ value: undefined, done: true });
        return Promise.resolve({ value: eventQueue.shift(), done: false });
      },
      close() { handle.closed = true; },
      interrupt() { handle.interrupted = true; return Promise.resolve(); },
    };
    callLog.push(handle);
    return handle;
  };
}

function baseUnit(overrides) {
  return { runId: 'r-' + Math.random().toString(36).slice(2), agent: 'ops', allowedTools: 'Bash', toolsMode: 'merge', promptText: 'do the thing', ...overrides };
}

test.beforeEach(() => {
  sdk.__resetLiveSessionForTest();
});

// ---- open / reuse / re-open (D4) -------------------------------------------

test('identical fingerprint units reuse the one open session', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: ['Bash'], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'first answer', is_error: false },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'second answer', is_error: false },
  ]));

  const outA = await sdk.executeUnit(baseUnit({ systemPrompt: 'be terse', maxTurns: 10 }));
  assert.strictEqual(outA.status, 'succeeded');
  assert.strictEqual(outA.result, 'first answer');
  assert.strictEqual(outA.sessionId, 's1');
  assert.strictEqual(outA.continuity, 'new');

  const outB = await sdk.executeUnit(baseUnit({ systemPrompt: 'be terse', maxTurns: 10 }));
  assert.strictEqual(outB.result, 'second answer');

  assert.strictEqual(callLog.length, 1, 'only one session was ever opened');
});

test('a changed systemPrompt tears the old session down and re-opens with resume (D4)', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a1', is_error: false },
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a2', is_error: false },
  ]));

  const outA = await sdk.executeUnit(baseUnit({ systemPrompt: 'be terse' }));
  const outB = await sdk.executeUnit(baseUnit({ systemPrompt: 'be verbose', runtimeContextId: outA.sessionId }));

  assert.strictEqual(callLog.length, 2, 'the changed systemPrompt forced a close and a fresh open');
  assert.strictEqual(callLog[0].closed, true, 'the superseded session was closed before the replacement opened');
  assert.strictEqual(callLog[1].options.resume, outA.sessionId, 'the replacement resumes from the recorded handle (4.3)');
  assert.strictEqual(outB.continuity, 'continued');
});

test('a changed agent name re-opens even though no vendor option carries it (D4 fingerprint input, never Options.agent)', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a1', is_error: false },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a2', is_error: false },
  ]));
  await sdk.executeUnit(baseUnit({ agent: 'ops' }));
  await sdk.executeUnit(baseUnit({ agent: 'release-notes' }));
  assert.strictEqual(callLog.length, 2);
  for (const call of callLog) assert.strictEqual('agent' in call.options, false, 'Options never carries `agent`');
});

test('an allowlist-only change reuses the session, and the live callback reflects the new list immediately (4.2/4.4, D3)', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: ['Bash', 'Read'], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a1', is_error: false },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a2', is_error: false },
  ]));

  await sdk.executeUnit(baseUnit({ allowedTools: 'Bash', toolsMode: 'overwrite' }));
  await sdk.executeUnit(baseUnit({ allowedTools: 'Read', toolsMode: 'overwrite' }));

  assert.strictEqual(callLog.length, 1, 'the allowlist alone never forces a re-open');
  const allowedNow = await callLog[0].options.canUseTool('Read', {});
  assert.strictEqual(allowedNow.behavior, 'allow', 'the live callback reflects the second unit\'s allowlist');
  const deniedNow = await callLog[0].options.canUseTool('Bash', {});
  assert.strictEqual(deniedNow.behavior, 'deny', 'Bash left the allowlist when the second unit overwrote it');
});

// ---- Options never reintroduce what D3 removed (5.2) -----------------------

test('Options passed to query() never carry allowedTools or a dontAsk permission mode, and no bare tool auto-approves', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'ok', is_error: false },
  ]));
  await sdk.executeUnit(baseUnit());
  assert.strictEqual('allowedTools' in callLog[0].options, false);
  assert.strictEqual('agent' in callLog[0].options, false);
  assert.strictEqual(callLog[0].options.permissionMode, 'default');
  assert.notStrictEqual(callLog[0].options.permissionMode, 'dontAsk');
  assert.strictEqual(typeof callLog[0].options.canUseTool, 'function', 'every call is decided by the callback');
});

// ---- session id reporting (D5, 6.1) ----------------------------------------

test('a unit served from a REUSED session still reports this turn\'s own session id', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    { type: 'system', subtype: 'init', session_id: 's1', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 1, duration_ms: 1, result: 'a1', is_error: false },
    { type: 'result', subtype: 'success', session_id: 's1', num_turns: 2, duration_ms: 1, result: 'a2', is_error: false },
  ]));
  await sdk.executeUnit(baseUnit());
  const outB = await sdk.executeUnit(baseUnit());
  assert.strictEqual(outB.sessionId, 's1');
  assert.strictEqual(callLog.length, 1, 'served from the reused session, not a fresh open');
});

// ---- the spin breaker, carried through unchanged -----------------------------

test('a repeated unparsable tool call stops the run, reports the spin message, and carries continuity normally', async () => {
  const unparsed = { __unparsedToolInput: { raw: '{bad json' } };
  const events = [
    { type: 'system', subtype: 'init', session_id: 's-spin', model: 'x', tools: [], mcp_servers: [] },
    ...Array.from({ length: 5 }, () => ({ type: 'assistant', message: { id: 'm1', content: [{ type: 'tool_use', id: 't1', name: 'Bash', input: unparsed }] } })),
    { type: 'result', subtype: 'error_during_execution', session_id: 's-spin', num_turns: 5, duration_ms: 1, result: '', is_error: true },
  ];
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, events));
  const out = await sdk.executeUnit(baseUnit());
  assert.strictEqual(out.status, 'failed');
  assert.match(out.result, /Stopped: a tool call that could not be formed/);
  assert.strictEqual(out.continuity, 'new');
  assert.strictEqual(callLog[0].interrupted, true, 'the breaker interrupted the turn rather than letting it run on');
});

// ---- unrecoverable session failure tears the session down (D6) --------------

test('the live session ending mid-turn with no result is reported failed, with no continuity claim, and torn down', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [])); // empty queue — next() reports done immediately
  const out = await sdk.executeUnit(baseUnit());
  assert.strictEqual(out.status, 'failed');
  assert.strictEqual('continuity' in out, false, 'an unattributable failure claims no continuity outcome');
  assert.match(out.result, /ended unexpectedly/);
  assert.strictEqual(sdk.__getLiveSessionForTest(), null, 'the session was torn down');
});

test('an exception reading the live session tears it down, and the next unit opens a fresh one', async () => {
  const callLog = [];
  let shouldThrow = true;
  const query = ({ options }) => {
    const handle = {
      options,
      closed: false,
      next() {
        return shouldThrow ? Promise.reject(new Error('boom')) : Promise.resolve({ value: undefined, done: true });
      },
      close() { handle.closed = true; },
      interrupt() { return Promise.resolve(); },
    };
    callLog.push(handle);
    return handle;
  };
  sdk.__setQueryForTest(query);

  const out = await sdk.executeUnit(baseUnit());
  assert.strictEqual(out.status, 'failed');
  assert.match(out.result, /session error/);
  assert.strictEqual(callLog[0].closed, true);

  shouldThrow = false;
  await sdk.executeUnit(baseUnit());
  assert.strictEqual(callLog.length, 2, 'the next unit opened a brand new session rather than reusing the torn-down one');
});

// ---- continuity ladder ported to the re-open path (D5, 6.2, 6.3) ------------

test('a resume whose context is confirmed missing FAILS the run with the existing user-facing text and claims no success', async () => {
  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    // No init at all — the vendor could not find the session to resume.
    { type: 'result', subtype: 'error_during_execution', num_turns: 0, duration_ms: 1, result: '', is_error: true },
  ]));
  const unit = baseUnit({ runtimeContextId: 'genuinely-nowhere-sdk' });
  const start = Date.now();
  const out = await sdk.executeUnit(unit);
  assert.ok(Date.now() - start >= 5000, 'the full ladder (500+1500+3000ms) is walked before concluding loss');
  assert.strictEqual(out.status, 'failed');
  assert.strictEqual(out.continuity, 'unavailable');
  assert.match(out.result, /This conversation cannot be continued\.\*\*/);
  assert.strictEqual('sessionId' in out, false, 'no handle was ever produced to surrender');
});

test('a resume that reappears on re-check retries once, succeeds, and reports continued (D5)', async () => {
  const projDir = path.join(SESSIONS_DIR, 'proj-sdk-reappear');
  fs.mkdirSync(projDir, { recursive: true });
  fs.writeFileSync(path.join(projDir, 'sess-reappear-sdk-ctx.jsonl'), '{}');

  const callLog = [];
  sdk.__setQueryForTest(fakeQueryFactory(callLog, [
    // First attempt: looks lost (no session id, an error result).
    { type: 'result', subtype: 'error_during_execution', num_turns: 0, duration_ms: 1, result: '', is_error: true },
    // Retry, after the store answered present: this one lands.
    { type: 'system', subtype: 'init', session_id: 'reappear-sdk-ctx', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 'reappear-sdk-ctx', num_turns: 1, duration_ms: 1, result: 'recovered', is_error: false },
  ]));
  const out = await sdk.executeUnit(baseUnit({ runtimeContextId: 'reappear-sdk-ctx' }));
  assert.strictEqual(out.continuity, 'continued');
  assert.strictEqual(out.result, 'recovered');
  assert.strictEqual(callLog.length, 2, 'the retry opened a second session');
  assert.strictEqual(callLog[1].options.resume, 'reappear-sdk-ctx');
});

test('a fresh unit with no runtimeContextId reports continuity "new"', async () => {
  sdk.__setQueryForTest(fakeQueryFactory([], [
    { type: 'system', subtype: 'init', session_id: 's-fresh', model: 'x', tools: [], mcp_servers: [] },
    { type: 'result', subtype: 'success', session_id: 's-fresh', num_turns: 1, duration_ms: 1, result: 'ok', is_error: false },
  ]));
  const out = await sdk.executeUnit(baseUnit());
  assert.strictEqual(out.continuity, 'new');
});
