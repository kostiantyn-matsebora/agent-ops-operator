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

// A real end-to-end run through the main loop (not a require()'d unit test):
// a `git` on PATH that must never be invoked, and a workspace whose contents
// must survive untouched — populating WORKSPACE is workspace-sync's job now,
// never this process's.
test('the main loop never touches the workspace contents or invokes git', async () => {
  const workspace = tmp('agentops-noclone-ws-');
  fs.writeFileSync(path.join(workspace, 'marker.txt'), 'populated by workspace-sync');

  const binDir = tmp('agentops-noclone-bin-');
  fs.writeFileSync(
    path.join(binDir, 'git'),
    '#!/bin/sh\necho "git invoked despite workspace-sync owning the clone" 1>&2\nexit 1\n',
    { mode: 0o755 },
  );

  const child = spawn(process.execPath, [path.join(__dirname, 'runtime.js')], {
    cwd: __dirname,
    env: {
      ...process.env,
      PATH: `${binDir}${path.delimiter}${process.env.PATH}`,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-noclone',
      POD_NAME: 'pod-noclone',
      WORKSPACE: workspace,
      RUNTIME_IDLE_TTL_M: '0',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stderr = '';
  child.stderr.on('data', (c) => { stderr += c; });
  const code = await new Promise((resolve) => child.once('exit', resolve));

  assert.strictEqual(code, 0);
  assert.strictEqual(
    fs.readFileSync(path.join(workspace, 'marker.txt'), 'utf8'),
    'populated by workspace-sync',
    'the workspace is never cleared or re-synced by this process',
  );
  assert.ok(!stderr.includes('git invoked'), `git must never run from this process, got stderr: ${stderr}`);
});
