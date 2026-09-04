// cd runtime-copilot && node --test
//
// The two startup validation exit(1) branches (missing CONTROL_URL/CONVO_ID,
// missing credential) only run when this file IS the process entry point
// (require.main === module), so they cannot be reached by requiring the
// module like every other test file here. Each is exercised as a real
// subprocess of the compiled test file itself — the standard os/exec
// "crasher" pattern — asserting its exit code and stderr, rather than
// mocking the exit away.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync, spawn } = require('child_process');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

const RUNTIME = path.join(__dirname, 'runtime.js');

function runMain(env) {
  try {
    const stdout = execFileSync(process.execPath, [RUNTIME], {
      env: { ...process.env, ...env },
      stdio: 'pipe',
      timeout: 2000,
    });
    return { code: 0, stdout: String(stdout), stderr: '' };
  } catch (e) {
    return { code: e.status, stdout: String(e.stdout || ''), stderr: String(e.stderr || '') };
  }
}

test('main entry exits 1 when CONTROL_URL/CONVO_ID are unset', () => {
  const { code, stderr } = runMain({ CONTROL_URL: '', CONVO_ID: '', COPILOT_GITHUB_TOKEN: 'x' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /CONTROL_URL and CONVO_ID are required/);
});

test('main entry exits 1 when no credential is configured', () => {
  const { code, stderr } = runMain({ CONTROL_URL: 'http://127.0.0.1:1', CONVO_ID: 'c1', COPILOT_GITHUB_TOKEN: '', COPILOT_PROVIDER_JSON: '' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /COPILOT_GITHUB_TOKEN is required/);
});

test('main entry exits 1 when COPILOT_PROVIDER_JSON is not valid JSON', () => {
  const { code, stderr } = runMain({ CONTROL_URL: 'http://127.0.0.1:1', CONVO_ID: 'c1', COPILOT_GITHUB_TOKEN: '', COPILOT_PROVIDER_JSON: '{not-json' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /COPILOT_PROVIDER_JSON is not JSON/);
});

test('with RUNTIME_IDLE_TTL_M=0 the process eventually reports the idle TTL and exits 0', async () => {
  // CONTROL_URL points at a closed port, so the loop's first fetch fails and
  // sleeps out its 5s retry before the idle check gets a second, successful
  // look — the same shape runtime-claude's identical test accepts (no
  // fixed-duration kill, just a wait for the real exit event). Exercises the
  // main loop's own scaffolding (console.log, syncRepo, the fetch-failure
  // retry, the idle-TTL branch, process.exit(0)) without ever touching the
  // Copilot SDK.
  const child = spawn(process.execPath, [RUNTIME], {
    env: {
      ...process.env,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-idle',
      COPILOT_GITHUB_TOKEN: 'test-token',
      REPO_URL: '',
      HOME: tmp('agentops-copilot-idle-home-'),
      WORKSPACE: tmp('agentops-copilot-idle-ws-'),
      RUNTIME_IDLE_TTL_M: '0',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stdout = '';
  child.stdout.on('data', (c) => { stdout += c; });
  const code = await new Promise((resolve) => child.once('exit', resolve));
  assert.strictEqual(code, 0);
  assert.match(stdout, /idle TTL reached/);
});
