// cd runtime-copilot && node --test
//
// P7: a failed clone fails the run with a reason, and git fetch/reset exit
// codes are checked rather than ignored. Its own process/file, since
// REPO_URL/WORKSPACE are module-level consts captured at require time, and
// runtime.gitclone.test.js already owns the two SUCCESS paths against a real
// origin.
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

// syncRepo() closes over module-level consts captured at require time, so the
// CLONE-failure case (a fresh, still-.git-less workspace) is driven as a real
// subprocess — its own env, no module-cache trickery — exactly the pattern
// runtime.mainentry.test.js already uses for the other require-time branches.
function runSyncRepo(env) {
  const script = "require('./runtime').syncRepo().then(() => process.exit(0), (e) => { console.error(e.message); process.exit(1); });";
  try {
    execFileSync(process.execPath, ['-e', script], { cwd: __dirname, env: { ...process.env, ...env }, stdio: 'pipe', timeout: 10000 });
    return { code: 0, stderr: '' };
  } catch (e) {
    return { code: e.status, stderr: String(e.stderr || '') };
  }
}

test('syncRepo fails the run with a reason when the clone itself fails', () => {
  const badWorkspace = tmp('agentops-copilot-giterr-badws-');
  const notARepo = tmp('agentops-copilot-giterr-notarepo-'); // exists, but is no git remote at all
  const { code, stderr } = runSyncRepo({
    CONTROL_URL: 'http://127.0.0.1:1',
    CONVO_ID: 'conv-giterr-clone',
    COPILOT_GITHUB_TOKEN: 'test-token',
    REPO_URL: notARepo,
    REPO_REF: 'master',
    WORKSPACE: badWorkspace,
  });
  assert.strictEqual(code, 1, 'a failed clone must fail the process rather than being swallowed');
  assert.match(stderr, /git clone:/);
});

// A real origin, so the first sync succeeds and the second one can be made to
// fail the FETCH specifically, once the local checkout already exists.
const srcDir = tmp('agentops-copilot-giterr-src-');
git(['init', '-q', '-b', 'master'], srcDir);
fs.writeFileSync(path.join(srcDir, 'f.txt'), 'v1');
git(['add', '-A'], srcDir);
git(['commit', '-q', '-m', 'init'], srcDir);

const originDir = tmp('agentops-copilot-giterr-origin-') + '.git';
git(['clone', '-q', '--bare', srcDir, originDir], os.tmpdir());

const workspace = tmp('agentops-copilot-giterr-ws-');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-giterr-fetch';
process.env.COPILOT_GITHUB_TOKEN = 'test-token';
process.env.REPO_URL = originDir;
process.env.REPO_REF = 'master';
process.env.WORKSPACE = workspace;

const { syncRepo } = require('./runtime');

test('syncRepo clones successfully, then fails the run when a later fetch fails', async () => {
  await syncRepo();
  assert.ok(fs.existsSync(path.join(workspace, '.git')), 'the first sync landed a real checkout');

  // Break the origin the existing checkout's "origin" remote points to, so a
  // later sync (fetch+reset, not a clone — the .git directory already
  // exists) fails at the fetch, and that failure must not be ignored.
  fs.rmSync(originDir, { recursive: true, force: true });

  await assert.rejects(syncRepo(), /git fetch:/);
});
