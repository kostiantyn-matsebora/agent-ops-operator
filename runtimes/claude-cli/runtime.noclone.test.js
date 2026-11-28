// cd runtime-claude && node --test
//
// multi-repo-workspace: cloning left every runtime for the `workspace-sync`
// init container. This runtime now receives an already-populated WORKSPACE
// and must perform no clone of its own and invoke no git command — replaces
// the old runtime.gitclone.test.js/runtime.httpsenv.test.js/
// runtime.sshenv.test.js/runtime.syncfail.test.js, which exercised the
// clone this file deleted.
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

test('runtime.js exports no clone-related function anymore — cloning left the runtime', () => {
  const mod = require('./runtime');
  for (const name of ['gitEnv', 'repoURL', 'run', 'clearDir', 'syncRepo']) {
    assert.strictEqual(name in mod, false, `${name} must not exist on the module`);
  }
});

function startStubManager(unit) {
  let served = false;
  let resolveDone;
  const donePosted = new Promise((r) => { resolveDone = r; });
  const server = http.createServer((req, res) => {
    if (req.method === 'GET' && req.url.startsWith('/work')) {
      if (!served) {
        served = true;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(unit));
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

// A real end-to-end run through the main loop (not a require()'d unit
// test): a `git` on PATH that must never be invoked, a `claude` that
// stands in for the real CLI, and a workspace whose contents must survive
// untouched — populating WORKSPACE is workspace-sync's job now, never this
// process's.
test('the main loop never touches the workspace contents or invokes git, end to end', async () => {
  const workspace = tmp('agentops-noclone-ws-');
  fs.writeFileSync(path.join(workspace, 'marker.txt'), 'populated by workspace-sync');

  const binDir = tmp('agentops-noclone-bin-');
  fs.writeFileSync(
    path.join(binDir, 'git'),
    '#!/bin/sh\necho "git invoked despite workspace-sync owning the clone" 1>&2\nexit 1\n',
    { mode: 0o755 },
  );
  fs.writeFileSync(
    path.join(binDir, 'claude'),
    '#!/bin/sh\necho \'{"type":"result","subtype":"success","num_turns":1,"duration_ms":1,"result":"ok","session_id":"sess-noclone"}\'\nexit 0\n',
    { mode: 0o755 },
  );

  const { server, port, donePosted } = await startStubManager({ runId: 'run-noclone-1', promptText: 'hello' });
  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: {
      ...process.env,
      PATH: `${binDir}${path.delimiter}${process.env.PATH}`,
      CONTROL_URL: `http://127.0.0.1:${port}`,
      CONVO_ID: 'conv-noclone',
      POD_NAME: 'pod-noclone',
      WORKSPACE: workspace,
      RUNTIME_IDLE_TTL_M: '10',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stderr = '';
  child.stderr.on('data', (c) => { stderr += c; });

  try {
    const done = await donePosted;
    assert.strictEqual(done.status, 'succeeded');
  } finally {
    child.kill('SIGTERM');
    await new Promise((r) => child.once('exit', r));
    server.close();
  }

  assert.strictEqual(
    fs.readFileSync(path.join(workspace, 'marker.txt'), 'utf8'),
    'populated by workspace-sync',
    'the workspace is never cleared or re-synced by this process',
  );
  assert.ok(!stderr.includes('git invoked'), `git must never run from this process, got stderr: ${stderr}`);
});
