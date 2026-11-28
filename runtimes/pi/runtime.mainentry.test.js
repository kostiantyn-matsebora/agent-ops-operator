// cd runtimes/pi && node --test
//
// The startup validation exit(1) branches (LC-4: missing CONTROL_URL/
// CONVO_ID, missing credential, missing model) only run when this file IS
// the process entry point (require.main === module), so they cannot be
// reached by requiring the module like every other test file here. Each is
// exercised as a real subprocess of runtime.js itself — the standard
// os/exec "crasher" pattern — asserting its exit code and stderr, rather
// than mocking the exit away.
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
  const { code, stderr } = runMain({ CONTROL_URL: '', CONVO_ID: '', PI_API_KEY: 'x', PI_MODEL: 'fake-model' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /CONTROL_URL and CONVO_ID are required/);
});

test('main entry exits 1 when no credential is configured', () => {
  const { code, stderr } = runMain({ CONTROL_URL: 'http://127.0.0.1:1', CONVO_ID: 'c1', PI_API_KEY: '', PI_MODEL: 'fake-model' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /PI_API_KEY is required/);
});

test('main entry exits 1 when no model is configured', () => {
  const { code, stderr } = runMain({ CONTROL_URL: 'http://127.0.0.1:1', CONVO_ID: 'c1', PI_API_KEY: 'x', PI_MODEL: '' });
  assert.strictEqual(code, 1);
  assert.match(stderr, /PI_MODEL is required/);
});

test('with RUNTIME_IDLE_TTL_M=0 the process eventually reports the idle TTL and exits 0', async () => {
  // CONTROL_URL points at a closed port, so the loop's first fetch fails and
  // sleeps out its 5s retry before the idle check gets a second, successful
  // look — the same shape the other three runtimes' identical test accepts
  // (no fixed-duration kill, just a wait for the real exit event). Exercises
  // the main loop's own scaffolding (console.log, the fetch-failure retry,
  // the idle-TTL branch, process.exit(0)) without ever touching the pi SDK.
  const child = spawn(process.execPath, [RUNTIME], {
    env: {
      ...process.env,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-idle',
      PI_API_KEY: 'test-key',
      PI_MODEL: 'fake-model',
      HOME: tmp('agentops-pi-idle-home-'),
      WORKSPACE: tmp('agentops-pi-idle-ws-'),
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

test('SIGTERM exits 0 promptly through its own handler, not the idle-TTL path (LC-5)', async () => {
  // A long idle TTL rules out LC-2's exit racing this one — the only way the
  // process can exit 0 here is the signal handler itself.
  const child = spawn(process.execPath, [RUNTIME], {
    env: {
      ...process.env,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-sigterm',
      PI_API_KEY: 'test-key',
      PI_MODEL: 'fake-model',
      HOME: tmp('agentops-pi-sigterm-home-'),
      WORKSPACE: tmp('agentops-pi-sigterm-ws-'),
      RUNTIME_IDLE_TTL_M: '60',
    },
    stdio: ['ignore', 'pipe', 'ignore'],
  });
  let stdout = '';
  child.stdout.on('data', (c) => { stdout += c; });
  // Give the loop one tick so the signal lands mid-poll, not before the
  // handler is installed.
  await new Promise((r) => setTimeout(r, 200));
  child.kill('SIGTERM');
  const code = await new Promise((resolve) => child.once('exit', resolve));
  assert.strictEqual(code, 0);
  assert.match(stdout, /SIGTERM — exiting/);
  assert.doesNotMatch(stdout, /idle TTL reached/);
});
