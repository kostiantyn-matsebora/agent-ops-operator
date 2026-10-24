// cd runtime-claude && node --test
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

const srcDir = tmp('agentops-src-');
git(['init', '-q', '-b', 'master'], srcDir);
fs.writeFileSync(path.join(srcDir, 'f.txt'), 'v1');
git(['add', '-A'], srcDir);
git(['commit', '-q', '-m', 'init'], srcDir);

const originDir = tmp('agentops-origin-') + '.git';
git(['clone', '-q', '--bare', srcDir, originDir], os.tmpdir());

const workspace = tmp('agentops-ws-');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-git';
process.env.REPO_URL = originDir;
process.env.REPO_REF = 'master';
process.env.WORKSPACE = workspace;

const { syncRepo } = require('./driver-cli');

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

// C5: a clone that cannot reach its origin must fail the run with a clear
// reason, never proceed silently against an empty/missing workspace.
test('syncRepo throws with the clone failure reason when the origin cannot be reached', async () => {
  const badOrigin = path.join(os.tmpdir(), 'agentops-origin-does-not-exist.git');
  const freshWorkspace = tmp('agentops-ws-badclone-');
  const real = { REPO_URL: process.env.REPO_URL, WORKSPACE: process.env.WORKSPACE };
  process.env.REPO_URL = badOrigin;
  process.env.WORKSPACE = freshWorkspace;
  delete require.cache[require.resolve('./driver-cli')];
  const { syncRepo: freshSyncRepo } = require('./driver-cli');
  try {
    await assert.rejects(freshSyncRepo(), /git clone/);
  } finally {
    process.env.REPO_URL = real.REPO_URL;
    process.env.WORKSPACE = real.WORKSPACE;
  }
});

// C5: `git fetch`'s exit code must be checked, not ignored — a fetch that
// fails (an origin that has gone away) must fail the sync rather than
// silently reset against whatever the last fetch happened to leave behind.
test('syncRepo throws with the fetch failure reason when origin is unreachable on an existing checkout', async () => {
  const goneOrigin = path.join(os.tmpdir(), 'agentops-origin-gone-' + Date.now() + '.git');
  git(['clone', '-q', '--bare', srcDir, goneOrigin], os.tmpdir());
  const ws = tmp('agentops-ws-badfetch-');
  git(['clone', '-q', goneOrigin, ws], os.tmpdir());
  fs.rmSync(goneOrigin, { recursive: true, force: true }); // origin now unreachable

  const real = { REPO_URL: process.env.REPO_URL, WORKSPACE: process.env.WORKSPACE };
  process.env.REPO_URL = goneOrigin;
  process.env.WORKSPACE = ws;
  delete require.cache[require.resolve('./driver-cli')];
  const { syncRepo: freshSyncRepo } = require('./driver-cli');
  try {
    await assert.rejects(freshSyncRepo(), /git fetch/);
  } finally {
    process.env.REPO_URL = real.REPO_URL;
    process.env.WORKSPACE = real.WORKSPACE;
  }
});

// C5: a ref that does not exist upstream must fail the sync rather than
// silently reset onto whatever the workspace already had — whichever of
// `fetch` or `reset` is the one that actually trips over it, its exit code
// must be checked, not ignored.
test('syncRepo throws rather than resetting silently when the configured ref does not exist', async () => {
  const ws = tmp('agentops-ws-badreset-');
  git(['clone', '-q', originDir, ws], os.tmpdir());

  const real = { REPO_URL: process.env.REPO_URL, REPO_REF: process.env.REPO_REF, WORKSPACE: process.env.WORKSPACE };
  process.env.REPO_URL = originDir;
  process.env.REPO_REF = 'branch-that-does-not-exist';
  process.env.WORKSPACE = ws;
  delete require.cache[require.resolve('./driver-cli')];
  const { syncRepo: freshSyncRepo } = require('./driver-cli');
  try {
    await assert.rejects(freshSyncRepo(), /git (fetch|reset)/);
  } finally {
    process.env.REPO_URL = real.REPO_URL;
    process.env.REPO_REF = real.REPO_REF;
    process.env.WORKSPACE = real.WORKSPACE;
  }
});
