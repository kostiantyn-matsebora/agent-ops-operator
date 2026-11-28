// cd runtimes/pi && node --test
//
// multi-repo-workspace moved cloning into the `workspace-sync` init
// container — this runtime's own bootstrap must clone nothing, read no
// REPO_URL/REPO_REF/GIT_* var, and hold no git credential. Proven the same
// way runtime-claude/runtime-claude-cli/runtime-copilot's own noclone tests
// prove it: a fake `git` placed first on PATH that fails loudly if ever
// invoked, and a sentinel file already in the workspace that must survive
// untouched, driven through a real subprocess of the compiled entrypoint
// rather than a mock.
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

const RUNTIME = path.join(__dirname, 'runtime.js');

// A fake `git` that records it ran and fails loudly — this runtime's own
// startup must never reach it.
function installFakeGit(binDir) {
  const marker = path.join(binDir, 'git-invoked');
  const script = path.join(binDir, 'git');
  fs.writeFileSync(script, `#!/bin/sh\necho "git invoked: $@" >> "${marker}"\nexit 1\n`);
  fs.chmodSync(script, 0o755);
  return marker;
}

test('module exports no clone/repo function — cloning is not this runtime\'s job', () => {
  const mod = require('./runtime');
  for (const name of ['gitEnv', 'repoURL', 'run', 'clearDir', 'syncRepo']) {
    assert.strictEqual(mod[name], undefined, `runtime.js still exports ${name}`);
  }
});

test('the runtime clones nothing on startup — a fake git on PATH is never invoked, and the pre-populated workspace survives untouched', async () => {
  const bin = tmp('agentops-pi-noclone-bin-');
  const gitMarker = installFakeGit(bin);
  const workspace = tmp('agentops-pi-noclone-ws-');
  const sentinel = path.join(workspace, 'already-here.txt');
  fs.writeFileSync(sentinel, 'workspace-sync put this here');

  // CONTROL_URL points at a closed port, so the poll loop never reaches
  // real work — RUNTIME_IDLE_TTL_M=0 drives it straight to its idle exit,
  // the same shape the other three runtimes' own noclone test accepts: a
  // real exit event, no fixed-duration kill.
  const child = spawn(process.execPath, [RUNTIME], {
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      CONTROL_URL: 'http://127.0.0.1:1',
      CONVO_ID: 'conv-noclone',
      PI_API_KEY: 'test-key',
      PI_MODEL: 'fake-model',
      HOME: tmp('agentops-pi-noclone-home-'),
      WORKSPACE: workspace,
      RUNTIME_IDLE_TTL_M: '0',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stdout = '';
  child.stdout.on('data', (c) => { stdout += c; });
  const code = await new Promise((resolve) => child.once('exit', resolve));

  assert.strictEqual(code, 0);
  assert.match(stdout, /idle TTL reached/);
  assert.strictEqual(fs.existsSync(gitMarker), false, 'the runtime invoked git on startup');
  assert.strictEqual(fs.readFileSync(sentinel, 'utf8'), 'workspace-sync put this here');
});
