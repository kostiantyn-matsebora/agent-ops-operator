// cd runtime-copilot && node --test
//
// repoURL()'s https branch reads GIT_AUTH_TYPE/GIT_TOKEN/REPO_URL, all
// module-level consts captured at require time — its own process/file, since
// runtime.test.js exercises the false branch with the default (empty) env.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-https';
process.env.COPILOT_GITHUB_TOKEN = 'test-token';
process.env.GIT_AUTH_TYPE = 'https';
process.env.GIT_TOKEN = 'tok123';
process.env.REPO_URL = 'https://example.com/org/repo.git';

const { repoURL } = require('./runtime');

test('repoURL embeds an x-access-token credential into an https remote', () => {
  assert.strictEqual(repoURL(), 'https://x-access-token:tok123@example.com/org/repo.git');
});
