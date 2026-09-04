// cd runtime-claude && node --test
//
// Exercises the bottom-of-file main loop itself — the one piece of runtime.js
// that only runs when the file is the process entry point (`require.main ===
// module`), so it cannot be reached by requiring the module like every other
// test file here. Instead this spawns `node runtime.js` for real, against a
// real local HTTP server standing in for the manager's /work + /work/done
// long-poll contract, and a real fake `claude` binary on PATH — the actual
// long-poll -> dispatch -> report cycle, end to end, no mocking of runtime.js
// itself.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const http = require('http');
const { spawn } = require('child_process');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

const FAKE_CLAUDE = `#!/bin/sh
echo '{"type":"result","subtype":"success","num_turns":1,"duration_ms":1,"result":"ok from main loop","session_id":"sess-main"}'
exit 0
`;
const binDir = tmp('agentops-mainloop-bin-');
fs.writeFileSync(path.join(binDir, 'claude'), FAKE_CLAUDE, { mode: 0o755 });

function startStubManager() {
  let served = false;
  let resolveDone;
  const donePosted = new Promise((r) => { resolveDone = r; });
  const server = http.createServer((req, res) => {
    if (req.method === 'GET' && req.url.startsWith('/work')) {
      if (!served) {
        served = true;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ runId: 'run-main-1', promptText: 'hello from the stub manager' }));
      } else {
        res.writeHead(204);
        res.end();
      }
      return;
    }
    if (req.method === 'POST' && req.url === '/work/done') {
      let body = '';
      req.on('data', (c) => { body += c; });
      req.on('end', () => {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end('{}');
        resolveDone(JSON.parse(body));
      });
      return;
    }
    res.writeHead(404);
    res.end();
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({ server, port: server.address().port, donePosted }));
  });
}

test('the main loop long-polls for work, runs it, and reports it back', async () => {
  const { server, port, donePosted } = await startStubManager();
  const workspace = tmp('agentops-mainloop-ws-');
  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: {
      ...process.env,
      PATH: `${binDir}${path.delimiter}${process.env.PATH}`,
      CONTROL_URL: `http://127.0.0.1:${port}`,
      CONVO_ID: 'conv-main',
      POD_NAME: 'pod-main',
      WORKSPACE: workspace,
      REPO_URL: '',
      RUNTIME_IDLE_TTL_M: '10',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stderr = '';
  child.stderr.on('data', (c) => { stderr += c; });

  try {
    const done = await donePosted;
    assert.strictEqual(done.convo, 'conv-main');
    assert.strictEqual(done.runId, 'run-main-1');
    assert.strictEqual(done.status, 'succeeded');
    assert.match(done.result, /ok from main loop/);
    // Reported under BOTH the current and the retired handle name, for one release.
    assert.strictEqual(done.runtimeContextId, 'sess-main');
    assert.strictEqual(done.sessionId, 'sess-main');
  } finally {
    child.kill('SIGTERM');
    await new Promise((r) => child.once('exit', r));
    server.close();
  }
  assert.strictEqual(stderr, '', `runtime.js wrote to stderr: ${stderr}`);
});

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
