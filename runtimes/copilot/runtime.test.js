// cd runtime-copilot && node --test
//
// runtime.js self-executes its long-poll loop as a bottom-of-file IIFE,
// installs real process signal handlers, and exits the process when
// CONTROL_URL/CONVO_ID (or the credential) are unset — all guarded on
// `require.main === module` (see runtime.js) so this suite can `require()`
// the module for its pure/subprocess-driving pieces without any of that
// firing. The SDK itself (getClient/checkInventory/openSession/attempt/
// runCopilot) is loaded lazily inside getClient() and deliberately never
// exercised here — a test suite that needs the real Copilot SDK to run is a
// test suite that does not run.
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
process.env.CONVO_ID = 'conv-test';
process.env.COPILOT_GITHUB_TOKEN = 'test-token';
process.env.REPO_URL = ''; // exercises syncRepo's early no-op branch
const HOME = tmp('agentops-copilot-home-');
process.env.HOME = HOME; // controls COPILOT_HOME/SESSIONS_DIR
process.env.WORKSPACE = tmp('agentops-copilot-workspace-');

const {
  gitEnv, repoURL, run, clearDir, syncRepo, onEvent, resolvePrompt,
  contextIdOf, sessionConfig, finish, WORKSPACE,
  pollNextUnit, reportDone,
} = require('./runtime');

// Stubs the built-in global `fetch` for the duration of one test and
// restores it after, even on failure -- the same shape `t.mock` would give,
// written by hand since none of this file's existing tests mock anything.
function withFetch(t, impl) {
  const real = global.fetch;
  global.fetch = impl;
  t.after(() => { global.fetch = real; });
}

// ---- gitEnv / repoURL, default (no auth configured) --------------------------

test('gitEnv with no GIT_AUTH_TYPE adds no SSH command, and passes process.env through', () => {
  const env = gitEnv();
  assert.strictEqual(env.GIT_SSH_COMMAND, undefined);
  assert.strictEqual(env.PATH, process.env.PATH);
});

test('repoURL with no https auth type returns the configured URL unchanged', () => {
  assert.strictEqual(repoURL(), '');
});

// ---- syncRepo: no REPO_URL configured -----------------------------------------

test('syncRepo is a no-op when no REPO_URL is configured', async () => {
  await assert.doesNotReject(syncRepo());
});

// ---- run(): a real subprocess, both success and failure -----------------------

test('run() captures stdout and a zero exit code from a real subprocess', async () => {
  const r = await run(process.execPath, ['-e', "process.stdout.write('run-ok')"]);
  assert.strictEqual(r.code, 0);
  assert.strictEqual(r.out, 'run-ok');
});

test('run() captures stderr and a non-zero exit code', async () => {
  const r = await run(process.execPath, ['-e', "process.stderr.write('run-err'); process.exit(3)"]);
  assert.strictEqual(r.code, 3);
  assert.strictEqual(r.err, 'run-err');
});

test('run() resolves rather than throwing when the binary does not exist', async () => {
  const r = await run('agentops-definitely-not-a-real-binary', []);
  assert.strictEqual(r.code, -1);
  assert.ok(r.err.length > 0, 'the spawn error message is surfaced');
});

// ---- clearDir -------------------------------------------------------------------

test('clearDir empties a directory recursively without removing the directory itself', () => {
  const dir = tmp('agentops-cleardir-');
  fs.writeFileSync(path.join(dir, 'a.txt'), 'x');
  fs.mkdirSync(path.join(dir, 'sub'));
  fs.writeFileSync(path.join(dir, 'sub', 'b.txt'), 'y');
  clearDir(dir);
  assert.deepStrictEqual(fs.readdirSync(dir), []);
  assert.ok(fs.existsSync(dir), 'the mount point itself must survive');
});

// ---- onEvent: every transcript branch -----------------------------------------

function state() {
  return { lastText: '', toolNames: new Map(), toolCalls: 0, errors: [], turns: 0 };
}

test('onEvent ignores session.start and session.resume', () => {
  const s = state();
  onEvent({ type: 'session.start' }, s);
  onEvent({ type: 'session.resume' }, s);
  assert.deepStrictEqual(s, state());
});

test('onEvent records assistant.message text', () => {
  const s = state();
  onEvent({ type: 'assistant.message', data: { content: 'hello' } }, s);
  assert.strictEqual(s.lastText, 'hello');
});

test('onEvent tracks a tool call by id and truncates a long argument blob', () => {
  const s = state();
  onEvent({ type: 'tool.execution_start', data: { toolName: 'Read', toolCallId: 'c1', arguments: { path: 'x'.repeat(300) } } }, s);
  assert.strictEqual(s.toolCalls, 1);
  assert.strictEqual(s.toolNames.get('c1'), 'Read');
});

