// cd runtime-claude && node --test
//
// openspec/changes/claude-runtime-sdk/tasks.md 9.5 — the missing-context
// ladder (checkContextPresence/sessionFileExists, the SAME pure functions
// runtime.test.js already covers) exercised end to end THROUGH the SDK
// path: executeUnit(), with a resume attempt whose first turn tells the
// ladder "nothing landed" — a scripted fake query() reporting no session id
// and no result, exactly what a vendor that did not resume looks like
// (resumeSucceeded's own definition in runtime.js).
//
// HOME is pointed at a fresh temp directory BEFORE require('./runtime') so
// SESSIONS_DIR (computed at module load) is controllable per test, the same
// pattern runtime.test.js and runtime.syncfail.test.js already use.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

process.env.HOME = tmp('agentops-continuity-home-'); // controls SESSIONS_DIR
process.env.MCP_CONFIG = path.join(tmp('agentops-continuity-mcp-'), 'absent-mcp.json'); // never exists -> {} every unit, consistently

const {
  executeUnit, SESSIONS_DIR,
  __setQueryForTest, __resetLiveSessionForTest,
} = require('./runtime');

function freshSession(t) {
  __resetLiveSessionForTest();
  t.after(() => __resetLiveSessionForTest());
}

// makeFakeQuery mirrors runtime.sessionreuse.test.js's own helper — a
// scripted fake substituted via __setQueryForTest, never the real SDK.
function makeFakeQuery(sessionsScript) {
  const sessions = [];
  function fakeQuery() {
    const session = { events: (sessionsScript[sessions.length] || []).slice() };
    sessions.push(session);
    return {
      async next() {
        if (!session.events.length) return { value: undefined, done: true };
        return { value: session.events.shift(), done: false };
      },
      async close() {},
      async interrupt() {},
    };
  }
  fakeQuery.sessions = sessions;
  return fakeQuery;
}

// A turn that completes (reaches `result`) but tells resumeSucceeded's own
// test "nothing landed" — no session_id anywhere in the turn, and an empty
// result — exactly what a vendor that did not resume looks like, per
// resumeSucceeded's own comment in runtime.js.
function unresumedTurnEvent() {
  return { type: 'result', subtype: 'error_during_execution', num_turns: 0, duration_ms: 5, result: '', is_error: true };
}

function resumedTurnEvent(sessionId, text) {
  return { type: 'result', subtype: 'success', num_turns: 1, duration_ms: 5, result: text, is_error: false, session_id: sessionId };
}

function writeSessionFile(contextId) {
  const projDir = path.join(SESSIONS_DIR, `proj-${contextId}`);
  fs.mkdirSync(projDir, { recursive: true });
  fs.writeFileSync(path.join(projDir, `sess-${contextId}.jsonl`), '{}');
}

// ---- context reappears on recheck: the run continues -----------------------

test('a resume attempt whose context reappears on re-check retries and continues', async (t) => {
  freshSession(t);
  const contextId = 'reappear-1';
  writeSessionFile(contextId); // present from the start -> found on the ladder's first wait

  const fakeQuery = makeFakeQuery([[unresumedTurnEvent()], [resumedTurnEvent('sess-reappear-cont', 'resumed answer')]]);
  __setQueryForTest(fakeQuery);

  const out = await executeUnit({ runId: 'r1', promptText: 'hi', runtimeContextId: contextId });
  assert.strictEqual(out.status, 'succeeded');
  assert.strictEqual(out.continuity, 'continued');
  assert.strictEqual(out.sessionId, 'sess-reappear-cont');
  assert.match(out.result, /resumed answer/);
  assert.strictEqual(fakeQuery.sessions.length, 2, 'the failed attempt plus one retry after the context reappeared');
});

// ---- confirmed missing: fails with the exact existing user-facing text ----

test('a resume attempt that is confirmed missing fails with the existing user-facing text, after walking the full ladder', async (t) => {
  freshSession(t);
  const contextId = 'genuinely-gone-1'; // no session file ever created for it

  const fakeQuery = makeFakeQuery([[unresumedTurnEvent()]]);
  __setQueryForTest(fakeQuery);

  const start = Date.now();
  const out = await executeUnit({ runId: 'r2', promptText: 'hi', runtimeContextId: contextId });
  assert.ok(Date.now() - start >= 5000, 'confirmed absence must walk the full 500+1500+3000ms ladder');

  assert.strictEqual(out.status, 'failed');
  assert.strictEqual(out.continuity, 'unavailable');
  assert.match(out.continuityReason, /no session files under/);
  assert.match(out.result, /This conversation cannot be continued\.\*\*/, 'the existing user-facing text for a confirmed loss');
  assert.doesNotMatch(out.result, /right now/, 'a confirmed loss must not be worded as a transient outage');
  assert.strictEqual(fakeQuery.sessions.length, 1, 'a confirmed loss spends no second vendor invocation on an answer that should not exist');
});

// ---- store unavailable: fails as an outage, never a loss ------------------

test('a resume attempt whose store cannot be read fails as an outage, not a loss', async (t) => {
  freshSession(t);
  const contextId = 'store-down-1';

  const real = fs.promises.readdir;
  fs.promises.readdir = (dir, opts) => (
    dir === SESSIONS_DIR ? Promise.reject(Object.assign(new Error('stale handle'), { code: 'ESTALE' })) : real(dir, opts)
  );
  t.after(() => { fs.promises.readdir = real; });

  const fakeQuery = makeFakeQuery([[unresumedTurnEvent()]]);
  __setQueryForTest(fakeQuery);

  const start = Date.now();
  const out = await executeUnit({ runId: 'r3', promptText: 'hi', runtimeContextId: contextId });
  assert.ok(Date.now() - start < 3000, 'an unreadable store fails fast, not after the full ladder');

  assert.strictEqual(out.status, 'failed');
  assert.strictEqual(out.continuity, 'unavailable');
  assert.match(out.continuityReason, /store itself errored/);
  assert.doesNotMatch(out.continuityReason, /no session files under/, 'an outage must not be worded as a confirmed loss');
  assert.match(out.result, /This conversation cannot be continued right now\.\*\*/, 'the existing user-facing text for a storage outage');
  assert.strictEqual(fakeQuery.sessions.length, 1, 'an outage spends no second vendor invocation either');
});
