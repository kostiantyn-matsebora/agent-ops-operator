// cd runtimes/pi && node --test
//
// runtime.js self-executes its long-poll loop as a bottom-of-file IIFE,
// installs real process signal handlers, and exits the process when
// CONTROL_URL/CONVO_ID (or the credential/model) are unset — all guarded on
// `require.main === module` (see runtime.js, and runtime.mainentry.test.js)
// so this suite can `require()` the module for its pure, SDK-free pieces.
// `getSdk()`/`getModelContext()`/`runPi()` load the real pi SDK lazily and
// are exercised separately in runtime.live.test.js, against a real session
// and a local fake OpenAI-compatible provider — not here.
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
process.env.PI_API_KEY = 'test-key';
process.env.PI_MODEL = 'fake-model';
process.env.REPO_URL = ''; // exercises syncRepo's early no-op branch
const HOME = tmp('agentops-pi-home-');
process.env.HOME = HOME; // controls PI_AGENT_DIR/SESSIONS_ROOT
process.env.WORKSPACE = tmp('agentops-pi-workspace-');

const {
  gitEnv, repoURL, run, clearDir, syncRepo, onEvent, resolvePrompt,
  contextIdOf, finish, resolveContinuity, extractBrief, WORKSPACE, SESSIONS_ROOT,
  pollNextUnit, reportDone,
} = require('./runtime');

// Stubs the built-in global `fetch` for the duration of one test and
// restores it after, even on failure -- the same shape `t.mock` would give.
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

test('syncRepo is a no-op when no REPO_URL is configured (MGR-5)', async () => {
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
  const dir = tmp('agentops-pi-cleardir-');
  fs.writeFileSync(path.join(dir, 'a.txt'), 'x');
  fs.mkdirSync(path.join(dir, 'sub'));
  fs.writeFileSync(path.join(dir, 'sub', 'b.txt'), 'y');
  clearDir(dir);
  assert.deepStrictEqual(fs.readdirSync(dir), []);
  assert.ok(fs.existsSync(dir), 'the mount point itself must survive');
});

// ---- onEvent: every transcript branch -----------------------------------------

function state() {
  return { lastText: '', toolCalls: 0, turns: 0, errors: [] };
}

test('onEvent records turn_end text content and counts a turn', () => {
  const s = state();
  onEvent({ type: 'turn_end', message: { stopReason: 'stop', content: [{ type: 'text', text: 'hello' }] } }, s);
  assert.strictEqual(s.lastText, 'hello');
  assert.strictEqual(s.turns, 1);
});

test('onEvent records a turn_end error rather than text', () => {
  const s = state();
  onEvent({ type: 'turn_end', message: { stopReason: 'error', errorMessage: 'boom' } }, s);
  assert.deepStrictEqual(s.errors, ['boom']);
  assert.strictEqual(s.lastText, '');
  assert.strictEqual(s.turns, 1);
});

test('onEvent tracks a tool call start and completion', () => {
  const s = state();
  onEvent({ type: 'tool_execution_start', toolCallId: 'c1', toolName: 'read', args: { path: 'x'.repeat(300) } }, s);
  assert.strictEqual(s.toolCalls, 1);
  onEvent({ type: 'tool_execution_end', toolCallId: 'c1', toolName: 'read', result: 'ok', isError: false }, s);
  // No throw is the assertion for the completion branch — the write path is
  // stdout, not state, matching every other runtime's onEvent shape.
});

