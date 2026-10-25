// cd runtime-claude && node --test
//
// Exercises the bottom-of-file main loop itself — the one piece of runtime.js
// that only runs when the file is the process entry point (`require.main ===
// module`), so it cannot be reached by requiring the module like every other
// test file here. Both tests below spawn `node runtime.js` for real, but
// neither reaches a work unit (one refuses to start at all, the other exits
// on idle TTL before its first poll) — so neither needs the SDK to be
// installed. A main-loop test that actually DRIVES a unit through the SDK
// belongs to the session-reuse test task (openspec tasks.md 9.4/9.6), with
// query() substituted via runtime.js's __setQueryForTest seam, the same way
// driver-cli.js's old fake-`claude`-on-PATH test stood in for a real spawn.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

test('run directly with CONTROL_URL and CONVO_ID unset, the process refuses to start', async () => {
  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: { ...process.env, CONTROL_URL: '', CONVO_ID: '' },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stderr = '';
  child.stderr.on('data', (c) => { stderr += c; });
  const code = await new Promise((resolve) => child.once('exit', resolve));
  assert.strictEqual(code, 1);
  assert.match(stderr, /CONTROL_URL and CONVO_ID are required/);
});

test('with RUNTIME_IDLE_TTL_M=0 the process exits immediately, before ever polling for work', async () => {
  // CONTROL_URL points at a closed port — reaching it at all would be a bug,
  // since the idle check runs before the first fetch.
  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: {
      ...process.env,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-idle',
      POD_NAME: 'pod-idle',
      WORKSPACE: tmp('agentops-mainloop-idle-'),
      REPO_URL: '',
      RUNTIME_IDLE_TTL_M: '0',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stdout = '';
  child.stdout.on('data', (c) => { stdout += c; });
  const [code] = await new Promise((resolve) => child.once('exit', (c) => resolve([c])));
  assert.strictEqual(code, 0);
  assert.match(stdout, /idle TTL reached/);
});
