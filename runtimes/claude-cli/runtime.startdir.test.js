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
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

const WORKSPACE = tmp('agentops-startdir-ws-');
const START_DIR = path.join(WORKSPACE, 'app');
fs.mkdirSync(START_DIR, { recursive: true });

const binDir = tmp('agentops-startdir-bin-');
fs.writeFileSync(
  path.join(binDir, 'claude'),
  '#!/bin/sh\npwd 1>&2\necho \'{"type":"result","subtype":"success","num_turns":1,"duration_ms":1,"result":"ok","session_id":"sess-startdir"}\'\nexit 0\n',
  { mode: 0o755 },
);
process.env.PATH = `${binDir}${path.delimiter}${process.env.PATH}`;
process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-startdir';
process.env.WORKSPACE = WORKSPACE;
process.env.START_DIR = START_DIR;

const { runClaude, startDirectory } = require('./runtime');

test('startDirectory prefers START_DIR over WORKSPACE when both are set', () => {
  assert.strictEqual(startDirectory(), START_DIR);
});

test('the claude CLI is spawned with cwd=START_DIR, not the bare workspace root', async (t) => {
  // spawnClaude forwards the child's stderr straight to this process's own
  // stderr (`process.stderr.write(c)`) — the fake claude script below writes
  // its cwd (`pwd`) there, so intercepting this process's stderr for the
  // duration of the call is the only way to see what cwd the real spawn()
  // call actually used; `strip()` removes the internal capture from the
  // returned report before this test ever sees it.
  let captured = '';
  const realWrite = process.stderr.write.bind(process.stderr);
  process.stderr.write = (chunk, ...rest) => { captured += chunk; return realWrite(chunk, ...rest); };
  t.after(() => { process.stderr.write = realWrite; });

  const out = await runClaude({ promptText: 'hi', runId: 'r1' });
  assert.strictEqual(out.status, 'succeeded');
  // `realpathSync` resolves any symlink in the platform's tmp dir (macOS'
  // /tmp -> /private/tmp) so the comparison is exact either way.
  assert.strictEqual(captured.trim(), fs.realpathSync(START_DIR));
});
