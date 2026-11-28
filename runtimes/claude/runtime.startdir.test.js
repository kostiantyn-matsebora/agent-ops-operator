// cd runtime-claude && node --test
//
// multi-repo-workspace (design D7): START_DIR, when the manager set one, is
// where the agent starts — WORKSPACE itself otherwise. WORKSPACE/START_DIR
// are module-level consts captured at require time, so this environment
// gets its own test FILE, same convention runtime.httpsenv.test.js (now
// deleted) and runtime.sshenv.test.js (now deleted) used for their own env.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-startdir';
process.env.WORKSPACE = '/workspace';
process.env.START_DIR = '/workspace/app';

const {
  computeFingerprint, openSession, startDirectory, WORKSPACE, START_DIR,
  __setQueryForTest, __resetLiveSessionForTest,
} = require('./runtime');

test('startDirectory prefers START_DIR over WORKSPACE when both are set', () => {
  assert.strictEqual(START_DIR, '/workspace/app');
  assert.strictEqual(startDirectory(), '/workspace/app');
  assert.notStrictEqual(startDirectory(), WORKSPACE);
});

test('computeFingerprint.cwd is the start directory, not the workspace root', () => {
  const fp = computeFingerprint({}, {});
  assert.strictEqual(fp.cwd, '/workspace/app');
});

test('openSession hands the SDK cwd=START_DIR, never the bare workspace root', (t) => {
  const calls = [];
  __setQueryForTest(({ options }) => {
    calls.push(options);
    return { async next() { return { value: undefined, done: true }; }, async close() {}, async interrupt() {} };
  });
  t.after(() => { __setQueryForTest(null); __resetLiveSessionForTest(); });

  const unit = { runId: 'r1' };
  openSession(unit, computeFingerprint(unit, {}, []), [], {});
  assert.strictEqual(calls.length, 1);
  assert.strictEqual(calls[0].cwd, '/workspace/app');
});