test('onEvent ignores an event type it does not know', () => {
  const s = state();
  onEvent({ type: 'agent_start' }, s);
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
  const out = resolvePrompt({ promptFile: '../../../etc/passwd' });
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

// ---- extractBrief -----------------------------------------------------------------

test('extractBrief pulls the <brief> block out and never duplicates it into text', () => {
  const out = extractBrief('All done.\n\n<brief>\nFixed the restart loop\n</brief>');
  assert.strictEqual(out.brief, 'Fixed the restart loop');
  assert.strictEqual(out.text, 'All done.');
});

test('extractBrief reports an empty brief when the agent writes none', () => {
  const out = extractBrief('just an ordinary answer');
  assert.strictEqual(out.brief, '');
  assert.strictEqual(out.text, 'just an ordinary answer');
});

// ---- finish -----------------------------------------------------------------------

test('finish drops the internal openError field and attaches continuity', () => {
  const out = finish({ status: 'succeeded', result: 'ok', openError: 'internal' }, 'continued');
  assert.deepStrictEqual(out, { status: 'succeeded', result: 'ok', continuity: 'continued' });
  assert.ok(!('openError' in out));
});

// ---- resolveContinuity: 2.8a — any resume failure re-checks, unreadable -----
// ---- counts as present, a failed retry is never reported `continued` -------

const noSleep = { delays: [0, 0, 0], sleep: async () => {} };

function markPresent(id) {
  fs.mkdirSync(path.join(SESSIONS_ROOT, id), { recursive: true });
}

test('resolveContinuity reports continued straight away when the first attempt opened fine', async () => {
  const first = { status: 'succeeded', result: 'ok', sessionId: 'present-1' };
  const out = await resolveContinuity('present-1', first, () => { throw new Error('must not retry'); }, noSleep);
  assert.strictEqual(out.continuity, 'continued');
  assert.strictEqual(out.result, 'ok');
});

test('a resume failure whose text does NOT say "not found" still triggers the re-check, not an immediate continued claim', async () => {
  const id = 'weird-error-id';
  markPresent(id);
  let retries = 0;
  const first = { status: 'failed', openError: 'ECONNRESET: the vendor transport dropped', result: 'x' };
  const retry = () => { retries++; return { status: 'succeeded', result: 'recovered', sessionId: id }; };
  const out = await resolveContinuity(id, first, retry, noSleep);
  assert.strictEqual(retries, 1, 'any open failure re-checks and retries, not only one naming the session missing');
  assert.strictEqual(out.continuity, 'continued');
  assert.strictEqual(out.result, 'recovered');
});

test('an unreadable store counts as PRESENT, not absence, so it retries rather than failing as lost', async () => {
  let retries = 0;
  const first = { status: 'failed', openError: 'no pi session file found', result: 'x' };
  const retry = () => { retries++; return { status: 'succeeded', result: 'recovered', sessionId: 'unreadable-id' }; };
  const opts = { ...noSleep, probe: async () => { throw Object.assign(new Error('EIO'), { code: 'EIO' }); } };
  const out = await resolveContinuity('unreadable-id', first, retry, opts);
  assert.strictEqual(retries, 1);
  assert.strictEqual(out.continuity, 'continued');
});

test('a failed retry is never reported as continued', async () => {
  const id = 'present-but-retry-fails';
  markPresent(id);
  const first = { status: 'failed', openError: 'no pi session file found', result: 'x' };
  const retry = () => ({ status: 'failed', openError: 'still cannot open it', result: 'y' });
  const out = await resolveContinuity(id, first, retry, noSleep);
  assert.strictEqual('continuity' in out, false, 'no continuity claim is made for a retry that also failed');
  assert.match(out.result, /resume: still cannot open it/);
  assert.ok(!('openError' in out), 'the internal field never reaches the report');
});

test('a confirmed absence reports unavailable with no exitCode (no process ever ran)', async () => {
  const first = { status: 'failed', openError: 'no pi session file found', result: 'x' };
  const out = await resolveContinuity('genuinely-gone-id', first, () => { throw new Error('must not retry'); }, noSleep);
  assert.strictEqual(out.continuity, 'unavailable');
  assert.strictEqual('exitCode' in out, false);
  assert.match(out.continuityReason, /could not be reached/);
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
  assert.strictEqual(body.runtimeContextId, 'sid-1');
  assert.strictEqual('sessionId' in body, false, 'the internal field name is replaced by the wire name, not duplicated');
});

test('reportDone omits runtimeContextId when the run reported no handle', async (t) => {
  let body;
  withFetch(t, async (_url, init) => { body = JSON.parse(init.body); return { ok: true }; });
  await reportDone({ runId: 'r2' }, { result: 'ok' });
  assert.strictEqual('runtimeContextId' in body, false);
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

// ---- reportDone: a definitive 4xx is not retried (API-10) --------------------

test('reportDone does not retry a definitive 400 — one call, logged, then returns', async (t) => {
  let calls = 0;
  withFetch(t, async () => {
    calls += 1;
    return { ok: false, status: 400, text: async () => 'result exceeds the bound' };
  });
  const origError = console.error;
  const logged = [];
  console.error = (...args) => logged.push(args.join(' '));
  try {
    await reportDone({ runId: 'r5' }, { result: 'ok' });
  } finally {
    console.error = origError;
  }
  assert.strictEqual(calls, 1, 'a definitive rejection must not be retried');
  assert.ok(logged.some((l) => l.includes('400') && l.includes('result exceeds the bound')), 'the status and body are logged (API-12)');
});

test('reportDone retries a 429, which is transient despite being a 4xx', async (t) => {
  let calls = 0;
  withFetch(t, async () => {
    calls += 1;
    return { ok: calls >= 2, status: calls >= 2 ? 200 : 429, text: async () => 'rate limited' };
  });
  await reportDone({ runId: 'r6' }, { result: 'ok' });
  assert.strictEqual(calls, 2, '429 keeps the bounded retry rather than being treated as definitive');
});

test('reportDone retries a 5xx rather than treating it as a definitive rejection', async (t) => {
  let calls = 0;
  withFetch(t, async () => {
    calls += 1;
    return { ok: calls >= 2, status: calls >= 2 ? 200 : 503, text: async () => 'unavailable' };
  });
  await reportDone({ runId: 'r7' }, { result: 'ok' });
  assert.strictEqual(calls, 2);
});
