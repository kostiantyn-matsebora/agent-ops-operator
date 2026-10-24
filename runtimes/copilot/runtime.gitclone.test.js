// cd runtime-copilot && node --test
//
// Real git, real filesystem: exercises syncRepo()'s two branches — the fresh
// clone (no .git under WORKSPACE) and the fetch+hard-reset of an existing
// checkout — against a genuine local bare repository. No mocking of git.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

const GIT_ENV = { ...process.env, GIT_AUTHOR_NAME: 'test', GIT_AUTHOR_EMAIL: 'test@example.com', GIT_COMMITTER_NAME: 'test', GIT_COMMITTER_EMAIL: 'test@example.com' };
function git(args, cwd) {
  execFileSync('git', args, { cwd, env: GIT_ENV, stdio: 'pipe' });
}

const srcDir = tmp('agentops-copilot-src-');
git(['init', '-q', '-b', 'master'], srcDir);
fs.writeFileSync(path.join(srcDir, 'f.txt'), 'v1');
git(['add', '-A'], srcDir);
git(['commit', '-q', '-m', 'init'], srcDir);

const originDir = tmp('agentops-copilot-origin-') + '.git';
git(['clone', '-q', '--bare', srcDir, originDir], os.tmpdir());

const workspace = tmp('agentops-copilot-ws-');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-git';
process.env.COPILOT_GITHUB_TOKEN = 'test-token';
process.env.REPO_URL = originDir;
process.env.REPO_REF = 'master';
process.env.WORKSPACE = workspace;

const { syncRepo } = require('./runtime');

test('syncRepo clones a fresh workspace when no checkout exists yet', async () => {
  await syncRepo();
  assert.ok(fs.existsSync(path.join(workspace, '.git')), 'a real clone landed under WORKSPACE');
  assert.strictEqual(fs.readFileSync(path.join(workspace, 'f.txt'), 'utf8'), 'v1');
});

test('syncRepo fetches and hard-resets an existing checkout to the latest origin commit', async () => {
  fs.writeFileSync(path.join(srcDir, 'f.txt'), 'v2');
  git(['commit', '-aqm', 'update'], srcDir);
  git(['push', '-q', originDir, 'master'], srcDir);

  await syncRepo();

  assert.strictEqual(fs.readFileSync(path.join(workspace, 'f.txt'), 'utf8'), 'v2', 'the second sync must land the new commit');
});