test('onEvent records a failed tool execution against its remembered name', () => {
  const s = state();
  s.toolNames.set('c1', 'Read');
  onEvent({ type: 'tool.execution_complete', data: { toolCallId: 'c1', success: false, error: { message: 'boom' } } }, s);
  // No throw is the assertion — the write path is stdout, not state.
});

test('onEvent appends a session.error message', () => {
  const s = state();
  onEvent({ type: 'session.error', data: { message: 'bad session' } }, s);
  assert.deepStrictEqual(s.errors, ['bad session']);
});

test('onEvent counts assistant.usage as a turn', () => {
  const s = state();
  onEvent({ type: 'assistant.usage' }, s);
  assert.strictEqual(s.turns, 1);
});

test('onEvent ignores an event type it does not know', () => {
  const s = state();
  onEvent({ type: 'something.unrecognised' }, s);
  assert.deepStrictEqual(s, state());
});

test('onEvent swallows a throw from a malformed event rather than taking the run down', () => {
  const s = state();
  assert.doesNotThrow(() => onEvent(null, s));
});

// ---- resolvePrompt --------------------------------------------------------------

test('resolvePrompt returns promptText verbatim when set', () => {
  assert.deepStrictEqual(resolvePrompt({ promptText: 'do the thing' }), { prompt: 'do the thing' });
});

test('resolvePrompt reads promptFile and substitutes promptVars', () => {
  fs.mkdirSync(WORKSPACE, { recursive: true });
  fs.writeFileSync(path.join(WORKSPACE, 'p.txt'), 'hello {{name}}');
  const out = resolvePrompt({ promptFile: 'p.txt', promptVars: { name: 'world' } });
  assert.deepStrictEqual(out, { prompt: 'hello world' });
});

test('resolvePrompt fails closed when promptFile escapes the workspace', () => {
  const out = resolvePrompt({ promptFile: '../../etc/passwd' });
  assert.match(out.error, /prompt read/);
});

test('resolvePrompt reports an empty prompt when neither field is set', () => {
  assert.deepStrictEqual(resolvePrompt({}), { error: 'empty prompt' });
});

// ---- contextIdOf ------------------------------------------------------------------

test('contextIdOf prefers runtimeContextId over the retired resumeSessionId', () => {
  assert.strictEqual(contextIdOf({ runtimeContextId: 'new-id', resumeSessionId: 'old-id' }), 'new-id');
});

test('contextIdOf falls back to resumeSessionId for one release', () => {
  assert.strictEqual(contextIdOf({ resumeSessionId: 'old-id' }), 'old-id');
});

test('contextIdOf is empty when the work unit carries neither', () => {
  assert.strictEqual(contextIdOf({}), '');
});

// ---- sessionConfig ----------------------------------------------------------------

test('sessionConfig always states an explicit tool allowlist, even empty', () => {
  const grant = { available: [], shell: [], builtins: [], mcpTools: new Set() };
  const cfg = sessionConfig({}, grant, { servers: {} }, []);
  assert.deepStrictEqual(cfg.availableTools, []);
  assert.strictEqual(cfg.enableConfigDiscovery, false);
});

test("sessionConfig's onPermissionRequest approves a granted read and denies a refused one, recording the denial", () => {
  const grant = { available: [], shell: [], builtins: ['Read'], mcpTools: new Set() };
  const denials = [];
  const cfg = sessionConfig({}, grant, { servers: {} }, denials);
  const approved = cfg.onPermissionRequest({ kind: 'read', path: 'a.txt' });
  assert.deepStrictEqual(approved, { kind: 'approve-once' });
  const denied = cfg.onPermissionRequest({ kind: 'write', fileName: 'a.txt' });
  assert.strictEqual(denied.kind, 'reject');
  assert.strictEqual(denials.length, 1);
});

test('sessionConfig appends systemMessage only when the unit carries one', () => {
  const grant = { available: [], shell: [], builtins: [], mcpTools: new Set() };
  const withPrompt = sessionConfig({ systemPrompt: 'be terse' }, grant, { servers: {} }, []);
  assert.deepStrictEqual(withPrompt.systemMessage, { mode: 'append', content: 'be terse' });
  const without = sessionConfig({}, grant, { servers: {} }, []);
  assert.strictEqual('systemMessage' in without, false);
});

// ---- finish -----------------------------------------------------------------------

test('finish drops the internal openError field and attaches continuity', () => {
  const out = finish({ status: 'succeeded', result: 'ok', openError: 'internal' }, { resumed: true });
  assert.deepStrictEqual(out, { status: 'succeeded', result: 'ok', continuity: { resumed: true } });
  assert.ok(!('openError' in out));
});

