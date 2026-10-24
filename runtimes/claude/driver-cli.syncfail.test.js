// cd runtime-claude && node --test
//
// C5: a workspace sync failure (here, a git clone against an origin that
// cannot be reached) must fail the WORK UNIT with a reason attributing it to
// the sync, not silently fall through to running the agent against a
// missing or stale checkout. Exercised against the real main loop — a real
// `node runtime.js` process (the entrypoint, with RUNTIME_CLAUDE_DRIVER=cli
// so it delegates to driver-cli.js), a real (deliberately unreachable) git
// origin, and a stub manager standing in for /work + /work/done — so a
// regression that reintroduces "log and continue anyway" is caught end to
// end, not only at the syncRepo() unit level (see driver-cli.gitclone.test.js).
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

// A `claude` on PATH that must NEVER run: if the sync failure were swallowed
// and the loop fell through to spawning it anyway, this would be the tell.
const CLAUDE_THAT_MUST_NOT_RUN = `#!/bin/sh
echo "claude invoked despite a failed sync" 1>&2
exit 1
`;
const binDir = tmp('agentops-syncfail-bin-');
fs.writeFileSync(path.join(binDir, 'claude'), CLAUDE_THAT_MUST_NOT_RUN, { mode: 0o755 });

function startStubManager() {
  let served = false;
  let resolveDone;
  const donePosted = new Promise((r) => { resolveDone = r; });
  const server = http.createServer((req, res) => {
    if (req.method === 'GET' && req.url.startsWith('/work')) {
      if (!served) {
        served = true;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ runId: 'run-syncfail-1', promptText: 'hello' }));
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

test('a per-unit sync failure fails that run with a reason, never falling through to the agent', async () => {
  const { server, port, donePosted } = await startStubManager();
  const workspace = tmp('agentops-syncfail-ws-');
  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: {
      ...process.env,
      PATH: `${binDir}${path.delimiter}${process.env.PATH}`,
      RUNTIME_CLAUDE_DRIVER: 'cli',
      CONTROL_URL: `http://127.0.0.1:${port}`,
      CONVO_ID: 'conv-syncfail',
      POD_NAME: 'pod-syncfail',
      WORKSPACE: workspace,
      REPO_URL: path.join(os.tmpdir(), 'agentops-origin-truly-does-not-exist.git'),
      REPO_REF: 'master',
      RUNTIME_IDLE_TTL_M: '10',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  try {
    const done = await donePosted;
    assert.strictEqual(done.convo, 'conv-syncfail');
    assert.strictEqual(done.status, 'failed');
    assert.match(done.result, /workspace sync failed/);
    assert.match(done.result, /git clone/);
  } finally {
    child.kill('SIGTERM');
    await new Promise((r) => child.once('exit', r));
    server.close();
  }
});
