// cd runtimes/copilot && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { confirmContextMissing, stateDirPresent, DEFAULT_DELAYS_MS } = require('./continuity');

const noSleep = { delays: [0, 0, 0], sleep: async () => {} };

function mkStore() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-state-'));
  const sessions = path.join(root, 'session-state');
  fs.mkdirSync(sessions);
  return sessions;
}

test('the default ladder is short and bounded', () => {
  assert.deepStrictEqual(DEFAULT_DELAYS_MS, [500, 1500, 3000]);
});

test('present state is present', async () => {
  const s = mkStore();
  fs.mkdirSync(path.join(s, 'abc'));
  assert.strictEqual(await stateDirPresent(s, 'abc'), true);
  assert.strictEqual(await confirmContextMissing(s, 'abc', noSleep), false);
});

test('confirmed absence after every re-check is missing', async () => {
  const s = mkStore();
  assert.strictEqual(await stateDirPresent(s, 'gone'), false);
  assert.strictEqual(await confirmContextMissing(s, 'gone', noSleep), true);
});

test('state that reappears during the ladder is NOT missing', async () => {
  const s = mkStore();
  let n = 0;
  const got = await confirmContextMissing(s, 'late', {
    delays: [0, 0, 0],
    sleep: async () => { if (++n === 2) fs.mkdirSync(path.join(s, 'late')); },
  });
  assert.strictEqual(got, false);
});

test('an unreadable store is NOT absence', async () => {
  const got = await confirmContextMissing('/x', 'id', { ...noSleep, probe: async () => { throw Object.assign(new Error('EIO'), { code: 'EIO' }); } });
  assert.strictEqual(got, false);
});

test('a session-state root that does not answer is not absence either', async () => {
  // The root itself is a FILE: stat of <root>/<id> gives ENOTDIR, which is the
  // store misbehaving, never the context being gone.
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-state-'));
  const notADir = path.join(root, 'session-state');
  fs.writeFileSync(notADir, 'x');
  await assert.rejects(stateDirPresent(notADir, 'id'));
  assert.strictEqual(await confirmContextMissing(notADir, 'id', noSleep), false);
});

test('a missing root is plain absence', async () => {
  // No volume mounted and nothing ever written: every check says "not there".
  const root = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-state-')), 'never-created');
  assert.strictEqual(await stateDirPresent(root, 'id'), false);
});

test('no handle is trivially missing', async () => {
  assert.strictEqual(await confirmContextMissing('/x', '', noSleep), true);
});

test('a root that exists but cannot be LISTED is not absence — re-throws past the ENOENT check', async (t) => {
  // Traversal (stat of the missing child) needs only EXECUTE on the root, so it
  // still resolves ENOENT; LISTING it (the root-must-answer readdir) needs READ
  // too, and its absence is EACCES, not ENOENT — the one path that reaches the
  // `pe.code !== 'ENOENT'` re-throw inside stateDirPresent, previously untested.
  //
  // The mode bits this relies on mean nothing to root — the kernel skips the
  // permission check entirely for uid 0 — so a root-run container (the
  // default for an unprivileged Docker image, as CI runs) would see readdir
  // succeed and this assertion fail for a reason that has nothing to do with
  // the behavior under test.
  if (process.getuid && process.getuid() === 0) {
    t.skip('running as root: permission bits are not enforced, so EACCES cannot occur');
    return;
  }
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-state-'));
  const sessions = path.join(root, 'session-state');
  fs.mkdirSync(sessions);
  fs.chmodSync(sessions, 0o111); // execute-only: lookup works, listing does not
  try {
    await assert.rejects(stateDirPresent(sessions, 'abc'), /EACCES/);
    assert.strictEqual(await confirmContextMissing(sessions, 'abc', noSleep), false);
  } finally {
    fs.chmodSync(sessions, 0o755); // restore, or the tmp cleanup itself fails
  }
});

test('omitting delays falls back to DEFAULT_DELAYS_MS, not just a caller-supplied ladder', async () => {
  // Every other test passes opts.delays explicitly, so `opts.delays ||
  // DEFAULT_DELAYS_MS` had never taken its fallback. sleep is stubbed so the
  // real three-step ladder costs nothing.
  let calls = 0;
  const got = await confirmContextMissing('/x', 'id', { sleep: async () => {}, probe: async () => { calls++; return false; } });
  assert.strictEqual(got, true);
  assert.strictEqual(calls, DEFAULT_DELAYS_MS.length);
});

test('the default ladder really sleeps, using the real timer when no override is given', async () => {
  // Every other test supplies opts.sleep, so the module's own default sleep
  // function (setTimeout-backed) has never run. Real, tiny delays keep this
  // fast while still exercising it.
  const s = mkStore();
  const start = Date.now();
  const got = await confirmContextMissing(s, 'gone', { delays: [5, 5] });
  assert.strictEqual(got, true);
  assert.ok(Date.now() - start >= 10, 'expected the real timer to have waited');
});