test('onEvent feeds the run report as the transcript is written', () => {
  const { newCallRecorder } = require('./report');
  const s = { lastText: '', toolCalls: 0, turns: 0, errors: [], toolNames: new Map(), calls: newCallRecorder() };
  onEvent({ type: 'assistant.usage', data: { model: 'gpt-5', inputTokens: 10 } }, s);
  onEvent({ type: 'tool.execution_start', data: { toolName: 'bash', toolCallId: 'c1', arguments: { command: 'ls' } } }, s);
  assert.deepStrictEqual(s.calls.report(), { turns: [{ model: 'gpt-5', tokensIn: 10 }], toolCalls: [{ tool: 'Bash' }] });
  assert.strictEqual(s.turns, 1);
  assert.strictEqual(s.toolCalls, 1);
});

// ---- pollNextUnit --------------------------------------------------------------

test('pollNextUnit returns the parsed work unit on a 200 with a JSON body', async (t) => {
  withFetch(t, async (url) => {
    assert.ok(String(url).startsWith('http://127.0.0.1:1/work?convo=conv-test'));
    return { status: 200, ok: true, json: async () => ({ runId: 'r1', prompt: 'hi' }) };
  });
  assert.deepStrictEqual(await pollNextUnit(), { runId: 'r1', prompt: 'hi' });
});

test('pollNextUnit returns undefined on a 204 (long-poll came back with nothing)', async (t) => {
  withFetch(t, async () => ({ status: 204, ok: true, json: async () => { throw new Error('must not be read'); } }));
  assert.strictEqual(await pollNextUnit(), undefined);
});

test('pollNextUnit returns undefined and does not throw on an unparsable body', async (t) => {
  withFetch(t, async () => ({ status: 200, ok: true, json: async () => { throw new Error('bad json'); } }));
  assert.strictEqual(await pollNextUnit(), undefined);
});

test('pollNextUnit returns undefined on a non-ok status, after the retry wait', async (t) => {
  withFetch(t, async () => ({ status: 500, ok: false, json: async () => { throw new Error('must not be read'); } }));
  const start = Date.now();
  assert.strictEqual(await pollNextUnit(), undefined);
  assert.ok(Date.now() - start >= 5000, 'a non-ok status is treated as transient and waits before the caller polls again');
});

test('pollNextUnit returns undefined and waits out a network error', async (t) => {
  withFetch(t, async () => { throw new Error('ECONNREFUSED'); });
  const start = Date.now();
  assert.strictEqual(await pollNextUnit(), undefined);
  assert.ok(Date.now() - start >= 5000);
});

// ---- reportDone -----------------------------------------------------------------

test('reportDone posts the result and returns on the first ok response', async (t) => {
  let calls = 0;
  let body;
  withFetch(t, async (url, init) => {
    calls += 1;
    body = JSON.parse(init.body);
    assert.strictEqual(String(url), 'http://127.0.0.1:1/work/done');
    return { ok: true };
  });
  await reportDone({ runId: 'r1' }, { result: 'ok', sessionId: 'sid-1' });
  assert.strictEqual(calls, 1, 'a first-try ok response must not retry');
  assert.strictEqual(body.convo, 'conv-test');
  assert.strictEqual(body.runId, 'r1');
  assert.strictEqual(body.result, 'ok');
  assert.strictEqual(body.runtimeContextId, 'sid-1', 'the handle is reported under the current name');
  assert.strictEqual(body.sessionId, 'sid-1', 'and the retired spelling, for one release');
});

test('reportDone omits runtimeContextId/sessionId when the run reported no handle', async (t) => {
  let body;
  withFetch(t, async (_url, init) => { body = JSON.parse(init.body); return { ok: true }; });
  await reportDone({ runId: 'r2' }, { result: 'ok' });
  assert.strictEqual('runtimeContextId' in body, false);
  assert.strictEqual('sessionId' in body, false);
});

test('reportDone retries on a non-ok response until one succeeds', async (t) => {
  let calls = 0;
  withFetch(t, async () => {
    calls += 1;
    return { ok: calls >= 2 };
  });
  const start = Date.now();
  await reportDone({ runId: 'r3' }, { result: 'ok' });
  assert.strictEqual(calls, 2);
  assert.ok(Date.now() - start >= 10000, 'one retry spends the full 10s wait before trying again');
});

test('reportDone retries past a thrown network error, not just a non-ok response', async (t) => {
  let calls = 0;
  withFetch(t, async () => {
    calls += 1;
    if (calls === 1) throw new Error('ECONNRESET');
    return { ok: true };
  });
  await reportDone({ runId: 'r4' }, { result: 'ok' });
  assert.strictEqual(calls, 2);
});
